# Known gaps

Weaknesses we know about and have not closed, by stable ID. A gap is closed by a named test, never by deleting the entry: closed gaps stay, marked **Closed**, with the test.

## GAP-01: the executor can patch any field of a node or deployment

**What:** RBAC grants verbs on resources, not fields. The executor needs `patch` on nodes (for `spec.unschedulable`) and deployments (replicas, one container image, one annotation), so its identity could also change labels, taints or any other field.

**Why it matters:** the executor is trusted to send only what `Compile` produced. A bug or a compromised executor could do more than the plan.

**Mitigations now:** `Compile` is pure and unit-tested per operation (C-11); the executor runs no code from the plan.

**Options:** a ValidatingAdmissionPolicy that allows the executor's patches to change only the fields `Compile` can produce (CEL can compare `object` and `oldObject`); and the translation check against the API audit log (C-17), which detects rather than prevents.

## GAP-02: approval is a flag, not an authenticated person

**What:** `planproof-exec --approved-plan sha256:…` takes the approved plan hash on the command line. Whoever runs the executor can supply it.

**Why it matters:** the policy's "a person must approve" isn't enforced against an identity.

**Options:** take the approval from the approval-gate CRD of `safe-ops-agent-k8s`, where the approver is stamped from the authenticated request and the agent can't write approvals.

## GAP-03: the cluster tests don't run in CI

**What:** `make kind-test` (fetcher and executor on kind) runs only locally. CI runs everything that doesn't need a cluster.

**Options:** a CI job that creates the kind cluster, as `safe-ops-agent-k8s` does for its eval gate.

## GAP-04: the drift check covers only what the plan depends on

**What:** before the first step, the executor compares the nodes the plan names, the pods on them, the deployments it names or moves, and the budgets in their namespaces. A change elsewhere (for example a new pod scheduled onto a node the plan will drain later) isn't caught by that check.

**Mitigations now:** every node a plan drains is cordoned first (rule `cordon-first`), so no new pods land on it; the drain refuses any pod that isn't a Deployment's; every write carries a `resourceVersion` precondition.

## GAP-05: the fidelity tests sample every 100 ms

**What:** the model-fidelity tests (C-20, C-21) find the real lowest number of ready pods by polling every 100 ms. A dip shorter than that can be missed.

**Why it matters only one way:** the tests check that the real lowest is at least the predicted lowest. A missed dip can hide a failure, never invent one.

**Options:** count from a watch on pods instead of polling, so every readiness change is seen.
