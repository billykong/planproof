# C - Fetcher, executor and model fidelity

**Goal:** connect the kernel to a real cluster without widening what you must trust: a read-only fetcher that writes the snapshot, an executor that runs only Proved plans and stops on any surprise, and tests that compare the kernel's model with what Kubernetes actually does. Covers goal G5.

Status: **In progress**: phase 2 milestone 1 (fetcher) done. Runs on kind (1 control plane, 4 workers, about 2 CPUs and 3 GiB allocatable each) on WSL2: `make kind-up`, then `make kind-test`. The kind tests don't run in CI yet.

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
| C-10 | Runs a plan only if the verdict is Proved, its plan, snapshot and policy hashes and kernel version match, and an approval of the plan hash exists when the verdict requires one. | Must | Planned |
| C-11 | Compiles each step to API calls with a fixed, pure function (`compile(plan)`), unit-tested per operation. | Must | Planned |
| C-12 | Before the first step, re-fetches and compares the `resourceVersion` of every object the plan depends on with the snapshot; on any difference, stops without changing anything. | Must | Planned |
| C-13 | Before each step, checks the step's `pre` assertions against live state; writes with `resourceVersion` preconditions. | Must | Planned |
| C-14 | Drains with the Eviction API only, never pod deletion (SEC-07). | Must | Planned |
| C-15 | A `wait` that times out stops the run, records the state, and offers the checked rollback to a person; it never rolls back on its own in v0. | Must | Planned |
| C-16 | RBAC limited to the seven operations (SEC-06). | Must | Planned |
| C-17 | Translation check: after a run, the API audit log entries for the executor's identity match `compile(plan)` exactly. | Should | Planned |

### Model fidelity

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| C-20 | For each operation, a kind test runs it and compares the observed state (desired fields, ready counts, the minimum ready count sampled during the step) with the kernel's During and After states. | Must | Planned |
| C-21 | Each example plan runs on kind; the minimum ready count per deployment observed during the run is never below the kernel's predicted lowest. | Must | Planned |
| C-22 | Each Unknown trigger has a kind fixture showing the behaviour the model would get wrong (e.g. an HPA changing replicas mid-plan). | Should | Planned |
| C-23 | Known differences between model and cluster are listed in `docs/known-gaps.md` with an ID, like `safe-ops-agent-k8s`. | Must | Planned |
