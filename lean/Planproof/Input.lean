/-!
# Inputs

The three inputs as the Go kernel has them after strict parsing and
validation (spec 01, 02, 04). The Lean checker only ever sees inputs the Go
kernel accepted, so these types carry no validation of their own: every
field the Go kernel requires is present. Optional values that the kernel
reads only for some operations are `Option`s or default to empty.
-/
namespace Planproof

/-- A goal change's `from` and `to` (spec 01): an integer, a boolean or a string. -/
inductive Value where
  | int (i : Int)
  | bool (b : Bool)
  | str (s : String)
deriving DecidableEq, Repr, Inhabited

/-- Resources: CPU in millicores, memory in MiB, and a pod count. -/
structure Res where
  cpu : Int := 0
  mem : Int := 0
  pods : Int := 0
deriving DecidableEq, Repr, Inhabited

def Res.add (a b : Res) : Res := ⟨a.cpu + b.cpu, a.mem + b.mem, a.pods + b.pods⟩

/-- `a` fits within `b` in every resource. -/
def Res.le (a b : Res) : Bool := a.cpu ≤ b.cpu && a.mem ≤ b.mem && a.pods ≤ b.pods

structure SNode where
  name : String
  ready : Bool
  unsched : Bool
  taints : Bool
  alloc : Res
deriving Repr, Inhabited

structure SDeployment where
  ns : String
  name : String
  replicas : Int
  ready : Int
  updated : Int
  recreate : Bool
  maxUnavail : Int
  maxSurge : Int
  containers : List (String × String)
  req : Res
  placement : Bool
  hpa : Bool
deriving Repr, Inhabited

structure SPod where
  ns : String
  name : String
  node : String
  ownerKind : String
  ownerName : String
  ready : Bool
  localStorage : Bool
  placement : Bool
  req : Res
deriving Repr, Inhabited

structure SPdb where
  ns : String
  name : String
  deployment : Option String
  min : Option Int
  max : Option Int
deriving Repr, Inhabited

structure Snapshot where
  nodes : List SNode
  deployments : List SDeployment
  pods : List SPod
  pdbs : List SPdb
deriving Repr, Inhabited

structure Assertions where
  replicas : Option Int := none
  image : Option String := none
  container : String := ""
  unsched : Option Bool := none
  podsOnNode : Option Int := none
deriving Repr, Inhabited

/-- One step (spec 01). `deployment`, `node`, `container` and `image` are ""
when the operation has no such parameter, as in the Go kernel. -/
structure Step where
  op : String
  deployment : String := ""
  node : String := ""
  container : String := ""
  image : String := ""
  replicas : Option Int := none
  deployments : List String := []
  timeout : Option Int := none
  pre : Option Assertions := none
  post : Option Assertions := none
deriving Repr, Inhabited

/-- One goal change: exactly one of `deployment` and `node` is set. -/
structure Change where
  deployment : String := ""
  node : String := ""
  field : String
  container : String := ""
  fromV : Value
  toV : Value
deriving Repr, Inhabited

structure Plan where
  goal : List Change
  steps : List Step
  rollback : List Step
deriving Repr, Inhabited

structure Limits where
  maxReplicas : Int
  maxUnsched : Int
  minSched : Int
  maxWait : Int
  requireDigest : Bool
  allowZero : Bool
deriving Repr, Inhabited

structure Policy where
  managedNamespaces : List String
  managedNodes : List String
  protectedDeps : List String
  criticalDeps : List String
  approvalFor : List String
  limits : Limits
deriving Repr, Inhabited

end Planproof
