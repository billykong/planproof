# 04 - Rules and policy

## Rules

Every rule has a short ID, used in facts and in the packet. Rules are checked at three points:

- **Static**: once, before the first step.
- **Each state**: on the During and After state of every step, and of every rollback step.
- **End**: on the final state.

| ID | When | The plan breaks it if |
| --- | --- | --- |
| `input` | Static | An input is over its size limit or fails strict parsing, or the plan's `snapshot` hash doesn't match the snapshot |
| `scope` | Static | A goal or step names a deployment outside `managedNamespaces`, a node outside `managedNodes`, or an object not in the snapshot |
| `protected` | Static | A goal or step names a deployment in `protectedDeployments`. (A drain may still move its pods; `budget` and `never-zero` cover that.) |
| `limits` | Static and each state | Static: a `replicas` above `maxReplicas`, a `timeoutSeconds` above `maxWaitSeconds`, an image without a digest when `requireImageDigest` is set. Each state: more than `maxUnschedulableNodes` nodes unschedulable, or fewer than `minSchedulableNodes` nodes ready and schedulable |
| `assertions` | Each state | A `pre` assertion is false on the state before its step, or a `post` assertion is false on the state after it |
| `cordon-first` | Each state | A drain targets a node that is not unschedulable |
| `no-orphans` | Each state | A drain would evict a pod with no owner |
| `budget` | Each state | For a PDB on deployment `d`: available(d) < `minAvailable`, or available(d) < replicas(d) − `maxUnavailable` |
| `never-zero` | Each state | A deployment with replicas ≥ 1 in the snapshot has available 0, unless the goal sets its replicas to 0 and `allowScaleToZero` is true. This covers deployments with no PDB |
| `capacity` | Each state | A pod the model creates has no first-fit placement |
| `settled` | End | A deployment still has `pending` or `rolling` pods: the plan ends without waiting for its own changes |
| `frame` | Static and end | Static: a goal change's `from` differs from the snapshot, or equals its `to`, or a field repeats. End: the set of desired fields that differ from the snapshot is not exactly the goal, with the goal's `to` values |
| `rollback` | End | The rollback, run from the final state, breaks a rule at any state, or leaves a reversible goal field (`replicas`, `image`, `unschedulable`) different from its `from` value |

Rules are independent of each other and of the order they're listed in. At a failing step, the kernel reports every rule that fails there.

## Policy

The policy is data. A human reviews it like code; its hash is in every verdict and packet.

```yaml
apiVersion: planproof/v0
kind: Policy
managedNamespaces: [shop]
managedNodes: [n1, n2, n3, n4]
protectedDeployments: [shop/payments]
criticalDeployments: [shop/checkout]
approvalRequiredFor: [drain, critical, scale-down]
limits:
  maxReplicas: 20
  maxUnschedulableNodes: 2
  minSchedulableNodes: 2
  maxWaitSeconds: 600
  requireImageDigest: true
  allowScaleToZero: false
```

| Field | Meaning |
| --- | --- |
| `managedNamespaces`, `managedNodes` | The only objects a plan may name. Empty means none |
| `protectedDeployments` | Deployments no plan may name |
| `criticalDeployments` | Deployments whose involvement makes approval required (with `critical`) |
| `approvalRequiredFor` | When a Proved plan still needs a person to approve it: `always`; `drain` (any drain); `critical` (the plan names a critical deployment, or moves or replaces its pods); `scale-down`; `image` (any `set-image`) |
| `limits` | As in the `limits` and `never-zero` rules. All fields required |

Unknown fields are rejected. A missing policy means the kernel returns Refuted for every plan (`scope`: nothing is managed).

Approval is separate from the verdict: Refuted and Unknown plans can't run at all; a Proved plan runs straight away only if no `approvalRequiredFor` condition matches.
