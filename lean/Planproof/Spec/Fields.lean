import Planproof.Input

/-!
# Specification: what a plan does to the fields, and three rules about it

This file is meant to be read. It says, without reference to how the
checker works, what each operation does to the fields a goal can name
(spec 02 and 03), and states three of spec 04's rules as properties of a
plan: `frame`, `cordon-first` and the each-state part of `limits`.

The checker proves these with its own data structures; `Proofs/` shows that
whenever the checker says Proved, these properties hold.
-/
namespace Planproof.Spec

/-- The fields a plan can change (spec 02): a deployment's replicas, its
"restarted" flag and each container's image; a node's "unschedulable" and
"drained" flags. `none` means the object doesn't exist. -/
abbrev Fields := Key → Option Value

def depName (d : SDeployment) : String := d.ns ++ "/" ++ d.name

/-- The fields in the snapshot. Nothing is restarted or drained yet. -/
def initial (s : Snapshot) : Fields
  | .replicas d => (s.deployments.find? (depName · == d)).map fun x => .int x.replicas
  | .restarted d => (s.deployments.find? (depName · == d)).map fun _ => .bool false
  | .image d c => ((s.deployments.find? (depName · == d)).bind fun x => (x.containers.find? (·.1 == c)).map (·.2)).map .str
  | .unsched n => (s.nodes.find? (·.name == n)).map fun x => .bool x.unsched
  | .drained n => (s.nodes.find? (·.name == n)).map fun _ => .bool false

/-- Set a field of an object that exists; a missing object stays missing. -/
def setField (f : Fields) (k : Key) (v : Value) : Fields :=
  fun k' => if k' = k then (f k).map fun _ => v else f k'

/-- What each operation does to the fields (spec 03). `wait` changes none. -/
def step (f : Fields) (s : Step) : Fields :=
  match s.op with
  | "cordon" => setField f (.unsched s.node) (.bool true)
  | "uncordon" => setField f (.unsched s.node) (.bool false)
  | "drain" => setField f (.drained s.node) (.bool true)
  | "scale" => setField f (.replicas s.deployment) (.int (s.replicas.getD 0))
  | "set-image" => setField f (.image s.deployment s.container) (.str s.image)
  | "rollout-restart" => setField f (.restarted s.deployment) (.bool true)
  | _ => f

/-- The fields after running some steps from the snapshot. -/
def after (s : Snapshot) (steps : List Step) : Fields := steps.foldl step (initial s)

/-- The value the goal gives a field, if the goal names it. -/
def goalValue (p : Plan) (k : Key) : Option Value :=
  (p.goal.find? fun c => c.key == some k).map (·.toV)

/-- **Rule `frame`.** Every goal change names a field that exists, with its
`from` equal to the snapshot's value and `to` different from it; and after
the plan's steps, every field the goal names has the goal's value and every
other field has its snapshot value. In short: the plan changes exactly what
its goal says. -/
def GoalExact (s : Snapshot) (p : Plan) : Prop :=
  (∀ c ∈ p.goal, ∃ k, c.key = some k ∧ initial s k = some c.fromV ∧ c.fromV ≠ c.toV) ∧
  ∀ k, after s p.steps k = match goalValue p k with
    | some v => some v
    | none => initial s k

/-- The steps the check runs, in order: the plan's steps, then its rollback. -/
def allSteps (p : Plan) : List Step := p.steps ++ p.rollback

/-- **Rule `cordon-first`.** Every drain, in the plan or its rollback, is of
a node that is unschedulable just before it. -/
def CordonFirst (s : Snapshot) (p : Plan) : Prop :=
  ∀ i (h : i < (allSteps p).length), (allSteps p)[i].op = "drain" →
    after s ((allSteps p).take i) (.unsched (allSteps p)[i].node) = some (.bool true)

/-- Nodes that are unschedulable, and nodes that are ready and schedulable,
under some fields. -/
def unschedCount (s : Snapshot) (f : Fields) : Nat :=
  (s.nodes.filter fun n => f (.unsched n.name) == some (.bool true)).length
def schedCount (s : Snapshot) (f : Fields) : Nat :=
  (s.nodes.filter fun n => n.ready && f (.unsched n.name) == some (.bool false)).length

/-- **Rule `limits`, each state.** After every step of the plan and its
rollback, at most `maxUnschedulableNodes` nodes are unschedulable and at
least `minSchedulableNodes` are ready and schedulable, or the count is no
worse than in the snapshot (spec 04: only what the plan makes worse). -/
def NodeLimits (s : Snapshot) (p : Plan) (pol : Policy) : Prop :=
  ∀ i, 1 ≤ i → i ≤ (allSteps p).length →
    let f := after s ((allSteps p).take i)
    ((unschedCount s f : Int) ≤ pol.limits.maxUnsched ∨ unschedCount s f ≤ unschedCount s (initial s)) ∧
    ((schedCount s f : Int) ≥ pol.limits.minSched ∨ schedCount s f ≥ schedCount s (initial s))

/-- What the Go kernel's validation guarantees and these rules rely on:
node names are unique (spec 02; Go's `validateSnapshot`). -/
def NodesUnique (s : Snapshot) : Prop := (s.nodes.map (·.name)).Nodup

end Planproof.Spec
