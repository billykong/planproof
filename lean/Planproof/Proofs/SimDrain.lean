import Planproof.Proofs.SimNodes

/-!
# Each operation keeps the correspondence (part 3: drain)

A drain evicts the pods really on the node. The checker counts every pod
that might be on it as moved, and gives each moved pod as candidates the
schedulable nodes and its other candidates (kernel 0.3.0). Rollouts never
lose their guarantee: a drain of a node a rolling deployment's pods may be
on is Unknown (kernel 0.4.0).
-/
namespace Planproof

open Spec

theorem mem_compact (l : List String) (x : String) : x ∈ compact l ↔ x ∈ l := by
  induction l using compact.induct with
  | case1 a b rest h ih =>
    have hab : a = b := beq_iff_eq.mp h
    subst hab; rw [compact, if_pos h]; simp [ih]
  | case2 a b rest h ih => rw [compact, if_neg h]; simp [ih]
  | case3 xs h => cases xs with
    | nil => simp [compact]
    | cons a t => cases t with
      | nil => simp [compact]
      | cons b r => exact absurd rfl (h a b r)

theorem sched_drained (s : Snapshot) (f : Fields) (n : String) :
    sched s (setField f (.drained n) (.bool true)) = sched s f := by
  unfold sched
  congr 1

/-- What a drain's lack of an Unknown trigger says. -/
theorem trigger_drain_none {st : State} {stp : Step} (hop : stp.op = "drain") (h : trigger st stp = none) :
    (∀ p ∈ st.pods, p.mightBeOn stp.node = true → p.owner ≠ "DaemonSet" → p.owner ≠ "None" →
      p.dep ≠ "" ∧ (st.dep? p.dep).isSome ∧ depTrigger st p.dep = none) ∧
    (∀ m ∈ st.deps, m.rolling = true → stp.node ∉ m.oldNodes) := by
  unfold trigger at h
  simp only [hop] at h
  by_cases hnr : (["cordon", "uncordon", "drain"].contains "drain" &&
      !(Option.map (fun x => x.ready) (st.node? stp.node)).getD false) = true
  · rw [if_pos hnr] at h; cases h
  · rw [if_neg hnr] at h
    have hdd : ("drain" == "drain") = true := rfl
    rw [if_pos hdd] at h
    generalize hfs : List.findSome? _ st.pods = fs at h
    cases fs with
    | some t => cases h
    | none =>
      simp only at h
      by_cases hany : (st.deps.any fun d => d.rolling && d.oldNodes.contains stp.node) = true
      · rw [if_pos hany] at h; cases h
      · refine ⟨?_, ?_⟩
        · intro p hp hm hds hnone
          have := List.findSome?_eq_none_iff.mp hfs p hp
          simp only [hm, Bool.not_true, Bool.false_or] at this
          have hds' : (p.owner == "DaemonSet") = false := by simpa using hds
          have hnone' : (p.owner == "None") = false := by simpa using hnone
          simp only [hds', hnone', Bool.or_false, Bool.false_eq_true, ite_false] at this
          by_cases hl : p.localStorage = true
          · rw [if_pos hl] at this; cases this
          · rw [if_neg hl] at this
            by_cases hd : (p.dep == "" || (st.dep? p.dep).isNone) = true
            · rw [if_pos hd] at this; cases this
            · rw [if_neg hd] at this
              simp only [Bool.or_eq_true, beq_iff_eq, not_or, Option.isNone_iff_eq_none] at hd
              refine ⟨hd.1, by cases h' : st.dep? p.dep <;> simp_all, ?_⟩
              cases ht : depTrigger st p.dep with
              | none => rfl
              | some t => simp [ht] at this
        · intro m hm hr hn
          apply hany
          simp only [List.any_eq_true, Bool.and_eq_true]
          exact ⟨m, hm, hr, by simpa using hn⟩

theorem depTrigger_none_rolling {st : State} {d : String} {m : MDep} (h : depTrigger st d = none)
    (hm : st.dep? d = some m) : m.rolling = false := by
  unfold depTrigger at h
  rw [hm] at h
  simp only at h
  split at h; · cases h
  split at h; · cases h
  split at h; · cases h
  split at h; · cases h
  rename_i h4; simpa using h4

theorem Sim.drain {s : Snapshot} {st : State} {w w' : World} {stp : Step} (hu : NodesUnique s)
    (hs : Sim s st w) (hop : stp.op = "drain") (htr : trigger st stp = none) (hdr : drainFails st stp.node = [])
    (hn : Next s w stp w') : Sim s (applyOp st stp).1 w' := by
  simp only [Next, hop] at hn
  subst hn
  obtain ⟨htp, hold⟩ := trigger_drain_none hop htr
  have hstep : Spec.step w.fields stp = setField w.fields (.drained stp.node) (.bool true) := by
    simp [Spec.step, hop]
  let schA := st.schedulable
  let sel : MPod → Bool := fun p => p.dep != "" && p.mightBeOn stp.node
  let upd : MPod → MPod := fun p =>
    { p with ready := false, created := true, nodes := compact (sortStrings (schA ++ p.nodes.filter (· != stp.node))) }
  let st1 := st.updPods sel upd
  let st2 := st1.updNode stp.node fun n => { n with drained := true }
  have happ : (applyOp st stp).1 = (st2.place ((st.pods.filter fun p => p.dep != "" && p.mightBeOn stp.node).map (·.name))).1 := by
    simp only [applyOp, hop]; rfl
  rw [happ]
  apply Sim.congr _ (SameUpTo.place _ _)
  have hsch : ∀ m, m ∈ sched s w.fields ↔ m ∈ schA := by
    intro m; rw [hs.fields]; exact (mem_schedulable hu hs.nodes m).symm
  have hrep : ∀ d, replicasOf (Spec.step w.fields stp) d = replicasOf w.fields d := by
    intro d; rw [hstep, replicasOf_setField_drained]
  have hsch' : sched s (Spec.step w.fields stp) = sched s w.fields := by rw [hstep, sched_drained]
  -- No pod of a deployment mid rollout might be on the node.
  have hnoroll : ∀ p ∈ st.pods, p.gone = false → st.rolling p.dep = true → p.mightBeOn stp.node = false := by
    intro p hp hg hr
    cases hm : p.mightBeOn stp.node with
    | false => rfl
    | true =>
        exfalso
        have hdne : p.dep ≠ "" := by
          intro h0
          cases hd : st.dep? p.dep with
          | none => simp [State.rolling, hd] at hr
          | some m => exact hs.names m (List.mem_of_find?_eq_some hd) (by rw [State.name_of_dep? hd]; exact h0)
        have ho := hs.owner p hp hdne
        obtain ⟨-, -, hdt⟩ := htp p hp hm (by rw [ho]; decide) (by rw [ho]; decide)
        cases hd : st.dep? p.dep with
        | none => simp [State.rolling, hd] at hr
        | some m =>
          have := depTrigger_none_rolling hdt hd
          simp [State.rolling, hd, this] at hr
  have hsel_roll : ∀ p ∈ st.pods, p.gone = false → st.rolling p.dep = true → sel p = false := by
    intro p hp hg hr; simp [sel, hnoroll p hp hg hr]
  refine ⟨?_, ?_, ?_, ?_, ?_, ?_, ?_, ?_⟩
  · show Spec.step w.fields stp = _
    rw [hstep, hs.fields]
    show _ = (st1.updNode stp.node fun n => { n with drained := true }).field
    rw [State.field_updNode_drained, State.field_updPods]
  · obtain ⟨al, hp, hf⟩ := hs.pods
    refine ⟨al.map (evict stp.node), hp.map _, ?_⟩
    show Forall2 _ (st1.updNode stp.node _).live _
    rw [State.live_updNode, State.live_updPods st sel upd (fun _ => ⟨rfl, rfl⟩)]
    simp only [hsch']
    refine hf.map_both_mem _ _ (fun p q hpl hm => ?_)
    have hpst : p ∈ st.pods := (List.mem_filter.mp hpl).1
    have hpg : p.gone = false := by
      simp only [State.live, List.mem_filter, Bool.and_eq_true, Bool.not_eq_true'] at hpl; exact hpl.2.1
    obtain ⟨h1, h2, h3, h4⟩ := hm
    have hupd : ∀ m ∈ schA, m ∈ (upd p).nodes := by
      intro m hm; simp [upd, mem_compact, mem_sortStrings, hm]
    by_cases hev : q.node = some stp.node ∧ q.owner ≠ "DaemonSet"
    · -- Really on the node: evicted, and the checker moved it.
      have hmight : p.mightBeOn stp.node = true := by
        have h4' := h4
        rw [hev.1] at h4'
        simp [MPod.mightBeOn, hpg, h4']
      have hpo : p.owner ≠ "DaemonSet" := by rw [← h2]; exact hev.2
      have hpn : p.owner ≠ "None" := by
        intro hnone
        unfold drainFails at hdr
        simp only [List.append_eq_nil_iff, List.map_eq_nil_iff, List.filter_eq_nil_iff] at hdr
        have := hdr.2 p hpst
        simp [hnone, hmight] at this
      have hdne := (htp p hpst hmight hpo hpn).1
      have hsel : sel p = true := by simp [sel, hdne, hmight]
      have hq' : evict stp.node q = { q with node := none, ready := false } := by
        unfold evict
        rw [if_pos]
        simp only [Bool.and_eq_true, beq_iff_eq, bne_iff_ne, ne_eq]
        exact ⟨hev.1, hev.2⟩
      rw [hq', if_pos hsel]
      exact ⟨h1, h2, fun h => by simp [upd] at h, rfl, rfl, fun m hm => hupd m ((hsch m).mp hm)⟩
    · have hq : evict stp.node q = q := by
        unfold evict
        rw [if_neg]
        simp only [Bool.and_eq_true, beq_iff_eq, bne_iff_ne, ne_eq]
        exact hev
      rw [hq]
      by_cases hs' : sel p = true
      · rw [if_pos hs']
        refine ⟨h1, h2, fun h => by simp [upd] at h, ?_⟩
        cases hqn : q.node with
        | some m =>
          rw [hqn] at h4
          show m ∈ (upd p).nodes
          by_cases hmn : m = stp.node
          · exfalso
            subst hmn
            have hds : q.owner = "DaemonSet" := by
              by_cases hne : q.owner = "DaemonSet"
              · exact hne
              · exact absurd ⟨hqn, hne⟩ hev
            have hdne : p.dep ≠ "" := by
              have := hs'
              simp only [sel, Bool.and_eq_true, bne_iff_ne, ne_eq] at this
              exact this.1
            have := hs.owner p hpst hdne
            rw [← h2, hds] at this; cases this
          · simp [upd, mem_compact, mem_sortStrings, h4, hmn]
        | none =>
          rw [hqn] at h4
          exact ⟨rfl, h4.2.1, fun m hm => hupd m ((hsch m).mp hm)⟩
      · rw [if_neg hs']
        exact ⟨h1, h2, h3, h4⟩
  · intro d
    show (World.rollout? _ d).isSome = (st1.updNode stp.node _).rolling d
    rw [State.rolling_updNode, State.rolling_updPods, ← hs.rollIff d]
    simp only [World.rollout?]
    rw [rollout?_map _ _ (fun r => by split <;> rfl)]
    cases w.rollouts.find? (·.dep == d) <;> rfl
  · intro d r' hr'
    simp only [World.rollout?] at hr'
    rw [rollout?_map _ _ (fun r => by split <;> rfl)] at hr'
    cases hr0 : w.rollouts.find? (·.dep == d) with
    | none => rw [hr0] at hr'; cases hr'
    | some r =>
      rw [hr0] at hr'
      simp only [Option.map_some, Option.some.injEq] at hr'
      obtain ⟨m, hm, hfl, hold', hnew, hcount, hmax⟩ := hs.roll d r hr0
      have hrolling : st.rolling d = true := by
        rw [← hs.rollIff d]; simp [World.rollout?, hr0]
      have hmr : m.rolling = true := by simpa [State.rolling, hm] using hrolling
      have hmmem : m ∈ st.deps := List.mem_of_find?_eq_some hm
      have hdne : d ≠ "" := by rw [← State.name_of_dep? hm]; exact hs.names m hmmem
      -- The rollout keeps its guarantee, or it has no pods at all.
      have hfloor : m.floor ≤ r'.floor := by
        rw [← hr']
        split
        · rename_i hc
          rcases hc with hc | hc
          · exact absurd (hold' _ hc) (hold m hmmem hmr)
          · by_cases hex : ∃ p ∈ st.pods, p.dep = d ∧ p.gone = false
            · obtain ⟨p, hp, hpd, hpg⟩ := hex
              have hmight : p.mightBeOn stp.node = true := by
                simp [MPod.mightBeOn, hpg, (hnew p hp hpd hpg).2 _ hc]
              exact absurd hmight (by rw [hnoroll p hp hpg (by rw [hpd]; exact hrolling)]; decide)
            · have hzero : (st.pods.filter fun p => p.dep == d && !p.gone).length = 0 := by
                rw [List.length_eq_zero_iff, List.filter_eq_nil_iff]
                intro p hp hc'
                apply hex
                simp only [Bool.and_eq_true, beq_iff_eq, Bool.not_eq_true'] at hc'
                exact ⟨p, hp, hc'⟩
              rw [hcount] at hzero
              have : replicasOf w.fields d ≤ 0 := by omega
              simp only
              omega
        · exact hfl
      refine ⟨m, ?_, hfloor, ?_, ?_, ?_, ?_⟩
      · show (st1.updNode stp.node _).dep? d = some m
        rfl
      · rw [← hr']; split <;> exact hold'
      · intro p hp hpd hpg
        simp only [State.updNode] at hp
        simp only [st1, State.updPods, List.mem_map] at hp
        obtain ⟨p0, hp0, rfl⟩ := hp
        have hd0 : p0.dep = d := by split at hpd <;> simpa [upd] using hpd
        have hg0 : p0.gone = false := by split at hpg <;> simpa [upd] using hpg
        have hs0 := hsel_roll p0 hp0 hg0 (by rw [hd0]; exact hrolling)
        rw [if_neg (by simp [hs0])]
        rw [← hr']
        have := hnew p0 hp0 hd0 hg0
        split <;> exact this
      · rw [hrep, ← hcount]
        show (List.filter _ (st.pods.map _)).length = _
        rw [List.filter_map, List.length_map]
        congr 1
        apply List.filter_congr
        intro p _
        simp only [Function.comp]
        split <;> rfl
      · rw [hrep]; exact hmax
  · intro p hp hd
    simp only [State.updNode, st1, State.updPods, List.mem_map] at hp
    obtain ⟨p0, hp0, rfl⟩ := hp
    have : p0.dep ≠ "" := by split at hd <;> simpa [upd] using hd
    have ho := hs.owner p0 hp0 this
    split <;> simpa [upd] using ho
  · show (st1.updNode stp.node _).nodes.map nodeKey = _
    rw [nodeKeys_updNode st1 stp.node (fun n => { n with drained := true }) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)]
    exact hs.nodes
  · exact hs.mu
  · exact hs.names

end Planproof
