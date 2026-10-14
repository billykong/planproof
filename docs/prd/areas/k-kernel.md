# K - The kernel

**Goal:** a pure function, small enough to read in an afternoon, that checks a plan against a snapshot and a policy and returns Proved, Refuted or Unknown, with the facts behind it. Covers goals G1, G2 and G3.

Status: **Planned**. Spec: [01](../../spec/01-plan.md) to [05](../../spec/05-verdict.md).

## User stories

- As an **approver**, I trust a Proved verdict because the code that produced it is small, tested, and I (or someone I trust) read it.
- As an **agent**, when my plan is Refuted, I get the step, the rule and the numbers, so I can fix the plan.
- As a **security reviewer**, I can list everything the kernel assumes, and see a test for each rule in both directions.

## Requirements

### Shape and size

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-01 | The kernel is one Go package, `kernel/`, exposing `Check(snapshot, plan, policy []byte) Verdict`. | Must | Planned |
| K-02 | It imports only the Go standard library, and does no I/O, no network calls, no reads of the clock or environment, no randomness and no goroutines. CI fails on any other import. | Must | Planned |
| K-03 | Non-test Go code in `kernel/` is at most 1,500 lines (counted by a script in CI). | Must | Planned |
| K-04 | Deterministic: the same inputs give byte-identical verdict JSON. Tested by running every golden case twice and with shuffled map iteration. | Must | Planned |

### Inputs

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-10 | Strict parsing of plan, snapshot and policy: unknown fields, duplicate keys, wrong types and unknown enum values rejected (rule `input`). | Must | Planned |
| K-11 | Size limits: plan 64 KiB, snapshot 4 MiB, policy 64 KiB; at most 50 steps. | Must | Planned |
| K-12 | The plan's `snapshot` must equal the snapshot's SHA-256. | Must | Planned |
| K-13 | Parsing is fuzzed; no input panics the kernel. | Should | Planned |

### Model and rules

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-20 | The seven operations behave as in [spec 03](../../spec/03-operations.md), each with unit tests for During and After states. | Must | Planned |
| K-21 | All 13 rules in [spec 04](../../spec/04-rules-and-policy.md), each with at least one plan it rejects and a nearby plan it accepts. | Must | Planned |
| K-22 | Every Unknown trigger in [spec 02](../../spec/02-snapshot-and-model.md#unknown-triggers) has a test that raises it. | Must | Planned |
| K-23 | The `frame` rule compares the final desired fields with the snapshot and requires exactly the goal. Tested with extra changes, missing changes and changes that cancel out (scale up then down). | Must | Planned |
| K-24 | The rollback is checked as a plan from the final state, and non-reversible fields are reported. | Must | Planned |
| K-25 | Soundness property test: for random small clusters and plans, a brute-force checker that enumerates every placement agrees with the kernel whenever the kernel says Proved. | Should | Planned |
| K-26 | Full branch coverage of `kernel/`; mutation testing finds no surviving mutant in rule code. | Should | Planned |

### Output

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| K-30 | The verdict carries the three input hashes, the kernel version and the facts in [spec 05](../../spec/05-verdict.md), in a fixed order. | Must | Planned |
| K-31 | Fail closed: no error path returns Proved. Tested by injecting a failure at each stage. | Must | Planned |
| K-32 | The worked examples in `examples/` pass as golden verdict tests. | Must | Planned |
