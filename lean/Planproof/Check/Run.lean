import Planproof.Check.Model

/-!
# The check (spec 05)

A transliteration of the Go kernel's `kernel/run.go`, `kernel/rules.go` and
the decision part of `kernel/check.go`. It keeps what the verdict's result,
failing rules, Unknown trigger and approval depend on; the evidence numbers
the packet shows stay in Go.
-/
namespace Planproof

inductive Result where
  | proved | refuted | unknown
deriving DecidableEq, Repr, Inhabited

/-- The rules, in the order of spec 04. -/
def rules : List String :=
  ["input", "scope", "protected", "limits", "assertions", "cordon-first", "no-orphans",
   "budget", "never-zero", "capacity", "settled", "frame", "rollback"]

/-- A check in progress. -/
structure Run where
  plan : Plan
  policy : Policy
  st : State
  depNames : List String
  initial : List (String × Int)
  initDesired : List ((String × String × String) × Value)
  initUnsched : Int
  initSched : Int
  failures : List String
  unknown : Option String
  rollback : Bool
  drains : Bool
  scaleDown : Bool
  setsImage : Bool
  disrupted : List String
deriving Inhabited

def newRun (snap : Snapshot) (plan : Plan) (pol : Policy) : Run :=
  let st := build snap
  let depNames := sortStrings (st.deps.map (·.name))
  let (u, s) := st.nodeCounts
  { plan, policy := pol, st, depNames, initial := depNames.map fun d => (d, st.avail d),
    initDesired := st.desired, initUnsched := u, initSched := s, failures := [], unknown := none,
    rollback := false, drains := false, scaleDown := false, setsImage := false, disrupted := [] }

namespace Run

/-- Record a broken rule; during the rollback it counts as rule `rollback`. -/
def fail (r : Run) (rule : String) : Run :=
  { r with failures := r.failures ++ [if r.rollback then "rollback" else rule] }

def initialOf (r : Run) (d : String) : Int := (lookup r.initial d).getD 0

def disrupt (r : Run) (d : String) : Run :=
  if r.rollback || r.disrupted.contains d then r else { r with disrupted := r.disrupted ++ [d] }

def depTrigger (r : Run) (d : String) : Option String :=
  match r.st.dep? d with
  | none => none
  | some m =>
    if m.hpa then some "hpa"
    else if m.unresolved then some "pdb-unresolved"
    else if m.unsettled then some "unsettled-start"
    else if m.rolling then some "overlapping-change"
    else none

/-- The first Unknown trigger that applies to a step (spec 02, 03). -/
def trigger (r : Run) (s : Step) : Option String :=
  let st := r.st
  let notReady := ["cordon", "uncordon", "drain"].contains s.op && !((st.node? s.node).map (·.ready) |>.getD false)
  if notReady then some "node-not-ready"
  else if s.op == "drain" then
    st.pods.findSome? fun p =>
      if !p.mightBeOn s.node || p.owner == "DaemonSet" || p.owner == "None" then none
      else if p.localStorage then some "local-storage"
      else if p.dep == "" || (st.dep? p.dep).isNone then some "stateful-owner"
      else if let some t := r.depTrigger p.dep then some t
      else if p.placement then some "placement-rules"
      else none
  else if ["scale", "set-image", "rollout-restart"].contains s.op then
    match r.depTrigger s.deployment with
    | some t => some t
    | none =>
      match st.dep? s.deployment with
      | none => none
      | some m =>
        if s.op == "scale" && decide (s.replicas.getD 0 ≤ m.replicas) then none
        else if m.placement then some "placement-rules"
        else none
  else none

def drainRequires (r : Run) (n : String) : Run :=
  let r := if (r.st.node? n).map (·.unsched) |>.getD false then r else r.fail "cordon-first"
  r.st.pods.foldl (init := r) fun r p => if p.owner == "None" && p.mightBeOn n then r.fail "no-orphans" else r

def placeOrFail (r : Run) (names : List String) : Run :=
  let (st, bad) := r.st.place names
  let r := { r with st }
  if bad.isSome then r.fail "capacity" else r

/-- Change the state as the step's operation does (spec 03). -/
def apply (r : Run) (s : Step) : Run :=
  let st := r.st
  let plan := !r.rollback
  match s.op with
  | "cordon" => { r with st := st.updNode s.node fun n => { n with unsched := true } }
  | "uncordon" => { r with st := st.updNode s.node fun n => { n with unsched := false } }
  | "drain" =>
    let sched := st.schedulable
    let moved := st.pods.filter fun p => p.dep != "" && p.mightBeOn s.node
    let st := st.updPods (fun p => p.dep != "" && p.mightBeOn s.node) fun p =>
      { p with ready := false, created := true, nodes := sched }
    let r := moved.foldl (init := { r with st }) fun r p => r.disrupt p.dep
    let r := { r with st := r.st.updNode s.node fun n => { n with drained := true }, drains := r.drains || plan }
    r.placeOrFail (moved.map (·.name))
  | "scale" =>
    match st.dep? s.deployment with
    | none => r
    | some m =>
      let n := s.replicas.getD 0
      let old := m.replicas
      let st := st.updDep s.deployment fun d => { d with replicas := n }
      if n > old then
        let (st, names) := st.newPods s.deployment (n - old)
        { r with st }.placeOrFail names
      else if n < old then
        let r := ({ r with st, scaleDown := r.scaleDown || plan }).disrupt s.deployment
        -- Pods that aren't ready go first; the rest may be anywhere they were.
        let pods := (st.podsOf s.deployment).mergeSort fun a b => !a.ready || b.ready
        let wher := compact (sortStrings (pods.flatMap (·.nodes)))
        let drop := pods.take (pods.length - n.toNat) |>.map (·.name)
        let keep := pods.drop (pods.length - n.toNat) |>.map (·.name)
        let st := st.updPods (fun p => drop.contains p.name) fun p => { p with gone := true, ready := false }
        let st := st.updPods (fun p => keep.contains p.name) fun p => { p with nodes := wher }
        { r with st }
      else { r with st }
  | "set-image" | "rollout-restart" =>
    match st.dep? s.deployment with
    | none => r
    | some m =>
      let r := r.disrupt s.deployment
      let r := { r with setsImage := r.setsImage || (plan && s.op == "set-image") }
      let floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail))
      let st := st.updDep s.deployment fun d => { d with floor }
      -- Surge pods must fit on top of the existing ones.
      let (st, bad) :=
        if m.recreate then (st, false)
        else
          let (st, surge) := st.newPods s.deployment m.maxSurge
          let (st, b) := st.place surge
          (st.remove surge, b.isSome)
      if bad then ({ r with st }).fail "capacity"
      else
        let st := st.remove ((st.podsOf s.deployment).map (·.name))
        let st := st.updDep s.deployment fun d =>
          if s.op == "set-image" then
            { d with rolling := true, images := d.images.map fun (c, i) => if c == s.container then (c, s.image) else (c, i) }
          else { d with rolling := true, restarted := true }
        let (st, names) := st.newPods s.deployment m.replicas
        { r with st }.placeOrFail names
  | "wait" =>
    let st := s.deployments.foldl (init := st) fun st d =>
      let st := st.updDep d fun x => { x with rolling := false }
      st.updPods (fun p => p.dep == d && !p.gone) fun p => { p with ready := true }
    { r with st }
  | _ => r

/-- The goal scales the deployment to zero and the policy allows it. -/
def zeroAllowed (r : Run) (d : String) : Bool :=
  r.policy.limits.allowZero && r.plan.goal.any fun c => c.deployment == d && c.field == "replicas" && c.toV == Value.int 0

/-- The rules checked at every state: budget, never-zero, limits (spec 04). -/
def eachState (r : Run) : Run :=
  let st := r.st
  let lim := r.policy.limits
  let r := st.pdbs.foldl (init := r) fun r b =>
    let a := st.avail b.dep
    if decide (a < st.need b) && decide (a < r.initialOf b.dep) then r.fail "budget" else r
  let r := r.depNames.foldl (init := r) fun r d =>
    if decide (r.initialOf d ≥ 1) && st.avail d == 0 && !r.zeroAllowed d then r.fail "never-zero" else r
  let (u, s) := st.nodeCounts
  let r := if decide (u > lim.maxUnsched) && decide (u > r.initUnsched) then r.fail "limits" else r
  if decide (s < lim.minSched) && decide (s < r.initSched) then r.fail "limits" else r

def assertOn (r : Run) (s : Step) : Option Assertions → Run
  | none => r
  | some a =>
    let st := r.st
    let r := match st.dep? s.deployment with
      | none => r
      | some d =>
        let r := match a.replicas with
          | some v => if v == d.replicas then r else r.fail "assertions"
          | none => r
        match a.image with
        | some v =>
          let c := if a.container == "" then s.container else a.container
          if v == ((lookup d.images c).getD "") then r else r.fail "assertions"
        | none => r
    match st.node? s.node with
    | none => r
    | some n =>
      let r := match a.unsched with
        | some v => if v == n.unsched then r else r.fail "assertions"
        | none => r
      match a.podsOnNode with
      | some v =>
        let count : Int := (st.pods.filter fun p => p.owner != "DaemonSet" && p.mightBeOn s.node).length
        if v == count then r else r.fail "assertions"
      | none => r

/-- Run one step (spec 05, step 4); `false` means stop. -/
def doStep (r : Run) (s : Step) : Run × Bool :=
  match r.trigger s with
  | some t => ({ r with unknown := some t }, false)
  | none =>
    let r := r.assertOn s s.pre
    let r := if s.op == "drain" then r.drainRequires s.node else r
    if !r.failures.isEmpty then (r, false)
    else
      let r := ((r.apply s).eachState).assertOn s s.post
      (r, r.failures.isEmpty)

/-- Run steps until one stops the check. -/
def doSteps (r : Run) : List Step → Run × Bool
  | [] => (r, true)
  | s :: rest =>
    let (r, ok) := r.doStep s
    if ok then r.doSteps rest else (r, false)

/-- An image pinned by digest: `name@sha256:` and 64 lowercase hex characters. -/
def isHash (s : String) : Bool :=
  s.startsWith "sha256:" &&
    let h := s.drop 7
    h.length == 64 && h.all fun c => c.isDigit || ('a' ≤ c && c ≤ 'f')

def noDigest (r : Run) (image : String) : Bool :=
  r.policy.limits.requireDigest &&
    match image.splitOn "@" with
    | _ :: rest@(_ :: _) => !isHash ("@".intercalate rest)
    | _ => true

def changeKey (c : Change) : String × String × String :=
  (if c.deployment != "" then "deployment/" ++ c.deployment else "node/" ++ c.node, c.field, c.container)

/-- The rules that need no simulation: scope, protected, static limits and
static frame (spec 04). -/
def static (r : Run) : Run :=
  let plan := r.plan
  let pol := r.policy
  let st := r.st
  let steps := plan.steps ++ plan.rollback
  let goalDeps := (plan.goal.filter (·.deployment != "")).map (·.deployment)
  let goalNodes := (plan.goal.filter (·.deployment == "")).map (·.node)
  let deps := goalDeps ++ steps.flatMap fun s => s.deployments ++ [s.deployment]
  let nodes := goalNodes ++ steps.map (·.node)
  let changed := goalDeps ++ steps.map (·.deployment)
  let r := (compact (sortStrings deps)).foldl (init := r) fun r d =>
    let ns := (d.splitOn "/").head?.getD ""
    let r := if d == "" then r
      else if !pol.managedNamespaces.contains ns then r.fail "scope"
      else if (st.dep? d).isNone then r.fail "scope"
      else r
    if changed.contains d && pol.protectedDeps.contains d then r.fail "protected" else r
  let r := (compact (sortStrings nodes)).foldl (init := r) fun r n =>
    if n == "" then r
    else if !pol.managedNodes.contains n then r.fail "scope"
    else if (st.node? n).isNone then r.fail "scope"
    else r
  if !r.failures.isEmpty then r
  else
    let lim := pol.limits
    let r := steps.foldl (init := r) fun r s =>
      let r :=
        if s.op == "scale" && decide (s.replicas.getD 0 > lim.maxReplicas) then r.fail "limits"
        else if s.op == "wait" && decide (s.timeout.getD 0 > lim.maxWait) then r.fail "limits"
        else if s.op == "set-image" && r.noDigest s.image then r.fail "limits"
        else r
      if s.op == "set-image" && ((st.dep? s.deployment).map fun d => (lookup d.images s.container).isNone).getD true
      then r.fail "scope" else r
    let (r, _) := plan.goal.foldl (init := (r, ([] : List (String × String × String)))) fun (r, seen) c =>
      let k := changeKey c
      let r := match lookup r.initDesired k with
        | none => r.fail "scope"
        | some cur =>
          if seen.contains k then r.fail "frame"
          else if cur != c.fromV then r.fail "frame"
          else if c.fromV == c.toV then r.fail "frame"
          else if c.field == "replicas" && (match c.toV with | .int i => decide (i > lim.maxReplicas) | _ => false) then r.fail "limits"
          else if c.field == "image" && (match c.toV with | .str i => r.noDigest i | _ => false) then r.fail "limits"
          else r
      (r, seen ++ [k])
    r

/-- settled, then frame: the plan's effect is exactly its goal. -/
def endCheck (r : Run) : Run :=
  let r := r.depNames.foldl (init := r) fun r d =>
    let rolling := (r.st.dep? d).map (·.rolling) |>.getD false
    if rolling || (r.st.podsOf d).any fun p => p.created && !p.ready then r.fail "settled" else r
  if !r.failures.isEmpty then r
  else
    let goal := r.plan.goal.map fun c => (changeKey c, c.toV)
    let bad := r.st.desired.any fun (k, v) =>
      match lookup goal k with
      | some g => v != g
      | none => some v != lookup r.initDesired k
    if bad then r.fail "frame" else r

/-- After the rollback, each reversible goal field is back to its `from` value. -/
def restored (r : Run) : Run :=
  let final := r.st.desired
  let ok := r.plan.goal.all fun c =>
    c.field == "drained" || c.field == "restarted" || lookup final (changeKey c) == some c.fromV
  if ok then r else r.fail "rollback"

/-- The algorithm of spec 05, steps 3 to 6. -/
def execute (r : Run) : Run :=
  let r := r.static
  if !r.failures.isEmpty then r
  else
    let (r, ok) := r.doSteps r.plan.steps
    if !ok then r
    else
      let r := r.endCheck
      if !r.failures.isEmpty || r.plan.rollback.isEmpty then r
      else
        let (r, ok) := ({ r with rollback := true }).doSteps r.plan.rollback
        if !ok then r
        else ({ r with rollback := false }).restored

def result (r : Run) : Result :=
  if r.unknown.isSome then .unknown else if r.failures.isEmpty then .proved else .refuted

/-- Critical deployments the plan names, or whose pods it moves, replaces or removes. -/
def criticalInvolved (r : Run) : List String :=
  let named := r.plan.goal.map (·.deployment) ++ r.plan.steps.map (·.deployment)
  r.policy.criticalDeps.filter fun d => named.contains d || r.disrupted.contains d

def approvalReasons (r : Run) : List String :=
  r.policy.approvalFor.filter fun reason =>
    match reason with
    | "always" => true
    | "drain" => r.drains
    | "critical" => !r.criticalInvolved.isEmpty
    | "scale-down" => r.scaleDown
    | "image" => r.setsImage
    | _ => false

end Run

/-- What the agreement tests compare: result, the failing rules in spec
order, the Unknown trigger, and the approval reasons of a Proved plan. -/
structure Summary where
  result : Result
  rules : List String
  trigger : Option String
  approval : List String
deriving Repr

def check (snap : Snapshot) (plan : Plan) (pol : Policy) : Summary :=
  let r := (newRun snap plan pol).execute
  { result := r.result,
    rules := rules.filter r.failures.contains,
    trigger := r.unknown,
    approval := if r.result == .proved then r.approvalReasons else [] }

end Planproof
