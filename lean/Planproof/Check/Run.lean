import Planproof.Check.Model

/-!
# The check (spec 05)

A transliteration of the Go kernel's `kernel/run.go`, `kernel/rules.go` and
the decision part of `kernel/check.go`. It keeps what the verdict's result,
failing rules, Unknown trigger and approval depend on; the evidence numbers
the packet shows stay in Go.

Each rule check is a function that returns the rules it finds broken, so
that "the check passed" is "the list is empty". The agreement tests (PRD
L-21, L-22) check that this gives the Go kernel's verdicts.
-/
namespace Planproof

inductive Result where
  | proved | refuted | unknown
deriving DecidableEq, Repr, Inhabited

/-- The rules, in the order of spec 04. -/
def rules : List String :=
  ["input", "scope", "protected", "limits", "assertions", "cordon-first", "no-orphans",
   "budget", "never-zero", "capacity", "settled", "frame", "rollback"]

/-- What stays fixed during a check: the inputs and the snapshot's numbers. -/
structure Ctx where
  plan : Plan
  policy : Policy
  st0 : State
  depNames : List String
  initial : List (String × Int)
  initUnsched : Int
  initSched : Int
deriving Inhabited

def Ctx.make (snap : Snapshot) (plan : Plan) (pol : Policy) : Ctx :=
  let st := build snap
  let depNames := sortStrings (st.deps.map (·.name))
  { plan, policy := pol, st0 := st, depNames, initial := depNames.map fun d => (d, st.avail d),
    initUnsched := st.nodeCounts.1, initSched := st.nodeCounts.2 }

def Ctx.initialOf (c : Ctx) (d : String) : Int := (lookup c.initial d).getD 0

/-- The goal scales the deployment to zero and the policy allows it. -/
def Ctx.zeroAllowed (c : Ctx) (d : String) : Bool :=
  c.policy.limits.allowZero && c.plan.goal.any fun ch => ch.deployment == d && ch.field == "replicas" && ch.toV == Value.int 0

/-! ## Unknown triggers (spec 02, 03) -/

def depTrigger (st : State) (d : String) : Option String :=
  match st.dep? d with
  | none => none
  | some m =>
    if m.hpa then some "hpa"
    else if m.unresolved then some "pdb-unresolved"
    else if m.unsettled then some "unsettled-start"
    else if m.rolling then some "overlapping-change"
    else none

/-- The first Unknown trigger that applies to a step. -/
def trigger (st : State) (s : Step) : Option String :=
  let notReady := ["cordon", "uncordon", "drain"].contains s.op && !((st.node? s.node).map (·.ready) |>.getD false)
  if notReady then some "node-not-ready"
  else if s.op == "drain" then
    match st.pods.findSome? fun p =>
      if !p.mightBeOn s.node || p.owner == "DaemonSet" || p.owner == "None" then none
      else if p.localStorage then some "local-storage"
      else if p.dep == "" || (st.dep? p.dep).isNone then some "stateful-owner"
      else if let some t := depTrigger st p.dep then some t
      else if p.placement then some "placement-rules"
      else none with
    | some t => some t
    | none =>
      -- Mid rollout, the old pods may still be on their nodes (kernel 0.4.0).
      if st.deps.any fun d => d.rolling && d.oldNodes.contains s.node then some "overlapping-change" else none
  else if ["scale", "set-image", "rollout-restart"].contains s.op then
    match depTrigger st s.deployment with
    | some t => some t
    | none =>
      match st.dep? s.deployment with
      | none => none
      | some m =>
        if s.op == "scale" && decide (s.replicas.getD 0 ≤ m.replicas) then none
        else if m.placement then some "placement-rules"
        else none
  else none

/-! ## Checks before and after a step -/

/-- Rule `assertions`: a step's `pre` or `post` assertions on a state. -/
def assertFails (st : State) (s : Step) : Option Assertions → List String
  | none => []
  | some a =>
    let depFails := match st.dep? s.deployment with
      | none => []
      | some d =>
        (match a.replicas with
          | some v => if v == d.replicas then [] else ["assertions"]
          | none => []) ++
        (match a.image with
          | some v =>
            let c := if a.container == "" then s.container else a.container
            if v == ((lookup d.images c).getD "") then [] else ["assertions"]
          | none => [])
    let nodeFails := match st.node? s.node with
      | none => []
      | some n =>
        (match a.unsched with
          | some v => if v == n.unsched then [] else ["assertions"]
          | none => []) ++
        (match a.podsOnNode with
          | some v =>
            let count : Int := (st.pods.filter fun p => p.owner != "DaemonSet" && p.mightBeOn s.node).length
            if v == count then [] else ["assertions"]
          | none => [])
    depFails ++ nodeFails

/-- Rules `cordon-first` and `no-orphans`: what a drain requires. -/
def drainFails (st : State) (n : String) : List String :=
  (if (st.node? n).map (·.unsched) |>.getD false then [] else ["cordon-first"]) ++
  (st.pods.filter fun p => p.owner == "None" && p.mightBeOn n).map fun _ => "no-orphans"

/-- The rules checked at every state: budget, never-zero, limits (spec 04). -/
def eachStateFails (c : Ctx) (st : State) : List String :=
  let lim := c.policy.limits
  let (u, s) := st.nodeCounts
  ((st.pdbs.filter fun b => decide (st.avail b.dep < st.need b) && decide (st.avail b.dep < c.initialOf b.dep)).map fun _ => "budget") ++
  ((c.depNames.filter fun d => decide (c.initialOf d ≥ 1) && st.avail d == 0 && !c.zeroAllowed d).map fun _ => "never-zero") ++
  (if decide (u > lim.maxUnsched) && decide (u > c.initUnsched) then ["limits"] else []) ++
  (if decide (s < lim.minSched) && decide (s < c.initSched) then ["limits"] else [])

/-! ## Operations (spec 03) -/

/-- Walk the pods, dropping `qn` that aren't ready and `qr` that are, of
deployment `d`, in list order; the pods it keeps may be anywhere in `U`. -/
def dropPods (d : String) (U : List String) : Nat → Nat → List MPod → List MPod
  | _, _, [] => []
  | qn, qr, p :: ps =>
    if p.dep == d && !p.gone then
      if !p.ready && qn > 0 then { p with gone := true, ready := false } :: dropPods d U (qn - 1) qr ps
      else if p.ready && qr > 0 then { p with gone := true, ready := false } :: dropPods d U qn (qr - 1) ps
      else { p with nodes := U } :: dropPods d U qn qr ps
    else p :: dropPods d U qn qr ps

/-- Scale a deployment down to `n`: pods that aren't ready go first, then
ready ones, each in list order (the Go kernel's stable sort); the rest may
be anywhere any of its pods was. -/
def State.scaleDown (st : State) (d : String) (n : Int) : State :=
  let pods := st.podsOf d
  let k := pods.length - n.toNat
  let notReady := (pods.filter (!·.ready)).length
  let wher := compact (sortStrings (pods.flatMap (·.nodes)))
  { st with pods := dropPods d wher (min k notReady) (k - min k notReady) st.pods }

/-- A rolling update's surge pods must fit on top of the existing ones
(none for `Recreate`). The surge pods are removed again. -/
def State.surgeFits (st : State) (d : String) (m : MDep) : State × Bool :=
  if m.recreate then (st, true)
  else
    let (st', surge) := st.newPods d m.maxSurge
    let (_, b) := st'.place surge
    -- Only the counter for new pod names changes; the surge pods go again.
    ({ st' with pods := st.pods }, b.isNone)

/-- What a set-image or rollout-restart does to the deployment itself;
`old` is where its old pods may still be. -/
def rolloutUpdate (s : Step) (old : List String) (d : MDep) : MDep :=
  if s.op == "set-image" then { d with rolling := true, oldNodes := old, images := setImage s.container s.image d.images }
  else { d with rolling := true, oldNodes := old, restarted := true }

/-- The rollout: the old pods go, new pending pods replace them. -/
def State.rollout (st : State) (s : Step) (m : MDep) : State × Bool :=
  let old := (st.podsOf s.deployment).flatMap (·.nodes)
  let st := { st with pods := st.pods.filter fun p => !(p.dep == s.deployment && !p.gone) }
  let st := st.updDep s.deployment (rolloutUpdate s old)
  let r := st.newPods s.deployment m.replicas
  let r2 := r.1.place r.2
  (r2.1, r2.2.isNone)

/-- The state after an operation, and whether every pod it created or moved
found a place (rule `capacity`). -/
def applyOp (st : State) (s : Step) : State × Bool :=
  match s.op with
  | "cordon" => (st.updNode s.node fun n => { n with unsched := true }, true)
  | "uncordon" =>
    let st := st.updNode s.node fun n => { n with unsched := false }
    -- A pod that isn't ready may still be pending and be scheduled onto it
    -- (kernel 0.5.0).
    if (st.node? s.node).any fun n => n.ready && !n.taints then
      (st.updPods (fun p => !p.ready && !p.gone && !p.nodes.contains s.node)
        fun p => { p with nodes := sortStrings (p.nodes ++ [s.node]) }, true)
    else (st, true)
  | "drain" =>
    let sched := st.schedulable
    let moved := (st.pods.filter fun p => p.dep != "" && p.mightBeOn s.node).map (·.name)
    -- It was on this node and moves to a schedulable one, or it was on one
    -- of its other candidates and stays there (kernel 0.3.0).
    let st := st.updPods (fun p => p.dep != "" && p.mightBeOn s.node) fun p =>
      { p with ready := false, created := true,
               nodes := compact (sortStrings (sched ++ p.nodes.filter (· != s.node))) }
    let st := st.updNode s.node fun n => { n with drained := true }
    let r := st.place moved
    (r.1, r.2.isNone)
  | "scale" =>
    match st.dep? s.deployment with
    | none => (st, true)
    | some m =>
      let n := s.replicas.getD 0
      let old := m.replicas
      let st := st.updDep s.deployment fun d => { d with replicas := n }
      if n > old then
        let r := st.newPods s.deployment (n - old)
        let r2 := r.1.place r.2
        (r2.1, r2.2.isNone)
      else if n < old then (st.scaleDown s.deployment n, true)
      else (st, true)
  | "set-image" | "rollout-restart" =>
    match st.dep? s.deployment with
    | none => (st, true)
    | some m =>
      let floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail))
      let (st, ok) := (st.updDep s.deployment fun d => { d with floor }).surgeFits s.deployment m
      if ok then st.rollout s m else (st, false)
  | "wait" =>
    (s.deployments.foldl (init := st) fun st d =>
      let st := st.updDep d fun x => { x with rolling := false, oldNodes := [] }
      st.updPods (fun p => p.dep == d && !p.gone) fun p => { p with ready := true }, true)
  | _ => (st, true)

/-! ## Approval (spec 04) -/

/-- What the plan's steps did, for `approvalRequiredFor`. -/
structure Flags where
  drains : Bool := false
  scaleDown : Bool := false
  setsImage : Bool := false
  disrupted : List String := []
deriving Inhabited

/-- The flags after a plan step, from the state before it. -/
def Flags.update (fl : Flags) (st : State) (s : Step) : Flags :=
  match s.op with
  | "drain" =>
    { fl with drains := true,
              disrupted := fl.disrupted ++ (st.pods.filter fun p => p.dep != "" && p.mightBeOn s.node).map (·.dep) }
  | "scale" =>
    match st.dep? s.deployment with
    | some m => if s.replicas.getD 0 < m.replicas
      then { fl with scaleDown := true, disrupted := fl.disrupted ++ [s.deployment] } else fl
    | none => fl
  | "set-image" | "rollout-restart" =>
    match st.dep? s.deployment with
    | some _ => { fl with setsImage := fl.setsImage || s.op == "set-image", disrupted := fl.disrupted ++ [s.deployment] }
    | none => fl
  | _ => fl

/-! ## Running the steps (spec 05, step 4) -/

/-- Where a run of steps stopped. -/
structure Stop where
  st : State
  flags : Flags
  failures : List String
  unknown : Option String

/-- Run steps until one is Unknown or breaks a rule. `inPlan` is false for
the rollback, whose steps don't count for approval. -/
def doSteps (c : Ctx) (inPlan : Bool) (st : State) (fl : Flags) : List Step → Stop
  | [] => ⟨st, fl, [], none⟩
  | s :: rest =>
    match trigger st s with
    | some t => ⟨st, fl, [], some t⟩
    | none =>
      let pre := assertFails st s s.pre ++ (if s.op == "drain" then drainFails st s.node else [])
      if !pre.isEmpty then ⟨st, fl, pre, none⟩
      else
        let fl := if inPlan then fl.update st s else fl
        let (st', ok) := applyOp st s
        let fs := (if ok then [] else ["capacity"]) ++ eachStateFails c st' ++ assertFails st' s s.post
        if fs.isEmpty then doSteps c inPlan st' fl rest else ⟨st', fl, fs, none⟩

/-! ## Static and end checks (spec 04) -/

/-- An image pinned by digest: `name@sha256:` and 64 lowercase hex characters. -/
def isHash (s : String) : Bool :=
  s.startsWith "sha256:" &&
    let h := s.drop 7
    h.length == 64 && h.all fun c => c.isDigit || ('a' ≤ c && c ≤ 'f')

def Ctx.noDigest (c : Ctx) (image : String) : Bool :=
  c.policy.limits.requireDigest &&
    match image.splitOn "@" with
    | _ :: rest@(_ :: _) => !isHash ("@".intercalate rest)
    | _ => true

/-- Rules `scope` and `protected`. -/
def scopeFails (c : Ctx) : List String :=
  let plan := c.plan
  let pol := c.policy
  let st := c.st0
  let steps := plan.steps ++ plan.rollback
  let goalDeps := (plan.goal.filter (·.deployment != "")).map (·.deployment)
  let goalNodes := (plan.goal.filter (·.deployment == "")).map (·.node)
  let deps := goalDeps ++ steps.flatMap fun s => s.deployments ++ [s.deployment]
  let nodes := goalNodes ++ steps.map (·.node)
  let changed := goalDeps ++ steps.map (·.deployment)
  ((compact (sortStrings deps)).flatMap fun d =>
    let ns := (d.splitOn "/").head?.getD ""
    (if d == "" then []
     else if !pol.managedNamespaces.contains ns then ["scope"]
     else if (st.dep? d).isNone then ["scope"]
     else []) ++
    (if changed.contains d && pol.protectedDeps.contains d then ["protected"] else [])) ++
  ((compact (sortStrings nodes)).flatMap fun n =>
    if n == "" then []
    else if !pol.managedNodes.contains n then ["scope"]
    else if (st.node? n).isNone then ["scope"]
    else [])

/-- Static `frame` (and `scope`, `limits`) for the goal changes, each after `seen`. -/
def goalFails (c : Ctx) : List (Option Key) → List Change → List String
  | _, [] => []
  | seen, ch :: rest =>
    let lim := c.policy.limits
    let k := ch.key
    (match k.bind c.st0.field with
      | none => ["scope"]
      | some cur =>
        if seen.contains k then ["frame"]
        else if cur != ch.fromV then ["frame"]
        else if ch.fromV == ch.toV then ["frame"]
        else if ch.field == "replicas" && (match ch.toV with | .int i => decide (i > lim.maxReplicas) | _ => false) then ["limits"]
        else if ch.field == "image" && (match ch.toV with | .str i => c.noDigest i | _ => false) then ["limits"]
        else []) ++ goalFails c (seen ++ [k]) rest

/-- Static `limits` and the container check, for the steps. -/
def stepStaticFails (c : Ctx) : List String :=
  let lim := c.policy.limits
  (c.plan.steps ++ c.plan.rollback).flatMap fun s =>
    (if s.op == "scale" && decide (s.replicas.getD 0 > lim.maxReplicas) then ["limits"]
     else if s.op == "wait" && decide (s.timeout.getD 0 > lim.maxWait) then ["limits"]
     else if s.op == "set-image" && c.noDigest s.image then ["limits"]
     else []) ++
    (if s.op == "set-image" && ((c.st0.dep? s.deployment).map fun d => (lookup d.images s.container).isNone).getD true
     then ["scope"] else [])

/-- The rules that need no simulation. `scope` first: the rest needs the
objects to exist. -/
def staticFails (c : Ctx) : List String :=
  let sf := scopeFails c
  if !sf.isEmpty then sf else stepStaticFails c ++ goalFails c [] c.plan.goal

/-- `settled`, then `frame`: the plan's effect is exactly its goal. -/
def endFails (c : Ctx) (st : State) : List String :=
  let settled := (c.depNames.filter fun d =>
    ((st.dep? d).map (·.rolling) |>.getD false) || (st.podsOf d).any fun p => p.created && !p.ready).map fun _ => "settled"
  if !settled.isEmpty then settled
  else
    let goal := c.plan.goal.filterMap fun ch => ch.key.map fun k => (k, ch.toV)
    let bad := st.keys.any fun k =>
      match lookup goal k with
      | some g => st.field k != some g
      | none => st.field k != c.st0.field k
    if bad then ["frame"] else []

/-- After the rollback, each reversible goal field is back to its `from` value. -/
def restoredFails (c : Ctx) (st : State) : List String :=
  if c.plan.goal.all fun ch => ch.field == "drained" || ch.field == "restarted" || ch.key.bind st.field == some ch.fromV
  then [] else ["rollback"]

/-! ## The check -/

/-- The algorithm of spec 05, steps 3 to 6. -/
def execute (c : Ctx) : Stop :=
  let sf := staticFails c
  if !sf.isEmpty then ⟨c.st0, {}, sf, none⟩
  else
    let r := doSteps c true c.st0 {} c.plan.steps
    if r.unknown.isSome || !r.failures.isEmpty then r
    else
      let ef := endFails c r.st
      if !ef.isEmpty || c.plan.rollback.isEmpty then { r with failures := ef }
      else
        let rb := doSteps c false r.st r.flags c.plan.rollback
        if rb.unknown.isSome || !rb.failures.isEmpty then { rb with failures := rb.failures.map fun _ => "rollback" }
        else { rb with failures := restoredFails c rb.st }

def Stop.result (r : Stop) : Result :=
  if r.unknown.isSome then .unknown else if r.failures.isEmpty then .proved else .refuted

/-- Critical deployments the plan names, or whose pods it moves, replaces or removes. -/
def criticalInvolved (c : Ctx) (fl : Flags) : List String :=
  let named := c.plan.goal.map (·.deployment) ++ c.plan.steps.map (·.deployment)
  c.policy.criticalDeps.filter fun d => named.contains d || fl.disrupted.contains d

def approvalReasons (c : Ctx) (fl : Flags) : List String :=
  c.policy.approvalFor.filter fun reason =>
    match reason with
    | "always" => true
    | "drain" => fl.drains
    | "critical" => !(criticalInvolved c fl).isEmpty
    | "scale-down" => fl.scaleDown
    | "image" => fl.setsImage
    | _ => false

/-- What the agreement tests compare: result, the failing rules in spec
order, the Unknown trigger, and the approval reasons of a Proved plan. -/
structure Summary where
  result : Result
  rules : List String
  trigger : Option String
  approval : List String
deriving Repr

def check (snap : Snapshot) (plan : Plan) (pol : Policy) : Summary :=
  let c := Ctx.make snap plan pol
  let r := execute c
  { result := r.result,
    rules := rules.filter r.failures.contains,
    trigger := r.unknown,
    approval := if r.result == .proved then approvalReasons c r.flags else [] }

end Planproof
