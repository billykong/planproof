# L - Lean model and proofs

**Goal:** turn "the checker is small and well tested" into "for the core rules, the checker is proved correct against a precise model". A Lean 4 model of the semantics, a checker written in Lean with theorems that a Proved verdict implies the core rules hold, and differential tests that the Go kernel gives the same verdicts as the Lean checker. Covers goals G1 and G2. The approach copies AWS's Cedar: a model in Lean, proofs about the model, and differential testing of the production code against it ([prior art](../../prior-art.md#4-language-models-with-a-verifier)).

Status: **In progress** (phase 5). Milestone 1 (toolchain, CI) and milestone 3 (the Lean checker and agreement tests) done 2026-10-21. Decided 2026-10-17: prove the core rules first (frame, budget, never-zero, cordon-first, limits, and capacity through a checked placement), with the model and differential tests covering every rule; core Lean only, no Mathlib.

## Findings

- **2026-10-21: a soundness bug in the Go kernel, found while planning the `never-zero` proof.** To state "for every concrete placement", the drain step has to say where a pod that *might* be on the drained node can be afterwards. The kernel said "on a schedulable node"; but the pod may have been on another of its candidate nodes and stayed there. A plan (scale cart from 1 to 2, cordon n1 and n2, drain n1, wait, drain n2) was proved although the new pod may sit on n2 with the old one, and draining n2 then takes cart to zero. Neither the corpus nor 10,000 generated plans contained the case. Fixed in kernel 0.3.0 and spec 03; the plan is now in the case table; the Lean checker has the same fix.

## What a proof adds, and what it doesn't

Tests try examples; a proof covers every possible plan, snapshot and policy within the model. Lean checks each proof mechanically.

| Proved (phase 5) | Still covered only by tests |
| --- | --- |
| The Lean checker's Proved implies the core rules hold at every state of the plan, under the model | That the model matches real Kubernetes (the kind fidelity tests, C-20 and C-21) |
| The worst case the checker counts is a true lower bound over every concrete placement the abstract state allows | Parsing and validation of the inputs (K-10 to K-13); both sides read the same parsed JSON |
| A capacity pass comes with a placement that fits every node | The review packet (R area) and the executor (C area) |
| | That the Go kernel computes what the Lean checker computes (differential tests, L-20 to L-22) |

## Design (2026-10-17)

- **Three layers in Lean, kept apart.**
  1. **The specification** (`lean/Planproof/Spec/`): the abstract state, the seven operations' During and After states (spec 03), and the core rules as *declarative* properties. This is what a reviewer reads; it gets a size budget like the kernel's.
  2. **The checker** (`lean/Planproof/Check/`): an executable function in the shape of the Go kernel (incremental, worst-case counting, first-fit placement), returning the result, the failing rules and the Unknown trigger.
  3. **The proofs** (`lean/Planproof/Proofs/`): `check … = .proved → Safe …`, one theorem per core rule, combined into one statement.
- **Declarative, not a copy of the checker.** A proof that the checker equals a definition written the same way would prove little. The rules are stated independently of how the checker computes them:
  - **budget / never-zero:** for *every concrete placement* consistent with the abstract state (each pod on one node from its candidate set, or gone), available pods ≥ the requirement. The checker counts a worst case from candidate sets; the theorem shows that count is a lower bound over all placements. This is the soundness of the abstraction (abstract interpretation, Cousot and Cousot 1977).
  - **frame:** the set of fields the final state changes, compared with the snapshot, equals the goal exactly.
  - **cordon-first:** every drain's node is unschedulable in the state before the drain.
  - **limits:** at every state, unschedulable and schedulable node counts within the policy, or no worse than the snapshot.
  - **capacity:** the checker returns the placement it found (a witness); the theorem shows each placement fits every node's allocatable resources. The first-fit search itself needs no proof, only the witness check (certifying algorithms, McConnell et al. 2011).
  - Each rule's "only what the plan makes worse" baseline (spec 04) is part of its statement.
- **Only Proved is claimed.** The theorems say nothing about Refuted or Unknown: the checker is sound, not complete, as in G2.
- **Same input, two checkers.** A Lean executable, `planproof-lean`, reads the same plan, snapshot and policy JSON as the Go kernel, but only inputs the Go kernel accepted (so it needs no validation of its own), and prints a summary: result, failing rules in spec order, Unknown trigger, approval reasons. A Go test runs both on a corpus and on generated plans, and fails on any disagreement.
- **Integers.** Lean's `Int` and `Nat` don't overflow; Go's `int64` could in principle. The input limits (K-11; replicas at most `maxReplicas`) keep every value far from overflow; the differential tests include values at those limits.
- **Core Lean only.** No Mathlib: the standard library and built-in tactics (`omega`, `simp`, `decide`). A fast CI build and a small trusted base, in keeping with G1. The toolchain is pinned in `lean/lean-toolchain`.
- **No escape hatches.** No `sorry`, no new `axiom`, no `native_decide`, no `implemented_by` in the specification or the proofs. CI prints `#print axioms` for the main theorem and fails on anything beyond Lean's standard three (`propext`, `Classical.choice`, `Quot.sound`).

## User stories

- As a **security reviewer**, I read a few hundred lines of Lean definitions and one theorem statement, and know that every Proved verdict for the core rules is correct within the model, without reading the checker's code or its proofs.
- As a **kernel maintainer**, when I change the Go kernel, the differential tests tell me at once if it no longer agrees with the proved checker.
- As an **engineering lead**, I can say which claims are proved, which are tested against a real cluster, and which are assumptions.

## Requirements

### Project and CI

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| L-01 | A Lake project in `lean/`, toolchain pinned in `lean-toolchain`, depending on core Lean only. `make lean` builds it and checks every proof. | Must | **Done** (2026-10-21): `lean/`, Lean v4.34.1 (the stable release before the newest, out about four weeks), `make lean` |
| L-02 | CI installs the pinned toolchain, runs `make lean`, and fails on `sorry`, a new `axiom`, `native_decide` or `implemented_by` in `Spec/` or `Proofs/`, and on any axiom beyond `propext`, `Classical.choice` and `Quot.sound` in `#print axioms` of the main theorem. | Must | **Done** (2026-10-21): the `lean` job in CI runs `make lean`; [scripts/lean-check.sh](../../../scripts/lean-check.sh) fails on the escape hatches anywhere in `lean/Planproof` (tested with a planted `sorry`) and checks the axioms of each theorem in `lean/theorems.txt` |
| L-03 | `Spec/` is at most 600 non-blank, non-comment lines, counted in CI: small enough for a reviewer to read in one sitting. | Should | **Partial**: counted by `scripts/lean-check.sh` once `Spec/` exists |

### Specification

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| L-10 | The abstract state of [spec 02](../../spec/02-snapshot-and-model.md) and the During and After states of the seven operations of [spec 03](../../spec/03-operations.md), as Lean definitions. | Must | Planned |
| L-11 | The core rules as declarative properties over a plan's states: `frame`, `budget`, `never-zero`, `cordon-first`, `limits` (each state), `capacity` (placement fits), each with the "only what the plan makes worse" baseline of [spec 04](../../spec/04-rules-and-policy.md). | Must | Planned |
| L-12 | Concrete placements: a definition of the placements an abstract state allows (each pod on one node from its candidate set, or gone), used by `budget` and `never-zero`. | Must | Planned |
| L-13 | The other rules (`scope`, `protected`, static `limits`, `assertions`, `no-orphans`, `settled`, `rollback`) and the Unknown triggers, as executable checks, so the checker covers all 13 rules for differential testing. | Must | **Done** (2026-10-21): `lean/Planproof/Check/` covers every rule (`input` is the Go kernel's alone) and every trigger |

### Checker and differential tests

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| L-20 | An executable Lean checker with the Go kernel's structure, and a binary `planproof-lean` that reads parsed inputs as JSON and prints result, sorted failing rules and trigger. | Must | **Done** (2026-10-21): `lean/Planproof/Check/Model.lean` and `Run.lean` (a function-by-function transliteration of `kernel/model.go`, `run.go`, `rules.go` and the decision part of `check.go`); `planproof-lean` reads one input per line and also prints the approval reasons |
| L-21 | A Go test runs the Go kernel and `planproof-lean` on: the shared case table (`internal/testcases`), the worked examples, every task's reference and naive plan, the 25 seeded plans, the 12 study items, and every plan submitted in the 247 stored agent runs. Zero disagreements. | Must | **Done** (2026-10-21): `TestCorpus` in `internal/leandiff`, 305 inputs (the study items are reference and seeded plans, already included), reaching every rule except `input` and every Unknown trigger; 0 disagreements |
| L-22 | The same comparison on plans generated by the Go fuzzer (`FuzzCheck`'s corpus and a generator of valid plans over the evals' snapshots), including values at the input limits, run in CI for a fixed time. | Must | **Done** (2026-10-21): `TestGenerated`, 10,000 plans from a fixed seed over the three evals snapshots and both policies, each plan's goal set to its own effect so most reach the simulation; the test fails unless every result, every simulated rule and the drain, critical and scale-down approvals are reached; 0 disagreements. In CI through `make lean`. Sensitivity: a planted off-by-one in `budget` gave 149 corpus and 886 (of 3,000) generated disagreements; scale-down removing ready pods first gave 0 corpus and 9 of 10,000 generated, the reason for the count. `FuzzCheck`'s own corpus is not yet included |

### Proofs

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| L-30 | `frame`: Proved implies the final state's changed fields equal the goal exactly. | Must | **Done** (2026-10-21): `Planproof.frame_sound` in `lean/Planproof/Proofs/Frame.lean`: `(check snap plan pol).result = .proved → Spec.GoalExact snap plan`, with `GoalExact` in `Spec/Fields.lean` (every goal change's `from` is the snapshot's value and differs from `to`; after the steps, every field the goal names has the goal's value and every other field its snapshot value). No hypotheses; only Lean's standard axioms |
| L-31 | `cordon-first`: Proved implies every drained node was unschedulable in the state before its drain. | Must | **Done** (2026-10-21): `Planproof.cordon_first_sound` (`Proofs/Nodes.lean`) → `Spec.CordonFirst`, over the plan's steps and its rollback; no hypotheses |
| L-32 | `limits` (each state): Proved implies every state's node counts are within the policy or no worse than the snapshot. | Must | **Done** (2026-10-21): `Planproof.limits_sound` → `Spec.NodeLimits`, after every step of the plan and its rollback; one hypothesis, `Spec.NodesUnique` (node names are unique, which the Go kernel's validation requires) |
| L-33 | `budget` and `never-zero`: Proved implies, for every state and every concrete placement it allows, the requirement holds or is no worse than in the snapshot. | Must | Planned |
| L-34 | `capacity`: Proved implies the checker's placement witness fits every node at every state. | Must | Planned |
| L-35 | One main theorem combining L-30 to L-34, whose statement and the definitions it depends on are listed in `lean/README.md` as the thing to review. | Must | Planned |
| L-36 | Proofs for the other rules (L-13). | Could | Planned |

### Documentation

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| L-40 | `lean/README.md`: what is proved, what is only tested, how to build, and the main theorem in plain English next to its Lean statement. | Must | Planned |
| L-41 | The review packet's NOT CHECKED section, the PRD and the README say which rules are proved (for the Lean checker) and that the Go kernel is linked to it by differential tests, not by proof. | Should | Planned |

## Relationship to other requirements

- **K-25** (a brute-force soundness test) checks small cases exhaustively; L-33 proves the same property for all cases. K-25 stays useful as a quick test of the Go kernel.
- **C-20, C-21** (fidelity on kind) remain the only evidence that the model matches Kubernetes. A proof about a wrong model would still be a proof about a wrong model.
