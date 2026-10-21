import Planproof.Proofs.Sim

/-!
# Each operation keeps the correspondence (part 1: bookkeeping)

Two checker states that differ only in bookkeeping the real cluster doesn't
have (where first fit charged each pod, the counter for new pod names)
correspond to the same real worlds.
-/
namespace Planproof

open Spec

def MPod.strip (p : MPod) : MPod := { p with atNode := "" }
def MDep.strip (d : MDep) : MDep := { d with made := 0 }

/-- Same state but for bookkeeping. -/
structure SameUpTo (st st' : State) : Prop where
  deps : st'.deps.map MDep.strip = st.deps.map MDep.strip
  nodes : st'.nodes = st.nodes
  pods : st'.pods.map MPod.strip = st.pods.map MPod.strip

theorem SameUpTo.refl (st : State) : SameUpTo st st := ⟨rfl, rfl, rfl⟩

theorem SameUpTo.trans {a b c : State} (h1 : SameUpTo a b) (h2 : SameUpTo b c) : SameUpTo a c :=
  ⟨h2.deps.trans h1.deps, h2.nodes.trans h1.nodes, h2.pods.trans h1.pods⟩

theorem SameUpTo.symm {a b : State} (h : SameUpTo a b) : SameUpTo b a :=
  ⟨h.deps.symm, h.nodes.symm, h.pods.symm⟩

theorem dep?_strip {st st' : State} (h : st'.deps.map MDep.strip = st.deps.map MDep.strip) (d : String) :
    (st'.dep? d).map MDep.strip = (st.dep? d).map MDep.strip := by
  unfold State.dep?
  have e : ∀ l : List MDep, (l.find? (·.name == d)).map MDep.strip = (l.map MDep.strip).find? (·.name == d) := by
    intro l
    rw [List.find?_map]
    rfl
  rw [e, e, h]

theorem SameUpTo.field {st st' : State} (h : SameUpTo st st') : st'.field = st.field := by
  funext k
  have hd := dep?_strip h.deps
  cases k with
  | replicas d =>
    have := congrArg (Option.map fun m => Value.int m.replicas) (hd d)
    simpa [State.field, Option.map_map, MDep.strip, Function.comp_def] using this
  | restarted d =>
    have := congrArg (Option.map fun m => Value.bool m.restarted) (hd d)
    simpa [State.field, Option.map_map, MDep.strip, Function.comp_def] using this
  | image d c =>
    have := congrArg (Option.map fun m => (m.images.find? (·.1 == c)).map (·.2)) (hd d)
    simp only [Option.map_map, Function.comp_def, MDep.strip] at this
    simp only [State.field]
    cases h1 : st'.dep? d <;> cases h2 : st.dep? d <;> simp_all
  | unsched n => simp [State.field, State.node?, h.nodes]
  | drained n => simp [State.field, State.node?, h.nodes]

theorem SameUpTo.rolling {st st' : State} (h : SameUpTo st st') (d : String) : st'.rolling d = st.rolling d := by
  have := congrArg (Option.map (·.rolling)) (dep?_strip h.deps d)
  simp only [Option.map_map, Function.comp_def, MDep.strip] at this
  simp [State.rolling, this]

theorem strip_eq_map {l l' : List MPod} (h : l'.map MPod.strip = l.map MPod.strip) :
    ∃ pairs : List (MPod × MPod), pairs.map Prod.fst = l ∧ pairs.map Prod.snd = l' ∧ ∀ x ∈ pairs, x.2.strip = x.1.strip := by
  induction l generalizing l' with
  | nil =>
    cases l' with
    | nil => exact ⟨[], rfl, rfl, by simp⟩
    | cons _ _ => simp at h
  | cons a l ih =>
    cases l' with
    | nil => simp at h
    | cons b l' =>
      simp only [List.map_cons, List.cons.injEq] at h
      obtain ⟨ps, h1, h2, h3⟩ := ih h.2
      refine ⟨(a, b) :: ps, by simp [h1], by simp [h2], ?_⟩
      intro x hx
      rcases List.mem_cons.mp hx with rfl | hx
      · exact h.1
      · exact h3 x hx

theorem MPod.strip_parts {p q : MPod} (h : q.strip = p.strip) :
    q.dep = p.dep ∧ q.owner = p.owner ∧ q.ready = p.ready ∧ q.nodes = p.nodes ∧ q.gone = p.gone := by
  have h1 := congrArg (fun x : MPod => x.dep) h
  have h2 := congrArg (fun x : MPod => x.owner) h
  have h3 := congrArg (fun x : MPod => x.ready) h
  have h4 := congrArg (fun x : MPod => x.nodes) h
  have h5 := congrArg (fun x : MPod => x.gone) h
  simp only [MPod.strip] at h1 h2 h3 h4 h5
  exact ⟨h1, h2, h3, h4, h5⟩

theorem match_strip {sch : List String} {p p' : MPod} {q : CPod} (h : p'.strip = p.strip) (hm : Match sch p q) :
    Match sch p' q := by
  obtain ⟨hd, ho, hr, hn, -⟩ := MPod.strip_parts h
  obtain ⟨h1, h2, h3, h4⟩ := hm
  refine ⟨h1.trans hd.symm, h2.trans ho.symm, fun hp => h3 (hr ▸ hp), ?_⟩
  cases hq : q.node with
  | some n => rw [hq] at h4; rw [hn]; exact h4
  | none => rw [hq] at h4; rw [hn, hr]; exact h4

theorem live_forall2 {sch : List String} {st st' : State} (h : SameUpTo st st') {al : List CPod}
    (hf : Forall2 (Match sch) st.live al) : Forall2 (Match sch) st'.live al := by
  unfold State.live at hf ⊢
  obtain ⟨ps, h1, h2, h3⟩ := strip_eq_map h.pods
  rw [← h1] at hf
  rw [← h2]
  clear h1 h2
  induction ps generalizing al with
  | nil => simpa using hf
  | cons x ps ih =>
    obtain ⟨hd, -, -, -, hg⟩ := MPod.strip_parts (h3 x (by simp))
    have hr := h.rolling x.1.dep
    simp only [List.map_cons, List.filter_cons, hd, hg, hr] at hf ⊢
    by_cases hc : (!x.fst.gone && !st.rolling x.fst.dep) = true
    · rw [if_pos hc] at hf ⊢
      cases hf with
      | cons hm hf' => exact .cons (match_strip (h3 x (by simp)) hm) (ih hf' (fun y hy => h3 y (by simp [hy])))
    · rw [if_neg hc] at hf ⊢
      exact ih hf (fun y hy => h3 y (by simp [hy]))

/-- A world that corresponds to a state corresponds to any state that is
the same but for bookkeeping. -/
theorem Sim.congr {s : Snapshot} {st st' : State} {w : World} (hs : Sim s st w) (h : SameUpTo st st') : Sim s st' w where
  fields := by rw [h.field]; exact hs.fields
  pods := by
    obtain ⟨al, hp, hf⟩ := hs.pods
    exact ⟨al, hp, live_forall2 h hf⟩
  rollIff := fun d => by rw [h.rolling]; exact hs.rollIff d
  roll := by
    intro d r hr
    obtain ⟨m, hm, hfl, hold, hnew, hcount, hmax⟩ := hs.roll d r hr
    have hd := dep?_strip h.deps d
    rw [hm] at hd
    cases hm' : st'.dep? d with
    | none => rw [hm'] at hd; cases hd
    | some m' =>
      rw [hm'] at hd
      have hd : m'.strip = m.strip := Option.some.inj hd
      obtain ⟨ps, h1, h2, h3⟩ := strip_eq_map h.pods
      refine ⟨m', rfl, ?_, ?_, ?_, ?_, ?_⟩
      · have e := congrArg (fun x : MDep => x.floor) hd
        simp only [MDep.strip] at e
        rw [e]; exact hfl
      · have e := congrArg (fun x : MDep => x.oldNodes) hd
        simp only [MDep.strip] at e
        rw [e]; exact hold
      · intro p hp hpd hpg
        rw [← h2] at hp
        obtain ⟨x, hx, rfl⟩ := List.mem_map.mp hp
        obtain ⟨hd', -, hr', hn', hg'⟩ := MPod.strip_parts (h3 x hx)
        have hx1 : x.1 ∈ st.pods := by rw [← h1]; exact List.mem_map_of_mem hx
        obtain ⟨a, b⟩ := hnew x.1 hx1 (hd' ▸ hpd) (hg' ▸ hpg)
        exact ⟨hr' ▸ a, hn' ▸ b⟩
      · rw [← hcount, ← h1, ← h2, List.filter_map, List.filter_map, List.length_map, List.length_map]
        congr 1
        apply List.filter_congr
        intro x hx
        obtain ⟨hd', -, -, -, hg'⟩ := MPod.strip_parts (h3 x hx)
        simp [hd', hg']
      · have e := congrArg (fun x : MDep => x.floor) hd
        simp only [MDep.strip] at e
        rw [e]; exact hmax
  owner := by
    intro p hp hd
    obtain ⟨ps, h1, h2, h3⟩ := strip_eq_map h.pods
    rw [← h2] at hp
    obtain ⟨x, hx, rfl⟩ := List.mem_map.mp hp
    obtain ⟨hd', ho', -⟩ := MPod.strip_parts (h3 x hx)
    have hx1 : x.1 ∈ st.pods := by rw [← h1]; exact List.mem_map_of_mem hx
    rw [ho']; exact hs.owner x.1 hx1 (hd' ▸ hd)
  nodes := by rw [h.nodes]; exact hs.nodes
  mu := by
    intro m' hm'
    have : m'.strip ∈ st.deps.map MDep.strip := by rw [← h.deps]; exact List.mem_map_of_mem hm'
    obtain ⟨m, hm, he⟩ := List.mem_map.mp this
    have e := congrArg (fun x : MDep => x.maxUnavail) he
    simp only [MDep.strip] at e
    rw [← e]; exact hs.mu m hm

/-! ## Bookkeeping steps -/

theorem SameUpTo.setAt (st : State) (names : List String) (n : String) : SameUpTo st (st.setAt names n) := by
  refine ⟨rfl, rfl, ?_⟩
  simp only [State.setAt, State.updPods, List.map_map]
  apply List.map_congr_left
  intro p _
  simp only [Function.comp]
  split <;> rfl

theorem SameUpTo.place (st : State) (names : List String) : SameUpTo st (st.place names).1 := by
  unfold State.place
  simp only
  generalize (List.filter _ _).mergeSort _ = order
  have h0 := SameUpTo.setAt st names ""
  suffices h : ∀ (acc : State × Option MPod), SameUpTo st acc.1 →
      SameUpTo st (order.foldl (fun (x : State × Option MPod) p =>
        match x with
        | (st, bad) => match bad with
          | some _ => (st, bad)
          | none => match p.nodes.find? (st.fits p) with
            | some n => (st.setAt [p.name] n, none)
            | none => (st, some p)) acc).1 from h _ h0
  induction order with
  | nil => intro acc h; exact h
  | cons p rest ih =>
    intro acc h
    apply ih
    obtain ⟨s1, bad⟩ := acc
    cases bad with
    | some _ => exact h
    | none =>
      simp only
      split
      · exact h.trans (SameUpTo.setAt _ _ _)
      · exact h

end Planproof
