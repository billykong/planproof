import Planproof.Proofs.SimDrain

/-!
# Each operation keeps the correspondence (part 4: scale)

Scaling up adds pending pods on both sides. Scaling down is the one place
the pairing has to be rebuilt: Kubernetes may remove different pods than
the checker chose. It removes pods that aren't ready first, so it keeps at
least as many ready pods as the checker counts; and the checker lets every
pod it keeps be anywhere any of the deployment's pods was. So the pods
both sides keep can be paired again (`match_exists`).
-/
namespace Planproof

open Spec

theorem forall2_exists_of_mem_right {α β : Type} {R : α → β → Prop} {L : List α} {al : List β}
    (h : Forall2 R L al) {b : β} (hb : b ∈ al) : ∃ a ∈ L, R a b := by
  induction h with
  | nil => cases hb
  | @cons a b' as bs hab _ ih =>
    rcases List.mem_cons.mp hb with rfl | hb
    · exact ⟨a, by simp, hab⟩
    · obtain ⟨x, hx, hr⟩ := ih hb
      exact ⟨x, by simp [hx], hr⟩

/-- Pods the checker keeps (all with candidates `U`) and pods Kubernetes
keeps can be paired, if there are as many of each, Kubernetes keeps at
least as many ready ones, and every real pod is on a node of `U` or is
pending with every schedulable node in `U`. -/
theorem match_exists (sch U : List String) (d : String) :
    ∀ (as : List MPod) (bs : List CPod),
    (∀ a ∈ as, a.nodes = U ∧ a.dep = d ∧ a.owner = "Deployment") →
    (∀ b ∈ bs, b.dep = d ∧ b.owner = "Deployment" ∧ (∀ m, b.node = some m → m ∈ U) ∧
      (b.node = none → b.ready = false ∧ ∀ m ∈ sch, m ∈ U)) →
    as.length = bs.length →
    (as.filter (·.ready)).length ≤ (bs.filter (·.ready)).length →
    ∃ bs', bs'.Perm bs ∧ Forall2 (Match sch) as bs' := by
  intro as
  induction as with
  | nil =>
    intro bs _ _ hlen _
    cases bs with
    | nil => exact ⟨[], .refl _, .nil⟩
    | cons _ _ => simp at hlen
  | cons a as ih =>
    intro bs has hbs hlen hready
    have ha := has a (by simp)
    have has' : ∀ x ∈ as, x.nodes = U ∧ x.dep = d ∧ x.owner = "Deployment" := fun x hx => has x (by simp [hx])
    -- Pair `a` with `b`, then the rest by induction.
    have pair : ∀ b ∈ bs, (a.ready = true → b.ready = true) →
        (as.filter (·.ready)).length ≤ ((bs.erase b).filter (·.ready)).length →
        ∃ bs', bs'.Perm bs ∧ Forall2 (Match sch) (a :: as) bs' := by
      intro b hb hrb hcnt
      have hb' := hbs b hb
      have hbs' : ∀ x ∈ bs.erase b, x.dep = d ∧ x.owner = "Deployment" ∧ (∀ m, x.node = some m → m ∈ U) ∧
          (x.node = none → x.ready = false ∧ ∀ m ∈ sch, m ∈ U) := fun x hx => hbs x (List.mem_of_mem_erase hx)
      have hlen' : as.length = (bs.erase b).length := by
        rw [List.length_erase_of_mem hb]; simp at hlen; omega
      obtain ⟨rest, hrest, hf⟩ := ih (bs.erase b) has' hbs' hlen' hcnt
      refine ⟨b :: rest, (hrest.cons b).trans (List.perm_cons_erase hb).symm, .cons ?_ hf⟩
      refine ⟨hb'.1.trans ha.2.1.symm, hb'.2.1.trans ha.2.2.symm, hrb, ?_⟩
      cases hn : b.node with
      | some m => rw [ha.1]; exact hb'.2.2.1 m hn
      | none =>
        obtain ⟨hbr, hall⟩ := hb'.2.2.2 hn
        refine ⟨?_, hbr, fun m hm => by rw [ha.1]; exact hall m hm⟩
        cases har : a.ready
        · rfl
        · have := hrb har; rw [hbr] at this; cases this
    have hcount_erase : ∀ b ∈ bs, ((bs.erase b).filter (·.ready)).length + (if b.ready then 1 else 0) =
        (bs.filter (·.ready)).length := by
      intro b hb
      have := (List.perm_cons_erase hb).filter (·.ready)
      rw [this.length_eq]
      simp only [List.filter_cons]
      split <;> simp_all
    by_cases har : a.ready = true
    · -- `a` is ready: pair it with a ready pod.
      have hra : (as.filter (·.ready)).length + 1 ≤ (bs.filter (·.ready)).length := by
        simp [List.filter_cons, har] at hready; omega
      obtain ⟨b, hb, hbr⟩ : ∃ b ∈ bs, b.ready = true := by
        have : (bs.filter (·.ready)) ≠ [] := by intro h0; rw [h0] at hra; simp at hra
        obtain ⟨b, hb⟩ := List.exists_mem_of_ne_nil _ this
        exact ⟨b, (List.mem_filter.mp hb).1, by simpa using (List.mem_filter.mp hb).2⟩
      apply pair b hb (fun _ => hbr)
      have := hcount_erase b hb; simp [hbr] at this; omega
    · have har' : a.ready = false := by simpa using har
      have hra : (as.filter (·.ready)).length ≤ (bs.filter (·.ready)).length := by
        simp [List.filter_cons, har'] at hready; omega
      by_cases hex : ∃ b ∈ bs, b.ready = false
      · obtain ⟨b, hb, hbr⟩ := hex
        apply pair b hb (fun h => by rw [har'] at h; cases h)
        have := hcount_erase b hb; simp [hbr] at this; omega
      · -- Every real pod is ready: any one will do.
        have hall : ∀ b ∈ bs, b.ready = true := by
          intro b hb; cases hbr : b.ready
          · exact absurd ⟨b, hb, hbr⟩ hex
          · rfl
        obtain ⟨b, hb⟩ : ∃ b, b ∈ bs := by
          cases bs with
          | nil => simp at hlen
          | cons b _ => exact ⟨b, by simp⟩
        apply pair b hb (fun h => by rw [har'] at h; cases h)
        have h1 : ((bs.erase b).filter (·.ready)).length = (bs.erase b).length := by
          rw [List.filter_eq_self.mpr (fun x hx => hall x (List.mem_of_mem_erase hx))]
        have h2 : (as.filter (·.ready)).length ≤ as.length := List.length_filter_le _ _
        rw [h1, List.length_erase_of_mem hb]; simp at hlen; omega

/-! ## What `dropPods` does -/

/-- A pod of deployment `d` that isn't gone. -/
def isD (d : String) (p : MPod) : Bool := p.dep == d && !p.gone

theorem dropPods_pointwise (d : String) (U : List String) :
    ∀ (qn qr : Nat) (A : List MPod), Forall2 (fun a a' =>
      (isD d a = false ∧ a' = a) ∨
      (isD d a = true ∧ (a' = { a with gone := true, ready := false } ∨ a' = { a with nodes := U })))
      A (dropPods d U qn qr A) := by
  intro qn qr A
  induction A generalizing qn qr with
  | nil => exact .nil
  | cons a A ih =>
    unfold dropPods
    by_cases h : (a.dep == d && !a.gone) = true
    · rw [if_pos h]
      have hd : isD d a = true := h
      split
      · exact .cons (Or.inr ⟨hd, Or.inl rfl⟩) (ih _ _)
      · split
        · exact .cons (Or.inr ⟨hd, Or.inl rfl⟩) (ih _ _)
        · exact .cons (Or.inr ⟨hd, Or.inr rfl⟩) (ih _ _)
    · rw [if_neg h]
      have hd : isD d a = false := by simpa [isD] using h
      exact .cons (Or.inl ⟨hd, rfl⟩) (ih _ _)

theorem isD_split (d : String) (A : List MPod) :
    (A.filter (isD d)).length = (A.filter fun p => isD d p && p.ready).length + (A.filter fun p => isD d p && !p.ready).length := by
  induction A with
  | nil => rfl
  | cons a A ih =>
    simp only [List.filter_cons]
    cases h1 : isD d a <;> cases h2 : a.ready <;> simp [h1, h2] <;> omega

theorem dropPods_counts (d : String) (U : List String) :
    ∀ (qn qr : Nat) (A : List MPod),
      ((dropPods d U qn qr A).filter (isD d)).length =
        (A.filter (isD d)).length - min qn ((A.filter fun p => isD d p && !p.ready).length)
          - min qr ((A.filter fun p => isD d p && p.ready).length) ∧
      ((dropPods d U qn qr A).filter fun p => isD d p && p.ready).length =
        (A.filter fun p => isD d p && p.ready).length - min qr ((A.filter fun p => isD d p && p.ready).length) := by
  intro qn qr A
  induction A generalizing qn qr with
  | nil => simp [dropPods]
  | cons a A ih =>
    unfold dropPods
    by_cases h : (a.dep == d && !a.gone) = true
    · rw [if_pos h]
      have hd : isD d a = true := h
      have hdep : a.dep = d := by simp at h; exact h.1
      have hgone : a.gone = false := by simp at h; exact h.2
      by_cases h1 : (!a.ready && decide (qn > 0)) = true
      · rw [if_pos h1]
        have hr : a.ready = false := by simp at h1; exact h1.1
        have hq : qn > 0 := by simp at h1; exact h1.2
        obtain ⟨i1, i2⟩ := ih (qn - 1) qr
        simp [List.filter_cons, isD, hdep, hgone, hr] at i1 i2 ⊢
        constructor <;> omega
      · rw [if_neg h1]
        by_cases h2 : (a.ready && decide (qr > 0)) = true
        · rw [if_pos h2]
          have hr : a.ready = true := by simp at h2; exact h2.1
          have hq : qr > 0 := by simp at h2; exact h2.2
          obtain ⟨i1, i2⟩ := ih qn (qr - 1)
          simp [List.filter_cons, isD, hdep, hgone, hr] at i1 i2 ⊢
          constructor <;> omega
        · rw [if_neg h2]
          obtain ⟨i1, i2⟩ := ih qn qr
          cases hr : a.ready
          · have hq : qn = 0 := by simp [hr] at h1; omega
            subst hq
            have hs := isD_split d A
            simp [List.filter_cons, isD, hdep, hgone, hr] at i1 i2 hs ⊢
            constructor <;> omega
          · have hq : qr = 0 := by simp [hr] at h2; omega
            subst hq
            have hs := isD_split d A
            simp [List.filter_cons, isD, hdep, hgone, hr] at i1 i2 hs ⊢
            constructor <;> omega
    · rw [if_neg h]
      have hd : isD d a = false := by simpa [isD] using h
      obtain ⟨i1, i2⟩ := ih qn qr
      simp only [List.filter_cons, hd] at i1 i2 ⊢
      simp at i1 i2 ⊢
      exact ⟨i1, i2⟩

/-! ## Scaling up -/

theorem newPods_spec (st : State) (d : String) (count : Int) (m : MDep) (hm : st.dep? d = some m) :
    ∃ P : List MPod, SameUpTo { st with pods := st.pods ++ P } (st.newPods d count).1 ∧ P.length = count.toNat ∧
      ∀ p ∈ P, p.dep = d ∧ p.owner = "Deployment" ∧ p.nodes = st.schedulable ∧ p.ready = false ∧ p.gone = false := by
  unfold State.newPods
  rw [hm]
  refine ⟨_, ⟨?_, rfl, rfl⟩, by simp, ?_⟩
  · simp only [State.updDep, List.map_map]
    apply List.map_congr_left
    intro x _
    simp only [Function.comp]
    split <;> rfl
  · intro p hp
    simp only [List.mem_map] at hp
    obtain ⟨_, _, rfl⟩ := hp
    exact ⟨rfl, rfl, rfl, rfl, rfl⟩

theorem forall2_replicate {sch : List String} (P : List MPod) (c : CPod) (h : ∀ p ∈ P, Match sch p c) :
    Forall2 (Match sch) P (List.replicate P.length c) := by
  induction P with
  | nil => exact .nil
  | cons a P ih => exact .cons (h a (by simp)) (ih fun p hp => h p (by simp [hp]))

theorem State.rolling_updDep (st : State) (d e : String) (f : MDep → MDep) (hn : ∀ x, (f x).name = x.name)
    (hr : ∀ x, (f x).rolling = x.rolling) : (st.updDep d f).rolling e = st.rolling e := by
  unfold State.rolling
  rw [State.dep?_updDep st d e f hn]
  cases st.dep? e <;> simp
  split <;> simp [hr]

theorem sched_setField_replicas (s : Snapshot) (f : Fields) (d : String) (v : Value) :
    sched s (setField f (.replicas d) v) = sched s f := by
  unfold sched; congr 1

theorem replicasOf_setField_other (f : Fields) (d e : String) (v : Value) (h : e ≠ d) :
    replicasOf (setField f (.replicas d) v) e = replicasOf f e := by
  simp [replicasOf, setField, h]

theorem State.live_updDep (st : State) (d : String) (f : MDep → MDep) (hn : ∀ x, (f x).name = x.name)
    (hr : ∀ x, (f x).rolling = x.rolling) : (st.updDep d f).live = st.live := by
  unfold State.live
  apply List.filter_congr
  intro p _
  rw [State.rolling_updDep st d p.dep f hn hr]

/-- Changing a deployment's replica count, while it isn't mid rollout. -/
theorem Sim.setReplicas {s : Snapshot} {st : State} {w : World} (hs : Sim s st w) (d : String) (n : Int)
    (hroll : st.rolling d = false) :
    Sim s (st.updDep d fun x => { x with replicas := n }) { w with fields := setField w.fields (.replicas d) (.int n) } := by
  have hrep : ∀ e, e ≠ d → replicasOf (setField w.fields (.replicas d) (.int n)) e = replicasOf w.fields e :=
    fun e he => replicasOf_setField_other w.fields d e _ he
  refine ⟨?_, ?_, ?_, ?_, ?_, ?_, ?_, ?_⟩
  · show setField w.fields _ _ = _
    rw [hs.fields, State.field_updDep_replicas]
  · obtain ⟨al, hp, hf⟩ := hs.pods
    refine ⟨al, hp, ?_⟩
    rw [State.live_updDep st d (fun x : MDep => { x with replicas := n }) (fun _ => rfl) (fun _ => rfl)]
    show Forall2 (Match (sched s (setField w.fields _ _))) _ _
    rw [sched_setField_replicas]; exact hf
  · intro e
    rw [State.rolling_updDep st d e (fun x : MDep => { x with replicas := n }) (fun _ => rfl) (fun _ => rfl)]
    exact hs.rollIff e
  · intro e r hr
    have hrw : w.rollout? e = some r := hr
    have hed : e ≠ d := by
      intro h; subst h
      have := hs.rollIff e
      rw [hrw, hroll] at this
      cases this
    obtain ⟨m, hm, h1, h2, h3, h4, h5⟩ := hs.roll e r hrw
    refine ⟨m, ?_, h1, h2, h3, ?_, ?_⟩
    · rw [State.dep?_updDep st d e (fun x : MDep => { x with replicas := n }) (fun _ => rfl), hm]
      simp [State.name_of_dep? hm, hed]
    · show _ = (replicasOf (setField w.fields _ _) e).toNat
      rw [hrep e hed]; exact h4
    · show _ ≤ max 0 (replicasOf (setField w.fields _ _) e)
      rw [hrep e hed]; exact h5
  · exact hs.owner
  · exact hs.nodes
  · intro m hm
    simp only [State.updDep, List.mem_map] at hm
    obtain ⟨x, hx, rfl⟩ := hm
    split <;> exact hs.mu x hx
  · intro m hm
    simp only [State.updDep, List.mem_map] at hm
    obtain ⟨x, hx, rfl⟩ := hm
    split <;> exact hs.names x hx

/-- Adding pending pods of a deployment that isn't mid rollout, on both sides. -/
theorem Sim.addPending {s : Snapshot} {st : State} {w : World} (hu : NodesUnique s) (hs : Sim s st w)
    (d : String) (P : List MPod) (hroll : st.rolling d = false)
    (hP : ∀ p ∈ P, p.dep = d ∧ p.owner = "Deployment" ∧ p.nodes = st.schedulable ∧ p.ready = false ∧ p.gone = false) :
    Sim s { st with pods := st.pods ++ P }
      { w with pods := w.pods ++ List.replicate P.length ⟨d, "Deployment", none, false⟩ } := by
  have hrol : ∀ e, ({ st with pods := st.pods ++ P } : State).rolling e = st.rolling e := fun _ => rfl
  have hlive : ({ st with pods := st.pods ++ P } : State).live = st.live ++ P := by
    unfold State.live
    simp only [List.filter_append]
    congr 1
    apply List.filter_eq_self.mpr
    intro p hp
    obtain ⟨hd, -, -, -, hg⟩ := hP p hp
    have : ({ st with pods := st.pods ++ P } : State).rolling p.dep = false := by rw [hd]; exact hroll
    simp [hg, this]
  refine ⟨?_, ?_, ?_, ?_, ?_, ?_, hs.mu, hs.names⟩
  · show w.fields = _
    rw [hs.fields]; exact (State.field_congr rfl rfl).symm
  · obtain ⟨al, hp, hf⟩ := hs.pods
    refine ⟨al ++ List.replicate P.length ⟨d, "Deployment", none, false⟩, hp.append_right _, ?_⟩
    rw [hlive]
    refine hf.append (forall2_replicate P _ (fun p hpP => ?_))
    obtain ⟨hd, ho, hn, hr, -⟩ := hP p hpP
    refine ⟨hd.symm, ho.symm, ?_, ?_⟩
    · intro h; rw [hr] at h; cases h
    refine ⟨hr, rfl, fun m hm => ?_⟩
    rw [hn]
    show m ∈ st.schedulable
    rw [mem_schedulable hu hs.nodes, ← hs.fields]; exact hm
  · intro e; rw [hrol]; exact hs.rollIff e
  · intro e r hr
    have hrw : w.rollout? e = some r := hr
    obtain ⟨m, hm, h1, h2, h3, h4, h5⟩ := hs.roll e r hrw
    have hed : e ≠ d := by
      intro h; subst h
      have := hs.rollIff e
      rw [hrw, hroll] at this; cases this
    refine ⟨m, hm, h1, h2, ?_, ?_, h5⟩
    · intro p hp hpd hpg
      rcases List.mem_append.mp hp with hp | hp
      · exact h3 p hp hpd hpg
      · exact absurd (hpd.symm.trans (hP p hp).1) hed
    · show (List.filter _ (st.pods ++ P)).length = _
      rw [List.filter_append, List.length_append, ← h4]
      have : (P.filter fun p => p.dep == e && !p.gone).length = 0 := by
        rw [List.length_eq_zero_iff, List.filter_eq_nil_iff]
        intro p hp
        simp [(hP p hp).1, Ne.symm hed]
      omega
  · intro p hp hd
    rcases List.mem_append.mp hp with hp | hp
    · exact hs.owner p hp hd
    · exact (hP p hp).2.1
  · exact hs.nodes

/-! ## Scaling down -/

theorem filter_eq_of_forall2 {R : MPod → MPod → Prop} {A B : List MPod} (h : Forall2 R A B) (q : MPod → Bool)
    (hq : ∀ a b, R a b → q a = q b ∧ (q a = true → b = a)) : B.filter q = A.filter q := by
  induction h with
  | nil => rfl
  | @cons a b _ _ hab _ ih =>
    obtain ⟨h1, h2⟩ := hq a b hab
    simp only [List.filter_cons, ← h1]
    split
    · rename_i hqa; rw [h2 hqa, ih]
    · exact ih

theorem length_filter_eq_of_forall2 {R : MPod → MPod → Prop} {A B : List MPod} (h : Forall2 R A B) (q : MPod → Bool)
    (hq : ∀ a b, R a b → q a = q b) : (B.filter q).length = (A.filter q).length := by
  induction h with
  | nil => rfl
  | @cons a b _ _ hab _ ih =>
    simp only [List.filter_cons, hq a b hab]
    split <;> simp [ih]

theorem mem_U {st : State} {d : String} {p : MPod} (hp : p ∈ st.pods) (hd : p.dep = d) (hg : p.gone = false)
    {m : String} (hm : m ∈ p.nodes) : m ∈ compact (sortStrings ((st.podsOf d).flatMap (·.nodes))) := by
  rw [mem_compact, mem_sortStrings, List.mem_flatMap]
  exact ⟨p, List.mem_filter.mpr ⟨hp, by simp [hd, hg]⟩, hm⟩

/-- Scaling down a deployment that isn't mid rollout: Kubernetes removes
`drop` (pods that aren't ready first), the checker drops its own choice;
the kept pods pair up again. -/
theorem Sim.scaleDown {s : Snapshot} {st : State} {w w' : World} (hs : Sim s st w) (d : String) (n : Int)
    (hroll : st.rolling d = false) (hdne : d ≠ "")
    (keep drop : List CPod) (hperm : w.pods.Perm (keep ++ drop)) (hdrop : ∀ p ∈ drop, p.dep = d)
    (hlen : drop.length = (w.pods.filter (·.dep == d)).length - n.toNat)
    (hfirst : (∃ p ∈ drop, p.ready = true) → ∀ p ∈ keep, p.dep = d → p.ready = true)
    (hw : w' = { w with pods := keep }) :
    Sim s (st.scaleDown d n) w' := by
  subst hw
  -- The checker's side.
  let A := st.pods
  let U := compact (sortStrings ((st.podsOf d).flatMap (·.nodes)))
  let K := (st.podsOf d).length
  let NRc := ((st.podsOf d).filter (!·.ready)).length
  let NR := (A.filter fun p => isD d p && !p.ready).length
  let RR := (A.filter fun p => isD d p && p.ready).length
  let k := K - n.toNat
  let qn := min k NRc
  let qr := k - qn
  have hpods : st.podsOf d = A.filter (isD d) := rfl
  have hNR : NRc = NR := by
    show ((A.filter (isD d)).filter (!·.ready)).length = _
    rw [List.filter_filter]
    congr 1
    apply List.filter_congr
    intro p _
    exact Bool.and_comm _ _
  have hsd : st.scaleDown d n = { st with pods := dropPods d U qn qr A } := rfl
  rw [hsd]
  let R := dropPods d U qn qr A
  have hPW := dropPods_pointwise d U qn qr A
  have hsplit := isD_split d A
  obtain ⟨hcK, hcR⟩ := dropPods_counts d U qn qr A
  have hK : K = (A.filter (isD d)).length := rfl
  have hqn : min qn NR = qn := Nat.min_eq_left (by rw [← hNR]; exact Nat.min_le_right _ _)
  have hqr : min qr RR = qr := by
    apply Nat.min_eq_left
    show k - min k NRc ≤ RR
    rw [hNR]
    omega
  have hrol : ∀ e, ({ st with pods := R } : State).rolling e = st.rolling e := fun _ => rfl
  -- The pairing before.
  obtain ⟨al, hal, hf⟩ := hs.pods
  have hsch : sched s ({ w with pods := keep } : World).fields = sched s w.fields := rfl
  -- Live pods of the scaled deployment, before.
  have hliveD : st.live.filter (fun p => p.dep == d) = A.filter (isD d) := by
    unfold State.live
    rw [List.filter_filter]
    apply List.filter_congr
    intro p _
    cases hpd : (p.dep == d)
    · simp [isD, hpd]
    · have : p.dep = d := by simpa using hpd
      subst this; simp [isD, hroll]
  -- Real pods of the scaled deployment, before: as many as the checker's.
  have hCd : (w.pods.filter (·.dep == d)).length = K := by
    have h1 := (hf.filter (fun p => p.dep == d) (fun c => c.dep == d) (fun a b hm => by rw [hm.1])).length_eq
    rw [hliveD] at h1
    have h2 := (hal.filter (fun c => c.dep == d)).length_eq
    show _ = (A.filter (isD d)).length
    omega
  have hq1 : qn ≤ k := Nat.min_le_left _ _
  have hq2 : qr = k - qn := rfl
  have hk : k = K - n.toNat := rfl
  have hkeepD : (keep.filter (·.dep == d)).length + drop.length = K := by
    have := (hperm.filter (·.dep == d)).length_eq
    rw [List.filter_append, List.length_append] at this
    have hd' : drop.filter (·.dep == d) = drop := List.filter_eq_self.mpr (fun p hp => by simp [hdrop p hp])
    rw [hd'] at this
    omega
  refine ⟨?_, ?_, ?_, ?_, ?_, ?_, hs.mu, hs.names⟩
  · show w.fields = _
    rw [hs.fields]; exact (State.field_congr rfl rfl).symm
  · -- The pairing after.
    have hLnd : ({ st with pods := R } : State).live.filter (fun p => !(p.dep == d)) = st.live.filter (fun p => !(p.dep == d)) := by
      unfold State.live
      rw [List.filter_filter, List.filter_filter]
      simp only [hrol]
      apply filter_eq_of_forall2 hPW
      intro a b hab
      rcases hab with ⟨ha, rfl⟩ | ⟨ha, rfl | rfl⟩
      · exact ⟨rfl, fun _ => rfl⟩
      · have hd : a.dep = d := by simp [isD] at ha; exact ha.1
        constructor
        · simp [hd]
        · intro h; simp [hd] at h
      · have hd : a.dep = d := by simp [isD] at ha; exact ha.1
        constructor
        · simp [hd]
        · intro h; simp [hd] at h
    have hLd : ({ st with pods := R } : State).live.filter (fun p => p.dep == d) = R.filter (isD d) := by
      unfold State.live
      rw [List.filter_filter]
      apply List.filter_congr
      intro p _
      cases hpd : (p.dep == d)
      · simp [isD, hpd]
      · have : p.dep = d := by simpa using hpd
        subst this; simp [isD, hrol, hroll]
    -- The pods both sides keep pair up again.
    have hX := hf.filter (fun p => !(p.dep == d)) (fun c => !(c.dep == d)) (fun a b hm => by rw [hm.1])
    obtain ⟨bs', hbs', hY⟩ := match_exists (sched s w.fields) U d (R.filter (isD d)) (keep.filter (·.dep == d))
      (by
        intro a ha
        obtain ⟨haR, haD⟩ := List.mem_filter.mp ha
        obtain ⟨a0, ha0, hrel⟩ : ∃ a0 ∈ A, (isD d a0 = false ∧ a = a0) ∨
            (isD d a0 = true ∧ (a = { a0 with gone := true, ready := false } ∨ a = { a0 with nodes := U })) := by
          exact forall2_exists_of_mem_right hPW haR
        rcases hrel with ⟨h0, rfl⟩ | ⟨h0, rfl | rfl⟩
        · rw [h0] at haD; cases haD
        · simp [isD] at haD
        · have hd : a0.dep = d := by simp [isD] at h0; exact h0.1
          exact ⟨rfl, hd, hs.owner a0 ha0 (by rw [hd]; exact hdne)⟩)
      (by
        intro b hb
        obtain ⟨hbk, hbd⟩ := List.mem_filter.mp hb
        have hbd : b.dep = d := by simpa using hbd
        have hbw : b ∈ w.pods := hperm.mem_iff.mpr (List.mem_append_left _ hbk)
        obtain ⟨a, hal', hm⟩ := forall2_exists_of_mem_right hf (hal.mem_iff.mpr hbw)
        obtain ⟨hm1, hm2, -, hm4⟩ := hm
        have had : a.dep = d := hm1.symm.trans hbd
        have hpl := List.mem_filter.mp hal'
        have hag : a.gone = false := by
          have := hpl.2; simp only [Bool.and_eq_true, Bool.not_eq_true'] at this; exact this.1
        refine ⟨hbd, hm2.trans (hs.owner a hpl.1 (by rw [had]; exact hdne)), ?_, ?_⟩
        · intro m hn; rw [hn] at hm4; exact mem_U hpl.1 had hag hm4
        · intro hn; rw [hn] at hm4
          exact ⟨hm4.2.1, fun m hm => mem_U hpl.1 had hag (hm4.2.2 m hm)⟩)
      (by
        show _ = _
        have : (R.filter (isD d)).length = K - qn - qr := by rw [hcK, hqn, hqr]; rfl
        rw [this]
        omega)
      (by
        have hA : ((R.filter (isD d)).filter (·.ready)).length = RR - qr := by
          rw [List.filter_filter, ← hqr]
          rw [← hcR]
          congr 1
          apply List.filter_congr
          intro p _
          cases p.ready <;> simp
        rw [hA]
        by_cases hdr : ∃ p ∈ drop, p.ready = true
        · have hall := hfirst hdr
          rw [List.filter_eq_self.mpr (fun p hp => hall p (List.mem_filter.mp hp).1 (by simpa using (List.mem_filter.mp hp).2))]
          have : (R.filter (isD d)).length = K - qn - qr := by rw [hcK, hqn, hqr]; rfl
          omega
        · -- No ready pod went: Kubernetes kept every ready one.
          have hnr : ∀ p ∈ drop, p.ready = false := by
            intro p hp; cases h : p.ready
            · rfl
            · exact absurd ⟨p, hp, h⟩ hdr
          have hk : ((keep.filter (·.dep == d)).filter (·.ready)).length = ((w.pods.filter (·.dep == d)).filter (·.ready)).length := by
            have := ((hperm.filter (·.dep == d)).filter (·.ready)).length_eq
            rw [this, List.filter_append, List.filter_append, List.length_append]
            have : (drop.filter (·.dep == d)).filter (·.ready) = [] := by
              rw [List.filter_eq_nil_iff]
              intro p hp; have := hnr p (List.mem_filter.mp hp).1; simp [this]
            rw [this]; rfl
          rw [hk]
          have hge : RR ≤ ((w.pods.filter (·.dep == d)).filter (·.ready)).length := by
            have h1 := (hf.filter (fun p => p.dep == d) (fun c => c.dep == d) (fun a b hm => by rw [hm.1])).countP_le
              (fun p => p.ready) (fun c => c.ready) (fun a b hm hr => hm.2.2.1 hr)
            rw [hliveD] at h1
            simp only [List.countP_eq_length_filter] at h1
            rw [← ((hal.filter _).filter (·.ready)).length_eq]
            have : RR = ((A.filter (isD d)).filter (·.ready)).length := by
              show (A.filter fun p => isD d p && p.ready).length = _
              rw [List.filter_filter]
              congr 1
              apply List.filter_congr
              intro p _
              exact Bool.and_comm _ _
            omega
          omega)
    obtain ⟨zs, hzs, hZ⟩ := Forall2.weave (fun p : MPod => p.dep == d) ({ st with pods := R } : State).live
      (al.filter fun c => !(c.dep == d)) bs' (by rw [hLnd]; exact hX) (by rw [hLd]; exact hY)
    refine ⟨zs, ?_, by rw [hsch]; exact hZ⟩
    -- And they are the pods Kubernetes kept.
    have h1 : (al.filter fun c => !(c.dep == d)).Perm (keep.filter fun c => !(c.dep == d)) := by
      have := (hal.trans hperm).filter (fun c => !(c.dep == d))
      rw [List.filter_append] at this
      have hd' : drop.filter (fun c => !(c.dep == d)) = [] := by
        rw [List.filter_eq_nil_iff]; intro p hp; simp [hdrop p hp]
      rw [hd', List.append_nil] at this
      exact this
    exact hzs.trans ((h1.append hbs').trans (List.perm_append_comm.trans (List.filter_append_perm _ _)))
  · intro e; rw [hrol]; exact hs.rollIff e
  · intro e r hr
    have hrw : w.rollout? e = some r := hr
    obtain ⟨m, hm, h1, h2, h3, h4, h5⟩ := hs.roll e r hrw
    have hed : e ≠ d := by
      intro h; subst h
      have := hs.rollIff e
      rw [hrw, hroll] at this; cases this
    refine ⟨m, hm, h1, h2, ?_, ?_, h5⟩
    · intro p hp hpd hpg
      obtain ⟨a0, ha0, hrel⟩ : ∃ a0 ∈ A, (isD d a0 = false ∧ p = a0) ∨
          (isD d a0 = true ∧ (p = { a0 with gone := true, ready := false } ∨ p = { a0 with nodes := U })) := by
        exact forall2_exists_of_mem_right hPW hp
      rcases hrel with ⟨-, rfl⟩ | ⟨h0, rfl | rfl⟩
      · exact h3 p ha0 hpd hpg
      · have hd : a0.dep = d := by simp [isD] at h0; exact h0.1
        exact absurd (hpd.symm.trans hd) hed
      · have hd : a0.dep = d := by simp [isD] at h0; exact h0.1
        exact absurd (hpd.symm.trans hd) hed
    · show (R.filter _).length = _
      rw [length_filter_eq_of_forall2 hPW]
      · exact h4
      · intro a b hab
        rcases hab with ⟨-, rfl⟩ | ⟨h0, rfl | rfl⟩
        · rfl
        · have hd : a.dep = d := by simp [isD] at h0; exact h0.1
          simp [hd, Ne.symm hed]
        · rfl
  · intro p hp hd
    obtain ⟨a0, ha0, hrel⟩ : ∃ a0 ∈ A, (isD d a0 = false ∧ p = a0) ∨
        (isD d a0 = true ∧ (p = { a0 with gone := true, ready := false } ∨ p = { a0 with nodes := U })) := by
      exact forall2_exists_of_mem_right hPW hp
    rcases hrel with ⟨-, rfl⟩ | ⟨-, rfl | rfl⟩
    · exact hs.owner _ ha0 hd
    · exact hs.owner a0 ha0 hd
    · exact hs.owner a0 ha0 hd
  · exact hs.nodes

/-! ## The scale step -/

theorem trigger_dep_none {st : State} {stp : Step} (hop : stp.op = "scale" ∨ stp.op = "set-image" ∨ stp.op = "rollout-restart")
    (h : trigger st stp = none) : depTrigger st stp.deployment = none := by
  unfold trigger at h
  have h1 : (["cordon", "uncordon", "drain"].contains stp.op) = false := by
    rcases hop with h | h | h <;> simp [h]
  have h2 : (stp.op == "drain") = false := by rcases hop with h | h | h <;> simp [h]
  have h3 : (["scale", "set-image", "rollout-restart"].contains stp.op) = true := by
    rcases hop with h | h | h <;> simp [h]
  simp only [h1, Bool.false_and, Bool.false_eq_true, ite_false, h2, h3, ite_true] at h
  cases hd : depTrigger st stp.deployment with
  | none => rfl
  | some t => rw [hd] at h; cases h

theorem Sim.rolling_false {st : State} {d : String} {m : MDep} (hm : st.dep? d = some m) (hdt : depTrigger st d = none) :
    st.rolling d = false := by
  simp [State.rolling, hm, depTrigger_none_rolling hdt hm]

theorem World.ext' {a b : World} (h1 : a.fields = b.fields) (h2 : a.pods = b.pods) (h3 : a.rollouts = b.rollouts) :
    a = b := by
  cases a; cases b; simp_all

theorem Sim.scale {s : Snapshot} {st : State} {w w' : World} {stp : Step} (hu : NodesUnique s)
    (hs : Sim s st w) (hop : stp.op = "scale") (hex : (st.dep? stp.deployment).isSome)
    (htr : trigger st stp = none) (hn : Next s w stp w') : Sim s (applyOp st stp).1 w' := by
  obtain ⟨m, hm⟩ := Option.isSome_iff_exists.mp hex
  have hdt := trigger_dep_none (Or.inl hop) htr
  have hroll := Sim.rolling_false hm hdt
  have hdne : stp.deployment ≠ "" := by
    rw [← State.name_of_dep? hm]; exact hs.names m (List.mem_of_find?_eq_some hm)
  simp only [Next, hop] at hn
  obtain ⟨hfw, hsn⟩ := hn
  have hstep : Spec.step w.fields stp = setField w.fields (.replicas stp.deployment) (.int (stp.replicas.getD 0)) := by simp only [Spec.step, hop]
  have hro : w.rollout? stp.deployment = none := by
    have := hs.rollIff stp.deployment; rw [hroll] at this
    cases h : w.rollout? stp.deployment with
    | none => rfl
    | some _ => rw [h] at this; cases this
  have hr : replicasOf w.fields stp.deployment = m.replicas := by
    simp [replicasOf, hs.fields, State.field, hm]
  unfold ScaleNext at hsn
  rw [hro] at hsn
  simp only [hr] at hsn
  obtain ⟨hro', hsn⟩ := hsn
  have hs1 := hs.setReplicas stp.deployment (stp.replicas.getD 0) hroll
  have hm1 : (st.updDep stp.deployment fun x => { x with replicas := (stp.replicas.getD 0) }).dep? stp.deployment = some { m with replicas := (stp.replicas.getD 0) } := by
    rw [State.dep?_updDep st stp.deployment stp.deployment (fun x => { x with replicas := (stp.replicas.getD 0) }) (fun _ => rfl), hm]
    simp [State.name_of_dep? hm]
  have hroll1 : (st.updDep stp.deployment fun x => { x with replicas := (stp.replicas.getD 0) }).rolling stp.deployment = false := by
    rw [State.rolling_updDep st stp.deployment stp.deployment (fun x => { x with replicas := (stp.replicas.getD 0) }) (fun _ => rfl) (fun _ => rfl)]; exact hroll
  have happ : applyOp st stp = (if (stp.replicas.getD 0) > m.replicas then
        let r := (st.updDep stp.deployment fun x => { x with replicas := (stp.replicas.getD 0) }).newPods stp.deployment ((stp.replicas.getD 0) - m.replicas)
        let r2 := r.1.place r.2
        (r2.1, r2.2.isNone)
      else if (stp.replicas.getD 0) < m.replicas then ((st.updDep stp.deployment fun x => { x with replicas := (stp.replicas.getD 0) }).scaleDown stp.deployment (stp.replicas.getD 0), true)
      else (st.updDep stp.deployment fun x => { x with replicas := (stp.replicas.getD 0) }, true)) := by
    simp only [applyOp, hop, hm]
  rw [happ]
  by_cases hgt : (stp.replicas.getD 0) > m.replicas
  · rw [if_pos hgt] at hsn ⊢
    obtain ⟨P, hsame, hlen, hP⟩ := newPods_spec _ stp.deployment ((stp.replicas.getD 0) - m.replicas) _ hm1
    have hadd := Sim.addPending hu hs1 stp.deployment P hroll1 hP
    have hw : w' = { ({ w with fields := setField w.fields (.replicas stp.deployment) (.int (stp.replicas.getD 0)) } : World) with
        pods := w.pods ++ List.replicate P.length ⟨stp.deployment, "Deployment", none, false⟩ } := by
      exact World.ext' (by rw [hfw, hstep]) (by rw [hsn, hlen]) hro'
    rw [hw]
    exact (hadd.congr hsame).congr (SameUpTo.place _ _)
  · rw [if_neg hgt] at hsn ⊢
    by_cases hlt : (stp.replicas.getD 0) < m.replicas
    · rw [if_pos hlt] at hsn ⊢
      obtain ⟨keep, drop, hperm, hdrop, hlen, hfirst, hpk⟩ := hsn
      exact Sim.scaleDown hs1 stp.deployment (stp.replicas.getD 0) hroll1 hdne keep drop hperm hdrop hlen hfirst
        (World.ext' (by rw [hfw, hstep]) hpk hro')
    · rw [if_neg hlt] at hsn ⊢
      have hw : w' = { w with fields := setField w.fields (.replicas stp.deployment) (.int (stp.replicas.getD 0)) } := by
        exact World.ext' (by rw [hfw, hstep]) hsn hro'
      rw [hw]; exact hs1

end Planproof
