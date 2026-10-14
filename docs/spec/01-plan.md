# 01 - The plan

A plan is what the agent submits. It is untrusted. The kernel treats every part of it as a claim to check.

## Example

```yaml
apiVersion: planproof/v0
kind: Plan
metadata:
  name: drain-two-nodes
  ticket: "OPS-1042: upgrade nodes n1 and n2"   # free text, shown unchecked
snapshot: sha256:9f2c…                           # the snapshot this plan was written for
rationale: "Cordon both first so moved pods can't land on n2."  # free text, shown unchecked
goal:
  - {node: n1, field: unschedulable, from: false, to: true}
  - {node: n2, field: unschedulable, from: false, to: true}
  - {node: n1, field: drained, from: false, to: true}
  - {node: n2, field: drained, from: false, to: true}
steps:
  - {op: cordon, node: n1, pre: {unschedulable: false}}
  - {op: cordon, node: n2, pre: {unschedulable: false}}
  - {op: drain, node: n1}
  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}
  - {op: drain, node: n2}
  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}
rollback:
  - {op: uncordon, node: n2}
  - {op: uncordon, node: n1}
```

## Fields

| Field | Required | Meaning |
| --- | --- | --- |
| `apiVersion`, `kind` | Yes | `planproof/v0`, `Plan` |
| `metadata.name` | Yes | A Kubernetes-style name (lowercase, digits, `-`, at most 63 characters) |
| `metadata.ticket` | No | Free text, at most 200 characters. Shown quoted, as unchecked |
| `snapshot` | Yes | `sha256:<hex>` of the snapshot. Must equal the hash of the snapshot passed to the kernel |
| `rationale` | No | Free text, at most 1,000 characters. Shown quoted, as unchecked |
| `goal` | Yes | A non-empty list of changes (below). The "theorem" |
| `steps` | Yes | A non-empty list of at most 50 steps |
| `rollback` | No | A list of steps that undoes the goal's reversible changes |

Unknown fields, duplicate keys and any value of the wrong type are rejected. The whole plan must be at most 64 KiB.

## Goal changes

Each change names one object, one field, its value in the snapshot (`from`) and its value at the end of the plan (`to`). The object is either `deployment: ns/name` or `node: name`, never both.

| Object | `field` | Type | Notes |
| --- | --- | --- | --- |
| Deployment | `replicas` | integer | |
| Deployment | `image` | string | Needs `container: <name>` |
| Deployment | `restarted` | boolean | `from: false, to: true`. A rollout restart |
| Node | `unschedulable` | boolean | Cordon (`true`) or uncordon (`false`) |
| Node | `drained` | boolean | `from: false, to: true`. All movable pods are off the node at the end |

A field may appear at most once per object. `from` must equal the value in the snapshot (for `restarted` and `drained`, always `false`).

There is no "frame" field: the frame is always closed. Anything the plan changes that is not in the goal is a failure (rule `frame`).

## Steps

| `op` | Parameters | Meaning (full semantics in [03](03-operations.md)) |
| --- | --- | --- |
| `scale` | `deployment`, `replicas` | Set the number of pods |
| `set-image` | `deployment`, `container`, `image` | Change one container's image (a rolling update) |
| `rollout-restart` | `deployment` | Replace all pods (a rolling update) |
| `cordon` | `node` | Stop new pods on the node |
| `uncordon` | `node` | Allow new pods on the node again |
| `drain` | `node` | Move all movable pods off the node (by eviction) |
| `wait` | `deployments`, `timeoutSeconds` | Wait until the listed deployments are ready; stop the run if the time passes |

### Assertions

Any step may carry `pre` and `post` assertions about the object it acts on. A `wait` step has no object and takes no assertions.

| Key | Applies to | Meaning |
| --- | --- | --- |
| `replicas` | Deployment | The deployment's replica count |
| `image` | Deployment | Needs `container`; that container's image |
| `unschedulable` | Node | The node's cordon state |
| `podsOnNode` | Node | Number of non-DaemonSet pods on the node |

The kernel checks every assertion against its model; a false assertion refutes the plan. The executor checks `pre` assertions again against the live cluster just before the step, so they double as runtime guards. Assertions are optional: they make the agent's expectations explicit and the run safer, but the kernel's rules don't depend on them.

## Rollback

`rollback` is a list of steps, run from the plan's final state, that restores every reversible goal field (`replicas`, `image`, `unschedulable`) to its `from` value. `drained` and `restarted` can't be reversed: pods don't move back. The kernel checks a rollback as a plan in its own right (rules at every state) and reports what it does not restore.
