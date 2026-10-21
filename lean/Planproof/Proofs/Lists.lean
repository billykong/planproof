import Planproof.Spec.World

/-!
# Lists related element by element

General facts about `Spec.Forall2` used by the budget proof: lengths,
filtering, appending, counting, and how a pairing survives reordering one
side (`List.Perm`).
-/
namespace Planproof.Spec

namespace Forall2

variable {α β γ : Type}

theorem length_eq {R : α → β → Prop} {l1 : List α} {l2 : List β} (h : Forall2 R l1 l2) : l1.length = l2.length := by
  induction h with
  | nil => rfl
  | cons _ _ ih => simp [ih]

theorem imp {R S : α → β → Prop} (hrs : ∀ a b, R a b → S a b) {l1 : List α} {l2 : List β}
    (h : Forall2 R l1 l2) : Forall2 S l1 l2 := by
  induction h with
  | nil => exact .nil
  | cons hab _ ih => exact .cons (hrs _ _ hab) ih

theorem append {R : α → β → Prop} {a1 a2 : List α} {b1 b2 : List β} (h1 : Forall2 R a1 b1) (h2 : Forall2 R a2 b2) :
    Forall2 R (a1 ++ a2) (b1 ++ b2) := by
  induction h1 with
  | nil => exact h2
  | cons hab _ ih => exact .cons hab ih

theorem filter {R : α → β → Prop} {l1 : List α} {l2 : List β} (p : α → Bool) (q : β → Bool)
    (hpq : ∀ a b, R a b → p a = q b) (h : Forall2 R l1 l2) : Forall2 R (l1.filter p) (l2.filter q) := by
  induction h with
  | nil => exact .nil
  | @cons a b _ _ hab _ ih =>
    simp only [List.filter_cons]
    rw [hpq a b hab]
    split
    · exact .cons hab ih
    · exact ih

theorem map {R : α → β → Prop} {S : α → γ → Prop} {l1 : List α} {l2 : List β} (f : β → γ)
    (hf : ∀ a b, R a b → S a (f b)) (h : Forall2 R l1 l2) : Forall2 S l1 (l2.map f) := by
  induction h with
  | nil => exact .nil
  | cons hab _ ih => exact .cons (hf _ _ hab) ih

theorem map_left {R : α → β → Prop} {S : γ → β → Prop} {l1 : List α} {l2 : List β} (f : α → γ)
    (hf : ∀ a b, R a b → S (f a) b) (h : Forall2 R l1 l2) : Forall2 S (l1.map f) l2 := by
  induction h with
  | nil => exact .nil
  | cons hab _ ih => exact .cons (hf _ _ hab) ih

theorem map_left_mem {R : α → β → Prop} {S : γ → β → Prop} {l1 : List α} {l2 : List β} (f : α → γ)
    (hf : ∀ a b, a ∈ l1 → R a b → S (f a) b) (h : Forall2 R l1 l2) : Forall2 S (l1.map f) l2 := by
  induction h with
  | nil => exact .nil
  | cons hab _ ih => exact .cons (hf _ _ (by simp) hab) (ih (fun a b ha hr => hf a b (by simp [ha]) hr))

theorem map_both_mem {δ : Type} {R : α → β → Prop} {S : γ → δ → Prop} {l1 : List α} {l2 : List β}
    (f : α → γ) (g : β → δ) (hf : ∀ a b, a ∈ l1 → R a b → S (f a) (g b)) (h : Forall2 R l1 l2) :
    Forall2 S (l1.map f) (l2.map g) := by
  induction h with
  | nil => exact .nil
  | cons hab _ ih => exact .cons (hf _ _ (by simp) hab) (ih (fun a b ha hr => hf a b (by simp [ha]) hr))

theorem comp {R : α → β → Prop} {S : β → γ → Prop} {l1 : List α} {l2 : List β} {l3 : List γ}
    (h1 : Forall2 R l1 l2) (h2 : Forall2 S l2 l3) : Forall2 (fun a c => ∃ b, R a b ∧ S b c) l1 l3 := by
  induction h1 generalizing l3 with
  | nil => cases h2; exact .nil
  | cons hab _ ih =>
    cases h2 with
    | cons hbc h2' => exact .cons ⟨_, hab, hbc⟩ (ih h2')

theorem countP_le {R : α → β → Prop} {l1 : List α} {l2 : List β} (P : α → Bool) (Q : β → Bool)
    (hpq : ∀ a b, R a b → P a = true → Q b = true) (h : Forall2 R l1 l2) : l1.countP P ≤ l2.countP Q := by
  induction h with
  | nil => simp
  | @cons a b _ _ hab _ ih =>
    simp only [List.countP_cons]
    have := hpq a b hab
    cases hpa : P a <;> cases hqb : Q b <;> simp_all <;> omega

/-- A pairing survives reordering the right-hand list. -/
theorem perm {R : α → β → Prop} {l1 : List α} {l2 l2' : List β} (h : Forall2 R l1 l2) (hp : l2.Perm l2') :
    ∃ l1', l1'.Perm l1 ∧ Forall2 R l1' l2' := by
  induction hp generalizing l1 with
  | nil => exact ⟨l1, .refl _, h⟩
  | cons x _ ih =>
    cases h with
    | cons hab h' =>
      obtain ⟨l, hl, hr⟩ := ih h'
      exact ⟨_, hl.cons _, .cons hab hr⟩
  | swap x y l =>
    cases h with
    | cons h1 h' =>
      cases h' with
      | cons h2 h'' => exact ⟨_, .swap _ _ _, .cons h2 (.cons h1 h'')⟩
  | trans _ _ ih1 ih2 =>
    obtain ⟨l, hl, hr⟩ := ih1 h
    obtain ⟨l', hl', hr'⟩ := ih2 hr
    exact ⟨l', hl'.trans hl, hr'⟩

/-- A pairing survives reordering the left-hand list. -/
theorem perm_left {R : α → β → Prop} {l1 l1' : List α} {l2 : List β} (h : Forall2 R l1 l2) (hp : l1.Perm l1') :
    ∃ l2', l2'.Perm l2 ∧ Forall2 R l1' l2' := by
  induction hp generalizing l2 with
  | nil => exact ⟨l2, .refl _, h⟩
  | cons x _ ih =>
    cases h with
    | cons hab h' =>
      obtain ⟨l, hl, hr⟩ := ih h'
      exact ⟨_, hl.cons _, .cons hab hr⟩
  | swap x y l =>
    cases h with
    | cons h1 h' =>
      cases h' with
      | cons h2 h'' => exact ⟨_, .swap _ _ _, .cons h2 (.cons h1 h'')⟩
  | trans _ _ ih1 ih2 =>
    obtain ⟨l, hl, hr⟩ := ih1 h
    obtain ⟨l', hl', hr'⟩ := ih2 hr
    exact ⟨l', hl'.trans hl, hr'⟩

/-- Weave: if the elements of `L` that satisfy `p` pair with `ys` and the
others with `xs`, then all of `L` pairs with a reordering of `xs ++ ys`. -/
theorem weave {R : α → β → Prop} (p : α → Bool) (L : List α) (xs ys : List β)
    (hx : Forall2 R (L.filter fun a => !p a) xs) (hy : Forall2 R (L.filter p) ys) :
    ∃ zs, zs.Perm (xs ++ ys) ∧ Forall2 R L zs := by
  induction L generalizing xs ys with
  | nil =>
    cases hx; cases hy; exact ⟨[], .refl _, .nil⟩
  | cons a L ih =>
    by_cases ha : p a = true
    · simp only [List.filter_cons, ha, Bool.not_true, Bool.false_eq_true, ite_false, ite_true] at hx hy
      cases hy with
      | cons hab hy' =>
        rename_i b ys'
        obtain ⟨zs, hz, hr⟩ := ih xs ys' hx hy'
        refine ⟨b :: zs, ?_, .cons hab hr⟩
        exact (hz.cons b).trans List.perm_middle.symm
    · simp only [Bool.not_eq_true] at ha
      simp only [List.filter_cons, ha, Bool.not_false, ite_true, Bool.false_eq_true, ite_false] at hx hy
      cases hx with
      | cons hab hx' =>
        rename_i b xs'
        obtain ⟨zs, hz, hr⟩ := ih xs' ys hx' hy
        exact ⟨b :: zs, hz.cons b, .cons hab hr⟩

end Forall2

end Planproof.Spec
