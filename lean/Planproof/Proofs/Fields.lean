import Planproof.Check.Run
import Planproof.Spec.Fields

/-!
# The checker's state follows the fields spec

`State.field` (the checker) agrees with `Spec.initial` on the snapshot, and
each operation changes it as `Spec.step` says.
-/
namespace Planproof

open Spec

/-! ## Lookups after updates -/

theorem find?_map_same {α : Type} (l : List α) (g : α → α) (p : α → Bool) (hg : ∀ x, p (g x) = p x) :
    (l.map g).find? p = (l.find? p).map g := by
  rw [List.find?_map]
  congr 1
  apply congrArg (fun q => List.find? q l)
  funext x
  simp [hg]

theorem State.dep?_updDep (st : State) (d e : String) (f : MDep → MDep) (hf : ∀ x, (f x).name = x.name) :
    (st.updDep d f).dep? e = (st.dep? e).map fun x => if x.name == d then f x else x := by
  unfold State.updDep State.dep?
  simp only
  rw [find?_map_same]
  intro x
  split <;> simp [hf]

theorem State.node?_updNode (st : State) (n m : String) (f : MNode → MNode) (hf : ∀ x, (f x).name = x.name) :
    (st.updNode n f).node? m = (st.node? m).map fun x => if x.name == n then f x else x := by
  unfold State.updNode State.node?
  simp only
  rw [find?_map_same]
  intro x
  split <;> simp [hf]

theorem State.name_of_dep? {st : State} {d : String} {x : MDep} (h : st.dep? d = some x) : x.name = d := by
  have := List.find?_some h
  simpa using this

theorem State.name_of_node? {st : State} {n : String} {x : MNode} (h : st.node? n = some x) : x.name = n := by
  have := List.find?_some h
  simpa using this

/-! ## Updates that keep the fields -/

/-- The fields depend only on the deployments' and nodes' field parts. -/
theorem State.field_congr {st st' : State} (hd : st'.deps = st.deps) (hn : st'.nodes = st.nodes) :
    st'.field = st.field := by
  funext k
  cases k <;> simp [State.field, State.dep?, State.node?, hd, hn]

theorem State.field_updPods (st : State) (sel : MPod → Bool) (f : MPod → MPod) :
    (st.updPods sel f).field = st.field :=
  State.field_congr rfl rfl

theorem State.deps_setAt (st : State) (names : List String) (n : String) : (st.setAt names n).deps = st.deps := rfl
theorem State.nodes_setAt (st : State) (names : List String) (n : String) : (st.setAt names n).nodes = st.nodes := rfl

/-- Forget where first fit charged a pod. -/
def MPod.unplace (p : MPod) : MPod := { p with atNode := "" }

theorem setIdx_unplace (ps : List MPod) (i : Nat) (n : String) :
    (State.setIdx ps i n).map MPod.unplace = ps.map MPod.unplace := by
  induction ps generalizing i with
  | nil => rfl
  | cons p ps ih =>
    cases i with
    | zero => rfl
    | succ k => simp only [State.setIdx, List.map_cons, ih]

/-- First fit changes only where pods are charged. -/
theorem State.place_shape (st : State) (names : List String) :
    ∃ ps, (st.place names).1 = { st with pods := ps } ∧ ps.map MPod.unplace = st.pods.map MPod.unplace := by
  unfold State.place
  simp only
  generalize (List.filter _ _).mergeSort _ = order
  suffices h : ∀ (acc : State × Option MPod), (∃ ps, acc.1 = { st with pods := ps } ∧ ps.map MPod.unplace = st.pods.map MPod.unplace) →
      ∃ ps, (order.foldl (fun (x : State × Option MPod) y =>
        match x with
        | (st, bad) => match bad with
          | some _ => (st, bad)
          | none => match y.1.nodes.find? (st.fits y.1) with
            | some n => (st.setAtIdx y.2 n, none)
            | none => (st, some y.1)) acc).1 = { st with pods := ps } ∧ ps.map MPod.unplace = st.pods.map MPod.unplace by
    apply h
    refine ⟨_, rfl, ?_⟩
    simp only [List.map_map]
    apply List.map_congr_left
    intro p _
    simp only [Function.comp]
    split <;> rfl
  induction order with
  | nil => intro acc h; exact h
  | cons y rest ih =>
    intro acc h
    apply ih
    obtain ⟨s1, bad⟩ := acc
    cases bad with
    | some _ => exact h
    | none =>
      simp only
      split
      · obtain ⟨ps, hps, hm⟩ := h
        simp only at hps
        subst hps
        exact ⟨_, rfl, (setIdx_unplace _ _ _).trans hm⟩
      · exact h

theorem State.deps_place (st : State) (names : List String) : (st.place names).1.deps = st.deps := by
  obtain ⟨ps, h, -⟩ := st.place_shape names; rw [h]

theorem State.nodes_place (st : State) (names : List String) : (st.place names).1.nodes = st.nodes := by
  obtain ⟨ps, h, -⟩ := st.place_shape names; rw [h]

theorem State.field_place (st : State) (names : List String) : (st.place names).1.field = st.field :=
  State.field_congr (State.deps_place st names) (State.nodes_place st names)

theorem State.field_remove (st : State) (names : List String) : (st.remove names).field = st.field :=
  State.field_congr rfl rfl

/-- An update of a deployment that keeps its name and field parts keeps the fields. -/
theorem State.field_updDep_keep (st : State) (d : String) (f : MDep → MDep)
    (hn : ∀ x, (f x).name = x.name) (hr : ∀ x, (f x).replicas = x.replicas)
    (hs : ∀ x, (f x).restarted = x.restarted) (hi : ∀ x, (f x).images = x.images) :
    (st.updDep d f).field = st.field := by
  funext k
  cases k with
  | replicas e => simp only [State.field, State.dep?_updDep st d e f hn]; cases st.dep? e <;> simp; split <;> simp [hr]
  | restarted e => simp only [State.field, State.dep?_updDep st d e f hn]; cases st.dep? e <;> simp; split <;> simp [hs]
  | image e c => simp only [State.field, State.dep?_updDep st d e f hn]; cases st.dep? e <;> simp; split <;> simp [hi]
  | unsched n => rfl
  | drained n => rfl

theorem State.field_newPods (st : State) (d : String) (k : Int) : (st.newPods d k).1.field = st.field := by
  unfold State.newPods
  split
  · rfl
  · simp only
    exact (State.field_congr rfl rfl).trans
      (State.field_updDep_keep st d _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl))

end Planproof

namespace Planproof

open Spec

/-! ## Updates that set one field -/

theorem State.field_updNode_unsched (st : State) (n : String) (b : Bool) :
    (st.updNode n fun x => { x with unsched := b }).field = setField st.field (.unsched n) (.bool b) := by
  funext k
  cases k with
  | unsched m =>
    simp only [State.field, setField]
    rw [State.node?_updNode st n m (fun x => { x with unsched := b }) (fun _ => rfl)]
    by_cases h : m = n
    · subst h
      cases hm : st.node? m with
      | none => simp
      | some x => simp [State.name_of_node? hm]
    · cases hm : st.node? m with
      | none => simp [h]
      | some x => simp [h, State.name_of_node? hm]
  | drained m =>
    simp only [State.field, setField]
    rw [State.node?_updNode st n m (fun x => { x with unsched := b }) (fun _ => rfl)]
    cases hm : st.node? m with
    | none => simp
    | some x => by_cases hb : x.name = n <;> simp [hb]
  | _ => simp [State.field, setField, State.updNode, State.dep?]

theorem State.field_updNode_drained (st : State) (n : String) :
    (st.updNode n fun x => { x with drained := true }).field = setField st.field (.drained n) (.bool true) := by
  funext k
  cases k with
  | drained m =>
    simp only [State.field, setField]
    rw [State.node?_updNode st n m (fun x => { x with drained := true }) (fun _ => rfl)]
    by_cases h : m = n
    · subst h
      cases hm : st.node? m with
      | none => simp
      | some x => simp [State.name_of_node? hm]
    · cases hm : st.node? m with
      | none => simp [h]
      | some x => simp [h, State.name_of_node? hm]
  | unsched m =>
    simp only [State.field, setField]
    rw [State.node?_updNode st n m (fun x => { x with drained := true }) (fun _ => rfl)]
    cases hm : st.node? m with
    | none => simp
    | some x => by_cases hb : x.name = n <;> simp [hb]
  | _ => simp [State.field, setField, State.updNode, State.dep?]

end Planproof

namespace Planproof

open Spec

theorem setField_none (f : Fields) (k : Key) (v : Value) (h : f k = none) : setField f k v = f := by
  funext k'
  simp only [setField]
  split
  · subst_vars; simp [h]
  · rfl

theorem State.field_updDep_replicas (st : State) (d : String) (n : Int) :
    (st.updDep d fun x => { x with replicas := n }).field = setField st.field (.replicas d) (.int n) := by
  funext k
  cases k with
  | replicas e =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with replicas := n }) (fun _ => rfl)]
    by_cases h : e = d
    · subst h
      cases hm : st.dep? e with
      | none => simp
      | some x => simp [State.name_of_dep? hm]
    · cases hm : st.dep? e with
      | none => simp [h]
      | some x => simp [h, State.name_of_dep? hm]
  | restarted e =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with replicas := n }) (fun _ => rfl)]
    cases st.dep? e with
    | none => simp
    | some x => by_cases hb : x.name = d <;> simp [hb]
  | image e c =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with replicas := n }) (fun _ => rfl)]
    cases st.dep? e with
    | none => simp
    | some x => by_cases hb : x.name = d <;> simp [hb]
  | _ => simp [State.field, setField, State.updDep, State.node?]

theorem State.field_updDep_restart (st : State) (d : String) (old : List String) :
    (st.updDep d fun x => { x with rolling := true, oldNodes := old, restarted := true }).field = setField st.field (.restarted d) (.bool true) := by
  funext k
  cases k with
  | restarted e =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with rolling := true, oldNodes := old, restarted := true }) (fun _ => rfl)]
    by_cases h : e = d
    · subst h
      cases hm : st.dep? e with
      | none => simp
      | some x => simp [State.name_of_dep? hm]
    · cases hm : st.dep? e with
      | none => simp [h]
      | some x => simp [h, State.name_of_dep? hm]
  | replicas e =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with rolling := true, oldNodes := old, restarted := true }) (fun _ => rfl)]
    cases st.dep? e with
    | none => simp
    | some x => by_cases hb : x.name = d <;> simp [hb]
  | image e c =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with rolling := true, oldNodes := old, restarted := true }) (fun _ => rfl)]
    cases st.dep? e with
    | none => simp
    | some x => by_cases hb : x.name = d <;> simp [hb]
  | _ => simp [State.field, setField, State.updDep, State.node?]

theorem find?_setImage (c img c' : String) (l : List (String × String)) :
    ((setImage c img l).find? (·.1 == c')).map (·.2) =
      ((l.find? (·.1 == c')).map (·.2)).map fun i => if c' == c then img else i := by
  unfold setImage
  rw [find?_map_same _ _ _ (fun x => by split <;> rfl)]
  cases h : l.find? (·.1 == c') with
  | none => simp
  | some x =>
    have hx : x.1 = c' := by simpa using List.find?_some h
    simp only [Option.map_some]
    by_cases hc : x.1 = c
    · simp [hc, ← hx]
    · have : (x.1 == c) = false := by simpa using hc
      simp only [this]
      have : (c' == c) = false := by simpa [← hx] using hc
      simp [this]

theorem State.field_updDep_image (st : State) (d c img : String) (old : List String) :
    (st.updDep d fun x => { x with rolling := true, oldNodes := old, images := setImage c img x.images }).field =
      setField st.field (.image d c) (.str img) := by
  funext k
  cases k with
  | image e c' =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with rolling := true, oldNodes := old, images := setImage c img x.images }) (fun _ => rfl)]
    by_cases h : e = d
    · subst h
      cases hm : st.dep? e with
      | none => simp
      | some x =>
        have hx := State.name_of_dep? hm
        simp only [Option.map_some, Option.bind_some, hx, beq_self_eq_true, ite_true]
        have := find?_setImage c img c' x.images
        rw [this]
        by_cases hc : c' = c
        · subst hc
          cases x.images.find? (·.1 == c') <;> simp
        · cases x.images.find? (·.1 == c') <;> simp [hc]
    · cases hm : st.dep? e with
      | none => simp [h]
      | some x => have hx := State.name_of_dep? hm; simp [h, hx]
  | replicas e =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with rolling := true, oldNodes := old, images := setImage c img x.images }) (fun _ => rfl)]
    cases st.dep? e with
    | none => simp
    | some x => by_cases hb : x.name = d <;> simp [hb]
  | restarted e =>
    simp only [State.field, setField]
    rw [State.dep?_updDep st d e (fun x => { x with rolling := true, oldNodes := old, images := setImage c img x.images }) (fun _ => rfl)]
    cases st.dep? e with
    | none => simp
    | some x => by_cases hb : x.name = d <;> simp [hb]
  | _ => simp [State.field, setField, State.updDep, State.node?]

end Planproof

namespace Planproof

open Spec

theorem State.field_updDep_floor (st : State) (d : String) (v : Int) :
    (st.updDep d fun x => { x with floor := v }).field = st.field :=
  State.field_updDep_keep st d _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)

theorem State.field_wait (st : State) (ds : List String) :
    (ds.foldl (init := st) fun st d =>
      let st := st.updDep d fun x => { x with rolling := false, oldNodes := [] }
      st.updPods (fun p => p.dep == d && !p.gone) fun p => { p with ready := true }).field = st.field := by
  induction ds generalizing st with
  | nil => rfl
  | cons d rest ih =>
    simp only [List.foldl_cons]
    rw [ih]
    rw [State.field_updPods]
    exact State.field_updDep_keep st d _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl)

theorem State.field_surgeFits (st : State) (d : String) (m : MDep) : (st.surgeFits d m).1.field = st.field := by
  unfold State.surgeFits
  split
  · rfl
  · exact (State.field_congr rfl rfl).trans (State.field_newPods st d m.maxSurge)

theorem State.field_scaleDown (st : State) (d : String) (n : Int) : (st.scaleDown d n).field = st.field :=
  State.field_congr rfl rfl

theorem State.field_rollout (st : State) (s : Step) (m : MDep) :
    (st.rollout s m).1.field = (st.updDep s.deployment (rolloutUpdate s ((st.podsOf s.deployment).flatMap (·.nodes)))).field := by
  unfold State.rollout
  simp only
  rw [State.field_place, State.field_newPods]
  exact State.field_congr rfl rfl

/-- Each operation that succeeds changes the checker's fields as `Spec.step` says. -/
theorem State.field_applyOp (st : State) (s : Step) (h : (applyOp st s).2 = true) :
    (applyOp st s).1.field = Spec.step st.field s := by
  by_cases h1 : s.op = "cordon"
  · simp only [applyOp, Spec.step, h1]; exact State.field_updNode_unsched st s.node true
  by_cases h2 : s.op = "uncordon"
  · simp only [applyOp, Spec.step, h2]
    split
    · rw [State.field_updPods]; exact State.field_updNode_unsched st s.node false
    · exact State.field_updNode_unsched st s.node false
  by_cases h3 : s.op = "drain"
  · simp only [applyOp, Spec.step, h3]
    rw [State.field_place, State.field_updNode_drained, State.field_updPods]
  by_cases h4 : s.op = "scale"
  · simp only [applyOp, h4] at h ⊢
    simp only [Spec.step, h4]
    cases hd : st.dep? s.deployment with
    | none =>
      simp only [hd]
      exact (setField_none _ _ _ (by simp [State.field, hd])).symm
    | some m =>
      simp only [hd] at h ⊢
      split
      · rw [State.field_place, State.field_newPods, State.field_updDep_replicas]
      · split
        · rw [State.field_scaleDown, State.field_updDep_replicas]
        · exact State.field_updDep_replicas st s.deployment _
  by_cases h5 : s.op = "set-image"
  · simp only [applyOp, h5] at h ⊢
    simp only [Spec.step, h5]
    cases hd : st.dep? s.deployment with
    | none =>
      simp only
      exact (setField_none _ _ _ (by simp [State.field, hd])).symm
    | some m =>
      simp only [hd] at h ⊢
      generalize hsf : State.surgeFits _ _ _ = sf at h ⊢
      obtain ⟨st2, ok⟩ := sf
      have h2 : st2.field = st.field := by
        have := congrArg (fun x => x.1.field) hsf
        simp only at this
        rw [← this, State.field_surgeFits, State.field_updDep_floor]
      cases ok
      · simp at h
      · simp only [ite_true]
        rw [State.field_rollout]
        have hr : ∀ old, rolloutUpdate s old = fun d => { d with rolling := true, oldNodes := old, images := setImage s.container s.image d.images } := by
          intro old
          funext d; simp [rolloutUpdate, h5]
        rw [hr, State.field_updDep_image, h2]
  by_cases h6 : s.op = "rollout-restart"
  · simp only [applyOp, h6] at h ⊢
    simp only [Spec.step, h6]
    cases hd : st.dep? s.deployment with
    | none =>
      simp only
      exact (setField_none _ _ _ (by simp [State.field, hd])).symm
    | some m =>
      simp only [hd] at h ⊢
      generalize hsf : State.surgeFits _ _ _ = sf at h ⊢
      obtain ⟨st2, ok⟩ := sf
      have h2 : st2.field = st.field := by
        have := congrArg (fun x => x.1.field) hsf
        simp only at this
        rw [← this, State.field_surgeFits, State.field_updDep_floor]
      cases ok
      · simp at h
      · simp only [ite_true]
        rw [State.field_rollout]
        have hr : ∀ old, rolloutUpdate s old = fun d => { d with rolling := true, oldNodes := old, restarted := true } := by
          intro old
          funext d; simp [rolloutUpdate, h6]
        rw [hr, State.field_updDep_restart, h2]
  by_cases h7 : s.op = "wait"
  · simp only [applyOp, Spec.step, h7]
    exact State.field_wait st s.deployments
  · simp only [applyOp, Spec.step, h1, h2, h3, h4, h5, h6, h7]

end Planproof

namespace Planproof

open Spec

theorem State.field_mapDeps (st : State) (g : MDep → MDep)
    (hn : ∀ x, (g x).name = x.name) (hr : ∀ x, (g x).replicas = x.replicas)
    (hs : ∀ x, (g x).restarted = x.restarted) (hi : ∀ x, (g x).images = x.images) :
    ({ st with deps := st.deps.map g } : State).field = st.field := by
  funext k
  cases k <;> simp only [State.field, State.dep?, State.node?] <;>
    rw [find?_map_same _ _ _ (fun x => by simp [hn])] <;>
    cases (st.deps.find? _) <;> simp [hr, hs, hi]

/-- The checker's starting state has the snapshot's fields. -/
theorem State.field_build (s : Snapshot) : (build s).field = Spec.initial s := by
  unfold build
  simp only
  generalize hst0 : ({ deps := _, nodes := _, pods := _, pdbs := [] } : State) = st0
  have h0 : st0.field = Spec.initial s := by
    subst hst0
    funext k
    cases k <;> simp only [State.field, State.dep?, State.node?, Spec.initial, List.find?_map, Option.map_map, Option.bind_map] <;> rfl
  rw [← h0]
  suffices h : ∀ st : State, (s.pdbs.foldl (init := st) fun st b =>
      match b.deployment with
      | none => { st with deps := st.deps.map fun d =>
          if (d.name.splitOn "/").head? == some b.ns then { d with unresolved := true } else d }
      | some dn =>
        if (st.dep? (b.ns ++ "/" ++ dn)).isSome then
          let st := st.updDep (b.ns ++ "/" ++ dn) fun d => { d with pdbs := d.pdbs + 1, unresolved := d.unresolved || decide (d.pdbs + 1 > 1) }
          { st with pdbs := st.pdbs ++ [{ name := b.ns ++ "/" ++ b.name, dep := b.ns ++ "/" ++ dn, min := b.min, max := b.max }] }
        else st).field = st.field by
    exact (State.field_congr rfl rfl).trans (h st0)
  intro st
  induction s.pdbs generalizing st with
  | nil => rfl
  | cons b rest ih =>
    simp only [List.foldl_cons]
    rw [ih]
    cases b.deployment with
    | none =>
      simp only
      exact State.field_mapDeps st _ (fun x => by split <;> rfl) (fun x => by split <;> rfl)
        (fun x => by split <;> rfl) (fun x => by split <;> rfl)
    | some dn =>
      simp only
      split
      · exact (State.field_congr rfl rfl).trans
          (State.field_updDep_keep st _ _ (fun _ => rfl) (fun _ => rfl) (fun _ => rfl) (fun _ => rfl))
      · rfl

end Planproof
