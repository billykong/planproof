import Planproof.Proofs.SimRollout

/-!
# Each operation keeps the correspondence (part 6: wait)

`wait` for a deployment: in the checker its pods become ready and it is no
longer mid rollout; in the real world a rollout ends with fresh ready pods
on nodes new pods may go to, or, with no rollout, every pod it has gets a
node and becomes ready.
-/
namespace Planproof

open Spec

def waitDep (x : MDep) : MDep := { x with rolling := false, oldNodes := [] }
def readyPod (p : MPod) : MPod := { p with ready := true }
def waitSel (d : String) (p : MPod) : Bool := p.dep == d && !p.gone
def waitPod (d : String) (p : MPod) : MPod := if waitSel d p then readyPod p else p

/-- What `wait` does for one deployment in the checker. -/
def State.waitOne (st : State) (d : String) : State := (st.updDep d waitDep).updPods (waitSel d) readyPod

theorem waitPod_parts (d : String) (p : MPod) :
    (waitPod d p).dep = p.dep ∧ (waitPod d p).owner = p.owner ∧ (waitPod d p).gone = p.gone ∧
    (waitPod d p).nodes = p.nodes := by
  unfold waitPod; split <;> exact ⟨rfl, rfl, rfl, rfl⟩

theorem waitPod_other {d : String} {p : MPod} (h : p.dep ≠ d) : waitPod d p = p := by
  unfold waitPod waitSel; simp [h]

theorem waitPod_self {d : String} {p : MPod} (h : p.dep = d) (hg : p.gone = false) : waitPod d p = readyPod p := by
  unfold waitPod waitSel; simp [h, hg]

theorem State.waitOne_pods (st : State) (d : String) : (st.waitOne d).pods = st.pods.map (waitPod d) := rfl

theorem State.waitOne_field (st : State) (d : String) : (st.waitOne d).field = st.field := by
  unfold State.waitOne
  rw [State.field_updPods]
  exact State.field_updDep_keep st d _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)

theorem State.waitOne_dep?_other (st : State) {d e : String} (h : e ≠ d) : (st.waitOne d).dep? e = st.dep? e :=
  dep?_updDep_other st d e waitDep (fun _ => rfl) h

theorem State.waitOne_rolling (st : State) (d e : String) :
    (st.waitOne d).rolling e = if e = d then false else st.rolling e := by
  split
  · rename_i h; subst h
    show ((((st.updDep e waitDep).dep? e).map (·.rolling)).getD false) = false
    rw [State.dep?_updDep st e e waitDep (fun _ => rfl)]
    cases hx : st.dep? e with
    | none => rfl
    | some x => simp [State.name_of_dep? hx, waitDep]
  · rename_i h
    unfold State.rolling
    rw [st.waitOne_dep?_other h]

/-- Lists related element by element, when every pair is related. -/
theorem forall2_of_all {α β : Type} {R : α → β → Prop} {l1 : List α} {l2 : List β} (hl : l1.length = l2.length)
    (h : ∀ a ∈ l1, ∀ b ∈ l2, R a b) : Forall2 R l1 l2 := by
  induction l1 generalizing l2 with
  | nil => cases l2 with
    | nil => exact .nil
    | cons _ _ => simp at hl
  | cons a l1 ih => cases l2 with
    | nil => simp at hl
    | cons b l2 =>
      exact .cons (h a (by simp) b (by simp))
        (ih (by simpa using hl) (fun x hx y hy => h x (by simp [hx]) y (by simp [hy])))

theorem find?_filter_dep (rs : List Rollout) (d e : String) :
    (rs.filter (·.dep != d)).find? (·.dep == e) = if e = d then none else rs.find? (·.dep == e) := by
  by_cases he : e = d
  · subst he
    rw [if_pos rfl, List.find?_eq_none]
    intro x hx
    simp only [List.mem_filter] at hx
    simpa using hx.2
  · rw [if_neg he]
    induction rs with
    | nil => rfl
    | cons r rs ih =>
      by_cases hr : r.dep = d
      · have h1 : (r.dep == e) = false := by simp [hr, Ne.symm he]
        have h2 : (d == e) = false := by simp [Ne.symm he]
        simp [List.filter_cons, List.find?_cons, hr, h2, ih]
      · simp [List.filter_cons, List.find?_cons, hr, ih]

/-- The parts of the correspondence about everything but the pairing and
the rollouts survive a `wait` for `d`. -/
theorem Sim.waitRollOther {s : Snapshot} {st : State} {w : World} (hs : Sim s st w) {d e : String} (he : e ≠ d)
    {r : Rollout} (hr : w.rollout? e = some r) (f' : Fields) (hf' : f' = w.fields) :
    ∃ m, (st.waitOne d).dep? e = some m ∧ m.floor ≤ r.floor ∧
      (∀ n ∈ r.oldNodes, n ∈ m.oldNodes) ∧
      (∀ p ∈ (st.waitOne d).pods, p.dep = e → p.gone = false → p.ready = false ∧ ∀ n ∈ r.newNodes, n ∈ p.nodes) ∧
      ((st.waitOne d).pods.filter fun p => p.dep == e && !p.gone).length = (replicasOf f' e).toNat ∧
      m.floor ≤ max 0 (replicasOf f' e) := by
  subst hf'
  obtain ⟨m, hm, h1, h2, h3, h4, h5⟩ := hs.roll e r hr
  refine ⟨m, by rw [st.waitOne_dep?_other he]; exact hm, h1, h2, ?_, ?_, h5⟩
  · intro p hp hpd hpg
    rw [State.waitOne_pods] at hp
    obtain ⟨q, hq, rfl⟩ := List.mem_map.mp hp
    obtain ⟨hd, -, hg, -⟩ := waitPod_parts d q
    have hqd : q.dep ≠ d := by rw [← hd, hpd]; exact he
    rw [waitPod_other hqd] at hpd hpg ⊢
    exact h3 q hq hpd hpg
  · rw [State.waitOne_pods, List.filter_map, List.length_map, ← h4]
    congr 1
    apply List.filter_congr
    intro p _
    obtain ⟨hd, -, hg, -⟩ := waitPod_parts d p
    simp only [Function.comp, hd, hg]

theorem Sim.waitOne {s : Snapshot} {st : State} {w w' : World} {d : String} (hs : Sim s st w)
    (hw : WaitOne s w d w') : Sim s (st.waitOne d) w' := by
  obtain ⟨hfw, hw⟩ := hw
  have hsch : sched s w'.fields = sched s w.fields := by rw [hfw]
  have hdeps : ∀ x ∈ (st.waitOne d).deps, ∃ y ∈ st.deps, x.name = y.name ∧ x.maxUnavail = y.maxUnavail ∧
      depStat x = depStat y := by
    intro x hx
    obtain ⟨y, hy, rfl⟩ := mem_updDep (f := waitDep) hx
    refine ⟨y, hy, ?_⟩
    split <;> exact ⟨rfl, rfl, rfl⟩
  have hfields : w'.fields = (st.waitOne d).field := by rw [hfw, hs.fields, State.waitOne_field]
  have howner : ∀ p ∈ (st.waitOne d).pods, p.dep ≠ "" → p.owner = "Deployment" := by
    intro p hp hd
    rw [State.waitOne_pods] at hp
    obtain ⟨q, hq, rfl⟩ := List.mem_map.mp hp
    obtain ⟨h1, h2, -, -⟩ := waitPod_parts d q
    rw [h2]; exact hs.owner q hq (h1 ▸ hd)
  have hmu : ∀ m ∈ (st.waitOne d).deps, 0 ≤ m.maxUnavail := by
    intro x hx; obtain ⟨y, hy, -, h, -⟩ := hdeps x hx; rw [h]; exact hs.mu y hy
  have hnames : ∀ m ∈ (st.waitOne d).deps, m.name ≠ "" := by
    intro x hx; obtain ⟨y, hy, h, -⟩ := hdeps x hx; rw [h]; exact hs.names y hy
  have hstat : ∀ e, ((st.waitOne d).dep? e).map (fun m => (m.recreate, m.maxUnavail)) =
      (deployment? s e).map (fun sd => (sd.recreate, sd.maxUnavail)) := by
    intro e
    rw [← hs.stat e]
    exact dep?_map_stat st _ (fun x => by split <;> rfl) (fun x => by split <;> rfl) e
  obtain ⟨al, hp, hf⟩ := hs.pods
  cases hro : w.rollout? d with
  | none =>
    rw [hro] at hw
    obtain ⟨hrs, hpw⟩ := hw
    have hroll : st.rolling d = false := by
      have := hs.rollIff d; rw [hro] at this; exact this.symm
    have hrolW : ∀ e, (st.waitOne d).rolling e = st.rolling e := by
      intro e; rw [State.waitOne_rolling]; split
      · rename_i h; rw [h, hroll]
      · rfl
    have hro' : ∀ e, w'.rollout? e = w.rollout? e := by intro e; simp [World.rollout?, hrs]
    have hlive : (st.waitOne d).live = st.live.map (waitPod d) := by
      unfold State.live
      rw [State.waitOne_pods, List.filter_map]
      congr 1
      apply List.filter_congr
      intro p _
      obtain ⟨hd, -, hg, -⟩ := waitPod_parts d p
      simp only [Function.comp, hd, hg, hrolW]
    refine ⟨hfields, ?_, fun e => by rw [hro', hrolW]; exact hs.rollIff e, ?_, howner, hs.nodes, hmu, hnames, hstat⟩
    · obtain ⟨al', hal', hr'⟩ := hpw.perm_left hp.symm
      refine ⟨al', hal', ?_⟩
      rw [hlive, hsch]
      refine (hf.comp hr').map_left_mem (waitPod d) (fun p q' hpl ⟨q, hm, hq⟩ => ?_)
      have hpg : p.gone = false := by
        have := (List.mem_filter.mp hpl).2; simp only [Bool.and_eq_true, Bool.not_eq_true'] at this; exact this.1
      obtain ⟨hd, ho, hr, hn⟩ := hm
      by_cases hqd : q.dep = d
      · rw [if_pos hqd] at hq
        obtain ⟨hd', ho', hr', hpend, hkeep⟩ := hq
        have hpd : p.dep = d := hd.symm.trans hqd
        rw [waitPod_self hpd hpg]
        refine ⟨hd'.trans hd, ho'.trans ho, fun _ => hr', ?_⟩
        cases hqn : q.node with
        | some n =>
          rw [hqn] at hn
          rw [hkeep (by simp [hqn]), hqn]; exact hn
        | none =>
          rw [hqn] at hn
          obtain ⟨m, hm, hq'n⟩ := hpend hqn
          rw [hq'n]; exact hn.2.2 m hm
      · rw [if_neg hqd] at hq
        subst hq
        rw [waitPod_other (fun h => hqd (hd.trans h))]
        exact ⟨hd, ho, hr, hn⟩
    · intro e r hr
      rw [hro'] at hr
      have he : e ≠ d := by rintro rfl; rw [hro] at hr; cases hr
      exact hs.waitRollOther he hr w'.fields hfw
  | some r =>
    rw [hro] at hw
    obtain ⟨hrs, fresh, hlen, hfresh, hpw⟩ := hw
    have hroll : st.rolling d = true := by
      have := hs.rollIff d; rw [hro] at this; exact this.symm
    obtain ⟨m, hm, -, -, hnew, hcount, -⟩ := hs.roll d r hro
    have hdne : d ≠ "" := by rw [← State.name_of_dep? hm]; exact hs.names m (List.mem_of_find?_eq_some hm)
    have hro' : ∀ e, w'.rollout? e = if e = d then none else w.rollout? e := by
      intro e; simp only [World.rollout?, hrs]; exact find?_filter_dep _ _ _
    have hrolW := st.waitOne_rolling d
    -- The live pods of other deployments are unchanged.
    have hother : (st.waitOne d).live.filter (fun p => !(p.dep == d)) = st.live := by
      unfold State.live
      rw [List.filter_filter, State.waitOne_pods, List.filter_map]
      have : (st.pods.filter ((fun p => !(p.dep == d) && (!p.gone && !(st.waitOne d).rolling p.dep)) ∘ waitPod d)) =
          st.pods.filter fun p => !p.gone && !st.rolling p.dep := by
        apply List.filter_congr
        intro p _
        obtain ⟨hd, -, hg, -⟩ := waitPod_parts d p
        simp only [Function.comp, hd, hg, hrolW]
        by_cases hpd : p.dep = d
        · simp [hpd, hroll]
        · simp [hpd]
      rw [this]
      conv => rhs; rw [← List.map_id (List.filter _ st.pods)]
      apply List.map_congr_left
      intro p hp
      have : p.dep ≠ d := by
        intro h
        have := (List.mem_filter.mp hp).2
        simp [h, hroll] at this
      exact waitPod_other this
    -- The deployment's own pods, now ready.
    have hmine : (st.waitOne d).live.filter (fun p => p.dep == d) = (st.podsOf d).map (waitPod d) := by
      unfold State.live State.podsOf
      rw [List.filter_filter, State.waitOne_pods, List.filter_map]
      congr 1
      apply List.filter_congr
      intro p _
      obtain ⟨hd, -, hg, -⟩ := waitPod_parts d p
      simp only [Function.comp, hd, hg, hrolW]
      by_cases hpd : p.dep = d
      · simp [hpd]
      · rw [show (p.dep == d) = false by simpa using hpd]; rfl
    have hpair : Forall2 (Match (sched s w'.fields)) ((st.podsOf d).map (waitPod d)) fresh := by
      apply forall2_of_all
      · rw [List.length_map, hlen]
        unfold State.podsOf
        exact hcount
      · intro a ha b hb
        obtain ⟨p, hp', rfl⟩ := List.mem_map.mp ha
        obtain ⟨hpst, hpdg⟩ := List.mem_filter.mp hp'
        have hpd : p.dep = d := by simp at hpdg; exact hpdg.1
        have hpg : p.gone = false := by simp at hpdg; exact hpdg.2
        obtain ⟨hbd, hbo, hbr, n, hn, hbn⟩ := hfresh b hb
        rw [waitPod_self hpd hpg]
        refine ⟨hbd.trans hpd.symm, hbo.trans (hs.owner p hpst (hpd ▸ hdne)).symm, fun _ => hbr, ?_⟩
        rw [hbn]
        exact (hnew p hpst hpd hpg).2 n hn
    refine ⟨hfields, ?_, ?_, ?_, howner, hs.nodes, hmu, hnames, hstat⟩
    · obtain ⟨zs, hz, hzf⟩ := Forall2.weave (R := Match (sched s w'.fields)) (fun p : MPod => p.dep == d) (st.waitOne d).live al fresh
        (by rw [hother, hsch]; exact hf) (by rw [hmine]; exact hpair)
      exact ⟨zs, by rw [hpw]; exact hz.trans (hp.append_right fresh), hzf⟩
    · intro e
      rw [hro' e, hrolW e]
      split
      · rfl
      · exact hs.rollIff e
    · intro e r' hr'
      rw [hro' e] at hr'
      split at hr'
      · cases hr'
      · rename_i he
        exact hs.waitRollOther he hr' w'.fields hfw

theorem wait_fold (st : State) (ds : List String) :
    (ds.foldl (init := st) fun st d =>
      let st := st.updDep d fun x => { x with rolling := false, oldNodes := [] }
      st.updPods (fun p => p.dep == d && !p.gone) fun p => { p with ready := true }) = ds.foldl State.waitOne st := rfl

theorem Sim.waitAll {s : Snapshot} {st : State} {w w' : World} {ds : List String} (hs : Sim s st w)
    (hw : WaitAll s w ds w') : Sim s (ds.foldl State.waitOne st) w' := by
  induction hw generalizing st with
  | nil => exact hs
  | cons w w1 w2 d ds h1 _ ih => exact ih (hs.waitOne h1)

theorem Sim.wait {s : Snapshot} {st : State} {w w' : World} {stp : Step} (hs : Sim s st w)
    (hop : stp.op = "wait") (hn : Next s w stp w') : Sim s (applyOp st stp).1 w' := by
  simp only [Next, hop] at hn
  have : (applyOp st stp).1 = stp.deployments.foldl State.waitOne st := by
    simp only [applyOp, hop]; rfl
  rw [this]
  exact hs.waitAll hn

end Planproof
