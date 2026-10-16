# 03 - Security

## Threat model

The attacker's best tool is the agent: a prompt injection in a ticket, log or tool output, or a model that is simply wrong. Assume the agent produces the worst plan it can, and tries to make it look good.

| ID | Threat | Example | Control |
| --- | --- | --- | --- |
| T1 | Unsafe plan | Drains two nodes without waiting, breaking a disruption budget | Kernel simulates every step and checks rules at each state (K area) |
| T2 | Plan does more than its goal says | Goal says "scale checkout to 5"; a step also changes cart's image | Frame rule: the computed effect must equal the goal exactly (`frame`) |
| T3 | Goal hides the real change in friendly words | Rationale says "small fix"; goal sets replicas of payments to 0 | The packet renders the goal from typed fields, not from the agent's text; free text is labelled unchecked |
| T4 | Plan exploits a gap in the model | Relies on an HPA, an operator or a webhook the kernel doesn't model | Unknown triggers for everything outside the model (spec 02); model-fidelity tests |
| T5 | Stale or forged state | Agent supplies its own snapshot showing spare capacity | Snapshot comes only from the fetcher; plan binds its hash |
| T6 | Time-of-check / time-of-use | The cluster changes between check and run | Executor re-checks each step's preconditions live; writes with `resourceVersion` preconditions |
| T7 | Executor runs something else | Bug or tampering in the compile step | Fixed compile function; translation check of the audit log against `compile(plan)` |
| T8 | Misleading packet | Renderer drops a failing check or words it softly | Renderer completeness tests; no free-text generation; banned words ("safe", "probably") |
| T9 | Automation bias | Reviewer reads "Proved" as "safe" | "Proved" is always worded as "no violation of the N rules below"; "Not checked" is always shown; seeded-plan drills measure reviewers |
| T10 | Parser confusion | Duplicate keys, unknown fields, huge numbers, deep nesting | Strict JSON decoding, size limits, rejection of duplicates and unknown fields; fuzz tests |
| T11 | Kernel bug | An off-by-one in a budget check | Size budget, coverage, mutation testing, property tests, differential tests (and later a Lean model) |
| T12 | Policy weakened quietly | A limit raised in the policy file | Policy hash in every verdict and packet; policy changes reviewed like code |

## Security requirements

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| SEC-01 | The agent holds no credential that can write to the cluster. | Must | Planned |
| SEC-02 | The kernel imports only the Go standard library, performs no I/O, and has no access to a clock, the network or randomness. A CI check fails on any other import. | Must | **Done**: `make budget` (K-02) |
| SEC-03 | Every input the kernel reads is parsed strictly: unknown fields, duplicate keys, unknown enum values and inputs over the size limits are rejected. | Must | **Done**: `TestInputRejected` (K-10, K-11) |
| SEC-04 | Fail closed: any error, missing input or hash mismatch gives Refuted or Unknown, never Proved. | Must | **Done**: K-31 |
| SEC-05 | The verdict binds the SHA-256 of the plan, snapshot and policy and the kernel version; the executor checks all four. | Must | Planned |
| SEC-06 | The executor's identity can do only what the operation set needs (scale, image patch, restart annotation, node `unschedulable`, the Eviction API) and is tested with `kubectl auth can-i`. | Must | Planned |
| SEC-07 | The executor uses the Eviction API for drains, never pod deletion, so the cluster's own disruption budgets still apply if the model is wrong. | Must | Planned |
| SEC-08 | Text from the plan (`rationale`, `ticket`, names) is never interpreted, and appears in the packet only quoted, in a section marked as unchecked. Object names are checked against Kubernetes name rules. | Must | **Done**: names and lengths checked, unknown keys never echoed (`TestInputRejected`); free text quoted and neutralised in the packet (`TestUncheckedText`) |
| SEC-09 | The kernel and parser are fuzzed in CI. | Should | **Done**: `FuzzCheck` (`make fuzz`) |

## Residual risks

- **The model is not the cluster.** Proved means "no rule broken under the model". Controllers, webhooks and timing can still differ. The cluster's own guards (Eviction API, PDBs, `resourceVersion`) stay on as a second layer.
- **The goal can be the wrong goal.** The kernel checks that the plan matches its goal, not that the goal matches the ticket. That stays a human judgement, deliberately small.
- **Readiness is outside the model.** The kernel can't know whether a new image starts. Waits with timeouts in the plan, and the executor's stop on timeout, bound the damage.
- **The renderer is trusted.** A renderer bug misleads as much as a kernel bug. It gets the same test discipline.
