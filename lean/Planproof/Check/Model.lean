import Planproof.Input

/-!
# The abstract state (spec 02), as the checker keeps it

A transliteration of the Go kernel's `kernel/model.go`, with lists in place
of maps and pointers. Every list the Go kernel iterates in order is kept in
the same order here; the agreement tests (PRD L-21, L-22) check that the two
give the same verdicts.
-/
namespace Planproof

/-- A modelled pod. `nodes` is where it may be: one node when known, else
the candidate nodes when the model created it. `atNode` is the node first fit
charged its requests to, or "". -/
structure MPod where
  name : String
  dep : String
  owner : String
  nodes : List String
  atNode : String
  req : Res
  ready : Bool
  created : Bool
  gone : Bool
  localStorage : Bool
  placement : Bool
deriving Repr, Inhabited

structure MDep where
  name : String
  replicas : Int
  images : List (String × String)
  restarted : Bool
  rolling : Bool
  /-- While rolling: where the old pods may still be (kernel 0.4.0). -/
  oldNodes : List String
  floor : Int
  maxUnavail : Int
  maxSurge : Int
  recreate : Bool
  req : Res
  placement : Bool
  hpa : Bool
  unsettled : Bool
  pdbs : Nat
  unresolved : Bool
  made : Nat
deriving Repr, Inhabited

structure MNode where
  name : String
  ready : Bool
  unsched : Bool
  taints : Bool
  drained : Bool
  alloc : Res
deriving Repr, Inhabited

structure MPdb where
  name : String
  dep : String
  min : Option Int
  max : Option Int
deriving Repr, Inhabited

structure State where
  deps : List MDep
  nodes : List MNode
  pods : List MPod
  pdbs : List MPdb
deriving Repr, Inhabited

/-- Strings in byte order (the same as Go's for valid UTF-8). -/
def strLe (a b : String) : Bool := decide (a < b) || a == b

def sortStrings (xs : List String) : List String := xs.mergeSort strLe

/-- Remove adjacent duplicates (Go's `slices.Compact`). -/
def compact : List String → List String
  | a :: b :: rest => if a == b then compact (b :: rest) else a :: compact (b :: rest)
  | xs => xs

/-- Set the image of container `c` in a deployment's container list. -/
def setImage (c img : String) (l : List (String × String)) : List (String × String) :=
  l.map fun p => if p.1 == c then (p.1, img) else p

namespace State

def dep? (st : State) (d : String) : Option MDep := st.deps.find? (·.name == d)
def node? (st : State) (n : String) : Option MNode := st.nodes.find? (·.name == n)

def updDep (st : State) (d : String) (f : MDep → MDep) : State :=
  { st with deps := st.deps.map fun x => if x.name == d then f x else x }

def updNode (st : State) (n : String) (f : MNode → MNode) : State :=
  { st with nodes := st.nodes.map fun x => if x.name == n then f x else x }

def updPods (st : State) (sel : MPod → Bool) (f : MPod → MPod) : State :=
  { st with pods := st.pods.map fun p => if sel p then f p else p }

end State

def build (s : Snapshot) : State :=
  let nodes := s.nodes.map fun n =>
    { name := n.name, ready := n.ready, unsched := n.unsched, taints := n.taints, drained := false, alloc := n.alloc : MNode }
  let deps := s.deployments.map fun d =>
    { name := d.ns ++ "/" ++ d.name, replicas := d.replicas, images := d.containers, restarted := false,
      rolling := false, oldNodes := [], floor := 0, maxUnavail := d.maxUnavail, maxSurge := d.maxSurge, recreate := d.recreate,
      req := d.req, placement := d.placement, hpa := d.hpa,
      unsettled := decide (d.ready < d.replicas) || decide (d.updated < d.replicas),
      pdbs := 0, unresolved := false, made := 0 : MDep }
  let pods := s.pods.map fun p =>
    { name := p.ns ++ "/" ++ p.name, dep := if p.ownerKind == "Deployment" then p.ns ++ "/" ++ p.ownerName else "",
      owner := p.ownerKind, nodes := [p.node], atNode := p.node, req := p.req, ready := p.ready, created := false,
      gone := false, localStorage := p.localStorage, placement := p.placement : MPod }
  let st0 : State := { deps, nodes, pods, pdbs := [] }
  let st := s.pdbs.foldl (init := st0) fun st b =>
    match b.deployment with
    | none =>
      -- Not tied to one deployment: any deployment in its namespace may be covered.
      { st with deps := st.deps.map fun d =>
          if (d.name.splitOn "/").head? == some b.ns then { d with unresolved := true } else d }
    | some dn =>
      let key := b.ns ++ "/" ++ dn
      if (st.dep? key).isSome then
        let st := st.updDep key fun d => { d with pdbs := d.pdbs + 1, unresolved := d.unresolved || decide (d.pdbs + 1 > 1) }
        { st with pdbs := st.pdbs ++ [{ name := b.ns ++ "/" ++ b.name, dep := key, min := b.min, max := b.max }] }
      else st
  { st with pdbs := st.pdbs.mergeSort fun a b => strLe a.dep b.dep && (a.dep != b.dep || strLe a.name b.name) }

namespace State

/-- Ready pods of a deployment, or its rollout floor while mid rolling update. -/
def avail (st : State) (d : String) : Int :=
  match st.dep? d with
  | some m => if m.rolling then m.floor else ((st.pods.filter fun p => p.dep == d && p.ready && !p.gone).length : Int)
  | none => ((st.pods.filter fun p => p.dep == d && p.ready && !p.gone).length : Int)

def podsOf (st : State) (d : String) : List MPod := st.pods.filter fun p => p.dep == d && !p.gone

/-- Schedulable nodes, by name: ready, not cordoned, not tainted. -/
def schedulable (st : State) : List String :=
  sortStrings ((st.nodes.filter fun n => n.ready && !n.unsched && !n.taints).map (·.name))

def nodeCounts (st : State) : Int × Int :=
  ((st.nodes.filter (·.unsched)).length, (st.nodes.filter fun n => !n.unsched && n.ready).length)

def need (st : State) (b : MPdb) : Int :=
  match b.min with
  | some m => m
  | none => max 0 (((st.dep? b.dep).map (·.replicas)).getD 0 - b.max.getD 0)

/-- Resources charged to a node. -/
def usage (st : State) (n : String) : Res :=
  st.pods.foldl (fun r p => if p.atNode == n && p.atNode != "" then r.add p.req else r) {}

end State

def MPod.mightBeOn (p : MPod) (n : String) : Bool := !p.gone && p.nodes.contains n

namespace State

/-- `count` new pending pods of a deployment, with the schedulable nodes as
candidates; returns the new state and the new pods' names. -/
def newPods (st : State) (d : String) (count : Int) : State × List String :=
  let sched := st.schedulable
  match st.dep? d with
  | none => (st, [])
  | some m =>
    let k := count.toNat
    let names := (List.range k).map fun i => d ++ "+" ++ toString (m.made + i + 1)
    let pods := names.map fun nm =>
      { name := nm, dep := d, owner := "Deployment", nodes := sched, atNode := "", req := m.req, ready := false,
        created := true, gone := false, localStorage := false, placement := m.placement : MPod }
    ({ (st.updDep d fun x => { x with made := x.made + k }) with pods := st.pods ++ pods }, names)

def fits (st : State) (p : MPod) (n : String) : Bool :=
  match st.node? n with
  | some nd => (st.usage n).add p.req |>.le nd.alloc
  | none => false

def setAt (st : State) (names : List String) (n : String) : State :=
  st.updPods (fun p => names.contains p.name) fun p => { p with atNode := n }

/-- Pod order for first-fit-decreasing: CPU request, largest first, then name. -/
def podBefore (a b : MPod) : Bool := decide (a.req.cpu > b.req.cpu) || (a.req.cpu == b.req.cpu && strLe a.name b.name)

/-- Charge the pod at position `i` to node `n`. -/
def setIdx : List MPod → Nat → String → List MPod
  | [], _, _ => []
  | p :: ps, 0, n => { p with atNode := n } :: ps
  | p :: ps, k + 1, n => p :: setIdx ps k n

def setAtIdx (st : State) (i : Nat) (n : String) : State := { st with pods := setIdx st.pods i n }

/-- First-fit-decreasing (spec 02): returns the new state and the first pod
that fits nowhere, if any. Pods are charged by position, as the Go kernel
charges them by pointer. -/
def place (st : State) (names : List String) : State × Option MPod :=
  let st := st.setAt names ""
  let order := (st.pods.zipIdx.filter fun x => names.contains x.1.name).mergeSort fun a b => podBefore a.1 b.1
  order.foldl (init := (st, none)) fun (st, bad) x =>
    match bad with
    | some _ => (st, bad)
    | none =>
      match x.1.nodes.find? (st.fits x.1) with
      | some n => (st.setAtIdx x.2 n, none)
      | none => (st, some x.1)

def remove (st : State) (names : List String) : State :=
  { st with pods := st.pods.filter fun p => !names.contains p.name }

/-- Every desired field that exists in the state, in a fixed order. -/
def keys (st : State) : List Key :=
  (st.deps.flatMap fun d => [Key.replicas d.name, Key.restarted d.name] ++ d.images.map fun (c, _) => Key.image d.name c) ++
  (st.nodes.flatMap fun n => [Key.unsched n.name, Key.drained n.name])

/-- A desired field's value in the state (spec 02), or `none` if the object
doesn't exist. -/
def field (st : State) : Key → Option Value
  | .replicas d => (st.dep? d).map fun m => .int m.replicas
  | .restarted d => (st.dep? d).map fun m => .bool m.restarted
  | .image d c => ((st.dep? d).bind fun m => (m.images.find? (·.1 == c)).map (·.2)).map .str
  | .unsched n => (st.node? n).map fun m => .bool m.unsched
  | .drained n => (st.node? n).map fun m => .bool m.drained

end State

def lookup {α β : Type} [BEq α] (xs : List (α × β)) (k : α) : Option β := (xs.find? (·.1 == k)).map (·.2)

end Planproof
