import Planproof.Proofs.SimWait

/-!
# Rules `budget` and `never-zero` are sound (PRD L-33)

Every step keeps the correspondence between the checker's state and the
real world (parts 1–6); the checker's availability is never above the real
one (`Sim.avail_ge`); so when every state the checker passes through passes
`budget` and `never-zero`, every world the plan can really pass through
does too.
-/
namespace Planproof

open Spec

/-! ## Rollout floors are never negative -/

def FloorsOK (st : State) : Prop := ∀ m ∈ st.deps, 0 ≤ m.floor

theorem FloorsOK.strip {a b : State} (h : b.deps.map MDep.strip = a.deps.map MDep.strip) (ha : FloorsOK a) :
    FloorsOK b := by
  intro x hx
  obtain ⟨y, hy, he⟩ := mem_strip h hx
  have := congrArg (fun x : MDep => x.floor) he
  simp only [MDep.strip] at this
  rw [this]; exact ha y hy

theorem FloorsOK.updDep {st : State} {d : String} {f : MDep → MDep} (hf : ∀ x, 0 ≤ x.floor → 0 ≤ (f x).floor)
    (h : FloorsOK st) : FloorsOK (st.updDep d f) := by
  intro x hx
  obtain ⟨y, hy, rfl⟩ := mem_updDep hx
  split
  · exact hf y (h y hy)
  · exact h y hy

theorem FloorsOK.place {st : State} (names : List String) (h : FloorsOK st) : FloorsOK (st.place names).1 := by
  intro x hx; rw [State.deps_place] at hx; exact h x hx

theorem FloorsOK.newPods {st : State} (d : String) (k : Int) (h : FloorsOK st) : FloorsOK (st.newPods d k).1 :=
  FloorsOK.strip (newPods_deps_strip st d k) h

theorem floors_applyOp (st : State) (s : Step) (h : FloorsOK st) : FloorsOK (applyOp st s).1 := by
  by_cases h1 : s.op = "cordon"
  · simp only [applyOp, h1]; exact h
  by_cases h2 : s.op = "uncordon"
  · simp only [applyOp, h2]; split <;> exact h
  by_cases h3 : s.op = "drain"
  · simp only [applyOp, h3]; exact FloorsOK.place _ h
  by_cases h4 : s.op = "scale"
  · simp only [applyOp, h4]
    split
    · exact h
    · have h' := FloorsOK.updDep (d := s.deployment) (f := fun x => { x with replicas := s.replicas.getD 0 }) (fun _ hx => hx) h
      split
      · exact FloorsOK.place _ (FloorsOK.newPods _ _ h')
      · split
        · exact h'
        · exact h'
  have hroll : ∀ (m : MDep), FloorsOK (let floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail))
      let (st, ok) := (st.updDep s.deployment fun d => { d with floor }).surgeFits s.deployment m
      if ok then st.rollout s m else (st, false)).1 := by
    intro m
    dsimp only
    have hA := FloorsOK.updDep (d := s.deployment)
      (f := fun d => { d with floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail)) })
      (fun _ _ => by dsimp only; split <;> omega) h
    have h2 := FloorsOK.strip (SameUpTo.surgeFits _ s.deployment m).deps hA
    generalize State.surgeFits _ _ _ = sf at h2 ⊢
    obtain ⟨st2, ok⟩ := sf
    cases ok
    · exact h2
    · simp only [ite_true, State.rollout]
      refine FloorsOK.place _ (FloorsOK.newPods _ _ (FloorsOK.updDep (fun x hx => ?_) h2))
      rw [(rolloutUpdate_parts s _ x).2.2.2.1]; exact hx
  by_cases h5 : s.op = "set-image"
  · simp only [applyOp, h5]
    split
    · exact h
    · exact hroll _
  by_cases h6 : s.op = "rollout-restart"
  · simp only [applyOp, h6]
    split
    · exact h
    · exact hroll _
  by_cases h7 : s.op = "wait"
  · simp only [applyOp, h7]
    rw [wait_fold]
    clear hroll
    generalize s.deployments = ds
    induction ds generalizing st with
    | nil => exact h
    | cons d rest ih => exact ih (st.waitOne d) (FloorsOK.updDep (f := waitDep) (d := d) (fun _ hx => hx) h)
  · simp only [applyOp, h1, h2, h3, h4, h5, h6, h7]; exact h

theorem floors_runOps (st : State) (steps : List Step) (h : FloorsOK st) : FloorsOK (runOps st steps) := by
  induction steps generalizing st with
  | nil => exact h
  | cons s rest ih => exact ih _ (floors_applyOp st s h)

theorem floors_build (snap : Snapshot) : FloorsOK (build snap) := by
  intro m hm; rw [((build_parts snap).2 m hm).2.2]; exact Int.le_refl 0

/-! ## Budgets never change -/

theorem State.pdbs_place (st : State) (names : List String) : (st.place names).1.pdbs = st.pdbs := by
  obtain ⟨ps, h, -⟩ := st.place_shape names; rw [h]

theorem State.pdbs_newPods (st : State) (d : String) (k : Int) : (st.newPods d k).1.pdbs = st.pdbs := by
  unfold State.newPods; split <;> rfl

theorem State.pdbs_surgeFits (st : State) (d : String) (m : MDep) : (st.surgeFits d m).1.pdbs = st.pdbs := by
  unfold State.surgeFits; split
  · rfl
  · exact State.pdbs_newPods st d m.maxSurge

theorem pdbs_applyOp (st : State) (s : Step) : (applyOp st s).1.pdbs = st.pdbs := by
  by_cases h1 : s.op = "cordon"
  · simp only [applyOp, h1]; rfl
  by_cases h2 : s.op = "uncordon"
  · simp only [applyOp, h2]; split <;> rfl
  by_cases h3 : s.op = "drain"
  · simp only [applyOp, h3]; rw [State.pdbs_place]; rfl
  by_cases h4 : s.op = "scale"
  · simp only [applyOp, h4]
    split
    · rfl
    · split
      · rw [State.pdbs_place, State.pdbs_newPods]; rfl
      · split <;> rfl
  have hroll : ∀ (m : MDep), (let floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail))
      let (st, ok) := (st.updDep s.deployment fun d => { d with floor }).surgeFits s.deployment m
      if ok then st.rollout s m else (st, false)).1.pdbs = st.pdbs := by
    intro m
    dsimp only
    have h2 : (State.surgeFits (st.updDep s.deployment fun d => { d with floor := if m.recreate then 0 else max 0 (min (st.avail s.deployment) (m.replicas - m.maxUnavail)) }) s.deployment m).1.pdbs = st.pdbs :=
      State.pdbs_surgeFits _ s.deployment m
    generalize State.surgeFits _ _ _ = sf at h2 ⊢
    obtain ⟨st2, ok⟩ := sf
    cases ok
    · exact h2
    · simp only [ite_true, State.rollout]
      rw [State.pdbs_place, State.pdbs_newPods]; exact h2
  by_cases h5 : s.op = "set-image"
  · simp only [applyOp, h5]
    split
    · rfl
    · exact hroll _
  by_cases h6 : s.op = "rollout-restart"
  · simp only [applyOp, h6]
    split
    · rfl
    · exact hroll _
  by_cases h7 : s.op = "wait"
  · simp only [applyOp, h7]
    rw [wait_fold]
    clear hroll
    generalize s.deployments = ds
    induction ds generalizing st with
    | nil => rfl
    | cons d rest ih => exact (ih (st.waitOne d)).trans rfl
  · simp only [applyOp, h1, h2, h3, h4, h5, h6, h7]

theorem pdbs_runOps (st : State) (steps : List Step) : (runOps st steps).pdbs = st.pdbs := by
  induction steps generalizing st with
  | nil => rfl
  | cons s rest ih => exact (ih _).trans (pdbs_applyOp st s)

/-! ## The snapshot's budgets, in the checker's state -/

/-- One budget, added by `build`. -/
def pdbStep (st : State) (b : SPdb) : State :=
  match b.deployment with
  | none =>
    { st with deps := st.deps.map fun d =>
        if (d.name.splitOn "/").head? == some b.ns then { d with unresolved := true } else d }
  | some dn =>
    let key := b.ns ++ "/" ++ dn
    if (st.dep? key).isSome then
      let st := st.updDep key fun d => { d with pdbs := d.pdbs + 1, unresolved := d.unresolved || decide (d.pdbs + 1 > 1) }
      { st with pdbs := st.pdbs ++ [{ name := b.ns ++ "/" ++ b.name, dep := key, min := b.min, max := b.max }] }
    else st

def buildBase (s : Snapshot) : State :=
  let nodes := s.nodes.map fun n =>
    { name := n.name, ready := n.ready, unsched := n.unsched, taints := n.taints, drained := false, alloc := n.alloc : MNode }
  let deps := s.deployments.map fun d =>
    { name := d.ns ++ "/" ++ d.name, replicas := d.replicas, images := d.containers, restarted := false,
      rolling := false, oldNodes := [], floor := 0, maxUnavail := d.maxUnavail, maxSurge := d.maxSurge, recreate := d.recreate,
      req := d.req, placement := d.placement, hpa := d.hpa,
      unsettled := decide (d.ready < d.replicas) || decide (d.updated < d.replicas),
      pdbs := 0, unresolved := false, made := 0 : MDep }
  let pods := s.pods.map fun p =>
    { name := p.ns ++ "/" ++ p.name, dep := if p.ownerKind == "Deployment" then p.ns ++ "/" ++ p.ownerName else "",
      owner := p.ownerKind, nodes := [p.node], atNode := p.node, req := p.req, ready := p.ready, created := false,
      gone := false, localStorage := p.localStorage, placement := p.placement : MPod }
  { deps, nodes, pods, pdbs := [] }

theorem build_eq (s : Snapshot) : build s =
    { (s.pdbs.foldl pdbStep (buildBase s)) with
      pdbs := (s.pdbs.foldl pdbStep (buildBase s)).pdbs.mergeSort fun a b => strLe a.dep b.dep && (a.dep != b.dep || strLe a.name b.name) } :=
  rfl

theorem isSome_updDep (st : State) (d e : String) (f : MDep → MDep) (hn : ∀ x, (f x).name = x.name) :
    ((st.updDep d f).dep? e).isSome = (st.dep? e).isSome := by
  rw [State.dep?_updDep st d e f hn]; simp

theorem pdbStep_isSome (st : State) (b : SPdb) (d : String) : ((pdbStep st b).dep? d).isSome = (st.dep? d).isSome := by
  unfold pdbStep
  split
  · show ((({ st with deps := st.deps.map _ } : State)).deps.find? _).isSome = _
    simp only
    rw [find?_map_same _ _ _ (fun x => by split <;> rfl)]
    simp [State.dep?]
  · dsimp only
    split
    · exact isSome_updDep _ _ _ _ (fun _ => rfl)
    · rfl

theorem pdbStep_mem (st : State) (b : SPdb) {mb : MPdb} (h : mb ∈ st.pdbs) : mb ∈ (pdbStep st b).pdbs := by
  unfold pdbStep
  split
  · exact h
  · dsimp only
    split
    · exact List.mem_append_left _ h
    · exact h

theorem buildBase_isSome (s : Snapshot) (d : String) : ((buildBase s).dep? d).isSome = (deployment? s d).isSome := by
  simp only [buildBase, State.dep?, deployment?, List.find?_map, Option.isSome_map]
  rfl

theorem build_pdbs (s : Snapshot) (b : SPdb) (hb : b ∈ s.pdbs) (dn : String) (hd : b.deployment = some dn)
    (hex : (deployment? s (b.ns ++ "/" ++ dn)).isSome) :
    ∃ mb ∈ (build s).pdbs, mb.dep = b.ns ++ "/" ++ dn ∧ mb.min = b.min ∧ mb.max = b.max := by
  rw [build_eq]
  simp only [List.mem_mergeSort]
  suffices h : ∀ (L : List SPdb) (st : State), (∀ d, (st.dep? d).isSome = (deployment? s d).isSome) →
      (∀ b ∈ L, ∀ dn, b.deployment = some dn → (deployment? s (b.ns ++ "/" ++ dn)).isSome →
        ∃ mb ∈ (L.foldl pdbStep st).pdbs, mb.dep = b.ns ++ "/" ++ dn ∧ mb.min = b.min ∧ mb.max = b.max) ∧
      (∀ mb ∈ st.pdbs, mb ∈ (L.foldl pdbStep st).pdbs) from
    (h s.pdbs (buildBase s) (buildBase_isSome s)).1 b hb dn hd hex
  intro L
  induction L with
  | nil => intro st _; exact ⟨fun _ h => (by cases h), fun _ h => h⟩
  | cons b0 rest ih =>
    intro st hst
    have hst' : ∀ d, ((pdbStep st b0).dep? d).isSome = (deployment? s d).isSome := fun d => by
      rw [pdbStep_isSome]; exact hst d
    obtain ⟨ih1, ih2⟩ := ih (pdbStep st b0) hst'
    refine ⟨?_, fun mb hmb => ih2 mb (pdbStep_mem st b0 hmb)⟩
    intro b' hb' dn' hd' hex'
    rcases List.mem_cons.mp hb' with heq | hb'
    · subst heq
      refine ⟨{ name := b'.ns ++ "/" ++ b'.name, dep := b'.ns ++ "/" ++ dn', min := b'.min, max := b'.max }, ih2 _ ?_, rfl, rfl, rfl⟩
      unfold pdbStep
      rw [hd']
      dsimp only
      rw [if_pos (by rw [hst]; exact hex')]
      exact List.mem_append_right _ (List.mem_singleton_self _)
    · exact ih1 b' hb' dn' hd' hex'

/-! ## Availability in the snapshot -/

theorem build_avail (s : Snapshot) (d : String) : (build s).avail d = (World.initial s).avail d := by
  rw [State.avail_eq _ _ (build_rolling s d), (build_parts s).1]
  simp only [World.avail, World.rollout?, World.initial, List.find?_nil]
  rw [List.filter_map, List.filter_map, List.length_map, List.length_map]
  congr 2
  apply List.filter_congr
  intro p _
  simp [mkPod]

theorem lookup_pairs (l : List String) (f : String → Int) {d : String} (h : d ∈ l) :
    lookup (l.map fun x => (x, f x)) d = some (f d) := by
  induction l with
  | nil => cases h
  | cons a l ih =>
    unfold lookup at ih ⊢
    simp only [List.map_cons, List.find?_cons]
    by_cases ha : a = d
    · subst ha; simp
    · have : (a == d) = false := by simpa using ha
      simp only [this]
      exact ih (by rcases List.mem_cons.mp h with h | h; exact absurd h.symm ha; exact h)

theorem mem_depNames (s : Snapshot) (plan : Plan) (pol : Policy) {d : String} (h : ((build s).dep? d).isSome) :
    d ∈ (Ctx.make s plan pol).depNames := by
  obtain ⟨m, hm⟩ := Option.isSome_iff_exists.mp h
  show d ∈ sortStrings _
  rw [mem_sortStrings]
  exact List.mem_map.mpr ⟨m, List.mem_of_find?_eq_some hm, State.name_of_dep? hm⟩

theorem initialOf_eq (s : Snapshot) (plan : Plan) (pol : Policy) {d : String} (h : ((build s).dep? d).isSome) :
    (Ctx.make s plan pol).initialOf d = (World.initial s).avail d := by
  unfold Ctx.initialOf
  rw [← build_avail]
  show (lookup ((Ctx.make s plan pol).depNames.map fun x => (x, (build s).avail x)) d).getD 0 = _
  rw [lookup_pairs _ _ (mem_depNames s plan pol h)]
  rfl

theorem build_isSome (s : Snapshot) (d : String) : ((build s).dep? d).isSome = (deployment? s d).isSome := by
  have := congrArg Option.isSome (build_stat s d)
  simpa using this

theorem Sim.isSome {s : Snapshot} {st : State} {w : World} (hs : Sim s st w) (d : String) :
    (st.dep? d).isSome = (deployment? s d).isSome := by
  have := congrArg Option.isSome (hs.stat d)
  simpa using this

theorem Sim.need_eq {s : Snapshot} {st : State} {w : World} (hs : Sim s st w) (mb : MPdb) (b : SPdb)
    (hmin : mb.min = b.min) (hmax : mb.max = b.max) : st.need mb = Spec.need w b mb.dep := by
  unfold State.need Spec.need
  rw [hmin, hmax]
  cases b.min with
  | some _ => rfl
  | none =>
    simp only
    congr 2
    unfold replicasOf
    rw [hs.fields]
    simp only [State.field]
    cases st.dep? mb.dep <;> rfl

theorem avail_nonneg {st : State} (hfl : FloorsOK st) (d : String) : 0 ≤ st.avail d := by
  unfold State.avail
  cases hm : st.dep? d with
  | none => simp
  | some m =>
    simp only
    split
    · exact hfl m (List.mem_of_find?_eq_some hm)
    · simp

/-- **One state.** A world that corresponds to a state the checker passed
(or to the snapshot's state) keeps every budget and never reaches zero. -/
theorem safe_state {s : Snapshot} {plan : Plan} {pol : Policy} {st : State} {w : World}
    (hs : Sim s st w) (hfl : FloorsOK st) (hpd : st.pdbs = (build s).pdbs)
    (he : eachStateFails (Ctx.make s plan pol) st = [] ∨ st = build s) : BudgetOK s w ∧ NeverZero s plan pol w := by
  rcases he with he | he
  · unfold eachStateFails at he
    generalize st.nodeCounts = nc at he
    obtain ⟨u, v⟩ := nc
    dsimp only at he
    simp only [List.append_eq_nil_iff, List.map_eq_nil_iff, List.filter_eq_nil_iff] at he
    have hb := he.1.1.1
    have hz := he.1.1.2
    constructor
    · intro b hbm dn hd hex
      obtain ⟨mb, hmb, hdep, hmin, hmax⟩ := build_pdbs s b hbm dn hd hex
      rw [← hpd] at hmb
      have h1 := hb mb hmb
      have hn := hs.need_eq mb b hmin hmax
      have hi := initialOf_eq s plan pol (d := b.ns ++ "/" ++ dn) (by rw [build_isSome]; exact hex)
      have hav := hs.avail_ge (b.ns ++ "/" ++ dn)
      rw [hdep] at h1 hn
      rw [hn, hi] at h1
      simp only [Bool.and_eq_true, decide_eq_true_eq, not_and, Int.not_lt] at h1
      by_cases hlt : st.avail (b.ns ++ "/" ++ dn) < need w b (b.ns ++ "/" ++ dn)
      · right; have := h1 hlt; omega
      · left; omega
    · intro sd hsd hinit hzero
      have hex : ((build s).dep? (depName sd)).isSome := by
        rw [build_isSome]
        unfold deployment?
        exact List.find?_isSome.mpr ⟨sd, hsd, by simp⟩
      have h1 := hz (depName sd) (mem_depNames s plan pol hex)
      rw [initialOf_eq s plan pol hex] at h1
      have hza : (Ctx.make s plan pol).zeroAllowed (depName sd) = zeroAllowed plan pol (depName sd) := rfl
      rw [hza, hzero] at h1
      have h0 := avail_nonneg hfl (depName sd)
      have hav := hs.avail_ge (depName sd)
      simp only [Bool.not_false, Bool.and_true, Bool.and_eq_true, decide_eq_true_eq, beq_iff_eq, not_and] at h1
      have := h1 hinit
      omega
  · subst he
    constructor
    · intro b _ dn _ _
      right
      rw [← build_avail]; exact hs.avail_ge _
    · intro sd _ hinit _
      have := hs.avail_ge (depName sd)
      rw [build_avail] at this
      omega

/-! ## Every step -/

theorem Sim.step {s : Snapshot} {c : Ctx} {st : State} {w w' : World} {stp : Step} (hu : NodesUnique s)
    (hs : Sim s st w) (hok : StepOK c st stp)
    (hex : (stp.op = "scale" ∨ stp.op = "set-image" ∨ stp.op = "rollout-restart") → (st.dep? stp.deployment).isSome)
    (hn : Next s w stp w') : Sim s (applyOp st stp).1 w' := by
  by_cases h1 : stp.op = "cordon"
  · exact hs.cordon h1 hn
  by_cases h2 : stp.op = "uncordon"
  · exact hs.uncordon hu h2 hn
  by_cases h3 : stp.op = "drain"
  · exact hs.drain hu h3 hok.noTrigger (hok.drainOK h3) hn
  by_cases h4 : stp.op = "scale"
  · exact hs.scale hu h4 (hex (Or.inl h4)) hok.noTrigger hn
  by_cases h5 : stp.op = "set-image"
  · exact hs.rollout hu (Or.inl h5) (hex (Or.inr (Or.inl h5))) hok.noTrigger hok.placed hn
  by_cases h6 : stp.op = "rollout-restart"
  · exact hs.rollout hu (Or.inr h6) (hex (Or.inr (Or.inr h6))) hok.noTrigger hok.placed hn
  by_cases h7 : stp.op = "wait"
  · exact hs.wait h7 hn
  · simp only [Next, h1, h2, h3, h4, h5, h6, h7] at hn
    subst hn
    simp only [applyOp, h1, h2, h3, h4, h5, h6, h7]
    exact hs

/-- `scope` passed: every deployment a step names exists. -/
theorem scope_exists {c : Ctx} (h : scopeFails c = []) {stp : Step} (hst : stp ∈ c.plan.steps ++ c.plan.rollback)
    (hd : stp.deployment ≠ "") : (c.st0.dep? stp.deployment).isSome := by
  unfold scopeFails at h
  dsimp only at h
  rw [List.append_eq_nil_iff, List.flatMap_eq_nil_iff] at h
  have hm : stp.deployment ∈ compact (sortStrings ((c.plan.goal.filter (·.deployment != "")).map (·.deployment) ++
      (c.plan.steps ++ c.plan.rollback).flatMap fun s => s.deployments ++ [s.deployment])) := by
    rw [mem_compact, mem_sortStrings]
    exact List.mem_append_right _ (List.mem_flatMap.mpr ⟨stp, hst, by simp⟩)
  have := h.1 _ hm
  rw [List.append_eq_nil_iff] at this
  have h0 := this.1
  have hne : (stp.deployment == "") = false := by simpa using hd
  rw [if_neg (by simp [hne])] at h0
  split at h0
  · cases h0
  · split at h0
    · cases h0
    · rename_i hn
      exact Option.ne_none_iff_isSome.mp (by simpa using hn)

theorem runOps_succ (st : State) (steps : List Step) (i : Nat) (hi : i < steps.length) :
    runOps st (steps.take (i + 1)) = (applyOp (runOps st (steps.take i)) steps[i]).1 := by
  rw [List.take_add_one, List.getElem?_eq_getElem hi, runOps_append]
  rfl

/-- **Rules `budget` and `never-zero` are sound.** If the checker says
Proved, then however the plan and its rollback really run — the scheduler
placing pods where it likes, pods becoming ready when they like, rollouts
going at the controller's pace — every budget holds (or availability is no
lower than in the snapshot) and no deployment with a ready pod loses its
last one, at every moment. -/
theorem availability_sound (snap : Snapshot) (plan : Plan) (pol : Policy) (hv : Valid snap) (hnamed : StepsNamed plan)
    (h : (check snap plan pol).result = .proved) : AvailabilitySafe snap plan pol := by
  let c := Ctx.make snap plan pol
  have hp : Passed c := execute_passed c h
  have hall := passed_all hp
  have hscope : scopeFails c = [] := by
    have hs := hp.static
    unfold staticFails at hs
    dsimp only at hs
    split at hs
    · exact hs
    · rename_i hne
      simpa using hne
  let steps := allSteps plan
  have hcp : c.plan = plan := rfl
  -- Each prefix of the run, by induction on how the world runs.
  suffices key : ∀ (w : World) (rest : List Step) (ws : List World), Runs snap w rest ws →
      ∀ i, i ≤ steps.length → rest = steps.drop i → Sim snap (runOps c.st0 (steps.take i)) w →
      (eachStateFails c (runOps c.st0 (steps.take i)) = [] ∨ runOps c.st0 (steps.take i) = build snap) →
      ∀ w' ∈ ws, BudgetOK snap w' ∧ NeverZero snap plan pol w' by
    intro ws hr
    exact key _ _ ws hr 0 (Nat.zero_le _) rfl (sim_initial snap hv) (Or.inr rfl)
  intro w rest ws hr
  induction hr with
  | nil => intro _ _ _ _ _ w' hw'; cases hw'
  | cons w w1 w2 stp rest ws hset hnext _ ih =>
    intro i hi hrest hs he
    have hlt : i < steps.length := by
      rcases Nat.lt_or_ge i steps.length with h | h
      · exact h
      · rw [List.drop_of_length_le h] at hrest; cases hrest
    rw [List.drop_eq_getElem_cons hlt] at hrest
    obtain ⟨hstp, hrest'⟩ := List.cons.inj hrest
    subst hstp
    let st := runOps c.st0 (steps.take i)
    have hfl : ∀ j, FloorsOK (runOps c.st0 (steps.take j)) := fun j => floors_runOps _ _ (floors_build snap)
    have hpd : ∀ j, (runOps c.st0 (steps.take j)).pdbs = (build snap).pdbs := fun j => pdbs_runOps _ _
    have hs1 : Sim snap st w1 := hs.settle hset
    have hok : StepOK c st steps[i] := hall i hlt
    have hex : (steps[i].op = "scale" ∨ steps[i].op = "set-image" ∨ steps[i].op = "rollout-restart") →
        (st.dep? steps[i].deployment).isSome := by
      intro hop
      have hmem : steps[i] ∈ allSteps plan := List.getElem_mem hlt
      have hd := hnamed _ hmem hop
      rw [hs.isSome, ← build_isSome]
      exact scope_exists hscope hmem hd
    have hs2 : Sim snap (runOps c.st0 (steps.take (i + 1))) w2 := by
      rw [runOps_succ _ _ _ hlt]
      exact hs1.step hv.1 hok hex hnext
    have he2 : eachStateFails c (runOps c.st0 (steps.take (i + 1))) = [] := by
      rw [runOps_succ _ _ _ hlt]; exact hok.eachOK
    intro w' hw'
    rcases List.mem_cons.mp hw' with rfl | hw'
    · exact safe_state hs1 (hfl i) (hpd i) he
    rcases List.mem_cons.mp hw' with rfl | hw'
    · exact safe_state hs2 (hfl (i + 1)) (hpd (i + 1)) (Or.inl he2)
    · exact ih (i + 1) hlt hrest' hs2 (Or.inl he2) w' hw'

end Planproof
