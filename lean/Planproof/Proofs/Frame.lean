import Planproof.Proofs.Run

/-!
# Rule `frame` is sound (PRD L-30)

If the checker says Proved, the plan changes exactly what its goal says:
`Spec.GoalExact`.
-/
namespace Planproof

open Spec

theorem lookup_goal (goal : List Change) (k : Key) :
    lookup (goal.filterMap fun ch => ch.key.map fun k => (k, ch.toV)) k =
      (goal.find? fun ch => ch.key == some k).map (·.toV) := by
  induction goal with
  | nil => rfl
  | cons ch rest ih =>
    simp only [List.filterMap_cons, List.find?_cons]
    cases hk : ch.key with
    | none => simp [ih]
    | some k' =>
      simp only [Option.map_some]
      unfold lookup at ih ⊢
      simp only [List.find?_cons]
      by_cases he : k' = k
      · subst he; simp
      · have : (k' == k) = false := by simpa using he
        simp only [this]
        have : (some k' == some k) = false := by simpa using he
        simp only [this]
        exact ih

theorem State.mem_keys {st : State} {k : Key} {v : Value} (h : st.field k = some v) : k ∈ st.keys := by
  unfold State.keys
  cases k with
  | replicas d =>
    simp only [State.field, Option.map_eq_some_iff] at h
    obtain ⟨m, hm, -⟩ := h
    have hmem := List.mem_of_find?_eq_some hm
    have hn := State.name_of_dep? hm
    simp only [List.mem_append, List.mem_flatMap]
    exact Or.inl ⟨m, hmem, by simp [hn]⟩
  | restarted d =>
    simp only [State.field, Option.map_eq_some_iff] at h
    obtain ⟨m, hm, -⟩ := h
    have hmem := List.mem_of_find?_eq_some hm
    have hn := State.name_of_dep? hm
    simp only [List.mem_append, List.mem_flatMap]
    exact Or.inl ⟨m, hmem, by simp [hn]⟩
  | image d c =>
    simp only [State.field, Option.map_eq_some_iff, Option.bind_eq_some_iff] at h
    obtain ⟨i, ⟨m, hm, p, hp, rfl⟩, -⟩ := h
    have hmem := List.mem_of_find?_eq_some hm
    have hn := State.name_of_dep? hm
    have hpm := List.mem_of_find?_eq_some hp
    have hpc : p.1 = c := by simpa using List.find?_some hp
    simp only [List.mem_append, List.mem_flatMap]
    exact Or.inl ⟨m, hmem, by simp only [List.mem_append, List.mem_map]; exact Or.inr ⟨p, hpm, by simp [hn, hpc]⟩⟩
  | unsched n =>
    simp only [State.field, Option.map_eq_some_iff] at h
    obtain ⟨m, hm, -⟩ := h
    have hmem := List.mem_of_find?_eq_some hm
    have hn := State.name_of_node? hm
    simp only [List.mem_append, List.mem_flatMap]
    exact Or.inr ⟨m, hmem, by simp [hn]⟩
  | drained n =>
    simp only [State.field, Option.map_eq_some_iff] at h
    obtain ⟨m, hm, -⟩ := h
    have hmem := List.mem_of_find?_eq_some hm
    have hn := State.name_of_node? hm
    simp only [List.mem_append, List.mem_flatMap]
    exact Or.inr ⟨m, hmem, by simp [hn]⟩

theorem setField_isSome (f : Fields) (k k' : Key) (v : Value) : (setField f k v k').isSome = (f k').isSome := by
  unfold setField
  split
  · subst_vars; simp
  · rfl

theorem step_cases (f : Fields) (s : Step) : Spec.step f s = f ∨ ∃ k v, Spec.step f s = setField f k v := by
  unfold Spec.step
  split <;> first | exact Or.inl rfl | exact Or.inr ⟨_, _, rfl⟩

theorem step_isSome (f : Fields) (s : Step) (k : Key) : (Spec.step f s k).isSome = (f k).isSome := by
  rcases step_cases f s with h | ⟨k', v, h⟩ <;> rw [h]
  exact setField_isSome f k' k v

theorem after_isSome (f : Fields) (steps : List Step) (k : Key) :
    (steps.foldl Spec.step f k).isSome = (f k).isSome := by
  induction steps generalizing f with
  | nil => rfl
  | cons s rest ih => simp only [List.foldl_cons]; rw [ih, step_isSome]

/-- What the static goal checks guarantee about each goal change. -/
theorem goalFails_nil (c : Ctx) (seen : List (Option Key)) (goal : List Change) (h : goalFails c seen goal = []) :
    ∀ ch ∈ goal, ∃ k, ch.key = some k ∧ c.st0.field k = some ch.fromV ∧ ch.fromV ≠ ch.toV := by
  induction goal generalizing seen with
  | nil => intro ch hch; cases hch
  | cons ch rest ih =>
    unfold goalFails at h
    simp only [List.append_eq_nil_iff] at h
    obtain ⟨h1, h2⟩ := h
    intro x hx
    rcases List.mem_cons.mp hx with rfl | hx
    · cases hk : x.key with
      | none => simp [hk] at h1
      | some k =>
        cases hf : c.st0.field k with
        | none => simp [hk, hf] at h1
        | some cur =>
          simp only [hk, hf, Option.bind_some] at h1
          refine ⟨k, rfl, ?_⟩
          split at h1
          · simp at h1
          split at h1
          · simp at h1
          rename_i hcur
          split at h1
          · simp at h1
          rename_i hne
          refine ⟨?_, ?_⟩
          · simp only [bne_iff_ne, ne_eq, Decidable.not_not] at hcur
            rw [hf, hcur]
          · simpa using hne
    · exact ih _ h2 x hx

/-- What the end check guarantees: every field of the final state has the
goal's value if the goal names it, else its starting value. -/
theorem endFails_nil (c : Ctx) (st : State) (h : endFails c st = []) :
    ∀ k ∈ st.keys, st.field k = match goalValue c.plan k with
      | some g => some g
      | none => c.st0.field k := by
  unfold endFails at h
  dsimp only at h
  split at h
  · simp_all
  · split at h
    · simp at h
    · rename_i hbad
      intro k hk
      have := List.any_eq_false.mp (by simpa using hbad) k hk
      rw [lookup_goal] at this
      unfold goalValue
      cases hg : (c.plan.goal.find? fun ch => ch.key == some k).map (·.toV) <;> simp_all

/-- **Rule `frame` is sound.** If the checker says Proved, the plan changes
exactly what its goal says. -/
theorem frame_sound (snap : Snapshot) (plan : Plan) (pol : Policy)
    (h : (check snap plan pol).result = .proved) : GoalExact snap plan := by
  let c := Ctx.make snap plan pol
  have hp : Passed c := execute_passed c h
  have hst0 : c.st0.field = initial snap := State.field_build snap
  -- The static checks.
  have hstatic := hp.static
  unfold staticFails at hstatic
  dsimp only at hstatic
  split at hstatic
  · simp_all
  · simp only [List.append_eq_nil_iff] at hstatic
    have hgoal := goalFails_nil c [] plan.goal hstatic.2
    -- The steps.
    have hcp : c.plan = plan := rfl
    obtain ⟨hrun, hok⟩ := doSteps_ok hp.steps
    rw [hcp] at hrun hok
    have hfin : (doSteps c true c.st0 {} plan.steps).st.field = after snap plan.steps := by
      rw [hrun, runOps_field c.st0 plan.steps (fun i hi => (hok i hi).placed), hst0]
      rfl
    have hend := endFails_nil c _ hp.ends
    rw [hcp] at hend
    unfold after at hfin
    refine ⟨fun ch hch => ?_, fun k => ?_⟩
    · obtain ⟨k, hk, hf, hne⟩ := hgoal ch hch
      exact ⟨k, hk, hst0 ▸ hf, hne⟩
    · show List.foldl Spec.step (initial snap) plan.steps k = _
      rw [← hfin]
      by_cases hk : k ∈ (doSteps c true c.st0 {} plan.steps).st.keys
      · rw [hend k hk, hst0]
        cases goalValue plan k <;> rfl
      · -- A field that isn't in the final state exists nowhere.
        have hnone : (doSteps c true c.st0 {} plan.steps).st.field k = none := by
          cases hf : (doSteps c true c.st0 {} plan.steps).st.field k with
          | none => rfl
          | some v => exact absurd (State.mem_keys hf) hk
        have hinit : initial snap k = none := by
          have := after_isSome (initial snap) plan.steps k
          rw [← hfin, hnone] at this
          cases hi : initial snap k <;> simp_all
        rw [hnone]
        cases hg : goalValue plan k with
        | none => exact hinit.symm
        | some v =>
          exfalso
          unfold goalValue at hg
          obtain ⟨ch, hch, -⟩ := Option.map_eq_some_iff.mp hg
          have hmem := List.mem_of_find?_eq_some hch
          have hkey : ch.key = some k := by simpa using List.find?_some hch
          obtain ⟨k', hk', hf, -⟩ := hgoal ch hmem
          rw [hkey] at hk'
          cases hk'
          rw [hst0, hinit] at hf
          cases hf

end Planproof
