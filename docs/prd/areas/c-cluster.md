# C - Fetcher, executor and model fidelity

**Goal:** connect the kernel to a real cluster without widening what you must trust: a read-only fetcher that writes the snapshot, an executor that runs only Proved plans and stops on any surprise, and tests that compare the kernel's model with what Kubernetes actually does. Covers goal G5.

Status: **In progress**: phase 2 milestones 1-3 (fetcher, executor, model fidelity) done; the translation check (C-17) is next. Runs on kind (1 control plane, 4 workers, about 2 CPUs and 3 GiB allocatable each) on WSL2: `make kind-up`, then `make kind-test`. The kind tests don't run in CI yet.

## User stories

- As an **operator**, a Proved plan runs exactly as the packet describes, and stops if the cluster isn't as the kernel expected.
- As a **security reviewer**, I can see that the executor can't do more than the seven operations, and that what it did matches the plan.
- As a **kernel maintainer**, when the model and Kubernetes disagree, a test tells me which operation and which field.

## Requirements

### Fetcher

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| C-01 | The fetcher writes a snapshot in the [spec 02](../../spec/02-snapshot-and-model.md#the-snapshot) format, resolving owners, quantities, strategies, PDB selectors, autoscalers and placement rules. | Must | **Done**: `fetch` and `cmd/planproof-fetch`; `TestSnapshot` (fake clientset: rounding, effective requests, owner chains, skipped pods, budgets resolved and unresolved, autoscalers, taints, hard vs soft placement rules; the kernel parses the result) |
| C-02 | Its identity can only `get`/`list` the objects it reads (tested with `kubectl auth can-i`). | Must | **Done**: `TestFetcherRBAC` (SubjectAccessReviews, 6 allowed and 8 denied, on kind); `TestFetchFixture` fetches with the fetcher's own identity |
| C-03 | Fixture tests: known manifests applied to kind give a known snapshot (ignoring `resourceVersion` and times). | Must | **Done**: `TestFetchFixture` on kind (pause-image deployments, budgets, an HPA; checked field by field, since pod placement varies) |

### Executor

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| C-10 | Runs a plan only if the verdict is Proved, its plan, snapshot and policy hashes and kernel version match, and an approval of the plan hash exists when the verdict requires one. | Must | **Done**: `executor.Admit` re-runs the kernel rather than trust a verdict file; `TestAdmit` (refuted, missing or wrong approval, a verdict for other inputs, an edited verdict, a changed snapshot); `TestExecuteRefusesWithoutApproval` on kind. Limit: [GAP-02](../../known-gaps.md#gap-02-approval-is-a-flag-not-an-authenticated-person) |
| C-11 | Compiles each step to API calls with a fixed, pure function (`compile(plan)`), unit-tested per operation. | Must | **Done**: `executor.Compile`; `TestCompile` (each operation; deterministic; restarts named by plan hash and step, not a time) |
| C-12 | Before the first step, re-fetches and compares every object the plan depends on with the snapshot; on any difference, stops without changing anything. It compares the fields the kernel used, not `resourceVersion`s: a node's changes every few minutes from kubelet status updates alone (wording changed 2026-10-16). | Must | **Done**: `TestExecuteStopsOnDrift` on kind (no call made, the other writer's change intact). Limit: [GAP-04](../../known-gaps.md#gap-04-the-drift-check-covers-only-what-the-plan-depends-on) |
| C-13 | Before each step, checks the step's `pre` assertions against live state; writes with `resourceVersion` preconditions. | Must | **Done**: `TestExecuteStopsOnLiveAssertion` and `TestExecuteStopsOnConflict` on kind (a change between read and write gets 409 and stops the run) |
| C-14 | Drains with the Eviction API only, never pod deletion (SEC-07). | Must | **Done**: `TestExecuteDrain` on kind (only evictions); the executor has no `delete` on pods (`TestExecutorRBAC`) |
| C-15 | A `wait` that times out stops the run, records the state, and offers the checked rollback to a person; it never rolls back on its own in v0. | Must | **Done**: `TestExecuteWaitTimeout` on kind (a rollout that never gets ready: stopped at the wait, rollback offered in the report, not run) |
| C-16 | RBAC limited to the seven operations (SEC-06). | Must | **Done**: [deploy/rbac/executor.yaml](../../../deploy/rbac/executor.yaml); `TestExecutorRBAC` (5 allowed, 12 denied). Limit: [GAP-01](../../known-gaps.md#gap-01-the-executor-can-patch-any-field-of-a-node-or-deployment) |
| C-17 | Translation check: after a run, the API audit log entries for the executor's identity match `compile(plan)` exactly. | Should | Planned |

### Model fidelity

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| C-20 | For each operation, a kind test runs it and compares the observed state (desired fields, ready counts, the minimum ready count sampled during the step) with the kernel's During and After states. | Must | **Done**: `TestFidelityOperations` on kind (rollout restart with surge and with one unavailable, scale down, scale up, cordon/drain/uncordon): the real lowest never fell below the predicted lowest, and equalled it in every case; After states via each plan's waits and `frame`. Numbers: [results/kind-test.txt](../../../results/kind-test.txt). Limit: [GAP-05](../../known-gaps.md#gap-05-the-fidelity-tests-sample-every-100-ms) |
| C-21 | Each example plan runs on kind; the minimum ready count per deployment observed during the run is never below the kernel's predicted lowest. | Must | **Done**: `TestFidelityExample` (the proved plan: real lowest = predicted; the refuted no-wait plan: the cluster's Eviction API refused an eviction, agreeing with the kernel). `TestFidelityCapacity` checks the scheduler-fit assumption (A3) both ways: 8 pods the kernel says fit are placed, a 9th it says doesn't fit stays Pending |
| C-22 | Each Unknown trigger has a kind fixture showing the behaviour the model would get wrong (e.g. an HPA changing replicas mid-plan). | Should | **Partial**: 2 of 8 triggers in `TestUnknownTriggersAreReal`: `hpa` (an HPA changes replicas on its own) and `placement-rules` (with required anti-affinity hidden, the kernel would say Proved; on the cluster the moved pod stays Pending) |
| C-23 | Known differences between model and cluster are listed in `docs/known-gaps.md` with an ID, like `safe-ops-agent-k8s`. | Must | **Done** for the gaps found so far: [docs/known-gaps.md](../../known-gaps.md) (GAP-01 to GAP-04); model-versus-cluster differences are added as milestone 3 finds them |
