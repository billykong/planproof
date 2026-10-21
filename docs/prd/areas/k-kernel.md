# K - The kernel

**Goal:** a pure function, small enough to read in an afternoon, that checks a plan against a snapshot and a policy and returns Proved, Refuted or Unknown, with the facts behind it. Covers goals G1, G2 and G3.

Status: **Done for phase 1 milestones 1-3**: budgets and CI; strict parsing, hashing and fuzzing; the model, the seven operations, the 13 rules and the full verdict. The worked examples match the hand-derived verdicts. Open: the brute-force soundness test (K-25) and mutation testing (K-26). Spec: [01](../../spec/01-plan.md) to [05](../../spec/05-verdict.md).

## User stories

- As an **approver**, I trust a Proved verdict because the code that produced it is small, tested, and I (or someone I trust) read it.
- As an **agent**, when my plan is Refuted, I get the step, the rule and the numbers, so I can fix the plan.
- As a **security reviewer**, I can list everything the kernel assumes, and see a test for each rule in both directions.

## Requirements

### Shape and size

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-01 | The kernel is one Go package, `kernel/`, exposing `Check(snapshot, plan, policy []byte) Verdict`. | Must | **Done**: `kernel/check.go` |
| K-02 | It imports only the Go standard library, and does no I/O, no network calls, no reads of the clock or environment, no randomness and no goroutines. CI fails on any other import. | Must | **Done**: `make budget` ([scripts/kernel-budget.sh](../../../scripts/kernel-budget.sh): import allowlist without `os`, `net`, `time` or random; no goroutines, channels, printing or scanning) |
| K-03 | Non-test Go code in `kernel/` is at most 1,800 non-blank, non-comment lines (counted by a script in CI). Raised from 1,500 on 2026-10-16: strict parsing and validation took about 500 lines, more than planned, and they are part of the trusted base too; trimming to fit would have made the code denser, against the point of the budget. The cap still blocks growth. | Must | **Done**: `make budget` |
| K-04 | Deterministic: the same inputs give byte-identical verdict JSON. Tested by running every golden case twice and with shuffled map iteration. | Must | **Done**: `TestExamples` (golden verdicts), `TestDeterministic`, `FuzzCheck` (two runs byte-identical; Go randomises map iteration on every run, and the kernel sorts wherever it iterates a map for output) |

### Inputs

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-10 | Strict parsing of plan, snapshot and policy: unknown fields, duplicate keys, wrong types and unknown enum values rejected (rule `input`). | Must | **Done**: `TestInputRejected` (one case per reason code, plus keys that differ from the schema only in case, schema keys in the wrong place, required fields, op parameters, assertions, names, uniqueness) |
| K-11 | Size limits: plan 64 KiB, snapshot 4 MiB, policy 64 KiB; at most 50 steps. | Must | **Done**: `TestInputRejected` (each input over its limit; 51 steps) |
| K-12 | The plan's `snapshot` must equal the snapshot's SHA-256. | Must | **Done**: `TestInputRejected` ("plan for another snapshot"), `TestVerdictBindsHashes` |
| K-13 | Parsing is fuzzed; no input panics the kernel. | Should | **Done**: `FuzzCheck`, run by `make fuzz` and in CI |

### Model and rules

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-20 | The seven operations behave as in [spec 03](../../spec/03-operations.md), each with unit tests for During and After states. | Must | **Done**: `TestOperations` (state after each operation, including the worst case for a moved pod) and `TestRules`. **Soundness fix, kernel 0.3.0 (2026-10-21):** a drained pod's candidates now keep its other candidate nodes; before, a pod created by a scale-up and possibly placed next to another on n2 was assumed to have left with the n1 drain, and a plan that takes the deployment to zero was proved. Found by reasoning about the Lean proof of `never-zero` (L-33), not by tests: the 305-input corpus and 10,000 generated plans never produced the case. Now in the case table (`TestRules`). **Second fix, kernel 0.4.0 (2026-10-21):** a drain of a node that may still run a rolling deployment's old pods is Unknown (`overlapping-change`), as spec 03 always said; the kernel had forgotten the old pods when the rollout started and proved such a drain, which can take a deployment without a budget to zero mid rollout. Also found while planning the Lean proof; of 10,000 generated plans, 10 reach the case, all already refuted for other reasons. In the case table |
| K-21 | All 13 rules in [spec 04](../../spec/04-rules-and-policy.md), each with at least one plan it rejects and a nearby plan it accepts. | Must | **Done**: `TestRules` (every rule both ways, including budgets and limits already broken in the snapshot) |
| K-22 | Every Unknown trigger in [spec 02](../../spec/02-snapshot-and-model.md#unknown-triggers) has a test that raises it. | Must | **Done**: `TestRules` ("unknown: …" cases, one per trigger) |
| K-23 | The `frame` rule compares the final desired fields with the snapshot and requires exactly the goal. Tested with extra changes, missing changes and changes that cancel out (scale up then down). | Must | **Done**: `TestRules` ("frame: …" cases) and `TestExamples` (plan-extra-change) |
| K-24 | The rollback is checked as a plan from the final state, and non-reversible fields are reported. | Must | **Done**: `TestRules` ("rollback: …" cases) and `TestExamples` (not-reversible facts in plan-proved) |
| K-25 | Soundness property test: for random small clusters and plans, a brute-force checker that enumerates every placement agrees with the kernel whenever the kernel says Proved. | Should | Planned |
| K-26 | Full branch coverage of `kernel/`; mutation testing finds no surviving mutant in rule code. | Should | **Partial**: statement coverage enforced at 95% or more in CI (`make cover`, 97% now); branch coverage and mutation testing not yet measured |

### Output

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-30 | The verdict carries the three input hashes, the kernel version and the facts in [spec 05](../../spec/05-verdict.md), in a fixed order. | Must | **Done**: `TestExamples` (golden verdicts), `TestVerdictBindsHashes` |
| K-31 | Fail closed: no error path returns Proved. Tested by injecting a failure at each stage. | Must | **Done**: a failure at each stage in `TestInputRejected` (input) and `TestRules` (static, step, end, rollback, Unknown); `FuzzCheck` fails on any Proved verdict with a failure, an Unknown or a check that didn't pass |
| K-32 | The worked examples in `examples/` pass as golden verdict tests. | Must | **Done**: `TestExamples` |
