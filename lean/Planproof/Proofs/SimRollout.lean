import Planproof.Proofs.SimScale

/-!
# Each operation keeps the correspondence (part 5: set-image and rollout-restart)

A rollout starts: in the real world the deployment's pods are summed up by
its rollout (the floor the Deployment controller keeps, where the old pods
are, where new pods may go); in the checker its old pods go and new
pending pods take their place, and the deployment is mid rollout.
-/
namespace Planproof

open Spec

theorem newPods_deps_strip (st : State) (d : String) (k : Int) :
    (st.newPods d k).1.deps.map MDep.strip = st.deps.map MDep.strip := by
  unfold State.newPods
  split
  · rfl
  · simp only [State.updDep, List.map_map]
    apply List.map_congr_left
    intro x _
    simp only [Function.comp]
    split <;> rfl

theorem SameUpTo.surgeFits (st : State) (d : String) (m : MDep) : SameUpTo st (st.surgeFits d m).1 := by
  unfold State.surgeFits
  split
  · exact SameUpTo.refl st
  · exact ⟨newPods_deps_strip st d m.maxSurge, nodes_newPods st d m.maxSurge, rfl⟩

theorem sched_setField_other (s : Snapshot) (f : Fields) (k : Key) (v : Value) (hk : ∀ n, k ≠ .unsched n) :
    sched s (setField f k v) = sched s f := by
  unfold sched
  congr 1
  apply List.filter_congr
  intro y _
  simp only [setField]
  rw [if_neg (fun h => hk y.name h.symm)]

theorem replicasOf_setField_nonrep (f : Fields) (k : Key) (v : Value) (hk : ∀ e, k ≠ .replicas e) (d : String) :
    replicasOf (setField f k v) d = replicasOf f d := by
  unfold replicasOf setField
  rw [if_neg (fun h => hk d h.symm)]

/-- Looking up another deployment through an update of `d`. -/
theorem dep?_updDep_other (st : State) (d e : String) (f : MDep → MDep) (hn : ∀ x, (f x).name = x.name) (h : e ≠ d) :
    (st.updDep d f).dep? e = st.dep? e := by
  rw [State.dep?_updDep st d e f hn]
  cases hx : st.dep? e with
  | none => rfl
  | some x =>
    have : x.name = e := State.name_of_dep? hx
    simp [this, h]

theorem dep?_updDep_self (st : State) (d : String) (f : MDep → MDep) (hn : ∀ x, (f x).name = x.name) {m : MDep}
    (hm : st.dep? d = some m) : (st.updDep d f).dep? d = some (f m) := by
  rw [State.dep?_updDep st d d f hn, hm]
  simp [State.name_of_dep? hm]

/-- Every deployment in the state after `st.updDep d f`, with `f` keeping
name and settings, has a twin in `st`. -/
theorem mem_updDep {st : State} {d : String} {f : MDep → MDep} {x : MDep} (hx : x ∈ (st.updDep d f).deps) :
    ∃ y ∈ st.deps, x = if y.name == d then f y else y := by
  simp only [State.updDep, List.mem_map] at hx
  obtain ⟨y, hy, rfl⟩ := hx
  exact ⟨y, hy, rfl⟩

theorem mem_strip {a b : State} (h : b.deps.map MDep.strip = a.deps.map MDep.strip) {x : MDep} (hx : x ∈ b.deps) :
    ∃ y ∈ a.deps, x.strip = y.strip := by
  have : x.strip ∈ a.deps.map MDep.strip := by rw [← h]; exact List.mem_map_of_mem hx
  obtain ⟨y, hy, he⟩ := List.mem_map.mp this
  exact ⟨y, hy, he.symm⟩

/-- The rollout the real cluster starts for `d` (as in `RolloutNext`). -/
def newRollout (s : Snapshot) (w : World) (d : String) : Rollout where
  dep := d
  floor := match deployment? s d with
    | some sd => if sd.recreate then 0 else max 0 (min (w.avail d) (replicasOf w.fields d - sd.maxUnavail))
    | none => 0
  oldNodes := (w.pods.filter (·.dep == d)).filterMap (·.node)
  newNodes := sched s w.fields

/-- The floor the checker gives a rollout. -/
def floorOf (st : State) (d : String) (m : MDep) : Int :=
  if m.recreate then 0 else max 0 (min (st.avail d) (m.replicas - m.maxUnavail))

theorem Sim.startRollout {s : Snapshot} {st stR : State} {w w' : World} (hu : NodesUnique s) (hs : Sim s st w)
    (d : String) (m : MDep) (hm : st.dep? d = some m) (hroll : st.rolling d = false) (hdne : d ≠ "")
    (P : List MPod) (hlen : P.length = m.replicas.toNat)
    (hP : ∀ p ∈ P, p.dep = d ∧ p.owner = "Deployment" ∧ p.nodes = st.schedulable ∧ p.ready = false ∧ p.gone = false)
    (hother : ∀ e, e ≠ d → (stR.dep? e).map MDep.strip = (st.dep? e).map MDep.strip)
    (mR : MDep) (hmR : stR.dep? d = some mR) (hRroll : mR.rolling = true) (hRfloor : mR.floor = floorOf st d m)
    (hRold : mR.oldNodes = (st.podsOf d).flatMap (·.nodes)) (hRstat : depStat mR = depStat m)
    (hpods : stR.pods = st.pods.filter (fun p => !(p.dep == d && !p.gone)) ++ P)
    (hnodes : stR.nodes = st.nodes)
    (hmems : ∀ x ∈ stR.deps, ∃ y ∈ st.deps, x.name = y.name ∧ x.maxUnavail = y.maxUnavail)
    (hfield : stR.field = w'.fields)
    (hsch : sched s w'.fields = sched s w.fields) (hrep : ∀ e, replicasOf w'.fields e = replicasOf w.fields e)
    (hwp : w'.pods = w.pods.filter (·.dep != d))
    (hwr : w'.rollouts = w.rollouts ++ [newRollout s w d]) :
    Sim s stR w' := by
  have hrep_d : replicasOf w.fields d = m.replicas := by simp [replicasOf, hs.fields, State.field, hm]
  have hro : w.rollout? d = none := by
    have := hs.rollIff d; rw [hroll] at this
    cases h : w.rollout? d with
    | none => rfl
    | some _ => rw [h] at this; cases this
  -- Looking up rollouts after the step.
  have hro' : ∀ e, w'.rollout? e = if e = d then some (newRollout s w d) else w.rollout? e := by
    intro e
    simp only [World.rollout?, hwr, List.find?_append]
    split
    · rename_i hed; subst e
      have : w.rollouts.find? (·.dep == d) = none := hro
      rw [this]; simp [newRollout]
    · rename_i hed
      cases h : w.rollouts.find? (·.dep == e) with
      | some _ => simp
      | none => simp [newRollout, Ne.symm hed]
  -- Which deployments are mid rollout after the step.
  have hrolR : ∀ e, stR.rolling e = if e = d then true else st.rolling e := by
    intro e
    split
    · rename_i hed; subst e; simp [State.rolling, hmR, hRroll]
    · rename_i hed
      have h := hother e hed
      unfold State.rolling
      cases h1 : stR.dep? e <;> cases h2 : st.dep? e <;> rw [h1, h2] at h <;> simp at h
      have := congrArg (fun x : MDep => x.rolling) h
      simp only [MDep.strip] at this
      simp [this]
  have hal := hs.pods
  obtain ⟨al, hp, hf⟩ := hal
  refine ⟨hfield.symm, ?_, ?_, ?_, ?_, ?_, ?_, ?_, ?_⟩
  · -- The pairing: everything but the deployment, unchanged.
    refine ⟨al.filter (·.dep != d), by rw [hwp]; exact hp.filter _, ?_⟩
    have hlive : stR.live = st.live.filter (fun p => p.dep != d) := by
      unfold State.live
      rw [hpods, List.filter_append, List.filter_filter, List.filter_filter]
      have hPnil : P.filter (fun p => !p.gone && !stR.rolling p.dep) = [] := by
        rw [List.filter_eq_nil_iff]
        intro p hp'
        obtain ⟨hd, -, -, -, -⟩ := hP p hp'
        simp [hrolR, hd]
      rw [hPnil, List.append_nil]
      apply List.filter_congr
      intro p _
      have hb : (p.dep != d) = !(p.dep == d) := rfl
      rw [hrolR]
      by_cases hd : p.dep = d
      · simp [hd]
      · have h0 : (p.dep == d) = false := by simp [hd]
        simp only [hd, if_false, hb, h0]
        cases p.gone <;> cases st.rolling p.dep <;> rfl
    rw [hlive, hsch]
    exact hf.filter _ _ (fun a b hm => by rw [hm.1])
  · intro e
    rw [hro' e, hrolR e]
    split
    · rfl
    · exact hs.rollIff e
  · intro e r hr
    rw [hro' e] at hr
    split at hr
    · -- The new rollout.
      rename_i hed; subst e
      simp only [Option.some.injEq] at hr
      subst hr
      refine ⟨mR, hmR, ?_, ?_, ?_, ?_, ?_⟩
      · -- The real controller keeps at least the checker's floor.
        rw [hRfloor]
        have hst := hs.stat d
        rw [hm] at hst
        cases hsd : deployment? s d with
        | none => rw [hsd] at hst; cases hst
        | some sd =>
          rw [hsd] at hst
          simp only [Option.map_some, Option.some.injEq, Prod.mk.injEq] at hst
          simp only [floorOf, newRollout, hsd, ← hst.1, ← hst.2, hrep_d]
          have hav := hs.avail_ge d
          split
          · exact Int.le_refl _
          · omega
      · intro n hn
        rw [hRold]
        simp only [newRollout, List.mem_filterMap, List.mem_filter] at hn
        obtain ⟨b, ⟨hbw, hbd⟩, hbn⟩ := hn
        obtain ⟨a, hal', hma⟩ := forall2_exists_of_mem_right hf (hp.mem_iff.mpr hbw)
        have had : a.dep = d := hma.1.symm.trans (by simpa using hbd)
        have hpl := List.mem_filter.mp hal'
        have hag : a.gone = false := by
          have := hpl.2; simp only [Bool.and_eq_true, Bool.not_eq_true'] at this; exact this.1
        have h4 := hma.2.2.2
        rw [hbn] at h4
        rw [List.mem_flatMap]
        exact ⟨a, List.mem_filter.mpr ⟨hpl.1, by simp [had, hag]⟩, h4⟩
      · intro p hp' hpd hpg
        rw [hpods] at hp'
        rcases List.mem_append.mp hp' with hp' | hp'
        · have := (List.mem_filter.mp hp').2
          simp [hpd, hpg] at this
        · obtain ⟨-, -, hn, hr, -⟩ := hP p hp'
          refine ⟨hr, fun n hn' => ?_⟩
          rw [hn, mem_schedulable hu hs.nodes, ← hs.fields]; exact hn'
      · rw [hpods, List.filter_append, List.length_append, hrep, hrep_d]
        have h1 : (st.pods.filter (fun p => !(p.dep == d && !p.gone))).filter (fun p => p.dep == d && !p.gone) = [] := by
          rw [List.filter_filter, List.filter_eq_nil_iff]
          intro p _; simp
        have h2 : P.filter (fun p => p.dep == d && !p.gone) = P := by
          apply List.filter_eq_self.mpr
          intro p hp'
          obtain ⟨hd, -, -, -, hg⟩ := hP p hp'
          simp [hd, hg]
        rw [h1, h2, hlen]; simp
      · rw [hRfloor, hrep, hrep_d]
        have := hs.mu m (List.mem_of_find?_eq_some hm)
        simp only [floorOf]
        split
        · omega
        · omega
    · -- Another deployment's rollout: unchanged.
      rename_i hed
      obtain ⟨m0, hm0, h1, h2, h3, h4, h5⟩ := hs.roll e r hr
      have h := hother e hed
      rw [hm0] at h
      cases h1' : stR.dep? e with
      | none => rw [h1'] at h; cases h
      | some m1 =>
        rw [h1'] at h
        have hstr : m1.strip = m0.strip := Option.some.inj h
        refine ⟨m1, rfl, ?_, ?_, ?_, ?_, ?_⟩
        · have e1 := congrArg (fun x : MDep => x.floor) hstr; simp only [MDep.strip] at e1; rw [e1]; exact h1
        · have e1 := congrArg (fun x : MDep => x.oldNodes) hstr; simp only [MDep.strip] at e1; rw [e1]; exact h2
        · intro p hp' hpd hpg
          rw [hpods] at hp'
          rcases List.mem_append.mp hp' with hp' | hp'
          · exact h3 p (List.mem_filter.mp hp').1 hpd hpg
          · exact absurd (hpd.symm.trans (hP p hp').1) hed
        · rw [hpods, List.filter_append, List.length_append, hrep, ← h4, List.filter_filter]
          have h2' : P.filter (fun p => p.dep == e && !p.gone) = [] := by
            rw [List.filter_eq_nil_iff]
            intro p hp'
            simp [(hP p hp').1, Ne.symm hed]
          rw [h2', List.length_nil, Nat.add_zero]
          congr 1
          apply List.filter_congr
          intro p _
          by_cases hpe : p.dep = e
          · simp [hpe, hed]
          · simp [hpe]
        · have e1 := congrArg (fun x : MDep => x.floor) hstr; simp only [MDep.strip] at e1; rw [e1, hrep]; exact h5
  · intro p hp' hd
    rw [hpods] at hp'
    rcases List.mem_append.mp hp' with hp' | hp'
    · exact hs.owner p (List.mem_filter.mp hp').1 hd
    · exact (hP p hp').2.1
  · rw [hnodes]; exact hs.nodes
  · intro x hx
    obtain ⟨y, hy, hn, hmu⟩ := hmems x hx
    rw [hmu]; exact hs.mu y hy
  · intro x hx
    obtain ⟨y, hy, hn, -⟩ := hmems x hx
    rw [hn]; exact hs.names y hy
  · intro e
    rw [← hs.stat e]
    by_cases hed : e = d
    · subst hed; rw [hmR, hm]; simp only [Option.map_some]; exact congrArg some hRstat
    · have h := hother e hed
      have := congrArg (Option.map fun x : MDep => (x.recreate, x.maxUnavail)) h
      simp only [Option.map_map, Function.comp_def, MDep.strip] at this
      exact this

theorem rolloutUpdate_parts (stp : Step) (old : List String) (x : MDep) :
    (rolloutUpdate stp old x).name = x.name ∧ (rolloutUpdate stp old x).maxUnavail = x.maxUnavail ∧
    (rolloutUpdate stp old x).rolling = true ∧ (rolloutUpdate stp old x).floor = x.floor ∧
    (rolloutUpdate stp old x).oldNodes = old ∧ depStat (rolloutUpdate stp old x) = depStat x := by
  unfold rolloutUpdate; split <;> exact ⟨rfl, rfl, rfl, rfl, rfl, rfl⟩

theorem ite_parts (c : Bool) (f : MDep → MDep) (y : MDep)
    (hf : ∀ x, (f x).name = x.name ∧ (f x).maxUnavail = x.maxUnavail) :
    (if c = true then f y else y).name = y.name ∧ (if c = true then f y else y).maxUnavail = y.maxUnavail := by
  split
  · exact hf y
  · exact ⟨rfl, rfl⟩

theorem surgeFits_pods (st : State) (d : String) (m : MDep) : (st.surgeFits d m).1.pods = st.pods := by
  unfold State.surgeFits; split <;> rfl

theorem Sim.rollout {s : Snapshot} {st : State} {w w' : World} {stp : Step} (hu : NodesUnique s)
    (hs : Sim s st w) (hop : stp.op = "set-image" ∨ stp.op = "rollout-restart") (hex : (st.dep? stp.deployment).isSome)
    (htr : trigger st stp = none) (hpl : (applyOp st stp).2 = true) (hn : Next s w stp w') :
    Sim s (applyOp st stp).1 w' := by
  obtain ⟨m, hm⟩ := Option.isSome_iff_exists.mp hex
  have hdt := trigger_dep_none (Or.inr hop) htr
  have hroll := Sim.rolling_false hm hdt
  have hdne : stp.deployment ≠ "" := by
    rw [← State.name_of_dep? hm]; exact hs.names m (List.mem_of_find?_eq_some hm)
  have hfin := State.field_applyOp st stp hpl
  -- The real cluster.
  have hn' : w'.fields = Spec.step w.fields stp ∧ RolloutNext s w stp.deployment w' := by
    rcases hop with h | h <;> simp only [Next, h] at hn <;> exact hn
  obtain ⟨hfw, hrn⟩ := hn'
  have hro : w.rollout? stp.deployment = none := by
    have := hs.rollIff stp.deployment; rw [hroll] at this
    cases h : w.rollout? stp.deployment with
    | none => rfl
    | some _ => rw [h] at this; cases this
  unfold RolloutNext at hrn
  rw [hro] at hrn
  obtain ⟨hwp, hwr⟩ := hrn
  have hsch : sched s w'.fields = sched s w.fields := by
    rw [hfw]; rcases hop with h | h <;> simp only [Spec.step, h] <;> exact sched_setField_other _ _ _ _ (fun n h => by cases h)
  have hrep : ∀ e, replicasOf w'.fields e = replicasOf w.fields e := by
    intro e
    rw [hfw]; rcases hop with h | h <;> simp only [Spec.step, h] <;> exact replicasOf_setField_nonrep _ _ _ (fun n h => by cases h) e
  -- The checker.
  let d := stp.deployment
  let fA : MDep → MDep := fun x => { x with floor := floorOf st d m }
  let sA := st.updDep d fA
  have happ : applyOp st stp = if (sA.surgeFits d m).2 then (sA.surgeFits d m).1.rollout stp m
      else ((sA.surgeFits d m).1, false) := by
    rcases hop with h | h <;> simp only [applyOp, h, hm] <;> rfl
  rw [happ] at hpl hfin ⊢
  cases hok : (sA.surgeFits d m).2 with
  | false => rw [hok] at hpl; simp at hpl
  | true =>
  simp only [hok, ↓reduceIte] at hfin ⊢
  let st2 := (sA.surgeFits d m).1
  have hA : SameUpTo sA st2 := SameUpTo.surgeFits sA d m
  have hpods2 : st2.pods = st.pods := surgeFits_pods sA d m
  have hnodes2 : st2.nodes = st.nodes := hA.nodes
  have hfAn : ∀ x, (fA x).name = x.name := fun _ => rfl
  -- The deployment in st2.
  have hm2 : ∃ m2, st2.dep? d = some m2 ∧ m2.strip = (fA m).strip := by
    have h := dep?_strip hA.deps d
    rw [dep?_updDep_self st d fA hfAn hm] at h
    cases h2 : st2.dep? d with
    | none => rw [h2] at h; cases h
    | some m2 => rw [h2] at h; exact ⟨m2, rfl, Option.some.inj h⟩
  obtain ⟨m2, hm2, hm2s⟩ := hm2
  let old := (st2.podsOf d).flatMap (·.nodes)
  let sC : State := { st2 with pods := st2.pods.filter fun p => !(p.dep == d && !p.gone) }
  let sD := sC.updDep d (rolloutUpdate stp old)
  have hrn : ∀ x, (rolloutUpdate stp old x).name = x.name := fun x => (rolloutUpdate_parts stp old x).1
  have hmD : sD.dep? d = some (rolloutUpdate stp old m2) := dep?_updDep_self sC d _ hrn hm2
  obtain ⟨P, hsame, hlen, hP⟩ := newPods_spec sD d m.replicas _ hmD
  have hschD : sD.schedulable = st.schedulable := by
    show State.schedulable { sC with deps := _ } = _
    unfold State.schedulable; rw [show sC.nodes = st.nodes from hnodes2]
  have hfinal : (st2.rollout stp m).1 = ((sD.newPods d m.replicas).1.place (sD.newPods d m.replicas).2).1 := rfl
  rw [hfinal] at hfin ⊢
  have hplace := SameUpTo.place (sD.newPods d m.replicas).1 (sD.newPods d m.replicas).2
  refine (Sim.startRollout (stR := { sD with pods := sD.pods ++ P }) hu hs d m hm hroll hdne P hlen ?_ ?_ (rolloutUpdate stp old m2) hmD ?_ ?_ ?_ ?_ ?_ hnodes2 ?_ ?_
    hsch hrep hwp hwr).congr hsame |>.congr hplace
  · intro p hp; rw [← hschD]; exact hP p hp
  · intro e he
    show (sD.dep? e).map MDep.strip = _
    rw [dep?_updDep_other sC d e _ hrn he]
    show (st2.dep? e).map MDep.strip = _
    rw [dep?_strip hA.deps e, dep?_updDep_other st d e fA hfAn he]
  · exact (rolloutUpdate_parts stp old m2).2.2.1
  · rw [(rolloutUpdate_parts stp old m2).2.2.2.1]
    have := congrArg (fun x : MDep => x.floor) hm2s
    simpa [MDep.strip] using this
  · rw [(rolloutUpdate_parts stp old m2).2.2.2.2.1]
    show (st2.pods.filter _).flatMap _ = (st.pods.filter _).flatMap _
    rw [hpods2]
  · rw [(rolloutUpdate_parts stp old m2).2.2.2.2.2]
    have := congrArg (fun x : MDep => depStat x) hm2s
    simpa [MDep.strip, depStat] using this
  · show sC.pods ++ P = _
    show st2.pods.filter _ ++ P = _
    rw [hpods2]
  · intro x hx
    obtain ⟨y, hy, rfl⟩ := mem_updDep hx
    obtain ⟨z, hz, hyz⟩ := mem_strip hA.deps hy
    obtain ⟨u, hu', rfl⟩ := mem_updDep hz
    have e1 := ite_parts (y.name == d) (rolloutUpdate stp old) y (fun x => ⟨(rolloutUpdate_parts stp old x).1, (rolloutUpdate_parts stp old x).2.1⟩)
    have e2 := ite_parts (u.name == d) fA u (fun _ => ⟨rfl, rfl⟩)
    have e3n := congrArg (fun x : MDep => x.name) hyz
    have e3m := congrArg (fun x : MDep => x.maxUnavail) hyz
    simp only [MDep.strip] at e3n e3m
    exact ⟨u, hu', by rw [e1.1, e3n, e2.1], by rw [e1.2, e3m, e2.2]⟩
  · rw [← hsame.field, ← hplace.field, hfin, hfw, hs.fields]

end Planproof
