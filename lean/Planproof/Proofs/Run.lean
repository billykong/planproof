import Planproof.Proofs.Fields

/-!
# What a successful run of steps tells us

When `doSteps` ends with no failure and no Unknown trigger, every step had
no trigger, passed its `pre` checks (and, for a drain, `cordon-first` and
`no-orphans`), found room for every pod (`capacity`), and left a state that
passes the each-state rules and its `post` checks. The state at the end is
the operations applied one after another.
-/
namespace Planproof

/-- The state after applying some operations, one after another. -/
def runOps (st : State) (steps : List Step) : State := steps.foldl (fun st s => (applyOp st s).1) st

/-- A run of steps ended with no failure and no Unknown trigger. -/
def Stop.ok (r : Stop) : Prop := r.failures = [] ∧ r.unknown = none

/-- What one step of a successful run passed, from the state before it. -/
structure StepOK (c : Ctx) (pre : State) (s : Step) : Prop where
  noTrigger : trigger pre s = none
  preOK : assertFails pre s s.pre = []
  drainOK : s.op = "drain" → drainFails pre s.node = []
  placed : (applyOp pre s).2 = true
  eachOK : eachStateFails c (applyOp pre s).1 = []
  postOK : assertFails (applyOp pre s).1 s s.post = []

theorem doSteps_ok {c : Ctx} {inPlan : Bool} {st : State} {fl : Flags} {steps : List Step}
    (h : (doSteps c inPlan st fl steps).ok) :
    (doSteps c inPlan st fl steps).st = runOps st steps ∧
    ∀ i (hi : i < steps.length), StepOK c (runOps st (steps.take i)) steps[i] := by
  induction steps generalizing st fl with
  | nil => exact ⟨rfl, fun i hi => absurd hi (Nat.not_lt_zero _)⟩
  | cons s rest ih =>
    unfold doSteps at h ⊢
    cases ht : trigger st s with
    | some t => simp [ht, Stop.ok] at h
    | none =>
      simp only [ht] at h ⊢
      by_cases hpre : (assertFails st s s.pre ++ (if s.op == "drain" then drainFails st s.node else [])).isEmpty
      · simp only [hpre, Bool.not_true, Bool.false_eq_true, ite_false] at h ⊢
        generalize happ : applyOp st s = a at h ⊢
        obtain ⟨st', ok⟩ := a
        simp only at h ⊢
        by_cases hfs : ((if ok then [] else ["capacity"]) ++ eachStateFails c st' ++ assertFails st' s s.post).isEmpty
        · simp only [hfs, ite_true] at h ⊢
          obtain ⟨hst, hall⟩ := ih h
          have hok : ok = true := by cases ok <;> simp_all
          have hrun : runOps st (s :: rest) = runOps st' rest := by
            simp [runOps, List.foldl_cons, happ]
          refine ⟨hst.trans hrun.symm, ?_⟩
          intro i hi
          cases i with
          | zero =>
            simp only [List.take_zero, List.getElem_cons_zero]
            have hrun0 : runOps st [] = st := rfl
            rw [hrun0]
            simp only [List.isEmpty_iff, List.append_eq_nil_iff] at hpre hfs
            refine ⟨ht, hpre.1, fun hd => by simpa [hd] using hpre.2, ?_, ?_, ?_⟩
            · rw [happ]; exact hok
            · rw [happ]; exact hfs.1.2
            · rw [happ]; exact hfs.2
          | succ j =>
            have := hall j (by simpa using hi)
            simp only [List.take_succ_cons, List.getElem_cons_succ]
            have hrunj : runOps st (s :: rest.take j) = runOps st' (rest.take j) := by
              simp [runOps, List.foldl_cons, happ]
            rw [hrunj]
            exact this
        · rw [if_neg hfs] at h
          exact absurd (List.isEmpty_iff.mpr h.1) hfs
      · have hpre' : (!(assertFails st s s.pre ++ (if s.op == "drain" then drainFails st s.node else [])).isEmpty) = true := by
          simpa using hpre
        rw [if_pos hpre'] at h
        exact absurd (List.isEmpty_iff.mpr h.1) hpre

/-- When every step finds room, the fields after the steps are `Spec.step`
applied one step after another. -/
theorem runOps_field (st : State) (steps : List Step)
    (h : ∀ i (hi : i < steps.length), (applyOp (runOps st (steps.take i)) steps[i]).2 = true) :
    (runOps st steps).field = steps.foldl Spec.step st.field := by
  induction steps generalizing st with
  | nil => rfl
  | cons s rest ih =>
    have h0 := h 0 (by simp)
    simp only [List.take_zero, List.getElem_cons_zero] at h0
    have : runOps st (s :: rest) = runOps (applyOp st s).1 rest := rfl
    rw [this, List.foldl_cons, ← State.field_applyOp st s h0]
    apply ih
    intro i hi
    have := h (i + 1) (by simpa using hi)
    simpa [List.take_succ_cons, runOps, List.foldl_cons] using this

end Planproof

namespace Planproof

/-- What a Proved check passed: the static rules, every step of the plan,
the end checks, and, when there is one, every step of the rollback and the
restore check. -/
structure Passed (c : Ctx) : Prop where
  static : staticFails c = []
  steps : (doSteps c true c.st0 {} c.plan.steps).ok
  ends : endFails c (doSteps c true c.st0 {} c.plan.steps).st = []
  rollback : c.plan.rollback ≠ [] →
    (doSteps c false (doSteps c true c.st0 {} c.plan.steps).st (doSteps c true c.st0 {} c.plan.steps).flags c.plan.rollback).ok

theorem execute_passed (c : Ctx) (h : (execute c).result = .proved) : Passed c := by
  unfold Stop.result at h
  unfold execute at h
  by_cases hs : (staticFails c).isEmpty
  · have hs' : staticFails c = [] := List.isEmpty_iff.mp hs
    simp only [hs, Bool.not_true, Bool.false_eq_true, ite_false] at h
    generalize hr : doSteps c true c.st0 {} c.plan.steps = r at h
    by_cases hok : r.unknown.isSome || !r.failures.isEmpty
    · rw [if_pos hok] at h
      exfalso
      cases hu : r.unknown with
      | some _ => simp [hu] at h
      | none =>
        simp only [hu, Option.isSome_none, Bool.false_or] at hok
        simp only [hu, Option.isSome_none, Bool.false_eq_true, ite_false] at h
        split at h
        · simp_all
        · simp at h
    · rw [if_neg hok] at h
      have hrok : r.ok := by
        simp only [Bool.or_eq_true, Bool.not_eq_true', not_or, Option.not_isSome_iff_eq_none] at hok
        exact ⟨List.isEmpty_iff.mp (by simpa using hok.2), hok.1⟩
      by_cases he : (!(endFails c r.st).isEmpty || c.plan.rollback.isEmpty)
      · rw [if_pos he] at h
        have hu : r.unknown = none := hrok.2
        simp only [hu, Option.isSome_none, Bool.false_eq_true, ite_false] at h
        split at h
        · rename_i hef
          refine ⟨hs', hr ▸ hrok, ?_, ?_⟩
          · subst hr; exact List.isEmpty_iff.mp hef
          · intro hne
            exfalso
            simp only [Bool.or_eq_true, Bool.not_eq_true', List.isEmpty_iff] at he
            rcases he with he | he
            · simp_all
            · exact hne he
        · simp at h
      · rw [if_neg he] at h
        simp only [Bool.or_eq_true, Bool.not_eq_true', not_or, List.isEmpty_iff] at he
        refine ⟨hs', hr ▸ hrok, hr ▸ (by simpa using he.1), fun _ => ?_⟩
        subst hr
        generalize hrb : doSteps c false _ _ c.plan.rollback = rb at h
        by_cases hrbok : rb.unknown.isSome || !rb.failures.isEmpty
        · rw [if_pos hrbok] at h
          exfalso
          cases hu : rb.unknown with
          | some _ => simp [hu] at h
          | none =>
            simp only [hu, Option.isSome_none, Bool.false_or, Bool.not_eq_true', List.isEmpty_iff] at hrbok
            simp only [hu, Option.isSome_none, Bool.false_eq_true, ite_false, List.isEmpty_iff, List.map_eq_nil_iff] at h
            simp_all
        · simp only [Bool.or_eq_true, Bool.not_eq_true', not_or, Option.not_isSome_iff_eq_none] at hrbok
          exact ⟨List.isEmpty_iff.mp (by simpa using hrbok.2), hrbok.1⟩
  · have hs'' : (!(staticFails c).isEmpty) = true := by simpa using hs
    rw [if_pos hs''] at h
    simp only [Option.isSome_none, Bool.false_eq_true, ite_false] at h
    split at h
    · simp_all
    · simp at h

end Planproof

namespace Planproof

theorem runOps_append (st : State) (a b : List Step) : runOps st (a ++ b) = runOps (runOps st a) b := by
  simp [runOps, List.foldl_append]

/-- In a Proved check, every step of the plan and then of its rollback
passed, each from the state the steps before it leave. -/
theorem passed_all {c : Ctx} (hp : Passed c) :
    ∀ i (hi : i < (Spec.allSteps c.plan).length),
      StepOK c (runOps c.st0 ((Spec.allSteps c.plan).take i)) (Spec.allSteps c.plan)[i] := by
  obtain ⟨hrun, hok⟩ := doSteps_ok hp.steps
  intro i hi
  unfold Spec.allSteps at hi ⊢
  by_cases hlt : i < c.plan.steps.length
  · rw [List.take_append_of_le_length (Nat.le_of_lt hlt), List.getElem_append_left hlt]
    exact hok i hlt
  · have hge : c.plan.steps.length ≤ i := Nat.le_of_not_lt hlt
    have hne : c.plan.rollback ≠ [] := by
      intro he; simp [he] at hi; omega
    obtain ⟨hrun2, hok2⟩ := doSteps_ok (hp.rollback hne)
    rw [hrun] at hok2
    have hj : i - c.plan.steps.length < c.plan.rollback.length := by simp at hi; omega
    have := hok2 (i - c.plan.steps.length) hj
    rw [List.getElem_append_right hge]
    rw [List.take_append, List.take_of_length_le hge, runOps_append]
    simpa using this

/-- The fields after a prefix of the steps of a Proved check. -/
theorem passed_field {c : Ctx} (hp : Passed c) (i : Nat) (hi : i ≤ (Spec.allSteps c.plan).length) :
    (runOps c.st0 ((Spec.allSteps c.plan).take i)).field = ((Spec.allSteps c.plan).take i).foldl Spec.step c.st0.field := by
  apply runOps_field
  intro j hj
  simp only [List.length_take] at hj
  have hj' : j < (Spec.allSteps c.plan).length := by omega
  have := (passed_all hp j hj').placed
  rw [List.take_take, Nat.min_eq_left (by omega)]
  simpa [List.getElem_take] using this

end Planproof

namespace Planproof

@[simp] theorem Ctx.make_plan (snap : Snapshot) (plan : Plan) (pol : Policy) : (Ctx.make snap plan pol).plan = plan := rfl
@[simp] theorem Ctx.make_policy (snap : Snapshot) (plan : Plan) (pol : Policy) : (Ctx.make snap plan pol).policy = pol := rfl
@[simp] theorem Ctx.make_st0 (snap : Snapshot) (plan : Plan) (pol : Policy) : (Ctx.make snap plan pol).st0 = build snap := rfl

end Planproof
