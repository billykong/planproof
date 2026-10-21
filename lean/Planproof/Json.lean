import Lean.Data.Json
import Planproof.Check.Run

/-!
# Reading inputs and writing the summary

Glue for the `planproof-lean` executable (PRD L-20), outside what the
theorems are about. It reads the inputs exactly as the Go kernel received
them (JSON; YAML already converted), only for inputs the Go kernel accepted,
so it does no validation: a missing required field is an error.
-/
namespace Planproof.Json

open Lean (Json)

abbrev Dec := Except String

def field (j : Json) (k : String) : Dec Json := j.getObjVal? k
def opt (j : Json) (k : String) : Option Json :=
  match j.getObjVal? k with
  | .ok Json.null => none
  | .ok v => some v
  | .error _ => none

def str (j : Json) (k : String) : Dec String := do (← field j k).getStr?
def strD (j : Json) (k : String) : String := (opt j k).bind (·.getStr?.toOption) |>.getD ""
def int (j : Json) (k : String) : Dec Int := do (← field j k).getInt?
def intO (j : Json) (k : String) : Dec (Option Int) :=
  match opt j k with
  | some v => some <$> v.getInt?
  | none => pure none
def bool (j : Json) (k : String) : Dec Bool := do (← field j k).getBool?
def arr (j : Json) (k : String) : Dec (List Json) :=
  match opt j k with
  | some v => do return (← v.getArr?).toList
  | none => pure []
def strs (j : Json) (k : String) : Dec (List String) := do (← arr j k).mapM (·.getStr?)

def value (j : Json) : Dec Value :=
  match j with
  | .bool b => pure (.bool b)
  | .str s => pure (.str s)
  | v => .int <$> v.getInt?

def res (j : Json) : Dec Res := do
  return { cpu := ← int j "cpuMilli", mem := ← int j "memoryMiB", pods := ← int j "pods" }

def snapshot (j : Json) : Dec Snapshot := do
  let nodes ← (← arr j "nodes").mapM fun n => do
    return { name := ← str n "name", ready := ← bool n "ready", unsched := ← bool n "unschedulable",
             taints := ← bool n "taints", alloc := ← res (← field n "allocatable") : SNode }
  let deployments ← (← arr j "deployments").mapM fun d => do
    let s ← field d "strategy"
    let containers ← (← arr d "containers").mapM fun c => do return (← str c "name", ← str c "image")
    return { ns := ← str d "namespace", name := ← str d "name", replicas := ← int d "replicas",
             ready := ← int d "readyReplicas", updated := ← int d "updatedReplicas",
             recreate := (← str s "type") == "Recreate",
             maxUnavail := (← intO s "maxUnavailable").getD 0, maxSurge := (← intO s "maxSurge").getD 0,
             containers, req := ← res (← field d "podRequests"),
             placement := ← bool d "placementRules", hpa := ← bool d "hpa" : SDeployment }
  let pods ← (← arr j "pods").mapM fun p => do
    let o ← field p "owner"
    return { ns := ← str p "namespace", name := ← str p "name", node := ← str p "node",
             ownerKind := ← str o "kind", ownerName := strD o "name", ready := ← bool p "ready",
             localStorage := ← bool p "localStorage", placement := ← bool p "placementRules",
             req := ← res (← field p "requests") : SPod }
  let pdbs ← (← arr j "pdbs").mapM fun b => do
    return { ns := ← str b "namespace", name := ← str b "name",
             deployment := (opt b "deployment").bind (·.getStr?.toOption),
             min := ← intO b "minAvailable", max := ← intO b "maxUnavailable" : SPdb }
  return { nodes, deployments, pods, pdbs }

def assertions (j : Json) : Dec Assertions := do
  return { replicas := ← intO j "replicas", image := (opt j "image").bind (·.getStr?.toOption),
           container := strD j "container", unsched := (opt j "unschedulable").bind (·.getBool?.toOption),
           podsOnNode := ← intO j "podsOnNode" }

def step (j : Json) : Dec Step := do
  return { op := ← str j "op", deployment := strD j "deployment", node := strD j "node",
           container := strD j "container", image := strD j "image", replicas := ← intO j "replicas",
           deployments := ← strs j "deployments", timeout := ← intO j "timeoutSeconds",
           pre := ← (opt j "pre").mapM assertions, post := ← (opt j "post").mapM assertions }

def plan (j : Json) : Dec Plan := do
  let goal ← (← arr j "goal").mapM fun c => do
    return { deployment := strD c "deployment", node := strD c "node", field := ← str c "field",
             container := strD c "container", fromV := ← value (← field c "from"),
             toV := ← value (← field c "to") : Change }
  return { goal, steps := ← (← arr j "steps").mapM step, rollback := ← (← arr j "rollback").mapM step }

def policy (j : Json) : Dec Policy := do
  let l ← field j "limits"
  return { managedNamespaces := ← strs j "managedNamespaces", managedNodes := ← strs j "managedNodes",
           protectedDeps := ← strs j "protectedDeployments", criticalDeps := ← strs j "criticalDeployments",
           approvalFor := ← strs j "approvalRequiredFor",
           limits := { maxReplicas := ← int l "maxReplicas", maxUnsched := ← int l "maxUnschedulableNodes",
                       minSched := ← int l "minSchedulableNodes", maxWait := ← int l "maxWaitSeconds",
                       requireDigest := ← bool l "requireImageDigest", allowZero := ← bool l "allowScaleToZero" } }

def resultName : Result → String
  | .proved => "Proved"
  | .refuted => "Refuted"
  | .unknown => "Unknown"

def summary (s : Summary) : Json :=
  Lean.Json.mkObj [("result", Lean.Json.str (resultName s.result)), ("rules", Lean.Json.arr (s.rules.map Lean.Json.str).toArray),
              ("trigger", Lean.Json.str (s.trigger.getD "")), ("approval", Lean.Json.arr (s.approval.map Lean.Json.str).toArray)]

/-- One line in: `{"snapshot": ..., "plan": ..., "policy": ...}`; one line out. -/
def runLine (line : String) : String :=
  let out : Dec Json := do
    let j ← Lean.Json.parse line
    return summary (check (← snapshot (← field j "snapshot")) (← plan (← field j "plan")) (← policy (← field j "policy")))
  match out with
  | .ok j => j.compress
  | .error e => (Lean.Json.mkObj [("error", Lean.Json.str e)]).compress

end Planproof.Json
