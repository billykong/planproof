import Planproof.Proofs.SimOps

/-!
# Each operation keeps the correspondence (part 2: cordon and uncordon)
-/
namespace Planproof

open Spec

theorem match_mono {sch sch' : List String} (h : ∀ m ∈ sch', m ∈ sch) {p : MPod} {q : CPod}
    (hm : Match sch p q) : Match sch' p q := by
  obtain ⟨h1, h2, h3, h4⟩ := hm
  refine ⟨h1, h2, h3, ?_⟩
  cases hq : q.node with
  | some n => rw [hq] at h4; exact h4
  | none => rw [hq] at h4; exact ⟨h4.1, h4.2.1, fun m hm => h4.2.2 m (h m hm)⟩

theorem replicasOf_setField_unsched (f : Fields) (n : String) (v : Value) (d : String) :
    replicasOf (setField f (.unsched n) v) d = replicasOf f d := by
  simp [replicasOf, setField]

theorem replicasOf_setField_drained (f : Fields) (n : String) (d : String) :
    replicasOf (setField f (.drained n) (.bool true)) d = replicasOf f d := by
  simp [replicasOf, setField]

/-- Cordoning only removes schedulable nodes. -/
theorem sched_cordon (s : Snapshot) (f : Fields) (n : String) :
    ∀ m ∈ sched s (setField f (.unsched n) (.bool true)), m ∈ sched s f := by
  intro m hm
  simp only [sched, List.mem_map, List.mem_filter] at hm ⊢
  obtain ⟨y, ⟨hy, hc⟩, rfl⟩ := hm
  refine ⟨y, ⟨hy, ?_⟩, rfl⟩
  by_cases he : y.name = n
  · subst he
    simp only [setField, ite_true] at hc
    cases f (.unsched y.name) <;> simp at hc
  · have : (Key.unsched y.name = Key.unsched n) = False := by simp [he]
    simpa [setField, he] using hc

/-- Uncordoning adds at most that node. -/
theorem sched_uncordon (s : Snapshot) (f : Fields) (n : String) :
    ∀ m ∈ sched s (setField f (.unsched n) (.bool false)), m ∈ sched s f ∨ m = n := by
  intro m hm
  by_cases he : m = n
  · exact Or.inr he
  · left
    simp only [sched, List.mem_map, List.mem_filter] at hm ⊢
    obtain ⟨y, ⟨hy, hc⟩, rfl⟩ := hm
    refine ⟨y, ⟨hy, ?_⟩, rfl⟩
    simpa [setField, he] using hc

theorem State.rolling_updNode (st : State) (n : String) (f : MNode → MNode) (d : String) :
    (st.updNode n f).rolling d = st.rolling d := rfl

theorem State.live_updNode (st : State) (n : String) (f : MNode → MNode) : (st.updNode n f).live = st.live := rfl

theorem Sim.cordon {s : Snapshot} {st : State} {w w' : World} {stp : Step}
    (hs : Sim s st w) (hop : stp.op = "cordon") (hn : Next s w stp w') : Sim s (applyOp st stp).1 w' := by
  simp only [Next, hop] at hn
  subst hn
  have happ : (applyOp st stp).1 = st.updNode stp.node (fun n => { n with unsched := true }) := by
    simp [applyOp, hop]
  have hstep : Spec.step w.fields stp = setField w.fields (.unsched stp.node) (.bool true) := by
    simp [Spec.step, hop]
  rw [happ]
  refine ⟨?_, ?_, ?_, ?_, ?_, ?_, hs.mu⟩
  · show Spec.step w.fields stp = _
    rw [hstep, hs.fields]
    exact (State.field_updNode_unsched st stp.node true).symm
  · obtain ⟨al, hp, hf⟩ := hs.pods
    refine ⟨al, hp, ?_⟩
    rw [State.live_updNode]
    simp only [hstep]
    exact hf.imp fun p q hm => match_mono (sched_cordon s w.fields stp.node) hm
  · intro d; rw [State.rolling_updNode]; exact hs.rollIff d
  · intro d r hr
    simp only [hstep, replicasOf_setField_unsched]
    exact hs.roll d r hr
  · exact hs.owner
  · rw [nodeKeys_updNode st stp.node (fun n => { n with unsched := true }) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)]
    exact hs.nodes

theorem State.live_updPods (st : State) (sel : MPod → Bool) (f : MPod → MPod)
    (hf : ∀ p, (f p).gone = p.gone ∧ (f p).dep = p.dep) :
    (st.updPods sel f).live = st.live.map fun p => if sel p then f p else p := by
  unfold State.live State.updPods
  simp only
  rw [List.filter_map]
  congr 1
  apply List.filter_congr
  intro p _
  simp only [Function.comp]
  split
  · rw [(hf p).1, (hf p).2]; rfl
  · rfl

theorem State.rolling_updPods (st : State) (sel : MPod → Bool) (f : MPod → MPod) (d : String) :
    (st.updPods sel f).rolling d = st.rolling d := rfl

theorem rollout?_map (rs : List Rollout) (g : Rollout → Rollout) (hg : ∀ r, (g r).dep = r.dep) (d : String) :
    (rs.map g).find? (·.dep == d) = (rs.find? (·.dep == d)).map g :=
  find?_map_same rs g _ (fun r => by simp [hg])

/-- Schedulable in the checker's state: the node exists, is ready, not cordoned and untainted. -/
theorem mem_schedulable_iff (st : State) (hnd : (st.nodes.map (·.name)).Nodup) (n : String) :
    n ∈ st.schedulable ↔ ∃ x, st.node? n = some x ∧ x.ready = true ∧ x.unsched = false ∧ x.taints = false := by
  unfold State.schedulable
  rw [mem_sortStrings]
  simp only [List.mem_map, List.mem_filter, Bool.and_eq_true, Bool.not_eq_true']
  constructor
  · rintro ⟨x, ⟨hx, ⟨hr, hu⟩, ht⟩, rfl⟩
    exact ⟨x, find?_name_self st.nodes hnd x hx, hr, hu, ht⟩
  · rintro ⟨x, hx, hr, hu, ht⟩
    exact ⟨x, ⟨List.mem_of_find?_eq_some hx, ⟨hr, hu⟩, ht⟩, State.name_of_node? hx⟩

theorem nodes_nodup {s : Snapshot} {st : State} (hu : NodesUnique s)
    (hk : st.nodes.map nodeKey = s.nodes.map fun n => (n.name, n.ready, n.taints)) : (st.nodes.map (·.name)).Nodup := by
  have : st.nodes.map (·.name) = s.nodes.map (·.name) := by
    have := congrArg (List.map Prod.fst) hk
    rw [List.map_map, List.map_map] at this
    exact this
  rw [this]; exact hu

theorem Sim.uncordon {s : Snapshot} {st : State} {w w' : World} {stp : Step} (hu : NodesUnique s)
    (hs : Sim s st w) (hop : stp.op = "uncordon") (hn : Next s w stp w') : Sim s (applyOp st stp).1 w' := by
  simp only [Next, hop] at hn
  subst hn
  have hstep : Spec.step w.fields stp = setField w.fields (.unsched stp.node) (.bool false) := by
    simp [Spec.step, hop]
  let st1 := st.updNode stp.node (fun n => { n with unsched := false })
  have hf1 : st1.field = setField w.fields (.unsched stp.node) (.bool false) := by
    rw [hs.fields]; exact State.field_updNode_unsched st stp.node false
  have hk1 : st1.nodes.map nodeKey = s.nodes.map fun n => (n.name, n.ready, n.taints) := by
    rw [nodeKeys_updNode st stp.node (fun n => { n with unsched := false }) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)]
    exact hs.nodes
  -- The node is schedulable now, in the checker's terms and in the spec's.
  have hcond : ((st1.node? stp.node).any fun n => n.ready && !n.taints) = true ↔
      stp.node ∈ sched s (setField w.fields (.unsched stp.node) (.bool false)) := by
    rw [← hf1, ← mem_schedulable hu hk1, mem_schedulable_iff st1 (nodes_nodup hu hk1)]
    constructor
    · intro h
      cases hx : st1.node? stp.node with
      | none => simp [hx] at h
      | some x =>
        simp only [hx, Option.any_some, Bool.and_eq_true, Bool.not_eq_true'] at h
        refine ⟨x, rfl, h.1, ?_, h.2⟩
        have := State.node?_updNode st stp.node stp.node (fun n => { n with unsched := false }) (fun _ => rfl)
        change st1.node? stp.node = _ at this
        rw [hx] at this
        cases hx0 : st.node? stp.node with
        | none => simp [hx0] at this
        | some y =>
          rw [hx0] at this
          simp only [Option.map_some, Option.some.injEq] at this
          rw [this]; split <;> simp_all [State.name_of_node? hx0]
    · rintro ⟨x, hx, hr, -, ht⟩
      simp [hx, hr, ht]
  let sel : MPod → Bool := fun p => !p.ready && !p.gone && !p.nodes.contains stp.node
  let upd : MPod → MPod := fun p => { p with nodes := sortStrings (p.nodes ++ [stp.node]) }
  have happ : (applyOp st stp).1 = if ((st1.node? stp.node).any fun n => n.ready && !n.taints) then
      st1.updPods sel upd else st1 := by
    simp only [applyOp, hop]
    split <;> rfl
  rw [happ]
  have hrep : ∀ d, replicasOf (Spec.step w.fields stp) d = replicasOf w.fields d := by
    intro d; rw [hstep, replicasOf_setField_unsched]
  by_cases hc : ((st1.node? stp.node).any fun n => n.ready && !n.taints) = true
  · rw [if_pos hc]
    have hsch := hcond.mp hc
    have hsch' : stp.node ∈ sched s (Spec.step w.fields stp) := by rw [hstep]; exact hsch
    let g : Rollout → Rollout := fun r =>
      if stp.node ∈ sched s (Spec.step w.fields stp) then { r with newNodes := r.newNodes ++ [stp.node] } else r
    have hg : ∀ r, g r = { r with newNodes := r.newNodes ++ [stp.node] } := by intro r; simp [g, hsch']
    have hro : ∀ d, ({ w with fields := Spec.step w.fields stp, rollouts := w.rollouts.map g } : World).rollout? d =
        (w.rollout? d).map g := by
      intro d; exact rollout?_map w.rollouts g (fun r => by simp [hg]) d
    -- A pod that isn't ready and isn't gone gets the node.
    have hgets : ∀ p : MPod, p.ready = false → p.gone = false → stp.node ∈ (if sel p = true then upd p else p).nodes := by
      intro p hr hg'
      by_cases hin : p.nodes.contains stp.node = true
      · split
        · simp [upd, mem_sortStrings]
        · simpa using hin
      · have hin' : p.nodes.contains stp.node = false := by simpa using hin
        have hnm : stp.node ∉ p.nodes := fun h => by simp [h] at hin'
        have : sel p = true := by simp [sel, hr, hg', hnm]
        rw [if_pos this]; simp [upd, mem_sortStrings]
    have hsub : ∀ p : MPod, ∀ m ∈ p.nodes, m ∈ (if sel p = true then upd p else p).nodes := by
      intro p m hm; split
      · simp [upd, mem_sortStrings, hm]
      · exact hm
    refine ⟨?_, ?_, ?_, ?_, ?_, ?_, hs.mu⟩
    · show Spec.step w.fields stp = _
      rw [hstep, State.field_updPods]; exact hf1.symm
    · obtain ⟨al, hp, hf⟩ := hs.pods
      refine ⟨al, hp, ?_⟩
      rw [State.live_updPods st1 sel upd (fun _ => ⟨rfl, rfl⟩)]
      change Forall2 _ (st.live.map _) al
      simp only [hstep]
      refine hf.map_left_mem _ (fun p q hpl hm => ?_)
      have hpg : p.gone = false := by
        simp only [State.live, List.mem_filter, Bool.and_eq_true, Bool.not_eq_true'] at hpl; exact hpl.2.1
      obtain ⟨h1, h2, h3, h4⟩ := hm
      refine ⟨?_, ?_, ?_, ?_⟩
      · split <;> exact h1
      · split <;> exact h2
      · split <;> exact h3
      · cases hq : q.node with
        | some m => rw [hq] at h4; exact hsub p m h4
        | none =>
          rw [hq] at h4
          obtain ⟨hpr, hqr, hall⟩ := h4
          refine ⟨by split <;> exact hpr, hqr, fun m hm => ?_⟩
          rcases sched_uncordon s w.fields stp.node m hm with hm | rfl
          · exact hsub p m (hall m hm)
          · exact hgets p hpr hpg
    · intro d
      rw [State.rolling_updPods, State.rolling_updNode, ← hs.rollIff d, hro]
      cases w.rollout? d <;> rfl
    · intro d r' hr'
      rw [hro] at hr'
      cases hr0 : w.rollout? d with
      | none => rw [hr0] at hr'; cases hr'
      | some r =>
        rw [hr0] at hr'
        simp only [Option.map_some, Option.some.injEq] at hr'
        subst hr'
        obtain ⟨m, hm, hfl, hold, hnew, hcount, hmax⟩ := hs.roll d r hr0
        refine ⟨m, hm, by simpa [hg] using hfl, by simpa [hg] using hold, ?_, ?_, by rw [hrep]; exact hmax⟩
        · intro p hp hpd hpg
          simp only [State.updPods, List.mem_map] at hp
          obtain ⟨p0, hp0, rfl⟩ := hp
          have hd0 : p0.dep = d := by split at hpd <;> simpa [upd] using hpd
          have hg0 : p0.gone = false := by split at hpg <;> simpa [upd] using hpg
          obtain ⟨hr0', hn0⟩ := hnew p0 (show p0 ∈ st.pods from hp0) hd0 hg0
          refine ⟨by split <;> simpa [upd] using hr0', ?_⟩
          intro n hn
          rw [hg] at hn
          simp only [List.mem_append, List.mem_singleton] at hn
          rcases hn with hn | rfl
          · exact hsub p0 n (hn0 n hn)
          · exact hgets p0 hr0' hg0
        · rw [hrep, ← hcount]
          simp only [State.updPods, List.filter_map, List.length_map]
          congr 1
          apply List.filter_congr
          intro p _
          simp only [Function.comp]
          split <;> rfl
    · intro p hp hd
      simp only [State.updPods, List.mem_map] at hp
      obtain ⟨p0, hp0, rfl⟩ := hp
      have : p0.dep ≠ "" := by split at hd <;> simpa [upd] using hd
      have ho := hs.owner p0 (show p0 ∈ st.pods from hp0) this
      split <;> simpa [upd] using ho
    · exact hk1
  · rw [if_neg hc]
    have hnot : stp.node ∉ sched s (Spec.step w.fields stp) := by
      rw [hstep]; intro h; exact hc (hcond.mpr h)
    have hroll : w.rollouts.map (fun r => if stp.node ∈ sched s (Spec.step w.fields stp)
        then { r with newNodes := r.newNodes ++ [stp.node] } else r) = w.rollouts := by
      simp [hnot]
    have hsub : ∀ m ∈ sched s (Spec.step w.fields stp), m ∈ sched s w.fields := by
      intro m hm
      rcases sched_uncordon s w.fields stp.node m (by rw [← hstep]; exact hm) with h | rfl
      · exact h
      · exact absurd hm hnot
    refine ⟨?_, ?_, ?_, ?_, ?_, ?_, hs.mu⟩
    · show Spec.step w.fields stp = _
      rw [hstep]; exact hf1.symm
    · obtain ⟨al, hp, hf⟩ := hs.pods
      refine ⟨al, hp, ?_⟩
      rw [State.live_updNode]
      exact hf.imp fun p q hm => match_mono hsub hm
    · intro d
      rw [State.rolling_updNode, ← hs.rollIff d]
      simp only [World.rollout?, hroll]
    · intro d r hr
      simp only [World.rollout?, hroll] at hr
      rw [hrep]
      exact hs.roll d r hr
    · exact hs.owner
    · exact hk1

end Planproof
