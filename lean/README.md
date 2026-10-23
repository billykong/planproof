# planproof in Lean

A Lean 4 model of the planproof kernel, a checker written in Lean, and proofs that when that checker says **Proved**, the core rules hold. It is part of a demo, not a production system: the point is the idea of trusting a checker small enough to read, made precise. Requirements and history: [PRD area L](../docs/prd/areas/l-lean.md).

## What is proved

One theorem, `Planproof.sound` in [Proofs/Sound.lean](Planproof/Proofs/Sound.lean), the five properties below together, covering six of the 13 rules (`budget` and `never-zero` share one). Each holds for every snapshot, plan and policy, not just the ones tested.

```lean
theorem sound (snap : Snapshot) (plan : Plan) (pol : Policy) (hv : Valid snap) (hn : StepsNamed plan)
    (h : (check snap plan pol).result = .proved) :
    GoalExact snap plan ∧ CordonFirst snap plan ∧ NodeLimits snap plan pol ∧ AvailabilitySafe snap plan pol ∧
    ∀ i, i ≤ (allSteps plan).length →
      CapOK (build snap).usage (runOps (build snap) ((allSteps plan).take i))
```

In plain English: if the snapshot and plan pass the Go kernel's validation (`Valid`, `StepsNamed`) and the Lean checker says Proved, then:

| Rule | Lean | What it says |
| --- | --- | --- |
| `frame` | `GoalExact` | The plan changes exactly what its goal says, and nothing else. |
| `cordon-first` | `CordonFirst` | Every drain, in the plan or its rollback, is of a node already cordoned. |
| `limits` | `NodeLimits` | After every step, the number of cordoned nodes and of schedulable nodes is within the policy, or no worse than in the snapshot. |
| `budget`, `never-zero` | `AvailabilitySafe` | However the plan and its rollback really run (the scheduler placing pods where it likes, pods becoming ready when they like, rollouts going at their own pace), at every moment every disruption budget holds or availability is no lower than in the snapshot, and no deployment with a ready pod loses its last one (unless the goal scales it to zero and the policy allows that). |
| `capacity` | `CapOK` | After every step, the placement the checker found (first fit) fits: every node's requests are within its allocatable, or no more than in the snapshot. |

Each is also a theorem of its own: `frame_sound`, `cordon_first_sound`, `limits_sound`, `availability_sound`, `capacity_sound`. CI checks that each depends only on Lean's three standard axioms ([theorems.txt](theorems.txt)).

## What to review

To trust the theorem you read its statement and the definitions it uses, not the checker or the proofs. Lean checks the proofs.

1. [Spec/Fields.lean](Planproof/Spec/Fields.lean): the fields a plan changes (`Fields`, `step`, `after`) and `GoalExact`, `CordonFirst`, `NodeLimits`, `NodesUnique`.
2. [Spec/World.lean](Planproof/Spec/World.lean): what the real cluster may do. A `World` is the fields, the pods (each on a node or pending, ready or not) and the rollouts under way. `Settles` is what may happen between steps; `Next` is what each operation may do; `Runs` is every way a plan can run. Then `BudgetOK`, `NeverZero`, `AvailabilitySafe`, `Valid` and `StepsNamed`.
3. [Input.lean](Planproof/Input.lean): the input types, the same JSON the Go kernel parses.
4. For `capacity` only: `CapOK` in [Proofs/Capacity.lean](Planproof/Proofs/Capacity.lean), and the checker's `build`, `usage`, `applyOp` and `runOps`. This rule is about the checker's own placement, so its statement uses the checker's state.

`Spec/` is held under 600 lines by CI; it is about 180.

## What is only tested, or assumed

- **The Go kernel.** The theorems are about the Lean checker. The Go kernel, which the CLI runs, is linked to it by tests, not proofs: `make lean` runs both on 305 inputs (the case table, the examples, every evals plan and agent run) and on 10,000 generated plans, and fails on any difference in result, failing rules, Unknown trigger or approval reasons ([internal/leandiff](../internal/leandiff)).
- **The model.** A proof about a wrong model is still a proof about a wrong model. That `World` and `Next` match real Kubernetes is tested on a kind cluster ([results/kind-test.txt](../results/kind-test.txt)), not proved. Like the kernel, the model takes ready pods to stay ready and nothing else to change the objects during the plan (the packet's assumptions A1 and A4), and only counts runs in which every wait ends; a wait that times out stops the plan.
- **The other rules.** `scope`, `protected`, the static half of `limits`, `assertions`, `no-orphans`, `settled` and `rollback` are in the Lean checker and covered by the agreement tests, but have no proofs (`input`, parsing, is Go's alone). The proved rules do hold through the rollback's steps; what isn't proved is that the rollback restores the goal. The Unknown triggers the proofs depend on are checked as part of them; the others are tested only.
- **The capacity witness.** `capacity_sound` says the checker's own placement fits. The real scheduler may place pods elsewhere; that a placement exists is what the rule promises.
- **Validation.** `planproof-lean` reads only inputs the Go kernel accepted, and the theorem assumes what that validation ensures: unique node and deployment names, no negative `maxUnavailable` or resource request (`Valid`), and a deployment named in every scale, set-image and rollout-restart (`StepsNamed`).

## Findings

Stating the theorem found three soundness bugs in the Go kernel, each a plan it proved although a real run could take a deployment to zero. They are fixed in kernel 0.3.0, 0.4.0 and 0.5.0 and are in the case table ([details](../docs/prd/areas/l-lean.md#findings)).

## Layout

| Path | Contents |
| --- | --- |
| `Planproof/Spec/` | The specification: what a reviewer reads |
| `Planproof/Input.lean` | Input types |
| `Planproof/Check/` | The checker: a function-by-function copy of the Go kernel |
| `Planproof/Json.lean`, `Main.lean` | `planproof-lean`: reads one input per line, prints the result |
| `Planproof/Proofs/` | The proofs. `Sim*.lean` relate the checker's state to every real world it stands for, one operation at a time; `Safety.lean` and `Capacity.lean` finish the two hardest rules; `Sound.lean` is the main theorem |
| `theorems.txt` | The theorems whose axioms CI checks |

## Build

Needs [elan](https://github.com/leanprover/elan); the toolchain is pinned in `lean-toolchain`. Core Lean only, no Mathlib.

```bash
make lean   # build, check for sorry and other escape hatches, the Spec budget, the axioms, then the agreement tests
```
