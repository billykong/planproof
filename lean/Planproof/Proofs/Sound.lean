import Planproof.Proofs.Frame
import Planproof.Proofs.Nodes
import Planproof.Proofs.Capacity

/-!
# The main theorem (PRD L-35)

If the checker says Proved, then, for a snapshot and plan that pass the Go
kernel's validation:

* the plan changes exactly what its goal says (`frame`);
* every drain is of a node already cordoned (`cordon-first`);
* after every step, the node counts are within the policy or no worse
  (`limits`);
* at every moment of every way the plan and its rollback can really run,
  every disruption budget holds or availability is no lower than in the
  snapshot, and no deployment with a ready pod loses its last one
  (`budget`, `never-zero`);
* after every step, the checker's own placement fits every node, or the
  node is no fuller than in the snapshot (`capacity`).
-/
namespace Planproof

open Spec

theorem sound (snap : Snapshot) (plan : Plan) (pol : Policy) (hv : Valid snap) (hn : StepsNamed plan)
    (h : (check snap plan pol).result = .proved) :
    GoalExact snap plan ∧ CordonFirst snap plan ∧ NodeLimits snap plan pol ∧ AvailabilitySafe snap plan pol ∧
    ∀ i, i ≤ (allSteps plan).length →
      CapOK (build snap).usage (runOps (build snap) ((allSteps plan).take i)) :=
  ⟨frame_sound snap plan pol h, cordon_first_sound snap plan pol h, limits_sound snap plan pol hv.1 h,
   availability_sound snap plan pol hv hn h, capacity_sound snap plan pol hv h⟩

end Planproof
