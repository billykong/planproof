import Planproof.Spec.Fields

/-!
# Specification: what Kubernetes may do, and rules `budget` and `never-zero`

This file is meant to be read. The checker works with a pessimistic picture
of the cluster; this file says what may really happen, pod by pod, and the
theorem in `Proofs/Budget.lean` says that for a plan the checker proves,
the disruption budgets and the "at least one ready pod" rule hold at every
moment of every way the plan can really run.

It describes Kubernetes as the planproof spec assumes it (spec 02, 03):
- ready pods stay ready (A1); a `wait` ends with every pod of the named
  deployments scheduled and ready (A2); nothing else changes these objects
  (A4); the snapshot is correct (A5);
- the scheduler places a pending pod on any node that is schedulable at the
  time, at any moment (A3 says the checker's model of the scheduler is
  simple; here the scheduler is free);
- a drain evicts every pod on the node except DaemonSet pods, all at once
  (the worst case of evicting them one by one);
- a scale-down removes pods that aren't ready before pods that are;
- during a rolling update, the Deployment controller keeps at least
  `min(available at the start, replicas − maxUnavailable)` pods available
  (none for `Recreate`), unless something else evicts its pods.
-/
namespace Planproof.Spec

/-- Two lists of the same length whose elements are related pairwise. -/
inductive Forall2 {α β : Type} (R : α → β → Prop) : List α → List β → Prop
  | nil : Forall2 R [] []
  | cons {a : α} {b : β} {as : List α} {bs : List β} : R a b → Forall2 R as bs → Forall2 R (a :: as) (b :: bs)

/-- A pod as Kubernetes may really have it. `node` is `none` while the pod
is pending, not yet scheduled; a pending pod is never ready. -/
structure CPod where
  dep : String
  owner : String
  node : Option String
  ready : Bool
deriving DecidableEq, Repr

/-- A deployment mid rollout. The Deployment controller keeps at least
`floor` of its pods available. Its pods are on `oldNodes` (the pods being
replaced) or on `newNodes` (where new pods may be scheduled). -/
structure Rollout where
  dep : String
  floor : Int
  oldNodes : List String
  newNodes : List String
deriving Repr

/-- The cluster at one moment: the fields, every pod except those of
deployments mid rollout, and the rollouts. -/
structure World where
  fields : Fields
  pods : List CPod
  rollouts : List Rollout

def World.initial (s : Snapshot) : World :=
  { fields := Spec.initial s,
    pods := s.pods.map fun p =>
      { dep := if p.ownerKind == "Deployment" then p.ns ++ "/" ++ p.ownerName else "",
        owner := p.ownerKind, node := some p.node, ready := p.ready },
    rollouts := [] }

/-- Nodes a pod may be scheduled on: ready, untainted, not cordoned. -/
def sched (s : Snapshot) (f : Fields) : List String :=
  (s.nodes.filter fun n => n.ready && !n.taints && f (.unsched n.name) == some (.bool false)).map (·.name)

def World.rollout? (w : World) (d : String) : Option Rollout := w.rollouts.find? (·.dep == d)

/-- Available pods of a deployment: its ready pods, or the rollout's floor. -/
def World.avail (w : World) (d : String) : Int :=
  match w.rollout? d with
  | some r => r.floor
  | none => (w.pods.filter fun p => p.dep == d && p.ready).length

def replicasOf (f : Fields) (d : String) : Int :=
  match f (.replicas d) with
  | some (.int n) => n
  | _ => 0

def deployment? (s : Snapshot) (d : String) : Option SDeployment := s.deployments.find? (depName · == d)

/-- A drain evicts the pods on the node, except DaemonSet pods: they are
pending again until the scheduler places them. -/
def evict (n : String) (p : CPod) : CPod :=
  if p.node == some n && p.owner != "DaemonSet" then { p with node := none, ready := false } else p

/-- No guarantee for a deployment's rollout any more (its floor is 0). -/
def dropFloor (d : String) (rs : List Rollout) : List Rollout :=
  rs.map fun r => if r.dep == d then { r with floor := 0 } else r

/-- `scale` of deployment `d` to `n`. -/
def ScaleNext (w : World) (d : String) (n : Int) (w' : World) : Prop :=
  match w.rollout? d with
  | some _ => w'.pods = w.pods ∧ w'.rollouts = dropFloor d w.rollouts
  | none =>
    let r := replicasOf w.fields d
    let mine := w.pods.filter (·.dep == d)
    w'.rollouts = w.rollouts ∧
    if n > r then w'.pods = w.pods ++ List.replicate (n - r).toNat ⟨d, "Deployment", none, false⟩
    else if n < r then
      -- Some of its pods go, those that aren't ready first.
      ∃ keep drop : List CPod, w.pods.Perm (keep ++ drop) ∧ (∀ p ∈ drop, p.dep = d) ∧
        drop.length = mine.length - n.toNat ∧
        ((∃ p ∈ drop, p.ready = true) → ∀ p ∈ keep, p.dep = d → p.ready = true) ∧ w'.pods = keep
    else w'.pods = w.pods

/-- `set-image` or `rollout-restart` of deployment `d`: a rollout starts. -/
def RolloutNext (s : Snapshot) (w : World) (d : String) (w' : World) : Prop :=
  match w.rollout? d with
  | some _ => w'.pods = w.pods ∧ w'.rollouts = dropFloor d w.rollouts
  | none =>
    let floor := match deployment? s d with
      | some sd => if sd.recreate then 0 else max 0 (min (w.avail d) (replicasOf w.fields d - sd.maxUnavail))
      | none => 0
    w'.pods = w.pods.filter (·.dep != d) ∧
    w'.rollouts = w.rollouts ++ [{ dep := d, floor, oldNodes := (w.pods.filter (·.dep == d)).filterMap (·.node),
                                   newNodes := sched s w.fields }]

/-- `wait` for one deployment: its rollout ends with `replicas` new pods,
ready, on nodes new pods may be scheduled on; or every pod it has is
scheduled and ready. -/
def WaitOne (s : Snapshot) (w : World) (d : String) (w' : World) : Prop :=
  w'.fields = w.fields ∧
  match w.rollout? d with
  | some r =>
    w'.rollouts = w.rollouts.filter (·.dep != d) ∧
    ∃ fresh : List CPod, fresh.length = (replicasOf w.fields d).toNat ∧
      (∀ p ∈ fresh, p.dep = d ∧ p.owner = "Deployment" ∧ p.ready = true ∧ ∃ m ∈ r.newNodes, p.node = some m) ∧
      w'.pods = w.pods ++ fresh
  | none =>
    w'.rollouts = w.rollouts ∧
    Forall2 (fun p p' =>
      if p.dep = d then p'.dep = p.dep ∧ p'.owner = p.owner ∧ p'.ready = true ∧
        (p.node = none → ∃ m ∈ sched s w.fields, p'.node = some m) ∧ (p.node ≠ none → p'.node = p.node)
      else p' = p) w.pods w'.pods

inductive WaitAll (s : Snapshot) : World → List String → World → Prop
  | nil (w : World) : WaitAll s w [] w
  | cons (w w1 w2 : World) (d : String) (ds : List String) :
      WaitOne s w d w1 → WaitAll s w1 ds w2 → WaitAll s w (d :: ds) w2

/-- What a step may do to the world (spec 03). -/
def Next (s : Snapshot) (w : World) (st : Step) (w' : World) : Prop :=
  match st.op with
  | "cordon" => w' = { w with fields := step w.fields st }
  | "uncordon" =>
    let f := step w.fields st
    w' = { w with fields := f,
                  rollouts := w.rollouts.map fun r =>
                    if st.node ∈ sched s f then { r with newNodes := r.newNodes ++ [st.node] } else r }
  | "drain" =>
    w' = { fields := step w.fields st, pods := w.pods.map (evict st.node),
           rollouts := w.rollouts.map fun r =>
             if st.node ∈ r.oldNodes ∨ st.node ∈ r.newNodes then { r with floor := 0 } else r }
  | "scale" => w'.fields = step w.fields st ∧ ScaleNext w st.deployment (st.replicas.getD 0) w'
  | "set-image" | "rollout-restart" => w'.fields = step w.fields st ∧ RolloutNext s w st.deployment w'
  | "wait" => WaitAll s w st.deployments w'
  | _ => w' = w

/-- Between steps the cluster settles: the scheduler may place pending pods
on schedulable nodes, and pods may become ready. -/
def Settles (s : Snapshot) (w w' : World) : Prop :=
  w'.fields = w.fields ∧ w'.rollouts = w.rollouts ∧
  Forall2 (fun p p' => p'.dep = p.dep ∧ p'.owner = p.owner ∧
      (p.node = none → p'.node = none ∨ ∃ m ∈ sched s w.fields, p'.node = some m) ∧
      (p.node ≠ none → p'.node = p.node) ∧ (p.ready = true → p'.ready = true) ∧
      (p'.node = none → p'.ready = false)) w.pods w'.pods

/-- A run of steps from a world: before each step the cluster settles, then
the step happens. `ws` is every world the run passes through. -/
inductive Runs (s : Snapshot) : World → List Step → List World → Prop
  | nil (w : World) : Runs s w [] []
  | cons (w w1 w2 : World) (st : Step) (rest : List Step) (ws : List World) :
      Settles s w w1 → Next s w1 st w2 → Runs s w2 rest ws → Runs s w (st :: rest) (w1 :: w2 :: ws)

/-- A disruption budget's requirement for its deployment `d`. -/
def need (w : World) (b : SPdb) (d : String) : Int :=
  match b.min with
  | some m => m
  | none => max 0 (replicasOf w.fields d - b.max.getD 0)

/-- **Rule `budget`.** Every disruption budget on a deployment holds, or the
deployment has no fewer available pods than in the snapshot. -/
def BudgetOK (s : Snapshot) (w : World) : Prop :=
  ∀ b ∈ s.pdbs, ∀ dn, b.deployment = some dn → (deployment? s (b.ns ++ "/" ++ dn)).isSome →
    w.avail (b.ns ++ "/" ++ dn) ≥ need w b (b.ns ++ "/" ++ dn) ∨
    w.avail (b.ns ++ "/" ++ dn) ≥ (World.initial s).avail (b.ns ++ "/" ++ dn)

/-- The goal scales the deployment to zero and the policy allows it. -/
def zeroAllowed (p : Plan) (pol : Policy) (d : String) : Bool :=
  pol.limits.allowZero && p.goal.any fun c => c.deployment == d && c.field == "replicas" && c.toV == Value.int 0

/-- **Rule `never-zero`.** A deployment with a ready pod in the snapshot
always has one, unless the goal scales it to zero and the policy allows it. -/
def NeverZero (s : Snapshot) (p : Plan) (pol : Policy) (w : World) : Prop :=
  ∀ sd ∈ s.deployments, (World.initial s).avail (depName sd) ≥ 1 → zeroAllowed p pol (depName sd) = false →
    w.avail (depName sd) ≥ 1

/-- **Both rules, at every moment of every way the plan and its rollback can
really run.** -/
def AvailabilitySafe (s : Snapshot) (p : Plan) (pol : Policy) : Prop :=
  ∀ ws, Runs s (World.initial s) (allSteps p) ws → ∀ w ∈ ws, BudgetOK s w ∧ NeverZero s p pol w

/-- No resource request is negative. -/
def ReqNonNeg (r : Res) : Prop := 0 ≤ r.cpu ∧ 0 ≤ r.mem ∧ 0 ≤ r.pods

/-- What the Go kernel's validation guarantees and the theorems rely on:
node and deployment names are unique, and neither `maxUnavailable` nor any
resource request is negative. -/
def Valid (s : Snapshot) : Prop :=
  NodesUnique s ∧ (s.deployments.map depName).Nodup ∧ (∀ d ∈ s.deployments, d.maxUnavail ≥ 0) ∧
  (∀ p ∈ s.pods, ReqNonNeg p.req) ∧ ∀ d ∈ s.deployments, ReqNonNeg d.req

/-- What the Go kernel's validation guarantees of a plan and this theorem
relies on: every scale, set-image and rollout-restart names a deployment. -/
def StepsNamed (p : Plan) : Prop :=
  ∀ st ∈ allSteps p, (st.op = "scale" ∨ st.op = "set-image" ∨ st.op = "rollout-restart") → st.deployment ≠ ""

end Planproof.Spec
