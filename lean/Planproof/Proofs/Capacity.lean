import Planproof.Proofs.Safety

/-!
# Rule `capacity` is sound (PRD L-34)

The checker's placement witness fits: in every state of a Proved plan and
its rollback, every node's charged requests are within its allocatable, or
no more than in the snapshot (a node the snapshot already overfills gets
no new pod). Requests are charged where first fit put each pod (`atNode`).
-/
namespace Planproof

open Spec

/-! ## Resources -/

/-- `a` is within `b` in every resource. -/
def Res.Le (a b : Res) : Prop := a.cpu ≤ b.cpu ∧ a.mem ≤ b.mem ∧ a.pods ≤ b.pods

theorem Res.Le.refl (a : Res) : Res.Le a a := ⟨Int.le_refl _, Int.le_refl _, Int.le_refl _⟩

theorem Res.Le.trans {a b c : Res} (h1 : Res.Le a b) (h2 : Res.Le b c) : Res.Le a c :=
  ⟨Int.le_trans h1.1 h2.1, Int.le_trans h1.2.1 h2.2.1, Int.le_trans h1.2.2 h2.2.2⟩

theorem Res.le_true {a b : Res} (h : a.le b = true) : Res.Le a b := by
  simp only [Res.le, Bool.and_eq_true, decide_eq_true_eq] at h
  exact ⟨h.1.1, h.1.2, h.2⟩

/-- Requests charged to node `n` by a list of pods. -/
def uses (n : String) : List MPod → Res
  | [] => {}
  | p :: ps => if p.atNode == n && p.atNode != "" then p.req.add (uses n ps) else uses n ps

theorem usage_foldl (n : String) (l : List MPod) (acc : Res) :
    l.foldl (fun r p => if p.atNode == n && p.atNode != "" then r.add p.req else r) acc = acc.add (uses n l) := by
  induction l generalizing acc with
  | nil => cases acc; simp [uses, Res.add]
  | cons p ps ih =>
    simp only [List.foldl_cons, uses]
    rw [ih]
    split
    · cases acc; simp [Res.add, Int.add_assoc]
    · rfl

theorem usage_eq (st : State) (n : String) : st.usage n = uses n st.pods := by
  unfold State.usage
  rw [usage_foldl]
  simp [Res.add]

theorem uses_cons_le (n : String) (p : MPod) (ps : List MPod) (h : Res.Le {} p.req) :
    Res.Le (uses n ps) (uses n (p :: ps)) := by
  simp only [uses]
  split
  · obtain ⟨h1, h2, h3⟩ := h
    simp only [Res.add, Res.Le] at *
    omega
  · exact Res.Le.refl _

/-- Requests never shrink when pods are added; removing pods frees them. -/
theorem uses_filter_le (n : String) (q : MPod → Bool) (l : List MPod) (h : ∀ p ∈ l, Res.Le {} p.req) :
    Res.Le (uses n (l.filter q)) (uses n l) := by
  induction l with
  | nil => exact Res.Le.refl _
  | cons p ps ih =>
    have ih' := ih (fun x hx => h x (by simp [hx]))
    rw [List.filter_cons]
    split
    · simp only [uses]
      split
      · obtain ⟨h1, h2, h3⟩ := ih'
        simp only [Res.add, Res.Le] at *
        omega
      · exact ih'
    · exact ih'.trans (uses_cons_le n p ps (h p (by simp)))

theorem uses_map (n : String) (f : MPod → MPod) (l : List MPod)
    (hf : ∀ p ∈ l, (f p).atNode = p.atNode ∧ (f p).req = p.req) : uses n (l.map f) = uses n l := by
  induction l with
  | nil => rfl
  | cons p ps ih =>
    simp only [List.map_cons, uses]
    obtain ⟨ha, hr⟩ := hf p (by simp)
    rw [ha, hr, ih (fun x hx => hf x (by simp [hx]))]

theorem uses_append_unplaced (n : String) (l P : List MPod) (hP : ∀ p ∈ P, p.atNode = "") :
    uses n (l ++ P) = uses n l := by
  induction l with
  | nil =>
    induction P with
    | nil => rfl
    | cons p ps ih =>
      simp only [List.nil_append, uses, hP p (by simp)]
      simpa [uses] using ih (fun x hx => hP x (by simp [hx]))
  | cons p ps ih => simp only [List.cons_append, uses, ih]

/-- Uncharging pods frees their requests. -/
theorem uses_unplace_le (n : String) (sel : MPod → Bool) (l : List MPod) (h : ∀ p ∈ l, Res.Le {} p.req) :
    Res.Le (uses n (l.map fun p => if sel p then { p with atNode := "" } else p)) (uses n l) := by
  induction l with
  | nil => exact Res.Le.refl _
  | cons p ps ih =>
    have ih' := ih (fun x hx => h x (by simp [hx]))
    simp only [List.map_cons]
    split
    · simp only [uses, bne_self_eq_false, Bool.and_false, Bool.false_eq_true, ite_false]
      exact ih'.trans (uses_cons_le n p ps (h p (by simp)))
    · simp only [uses]
      split
      · obtain ⟨h1, h2, h3⟩ := ih'
        simp only [Res.add, Res.Le] at *
        omega
      · exact ih'

/-! ## Charging one pod -/

theorem setIdx_getElem?_ne (l : List MPod) (i j : Nat) (n : String) (h : j ≠ i) :
    (State.setIdx l i n)[j]? = l[j]? := by
  induction l generalizing i j with
  | nil => rfl
  | cons p ps ih =>
    cases i with
    | zero =>
      cases j with
      | zero => exact absurd rfl h
      | succ j => rfl
    | succ i =>
      cases j with
      | zero => rfl
      | succ j => simp only [State.setIdx, List.getElem?_cons_succ]; exact ih i j (by omega)

theorem uses_setIdx_other (l : List MPod) (i : Nat) (n m : String) {q : MPod} (hq : l[i]? = some q)
    (hq0 : q.atNode = "") (hm : m ≠ n) : uses m (State.setIdx l i n) = uses m l := by
  induction l generalizing i with
  | nil => rfl
  | cons p ps ih =>
    cases i with
    | zero =>
      simp only [List.getElem?_cons_zero, Option.some.injEq] at hq
      subst hq
      simp only [State.setIdx, uses, hq0]
      have : (n == m) = false := by simpa using Ne.symm hm
      simp [this]
    | succ i =>
      simp only [State.setIdx, uses]
      rw [ih i (by simpa using hq)]

theorem uses_setIdx_self (l : List MPod) (i : Nat) (n : String) {q : MPod} (hq : l[i]? = some q)
    (hq0 : q.atNode = "") (hn : n ≠ "") : uses n (State.setIdx l i n) = q.req.add (uses n l) := by
  induction l generalizing i with
  | nil => simp at hq
  | cons p ps ih =>
    cases i with
    | zero =>
      simp only [List.getElem?_cons_zero, Option.some.injEq] at hq
      subst hq
      have : (n != "") = true := by simpa using hn
      simp only [State.setIdx, uses, hq0, beq_self_eq_true, this, Bool.and_true, ite_true]
      simp
    | succ i =>
      simp only [State.setIdx, uses]
      rw [ih i (by simpa using hq)]
      split
      · cases p.req; cases q.req; simp only [Res.add]; congr 1 <;> omega
      · rfl

/-! ## The invariant -/

/-- Every node within its allocatable, or no fuller than `base` says. -/
def CapOK (base : String → Res) (st : State) : Prop :=
  ∀ nd ∈ st.nodes, Res.Le (st.usage nd.name) nd.alloc ∨ Res.Le (st.usage nd.name) (base nd.name)

def allocKey (n : MNode) : String × Res := (n.name, n.alloc)

theorem CapOK.congr {base : String → Res} {st st' : State} (h : CapOK base st)
    (hn : st'.nodes.map allocKey = st.nodes.map allocKey) (hu : ∀ n, Res.Le (st'.usage n) (st.usage n)) :
    CapOK base st' := by
  intro nd hnd
  have : allocKey nd ∈ st.nodes.map allocKey := by rw [← hn]; exact List.mem_map_of_mem hnd
  obtain ⟨nd0, hnd0, he⟩ := List.mem_map.mp this
  simp only [allocKey, Prod.mk.injEq] at he
  rcases h nd0 hnd0 with h1 | h1
  · left; rw [← he.2, ← he.1]; exact (hu _).trans (he.1 ▸ h1)
  · right; rw [← he.1]; exact (hu _).trans h1

def ReqOK (st : State) : Prop := (∀ p ∈ st.pods, Res.Le {} p.req) ∧ ∀ m ∈ st.deps, Res.Le {} m.req

theorem mem_of_name {l : List MNode} (hnd : (l.map (·.name)).Nodup) {a b : MNode} (ha : a ∈ l) (hb : b ∈ l)
    (h : a.name = b.name) : a = b := by
  induction l with
  | nil => cases ha
  | cons x xs ih =>
    simp only [List.map_cons, List.nodup_cons, List.mem_map] at hnd
    rcases List.mem_cons.mp ha with h1 | ha'
    · rcases List.mem_cons.mp hb with h2 | hb'
      · rw [h1, h2]
      · exact absurd ⟨b, hb', by rw [← h, h1]⟩ hnd.1
    · rcases List.mem_cons.mp hb with h2 | hb'
      · exact absurd ⟨a, ha', by rw [h, h2]⟩ hnd.1
      · exact ih hnd.2 ha' hb'

/-! ## First fit keeps the invariant -/

def placeStep (x : State × Option MPod) (y : MPod × Nat) : State × Option MPod :=
  match x with
  | (st, bad) => match bad with
    | some _ => (st, bad)
    | none => match y.1.nodes.find? (st.fits y.1) with
      | some n => (st.setAtIdx y.2 n, none)
      | none => (st, some y.1)

theorem placeStep_some (R : List (MPod × Nat)) (s : State) (b : MPod) :
    R.foldl placeStep (s, some b) = (s, some b) := by
  induction R with
  | nil => rfl
  | cons y R ih => exact ih

theorem setIdx_unplaced (l : List MPod) (i : Nat) {q : MPod} (hq : l[i]? = some q) (hq0 : q.atNode = "") :
    State.setIdx l i "" = l := by
  induction l generalizing i with
  | nil => rfl
  | cons p ps ih =>
    cases i with
    | zero =>
      simp only [List.getElem?_cons_zero, Option.some.injEq] at hq
      subst hq
      cases p; simp_all [State.setIdx]
    | succ i => simp only [State.setIdx]; rw [ih i (by simpa using hq)]

theorem place_loop (base : String → Res) (R : List (MPod × Nat)) (acc : State)
    (hc : CapOK base acc) (hnd : (acc.nodes.map (·.name)).Nodup)
    (hR : ∀ x ∈ R, ∃ q, acc.pods[x.2]? = some q ∧ q.atNode = "" ∧ q.req = x.1.req)
    (hRn : (R.map (·.2)).Nodup)
    (hok : (R.foldl placeStep (acc, none)).2 = none) : CapOK base (R.foldl placeStep (acc, none)).1 := by
  induction R generalizing acc with
  | nil => exact hc
  | cons y R ih =>
    simp only [List.foldl_cons] at hok ⊢
    have hstep : placeStep (acc, none) y = match y.1.nodes.find? (acc.fits y.1) with
        | some n => (acc.setAtIdx y.2 n, none)
        | none => (acc, some y.1) := rfl
    rw [hstep] at hok ⊢
    simp only [List.map_cons, List.nodup_cons] at hRn
    cases hf : y.1.nodes.find? (acc.fits y.1) with
    | none => rw [hf] at hok; simp only [placeStep_some] at hok; cases hok
    | some n =>
      rw [hf] at hok
      obtain ⟨q, hq, hq0, hqr⟩ := hR y (by simp)
      have hR' : ∀ x ∈ R, ∃ q, (acc.setAtIdx y.2 n).pods[x.2]? = some q ∧ q.atNode = "" ∧ q.req = x.1.req := by
        intro x hx
        have hne : x.2 ≠ y.2 := fun h => hRn.1 (List.mem_map.mpr ⟨x, hx, h⟩)
        show ∃ q, (State.setIdx acc.pods y.2 n)[x.2]? = some q ∧ _
        rw [setIdx_getElem?_ne _ _ _ _ hne]
        exact hR x (by simp [hx])
      refine ih (acc.setAtIdx y.2 n) ?_ hnd hR' hRn.2 hok
      -- The pod fits where first fit put it.
      have hfit := List.find?_some hf
      unfold State.fits at hfit
      cases hnode : acc.node? n with
      | none => rw [hnode] at hfit; cases hfit
      | some nd0 =>
        rw [hnode] at hfit
        have hle := Res.le_true hfit
        have hnd0 : nd0 ∈ acc.nodes := List.mem_of_find?_eq_some hnode
        have hname : nd0.name = n := State.name_of_node? hnode
        by_cases hn : n = ""
        · subst hn
          show CapOK base { acc with pods := State.setIdx acc.pods y.2 "" }
          rw [setIdx_unplaced _ _ hq hq0]
          exact hc
        intro nd hnd'
        have hu : ∀ m, (acc.setAtIdx y.2 n).usage m = uses m (State.setIdx acc.pods y.2 n) := fun m => usage_eq _ m
        by_cases hm : nd.name = n
        · left
          have := mem_of_name hnd hnd' hnd0 (hm.trans hname.symm)
          subst this
          rw [hu, hm, uses_setIdx_self _ _ _ hq hq0 hn, ← usage_eq, hqr]
          obtain ⟨h1, h2, h3⟩ := hle
          simp only [Res.add, Res.Le] at *
          omega
        · rw [hu, uses_setIdx_other _ _ _ _ hq hq0 hm, ← usage_eq]
          exact hc nd hnd'

theorem cap_place (base : String → Res) (st : State) (names : List String) (hc : CapOK base st) (hr : ReqOK st)
    (hnd : (st.nodes.map (·.name)).Nodup) (hok : (st.place names).2 = none) : CapOK base (st.place names).1 := by
  unfold State.place at hok ⊢
  simp only at hok ⊢
  let st1 := st.setAt names ""
  have hc1 : CapOK base st1 := by
    refine hc.congr rfl (fun n => ?_)
    rw [usage_eq, usage_eq]
    exact uses_unplace_le n _ st.pods hr.1
  have hmem : ∀ x ∈ (st1.pods.zipIdx.filter fun x => names.contains x.1.name).mergeSort (fun a b => State.podBefore a.1 b.1),
      ∃ q, st1.pods[x.2]? = some q ∧ q.atNode = "" ∧ q.req = x.1.req := by
    intro x hx
    rw [List.mem_mergeSort, List.mem_filter, List.mem_zipIdx_iff_getElem?] at hx
    refine ⟨x.1, hx.1, ?_, rfl⟩
    have h1 : (st.pods.map fun p => if names.contains p.name then { p with atNode := "" } else p)[x.2]? = some x.1 := hx.1
    simp only [List.getElem?_map] at h1
    cases hp : st.pods[x.2]? with
    | none => rw [hp] at h1; cases h1
    | some p0 =>
      rw [hp] at h1
      simp only [Option.map_some, Option.some.injEq] at h1
      have hc0 := hx.2
      rw [← h1] at hc0 ⊢
      split
      · rfl
      · rename_i hn
        split at hc0
        · contradiction
        · exact absurd hc0 hn
  have hnodup : (((st1.pods.zipIdx.filter fun x => names.contains x.1.name).mergeSort (fun a b => State.podBefore a.1 b.1)).map (·.2)).Nodup := by
    have h0 : (st1.pods.zipIdx.map Prod.snd).Nodup := by
      rw [List.zipIdx_map_snd]; exact List.nodup_range' _
    have h1 := h0.sublist ((List.filter_sublist (p := fun x : MPod × Nat => names.contains x.1.name)).map Prod.snd)
    exact h1.perm ((List.mergeSort_perm _ _).map Prod.snd).symm
  exact place_loop base _ st1 hc1 hnd hmem hnodup hok

/-! ## Every operation keeps the invariant -/

/-- The capacity invariant, with the nodes' names and allocatable fixed. -/
structure CapInv (base : String → Res) (keys : List (String × Res)) (st : State) : Prop where
  cap : CapOK base st
  req : ReqOK st
  nodes : st.nodes.map allocKey = keys

theorem CapInv.nodup {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (hk : (keys.map Prod.fst).Nodup) : (st.nodes.map (·.name)).Nodup := by
  have : st.nodes.map (·.name) = keys.map Prod.fst := by rw [← h.nodes, List.map_map]; rfl
  rw [this]; exact hk

/-- A change to pods that charges nothing new and keeps requests. -/
theorem CapInv.pods {base : String → Res} {keys : List (String × Res)} {st st' : State} (h : CapInv base keys st)
    (hn : st'.nodes = st.nodes) (hd : ∀ m ∈ st'.deps, ∃ m0 ∈ st.deps, m.req = m0.req)
    (hp : ∀ p ∈ st'.pods, (∃ p0 ∈ st.pods, p.req = p0.req) ∨ ∃ m ∈ st.deps, p.req = m.req)
    (hu : ∀ n, Res.Le (uses n st'.pods) (uses n st.pods)) : CapInv base keys st' := by
  refine ⟨h.cap.congr (by rw [hn]) (fun n => by rw [usage_eq, usage_eq]; exact hu n), ⟨?_, ?_⟩, by rw [hn]; exact h.nodes⟩
  · intro p hp'
    rcases hp p hp' with ⟨p0, hp0, he⟩ | ⟨m, hm, he⟩
    · rw [he]; exact h.req.1 p0 hp0
    · rw [he]; exact h.req.2 m hm
  · intro m hm
    obtain ⟨m0, hm0, he⟩ := hd m hm
    rw [he]; exact h.req.2 m0 hm0

theorem CapInv.updNode {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (n : String) (f : MNode → MNode) (hf : ∀ x, (f x).name = x.name ∧ (f x).alloc = x.alloc) :
    CapInv base keys (st.updNode n f) := by
  refine ⟨h.cap.congr ?_ (fun _ => Res.Le.refl _), h.req, ?_⟩
  · simp only [State.updNode, List.map_map]
    apply List.map_congr_left
    intro x _
    simp only [Function.comp, allocKey]
    split
    · rw [(hf x).1, (hf x).2]
    · rfl
  · rw [← h.nodes]
    simp only [State.updNode, List.map_map]
    apply List.map_congr_left
    intro x _
    simp only [Function.comp, allocKey]
    split
    · rw [(hf x).1, (hf x).2]
    · rfl

theorem CapInv.updPods {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (sel : MPod → Bool) (f : MPod → MPod) (hf : ∀ p, (f p).atNode = p.atNode ∧ (f p).req = p.req) :
    CapInv base keys (st.updPods sel f) := by
  refine h.pods rfl (fun m hm => ⟨m, hm, rfl⟩) (fun p hp => Or.inl ?_) (fun n => ?_)
  · simp only [State.updPods, List.mem_map] at hp
    obtain ⟨p0, hp0, rfl⟩ := hp
    refine ⟨p0, hp0, ?_⟩
    split
    · exact (hf p0).2
    · rfl
  · show Res.Le (uses n (st.pods.map _)) _
    rw [uses_map]
    · exact Res.Le.refl _
    · intro p _; split
      · exact hf p
      · exact ⟨rfl, rfl⟩

theorem CapInv.updDep {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (d : String) (f : MDep → MDep) (hf : ∀ x, (f x).req = x.req) : CapInv base keys (st.updDep d f) := by
  refine h.pods rfl (fun m hm => ?_) (fun p hp => Or.inl ⟨p, hp, rfl⟩) (fun _ => Res.Le.refl _)
  obtain ⟨y, hy, rfl⟩ := mem_updDep hm
  refine ⟨y, hy, ?_⟩
  split
  · exact hf y
  · rfl

theorem CapInv.newPods {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (d : String) (k : Int) : CapInv base keys (st.newPods d k).1 := by
  unfold State.newPods
  split
  · exact h
  · rename_i m hm
    have h1 := h.updDep d (fun x => { x with made := x.made + k.toNat }) (fun _ => rfl)
    refine h1.pods rfl (fun x hx => ⟨x, hx, rfl⟩) (fun p hp => ?_) (fun n => ?_)
    · rcases List.mem_append.mp hp with hp | hp
      · exact Or.inl ⟨p, hp, rfl⟩
      · right
        simp only [List.mem_map] at hp
        obtain ⟨nm, -, rfl⟩ := hp
        have hm' : (st.updDep d fun x => { x with made := x.made + k.toNat }).dep? d = some { m with made := m.made + k.toNat } :=
          dep?_updDep_self st d (fun x => { x with made := x.made + k.toNat }) (fun _ => rfl) hm
        exact ⟨_, List.mem_of_find?_eq_some hm', rfl⟩
    · show Res.Le (uses n (st.pods ++ _)) (uses n st.pods)
      rw [uses_append_unplaced]
      · exact Res.Le.refl _
      · intro p hp
        simp only [List.mem_map] at hp
        obtain ⟨nm, -, rfl⟩ := hp
        rfl

theorem CapInv.place {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (hk : (keys.map Prod.fst).Nodup) (names : List String) (hok : (st.place names).2 = none) :
    CapInv base keys (st.place names).1 := by
  obtain ⟨ps, hps, hm⟩ := st.place_shape names
  refine ⟨cap_place base st names h.cap h.req (h.nodup hk) hok, ⟨?_, ?_⟩, ?_⟩
  · rw [hps]
    intro p hp
    have : p.unplace ∈ st.pods.map MPod.unplace := by rw [← hm]; exact List.mem_map_of_mem hp
    obtain ⟨p0, hp0, he⟩ := List.mem_map.mp this
    have h' : p.unplace.req = p0.unplace.req := congrArg (fun x : MPod => x.req) he.symm
    have : p.req = p0.req := h'
    rw [this]; exact h.req.1 p0 hp0
  · rw [hps]; exact h.req.2
  · rw [hps]; exact h.nodes

theorem uses_dropPods (n d : String) (U : List String) (qn qr : Nat) (l : List MPod) :
    uses n (dropPods d U qn qr l) = uses n l ∧ ∀ p ∈ dropPods d U qn qr l, ∃ p0 ∈ l, p.req = p0.req := by
  induction l generalizing qn qr with
  | nil => exact ⟨rfl, fun _ h => by cases h⟩
  | cons p ps ih =>
    unfold dropPods
    split
    · split
      · obtain ⟨i1, i2⟩ := ih (qn - 1) qr
        refine ⟨by simp only [uses, i1], fun x hx => ?_⟩
        rcases List.mem_cons.mp hx with rfl | hx
        · exact ⟨p, by simp, rfl⟩
        · obtain ⟨p0, h0, he⟩ := i2 x hx; exact ⟨p0, by simp [h0], he⟩
      · split
        · obtain ⟨i1, i2⟩ := ih qn (qr - 1)
          refine ⟨by simp only [uses, i1], fun x hx => ?_⟩
          rcases List.mem_cons.mp hx with rfl | hx
          · exact ⟨p, by simp, rfl⟩
          · obtain ⟨p0, h0, he⟩ := i2 x hx; exact ⟨p0, by simp [h0], he⟩
        · obtain ⟨i1, i2⟩ := ih qn qr
          refine ⟨by simp only [uses, i1], fun x hx => ?_⟩
          rcases List.mem_cons.mp hx with rfl | hx
          · exact ⟨p, by simp, rfl⟩
          · obtain ⟨p0, h0, he⟩ := i2 x hx; exact ⟨p0, by simp [h0], he⟩
    · obtain ⟨i1, i2⟩ := ih qn qr
      refine ⟨by simp only [uses, i1], fun x hx => ?_⟩
      rcases List.mem_cons.mp hx with rfl | hx
      · exact ⟨x, by simp, rfl⟩
      · obtain ⟨p0, h0, he⟩ := i2 x hx; exact ⟨p0, by simp [h0], he⟩

theorem CapInv.scaleDown {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (d : String) (n : Int) : CapInv base keys (st.scaleDown d n) := by
  unfold State.scaleDown
  dsimp only
  refine h.pods rfl (fun m hm => ⟨m, hm, rfl⟩) (fun p hp => Or.inl ((uses_dropPods "" _ _ _ _ st.pods).2 p hp)) (fun x => ?_)
  rw [(uses_dropPods x _ _ _ _ _).1]; exact Res.Le.refl _

theorem CapInv.filterPods {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (q : MPod → Bool) : CapInv base keys { st with pods := st.pods.filter q } :=
  h.pods rfl (fun m hm => ⟨m, hm, rfl⟩) (fun p hp => Or.inl ⟨p, (List.mem_filter.mp hp).1, rfl⟩)
    (fun n => uses_filter_le n q st.pods h.req.1)

theorem CapInv.surgeFits {base : String → Res} {keys : List (String × Res)} {st : State} (h : CapInv base keys st)
    (d : String) (m : MDep) : CapInv base keys (st.surgeFits d m).1 := by
  have hs := SameUpTo.surgeFits st d m
  refine h.pods hs.nodes (fun x hx => ?_) (fun p hp => Or.inl ⟨p, by rw [← surgeFits_pods st d m]; exact hp, rfl⟩)
    (fun n => by rw [surgeFits_pods]; exact Res.Le.refl _)
  obtain ⟨y, hy, he⟩ := mem_strip hs.deps hx
  have h' : x.strip.req = y.strip.req := congrArg (fun x : MDep => x.req) he
  exact ⟨y, hy, h'⟩

theorem cap_applyOp {base : String → Res} {keys : List (String × Res)} (hk : (keys.map Prod.fst).Nodup)
    (st : State) (s : Step) (h : CapInv base keys st) (hok : (applyOp st s).2 = true) :
    CapInv base keys (applyOp st s).1 := by
  by_cases h1 : s.op = "cordon"
  · simp only [applyOp, h1]; exact h.updNode _ _ (fun _ => ⟨rfl, rfl⟩)
  by_cases h2 : s.op = "uncordon"
  · simp only [applyOp, h2]
    have h' := h.updNode s.node (fun n => { n with unsched := false }) (fun _ => ⟨rfl, rfl⟩)
    split
    · exact h'.updPods _ _ (fun _ => ⟨rfl, rfl⟩)
    · exact h'
  by_cases h3 : s.op = "drain"
  · simp only [applyOp, h3] at hok ⊢
    have hA := h.updPods (fun p => p.dep != "" && p.mightBeOn s.node)
      (fun p => { p with ready := false, created := true,
                         nodes := compact (sortStrings (st.schedulable ++ p.nodes.filter (· != s.node))) })
      (fun _ => ⟨rfl, rfl⟩)
    have hB := hA.updNode s.node (fun n => { n with drained := true }) (fun _ => ⟨rfl, rfl⟩)
    exact hB.place hk _ (by simpa using hok)
  by_cases h4 : s.op = "scale"
  · simp only [applyOp, h4] at hok ⊢
    cases hd : st.dep? s.deployment with
    | none => exact h
    | some m =>
      simp only [hd] at hok ⊢
      have h' := h.updDep s.deployment (fun x => { x with replicas := s.replicas.getD 0 }) (fun _ => rfl)
      by_cases hgt : s.replicas.getD 0 > m.replicas
      · rw [if_pos hgt] at hok ⊢
        exact CapInv.place (h'.newPods _ _) hk _ (by simpa using hok)
      · rw [if_neg hgt] at hok ⊢
        split
        · exact h'.scaleDown _ _
        · exact h'
  have hroll : ∀ (m : MDep), (let floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail))
      let (st, ok) := (st.updDep s.deployment fun d => { d with floor }).surgeFits s.deployment m
      if ok then st.rollout s m else (st, false)).2 = true →
      CapInv base keys (let floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail))
      let (st, ok) := (st.updDep s.deployment fun d => { d with floor }).surgeFits s.deployment m
      if ok then st.rollout s m else (st, false)).1 := by
    intro m hok'
    dsimp only at hok' ⊢
    have h2 := (h.updDep s.deployment (fun d => { d with floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail)) })
      (fun _ => rfl)).surgeFits s.deployment m
    generalize State.surgeFits _ _ _ = sf at h2 hok' ⊢
    obtain ⟨st2, ok⟩ := sf
    cases ok
    · simp at hok'
    · simp only [ite_true, State.rollout] at hok' ⊢
      have hr : ∀ old x, (rolloutUpdate s old x).req = x.req := by
        intro old x; unfold rolloutUpdate; split <;> rfl
      exact CapInv.place (((h2.filterPods _).updDep _ _ (hr _)).newPods _ _) hk _ (by simpa using hok')
  by_cases h5 : s.op = "set-image"
  · simp only [applyOp, h5] at hok ⊢
    cases hd : st.dep? s.deployment with
    | none => exact h
    | some m => simp only [hd] at hok; exact hroll m hok
  by_cases h6 : s.op = "rollout-restart"
  · simp only [applyOp, h6] at hok ⊢
    cases hd : st.dep? s.deployment with
    | none => exact h
    | some m => simp only [hd] at hok; exact hroll m hok
  by_cases h7 : s.op = "wait"
  · simp only [applyOp, h7]
    rw [wait_fold]
    clear hroll hok
    generalize s.deployments = ds
    induction ds generalizing st with
    | nil => exact h
    | cons d rest ih => exact ih (st.waitOne d) ((h.updDep d waitDep (fun _ => rfl)).updPods _ _ (fun _ => ⟨rfl, rfl⟩))
  · simp only [applyOp, h1, h2, h3, h4, h5, h6, h7]; exact h

/-! ## The snapshot -/

theorem pdbStep_deps (st : State) (b : SPdb) : ∀ m ∈ (pdbStep st b).deps, ∃ m0 ∈ st.deps, m.req = m0.req := by
  intro m hm
  unfold pdbStep at hm
  split at hm
  · simp only [List.mem_map] at hm
    obtain ⟨y, hy, rfl⟩ := hm
    exact ⟨y, hy, by split <;> rfl⟩
  · dsimp only at hm
    split at hm
    · obtain ⟨y, hy, rfl⟩ := mem_updDep hm
      exact ⟨y, hy, by split <;> rfl⟩
    · exact ⟨m, hm, rfl⟩

theorem build_req (s : Snapshot) (hv : Valid s) : ReqOK (build s) := by
  have hz : ∀ r : Res, ReqNonNeg r → Res.Le {} r := fun r h => h
  constructor
  · intro p hp
    rw [(build_parts s).1] at hp
    obtain ⟨q, hq, rfl⟩ := List.mem_map.mp hp
    exact hz _ (hv.2.2.2.1 q hq)
  · rw [build_eq]
    show ∀ m ∈ (s.pdbs.foldl pdbStep (buildBase s)).deps, _
    suffices h : ∀ (L : List SPdb) (st : State), (∀ m ∈ st.deps, Res.Le {} m.req) →
        ∀ m ∈ (L.foldl pdbStep st).deps, Res.Le {} m.req from by
      apply h
      intro m hm
      simp only [buildBase, List.mem_map] at hm
      obtain ⟨d, hd, rfl⟩ := hm
      exact hz _ (hv.2.2.2.2 d hd)
    intro L
    induction L with
    | nil => intro st h; exact h
    | cons b rest ih =>
      intro st h
      apply ih
      intro m hm
      obtain ⟨m0, hm0, he⟩ := pdbStep_deps st b m hm
      rw [he]; exact h m0 hm0

/-- **Rule `capacity` is sound.** If the checker says Proved, then after every
step of the plan and its rollback, first fit's placement fits: every node's
charged requests are within its allocatable, or no more than in the
snapshot. -/
theorem capacity_sound (snap : Snapshot) (plan : Plan) (pol : Policy) (hv : Valid snap)
    (h : (check snap plan pol).result = .proved) :
    ∀ i, i ≤ (allSteps plan).length →
      CapOK (build snap).usage (runOps (build snap) ((allSteps plan).take i)) := by
  let c := Ctx.make snap plan pol
  have hall := passed_all (execute_passed c h)
  let keys := (build snap).nodes.map allocKey
  have hk : (keys.map Prod.fst).Nodup := by
    have : keys.map Prod.fst = (build snap).nodes.map (·.name) := by simp only [keys, List.map_map]; rfl
    rw [this]; exact nodes_nodup hv.1 (nodeKeys_build snap)
  suffices hinv : ∀ i, i ≤ (allSteps plan).length →
      CapInv (build snap).usage keys (runOps (build snap) ((allSteps plan).take i)) from
    fun i hi => (hinv i hi).cap
  intro i
  induction i with
  | zero =>
    intro _
    exact ⟨fun nd _ => Or.inr (Res.Le.refl _), build_req snap hv, rfl⟩
  | succ i ih =>
    intro hi
    have hlt : i < (allSteps plan).length := hi
    rw [runOps_succ _ _ _ hlt]
    exact cap_applyOp hk _ _ (ih (Nat.le_of_lt hlt)) (hall i hlt).placed

end Planproof
