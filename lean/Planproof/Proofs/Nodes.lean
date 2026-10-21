import Planproof.Proofs.Frame

/-!
# Rules `cordon-first` and node `limits` are sound (PRD L-31, L-32)
-/
namespace Planproof

open Spec

/-- The fields after a prefix of a Proved check's steps are the spec's. -/
theorem passed_after {snap : Snapshot} {plan : Plan} {pol : Policy} (hp : Passed (Ctx.make snap plan pol))
    (i : Nat) (hi : i ≤ (allSteps plan).length) :
    (runOps (build snap) ((allSteps plan).take i)).field = after snap ((allSteps plan).take i) := by
  have := passed_field hp i (by simpa using hi)
  simp only [Ctx.make_plan, Ctx.make_st0] at this
  rw [this, State.field_build]
  rfl

/-- **Rule `cordon-first` is sound.** If the checker says Proved, every
drain in the plan and its rollback is of a node that is unschedulable just
before it. -/
theorem cordon_first_sound (snap : Snapshot) (plan : Plan) (pol : Policy)
    (h : (check snap plan pol).result = .proved) : CordonFirst snap plan := by
  have hp : Passed (Ctx.make snap plan pol) := execute_passed _ h
  intro i hi hdrain
  have hok := (passed_all hp i hi).drainOK hdrain
  unfold drainFails at hok
  simp only [List.append_eq_nil_iff] at hok
  have h1 := hok.1
  simp only [Ctx.make_plan, Ctx.make_st0] at h1
  rw [← passed_after hp i (Nat.le_of_lt hi)]
  simp only [State.field]
  cases hn : (runOps (build snap) ((allSteps plan).take i)).node? (allSteps plan)[i].node with
  | none => simp [hn] at h1
  | some x => simp_all

/-! ## Nodes keep their names and readiness -/

def nodeKey (n : MNode) : String × Bool × Bool := (n.name, n.ready, n.taints)

theorem nodes_newPods (st : State) (d : String) (k : Int) : (st.newPods d k).1.nodes = st.nodes := by
  unfold State.newPods; split <;> rfl

theorem nodes_surgeFits (st : State) (d : String) (m : MDep) : (st.surgeFits d m).1.nodes = st.nodes := by
  unfold State.surgeFits; split
  · rfl
  · exact nodes_newPods st d m.maxSurge

theorem nodes_rollout (st : State) (s : Step) (m : MDep) : (st.rollout s m).1.nodes = st.nodes := by
  unfold State.rollout; simp only; rw [State.nodes_place, nodes_newPods]; rfl

theorem nodes_wait (st : State) (ds : List String) :
    (ds.foldl (init := st) fun st d =>
      let st := st.updDep d fun x => { x with rolling := false, oldNodes := [] }
      st.updPods (fun p => p.dep == d && !p.gone) fun p => { p with ready := true }).nodes = st.nodes := by
  induction ds generalizing st with
  | nil => rfl
  | cons d rest ih => simp only [List.foldl_cons]; rw [ih]; rfl

theorem nodeKeys_updNode (st : State) (n : String) (f : MNode → MNode) (hn : ∀ x, (f x).name = x.name)
    (hr : ∀ x, (f x).ready = x.ready) (ht : ∀ x, (f x).taints = x.taints) :
    (st.updNode n f).nodes.map nodeKey = st.nodes.map nodeKey := by
  unfold State.updNode; simp only [List.map_map]
  apply List.map_congr_left
  intro x _
  simp only [Function.comp, nodeKey]
  split <;> simp [hn, hr, ht]

theorem nodeKeys_applyOp (st : State) (s : Step) : (applyOp st s).1.nodes.map nodeKey = st.nodes.map nodeKey := by
  by_cases h1 : s.op = "cordon"
  · simp only [applyOp, h1]; exact nodeKeys_updNode _ _ _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)
  by_cases h2 : s.op = "uncordon"
  · simp only [applyOp, h2]
    split
    · exact nodeKeys_updNode _ _ _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)
    · exact nodeKeys_updNode _ _ _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)
  by_cases h3 : s.op = "drain"
  · simp only [applyOp, h3]; rw [State.nodes_place]; exact nodeKeys_updNode _ _ _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)
  by_cases h4 : s.op = "scale"
  · simp only [applyOp, h4]
    cases st.dep? s.deployment with
    | none => rfl
    | some m =>
      simp only
      split
      · rw [State.nodes_place, nodes_newPods]; rfl
      · split <;> rfl
  by_cases h5 : s.op = "set-image"
  · simp only [applyOp, h5]
    cases st.dep? s.deployment with
    | none => rfl
    | some m =>
      simp only
      generalize hsf : State.surgeFits _ _ _ = sf
      obtain ⟨st2, ok⟩ := sf
      have : st2.nodes = st.nodes := by
        have := congrArg (fun x => x.1.nodes) hsf; simp only at this; rw [← this, nodes_surgeFits]; rfl
      cases ok <;> simp only [Bool.false_eq_true, ite_false, ite_true] <;> first | rw [nodes_rollout, this] | rw [this]
  by_cases h6 : s.op = "rollout-restart"
  · simp only [applyOp, h6]
    cases st.dep? s.deployment with
    | none => rfl
    | some m =>
      simp only
      generalize hsf : State.surgeFits _ _ _ = sf
      obtain ⟨st2, ok⟩ := sf
      have : st2.nodes = st.nodes := by
        have := congrArg (fun x => x.1.nodes) hsf; simp only at this; rw [← this, nodes_surgeFits]; rfl
      cases ok <;> simp only [Bool.false_eq_true, ite_false, ite_true] <;> first | rw [nodes_rollout, this] | rw [this]
  by_cases h7 : s.op = "wait"
  · simp only [applyOp, h7]; rw [nodes_wait]
  · simp only [applyOp, h1, h2, h3, h4, h5, h6, h7]

theorem nodeKeys_runOps (st : State) (steps : List Step) : (runOps st steps).nodes.map nodeKey = st.nodes.map nodeKey := by
  induction steps generalizing st with
  | nil => rfl
  | cons s rest ih => exact (ih _).trans (nodeKeys_applyOp st s)

theorem nodeKeys_build (snap : Snapshot) : (build snap).nodes.map nodeKey = snap.nodes.map fun n => (n.name, n.ready, n.taints) := by
  unfold build
  simp only
  generalize hst0 : ({ deps := _, nodes := _, pods := _, pdbs := [] } : State) = st0
  suffices h : ∀ st : State, (snap.pdbs.foldl (init := st) fun st b =>
      match b.deployment with
      | none => { st with deps := st.deps.map fun d =>
          if (d.name.splitOn "/").head? == some b.ns then { d with unresolved := true } else d }
      | some dn =>
        if (st.dep? (b.ns ++ "/" ++ dn)).isSome then
          let st := st.updDep (b.ns ++ "/" ++ dn) fun d => { d with pdbs := d.pdbs + 1, unresolved := d.unresolved || decide (d.pdbs + 1 > 1) }
          { st with pdbs := st.pdbs ++ [{ name := b.ns ++ "/" ++ b.name, dep := b.ns ++ "/" ++ dn, min := b.min, max := b.max }] }
        else st).nodes = st.nodes by
    have h1 := congrArg (List.map nodeKey) (h st0)
    refine h1.trans ?_
    subst hst0
    simp [nodeKey, List.map_map, Function.comp_def]
  intro st
  induction snap.pdbs generalizing st with
  | nil => rfl
  | cons b rest ih =>
    simp only [List.foldl_cons]; rw [ih]
    cases b.deployment with
    | none => rfl
    | some dn => simp only; split <;> rfl

/-! ## Counting nodes by name -/

theorem find?_name_self (L : List MNode) (h : (L.map (·.name)).Nodup) (n : MNode) (hn : n ∈ L) :
    L.find? (·.name == n.name) = some n := by
  induction L with
  | nil => cases hn
  | cons a rest ih =>
    simp only [List.map_cons, List.nodup_cons] at h
    rcases List.mem_cons.mp hn with rfl | hn
    · simp
    · have hne : a.name ≠ n.name := by
        intro he; apply h.1; rw [he]; exact List.mem_map_of_mem hn
      simp only [List.find?_cons]
      have : (a.name == n.name) = false := by simpa using hne
      rw [this]; exact ih h.2 hn

theorem length_filter_comp {α β : Type} (l : List α) (k : α → β) (p : β → Bool) :
    (l.filter (p ∘ k)).length = ((l.map k).filter p).length := by
  rw [List.filter_map, List.length_map]

/-- The spec's counts are the checker's, for a state whose nodes are the
snapshot's nodes, by name and readiness, with unique names. -/
theorem counts_eq (snap : Snapshot) (st : State) (hk : st.nodes.map nodeKey = snap.nodes.map fun n => (n.name, n.ready, n.taints))
    (hu : NodesUnique snap) :
    (unschedCount snap st.field : Int) = st.nodeCounts.1 ∧ (schedCount snap st.field : Int) = st.nodeCounts.2 := by
  have hnd : (st.nodes.map (·.name)).Nodup := by
    have : st.nodes.map (·.name) = snap.nodes.map (·.name) := by
      have := congrArg (List.map Prod.fst) hk
      rw [List.map_map, List.map_map] at this
      exact this
    rw [this]; exact hu
  let P : String × Bool × Bool → Bool := fun p => ((st.nodes.find? (·.name == p.1)).map fun m => Value.bool m.unsched) == some (.bool true)
  let Q : String × Bool × Bool → Bool := fun p => p.2.1 && ((st.nodes.find? (·.name == p.1)).map fun m => Value.bool m.unsched) == some (.bool false)
  have e1 : unschedCount snap st.field = ((snap.nodes.map fun n => (n.name, n.ready, n.taints)).filter P).length := by
    unfold unschedCount; rw [← length_filter_comp]; rfl
  have e2 : schedCount snap st.field = ((snap.nodes.map fun n => (n.name, n.ready, n.taints)).filter Q).length := by
    unfold schedCount; rw [← length_filter_comp]; rfl
  rw [e1, e2, ← hk, ← length_filter_comp, ← length_filter_comp]
  unfold State.nodeCounts
  simp only
  constructor
  · congr 2
    apply List.filter_congr
    intro n hn
    simp only [Function.comp, nodeKey, P, find?_name_self st.nodes hnd n hn, Option.map_some]
    cases n.unsched <;> simp
  · congr 2
    apply List.filter_congr
    intro n hn
    simp only [Function.comp, nodeKey, Q, find?_name_self st.nodes hnd n hn, Option.map_some]
    cases n.unsched <;> cases n.ready <;> simp

/-- **Rule `limits` (each state) is sound.** If the checker says Proved,
then after every step of the plan and its rollback, the node counts are
within the policy's limits or no worse than in the snapshot. (Node names
are unique, as the Go kernel's validation requires.) -/
theorem limits_sound (snap : Snapshot) (plan : Plan) (pol : Policy) (hu : NodesUnique snap)
    (h : (check snap plan pol).result = .proved) : NodeLimits snap plan pol := by
  have hp : Passed (Ctx.make snap plan pol) := execute_passed _ h
  intro i hi1 hi2
  obtain ⟨j, rfl⟩ : ∃ j, i = j + 1 := ⟨i - 1, by omega⟩
  have hj : j < (allSteps plan).length := by omega
  have hok := (passed_all hp j (by simpa using hj)).eachOK
  simp only [Ctx.make_plan, Ctx.make_st0] at hok
  have hstate : runOps (build snap) ((allSteps plan).take (j + 1)) =
      (applyOp (runOps (build snap) ((allSteps plan).take j)) (allSteps plan)[j]).1 := by
    rw [List.take_add_one, runOps_append, List.getElem?_eq_getElem hj]
    rfl
  rw [← hstate] at hok
  have hf := passed_after hp (j + 1) hi2
  have hk := (nodeKeys_runOps (build snap) ((allSteps plan).take (j + 1))).trans (nodeKeys_build snap)
  obtain ⟨c1, c2⟩ := counts_eq snap _ hk hu
  obtain ⟨c01, c02⟩ := counts_eq snap (build snap) (nodeKeys_build snap) hu
  rw [State.field_build] at c01 c02
  rw [hf] at c1 c2
  dsimp only
  unfold eachStateFails at hok
  simp only [Ctx.make_policy, List.append_eq_nil_iff] at hok
  obtain ⟨⟨⟨-, -⟩, h3⟩, h4⟩ := hok
  have hi0 : (Ctx.make snap plan pol).initUnsched = (build snap).nodeCounts.1 := rfl
  have hs0 : (Ctx.make snap plan pol).initSched = (build snap).nodeCounts.2 := rfl
  rw [hi0] at h3
  rw [hs0] at h4
  generalize (runOps (build snap) ((allSteps plan).take (j + 1))).nodeCounts = nc at h3 h4 c1 c2
  have e1 : ((unschedCount snap (initial snap) : Nat) : Int) = (build snap).nodeCounts.1 := c01
  have e2 : ((schedCount snap (initial snap) : Nat) : Int) = (build snap).nodeCounts.2 := c02
  simp only [ite_eq_right_iff, reduceCtorEq, imp_false, Bool.and_eq_true, decide_eq_true_eq, not_and,
    Int.not_lt] at h3 h4
  constructor
  · rw [c1]
    by_cases hm : nc.1 ≤ pol.limits.maxUnsched
    · exact Or.inl hm
    · right; have := h3 (decide_eq_true (by omega)); omega
  · rw [c2]
    by_cases hm : nc.2 ≥ pol.limits.minSched
    · exact Or.inl hm
    · right; have := h4 (decide_eq_true (by omega)); omega

end Planproof
