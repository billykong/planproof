import Planproof.Proofs.Nodes
import Planproof.Proofs.Lists

/-!
# The checker's picture and the real cluster correspond

`Sim s st w`: the checker's state `st` is a pessimistic picture of a real
world `w` (`Spec.World`). Every real pod pairs with one of the checker's
pods (`Match`): same deployment and owner; ready if the checker's pod is
ready; on one of its candidate nodes, or, if still pending, the checker's
pod isn't ready and has every schedulable node as a candidate. A rollout in
the real world keeps at least the checker's floor.

From `Sim`, real availability is never below the checker's count
(`Sim.avail_ge`).
-/
namespace Planproof

open Spec

/-- A real pod `q` fits the checker's pod `p`. -/
def Match (sch : List String) (p : MPod) (q : CPod) : Prop :=
  q.dep = p.dep ∧ q.owner = p.owner ∧ (p.ready = true → q.ready = true) ∧
  match q.node with
  | some n => n ∈ p.nodes
  | none => p.ready = false ∧ q.ready = false ∧ ∀ m ∈ sch, m ∈ p.nodes

/-- The deployment is mid rollout in the checker's state. -/
def State.rolling (st : State) (d : String) : Bool := ((st.dep? d).map (·.rolling)).getD false

/-- The checker's pods that pair with real pods: not gone, and not of a
deployment mid rollout (those are summed up by its rollout). -/
def State.live (st : State) : List MPod := st.pods.filter fun p => !p.gone && !st.rolling p.dep

structure Sim (s : Snapshot) (st : State) (w : World) : Prop where
  fields : w.fields = st.field
  pods : ∃ al, al.Perm w.pods ∧ Forall2 (Match (sched s w.fields)) st.live al
  rollIff : ∀ d, (w.rollout? d).isSome = st.rolling d
  roll : ∀ d r, w.rollout? d = some r → ∃ m, st.dep? d = some m ∧ m.floor ≤ r.floor ∧
    (∀ n ∈ r.oldNodes, n ∈ m.oldNodes) ∧
    (∀ p ∈ st.pods, p.dep = d → p.gone = false → p.ready = false ∧ ∀ n ∈ r.newNodes, n ∈ p.nodes) ∧
    (st.pods.filter fun p => p.dep == d && !p.gone).length = (replicasOf w.fields d).toNat
  owner : ∀ p ∈ st.pods, p.dep ≠ "" → p.owner = "Deployment"
  nodes : st.nodes.map nodeKey = s.nodes.map fun n => (n.name, n.ready, n.taints)

/-! ## Schedulable nodes, two ways -/

theorem mem_sortStrings (xs : List String) (x : String) : x ∈ sortStrings xs ↔ x ∈ xs :=
  (List.mergeSort_perm xs strLe).mem_iff

theorem mem_schedulable {s : Snapshot} {st : State} (hu : NodesUnique s)
    (hk : st.nodes.map nodeKey = s.nodes.map fun n => (n.name, n.ready, n.taints)) (n : String) :
    n ∈ st.schedulable ↔ n ∈ sched s st.field := by
  have hnd : (st.nodes.map (·.name)).Nodup := by
    have : st.nodes.map (·.name) = s.nodes.map (·.name) := by
      have := congrArg (List.map Prod.fst) hk
      rw [List.map_map, List.map_map] at this
      exact this
    rw [this]; exact hu
  unfold State.schedulable sched
  rw [mem_sortStrings]
  simp only [List.mem_map, List.mem_filter]
  constructor
  · rintro ⟨x, ⟨hx, hc⟩, rfl⟩
    have : (x.name, x.ready, x.taints) ∈ s.nodes.map fun n => (n.name, n.ready, n.taints) := by
      rw [← hk]; exact List.mem_map_of_mem hx
    obtain ⟨y, hy, hyx⟩ := List.mem_map.mp this
    simp only [Prod.mk.injEq] at hyx
    refine ⟨y, ⟨hy, ?_⟩, hyx.1⟩
    simp only [Bool.and_eq_true, Bool.not_eq_true'] at hc
    simp only [State.field, State.node?, hyx.1, find?_name_self st.nodes hnd x hx, Option.map_some]
    simp [hyx.2.1, hyx.2.2, hc]
  · rintro ⟨y, ⟨hy, hc⟩, rfl⟩
    have : (y.name, y.ready, y.taints) ∈ st.nodes.map nodeKey := by
      rw [hk]; exact List.mem_map_of_mem (f := fun n => (n.name, n.ready, n.taints)) hy
    obtain ⟨x, hx, hxy⟩ := List.mem_map.mp this
    simp only [nodeKey, Prod.mk.injEq] at hxy
    refine ⟨x, ⟨hx, ?_⟩, hxy.1⟩
    simp only [State.field, State.node?, ← hxy.1, find?_name_self st.nodes hnd x hx, Option.map_some] at hc
    simp only [Bool.and_eq_true, Bool.not_eq_true', beq_iff_eq, Option.some.injEq, Value.bool.injEq] at hc
    simp [hxy.2.1, hxy.2.2, hc]

/-! ## The snapshot -/

def mkPod (p : SPod) : MPod :=
  { name := p.ns ++ "/" ++ p.name, dep := if p.ownerKind == "Deployment" then p.ns ++ "/" ++ p.ownerName else "",
    owner := p.ownerKind, nodes := [p.node], atNode := p.node, req := p.req, ready := p.ready, created := false,
    gone := false, localStorage := p.localStorage, placement := p.placement }

/-- The pod-budget fold in `build` keeps the pods and every deployment's rollout state. -/
theorem build_parts (snap : Snapshot) :
    (build snap).pods = snap.pods.map mkPod ∧ ∀ m ∈ (build snap).deps, m.rolling = false ∧ m.oldNodes = [] ∧ m.floor = 0 := by
  unfold build
  simp only
  generalize hst0 : ({ deps := _, nodes := _, pods := _, pdbs := [] } : State) = st0
  have h0 : st0.pods = snap.pods.map mkPod ∧ ∀ m ∈ st0.deps, m.rolling = false ∧ m.oldNodes = [] ∧ m.floor = 0 := by
    subst hst0
    refine ⟨rfl, ?_⟩
    intro m hm
    simp only [List.mem_map] at hm
    obtain ⟨d, -, rfl⟩ := hm
    exact ⟨rfl, rfl, rfl⟩
  suffices h : ∀ st : State, (st.pods = snap.pods.map mkPod ∧ ∀ m ∈ st.deps, m.rolling = false ∧ m.oldNodes = [] ∧ m.floor = 0) →
      ((snap.pdbs.foldl (init := st) fun st b =>
        match b.deployment with
        | none => { st with deps := st.deps.map fun d =>
            if (d.name.splitOn "/").head? == some b.ns then { d with unresolved := true } else d }
        | some dn =>
          if (st.dep? (b.ns ++ "/" ++ dn)).isSome then
            let st := st.updDep (b.ns ++ "/" ++ dn) fun d => { d with pdbs := d.pdbs + 1, unresolved := d.unresolved || decide (d.pdbs + 1 > 1) }
            { st with pdbs := st.pdbs ++ [{ name := b.ns ++ "/" ++ b.name, dep := b.ns ++ "/" ++ dn, min := b.min, max := b.max }] }
          else st).pods = snap.pods.map mkPod ∧
      ∀ m ∈ (snap.pdbs.foldl (init := st) fun st b =>
        match b.deployment with
        | none => { st with deps := st.deps.map fun d =>
            if (d.name.splitOn "/").head? == some b.ns then { d with unresolved := true } else d }
        | some dn =>
          if (st.dep? (b.ns ++ "/" ++ dn)).isSome then
            let st := st.updDep (b.ns ++ "/" ++ dn) fun d => { d with pdbs := d.pdbs + 1, unresolved := d.unresolved || decide (d.pdbs + 1 > 1) }
            { st with pdbs := st.pdbs ++ [{ name := b.ns ++ "/" ++ b.name, dep := b.ns ++ "/" ++ dn, min := b.min, max := b.max }] }
          else st).deps, m.rolling = false ∧ m.oldNodes = [] ∧ m.floor = 0) from h st0 h0
  intro st hst
  induction snap.pdbs generalizing st with
  | nil => exact hst
  | cons b rest ih =>
    simp only [List.foldl_cons]
    apply ih
    cases b.deployment with
    | none =>
      refine ⟨hst.1, ?_⟩
      intro m hm
      simp only [List.mem_map] at hm
      obtain ⟨d, hd, rfl⟩ := hm
      have := hst.2 d hd
      split <;> exact this
    | some dn =>
      simp only
      split
      · refine ⟨hst.1, ?_⟩
        intro m hm
        simp only [State.updDep, List.mem_map] at hm
        obtain ⟨d, hd, rfl⟩ := hm
        have := hst.2 d hd
        split <;> exact this
      · exact hst

theorem build_rolling (snap : Snapshot) (d : String) : (build snap).rolling d = false := by
  unfold State.rolling
  cases h : (build snap).dep? d with
  | none => rfl
  | some m => simp [(build_parts snap).2 m (List.mem_of_find?_eq_some h)]

theorem build_live (snap : Snapshot) : (build snap).live = snap.pods.map mkPod := by
  unfold State.live
  rw [(build_parts snap).1]
  apply List.filter_eq_self.mpr
  intro p hp
  simp only [List.mem_map] at hp
  obtain ⟨q, -, rfl⟩ := hp
  simp [mkPod, build_rolling]

theorem sim_initial (snap : Snapshot) : Sim snap (build snap) (World.initial snap) where
  fields := (State.field_build snap).symm
  pods := by
    refine ⟨(World.initial snap).pods, .refl _, ?_⟩
    rw [build_live]
    simp only [World.initial]
    induction snap.pods with
    | nil => exact .nil
    | cons p rest ih =>
      refine .cons ?_ ih
      refine ⟨rfl, rfl, fun h => h, ?_⟩
      simp [mkPod]
  rollIff := by
    intro d
    simp [World.rollout?, World.initial, build_rolling]
  roll := by
    intro d r h
    simp [World.rollout?, World.initial] at h
  owner := by
    intro p hp hd
    rw [(build_parts snap).1] at hp
    simp only [List.mem_map] at hp
    obtain ⟨q, -, rfl⟩ := hp
    simp only [mkPod] at hd ⊢
    split at hd
    · rename_i h; simpa using h
    · exact absurd rfl hd
  nodes := nodeKeys_build snap

/-! ## Availability -/

theorem count_avail (st : State) (d : String) (h : st.rolling d = false) :
    (st.pods.filter fun p => p.dep == d && p.ready && !p.gone).length = (st.live.filter fun p => p.dep == d && p.ready).length := by
  unfold State.live
  rw [List.filter_filter]
  congr 1
  apply List.filter_congr
  intro p _
  cases hd : (p.dep == d)
  · simp
  · have : p.dep = d := by simpa using hd
    subst this; simp [h]

theorem State.avail_eq (st : State) (d : String) (h : st.rolling d = false) :
    st.avail d = ((st.pods.filter fun p => p.dep == d && p.ready && !p.gone).length : Int) := by
  unfold State.avail
  cases hm : st.dep? d with
  | none => rfl
  | some m =>
    simp only
    have : m.rolling = false := by simpa [State.rolling, hm] using h
    simp [this]

/-- **Real availability is never below the checker's count.** -/
theorem Sim.avail_ge {s : Snapshot} {st : State} {w : World} (hs : Sim s st w) (d : String) :
    st.avail d ≤ w.avail d := by
  cases hr : w.rollout? d with
  | some r =>
    obtain ⟨m, hm, hf, -⟩ := hs.roll d r hr
    have hroll : st.rolling d = true := by rw [← hs.rollIff d, hr]; rfl
    have : m.rolling = true := by simpa [State.rolling, hm] using hroll
    simp only [State.avail, hm, this, ite_true, World.avail, hr]
    exact hf
  | none =>
    have hroll : st.rolling d = false := by rw [← hs.rollIff d, hr]; rfl
    rw [State.avail_eq st d hroll, count_avail st d hroll]
    simp only [World.avail, hr]
    obtain ⟨al, hp, hf⟩ := hs.pods
    have h1 := hf.countP_le (fun p => p.dep == d && p.ready) (fun q => q.dep == d && q.ready) (by
      intro p q hm hpq
      obtain ⟨hd, -, hr, -⟩ := hm
      simp only [Bool.and_eq_true, beq_iff_eq] at hpq ⊢
      exact ⟨hd.trans hpq.1, hr hpq.2⟩)
    simp only [List.countP_eq_length_filter] at h1
    have h2 := (hp.filter (fun q => q.dep == d && q.ready)).length_eq
    omega

/-! ## Settling -/

theorem match_settle {s : Snapshot} {f : Fields} {p : MPod} {q q' : CPod} (hm : Match (sched s f) p q)
    (he : q'.dep = q.dep ∧ q'.owner = q.owner ∧ (q.node = none → q'.node = none ∨ ∃ m ∈ sched s f, q'.node = some m) ∧
      (q.node ≠ none → q'.node = q.node) ∧ (q.ready = true → q'.ready = true) ∧ (q'.node = none → q'.ready = false)) :
    Match (sched s f) p q' := by
  obtain ⟨hd, ho, hr, hn⟩ := hm
  obtain ⟨hd', ho', hpend, hsched, hready, hpr⟩ := he
  refine ⟨hd'.trans hd, ho'.trans ho, fun h => hready (hr h), ?_⟩
  cases hq : q.node with
  | some n =>
    rw [hq] at hn
    rw [hsched (by simp [hq]), hq]
    exact hn
  | none =>
    rw [hq] at hn
    obtain ⟨hpr', -, hall⟩ := hn
    rcases hpend hq with h | ⟨m, hm, h⟩
    · rw [h]; exact ⟨hpr', hpr h, hall⟩
    · rw [h]; exact hall m hm

theorem Sim.settle {s : Snapshot} {st : State} {w w' : World} (hs : Sim s st w) (he : Settles s w w') : Sim s st w' := by
  obtain ⟨hf, hr, hp⟩ := he
  refine ⟨hf ▸ hs.fields, ?_, fun d => by rw [← hs.rollIff d]; simp [World.rollout?, hr],
    fun d r h => by rw [hf]; exact hs.roll d r (by simpa [World.rollout?, hr] using h), hs.owner, hs.nodes⟩
  obtain ⟨al, hal, hm⟩ := hs.pods
  obtain ⟨al', hal', he'⟩ := hp.perm_left hal.symm
  refine ⟨al', hal', ?_⟩
  rw [hf]
  exact (hm.comp he').imp fun p q' ⟨q, h1, h2⟩ => match_settle h1 h2

end Planproof
