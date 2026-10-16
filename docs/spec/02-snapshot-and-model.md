# 02 - Snapshot and model

## The snapshot

Written by the fetcher, a trusted component with a read-only identity. The agent never supplies it. The fetcher resolves Kubernetes details (owner chains, quantities, PDB selectors, default tolerations) so the kernel stays small.

```json
{
  "apiVersion": "planproof/v0",
  "kind": "Snapshot",
  "cluster": "kind-planproof",
  "fetchedAt": "2026-10-14T10:00:00Z",
  "nodes": [
    {"name": "n1", "ready": true, "unschedulable": false, "taints": false,
     "allocatable": {"cpuMilli": 4000, "memoryMiB": 8192, "pods": 110}, "resourceVersion": "1001"}
  ],
  "deployments": [
    {"namespace": "shop", "name": "checkout", "replicas": 3, "readyReplicas": 3, "updatedReplicas": 3,
     "strategy": {"type": "RollingUpdate", "maxUnavailable": 0, "maxSurge": 1},
     "containers": [{"name": "web", "image": "registry.example/checkout@sha256:aa11…"}],
     "podRequests": {"cpuMilli": 500, "memoryMiB": 512, "pods": 1},
     "placementRules": false, "hpa": false, "resourceVersion": "2001"}
  ],
  "pods": [
    {"namespace": "shop", "name": "checkout-a", "node": "n1", "ready": true,
     "owner": {"kind": "Deployment", "name": "checkout"},
     "requests": {"cpuMilli": 500, "memoryMiB": 512, "pods": 1},
     "localStorage": false, "placementRules": false}
  ],
  "pdbs": [
    {"namespace": "shop", "name": "checkout", "deployment": "checkout", "minAvailable": 2}
  ]
}
```

| Field | Meaning, as resolved by the fetcher |
| --- | --- |
| `nodes[].ready` | The node's `Ready` condition is `True` |
| `nodes[].taints` | The node has a `NoSchedule` or `NoExecute` taint, other than the `unschedulable` taint a cordon adds |
| `deployments[].strategy` | `type` is `RollingUpdate` or `Recreate`; `maxUnavailable` and `maxSurge` resolved to integers against `replicas` (unavailable rounded down, surge rounded up, as Kubernetes does) |
| `deployments[].podRequests` | Sum of the pod template's container requests, rounded up; `pods: 1` |
| `…placementRules` | The pod (template) has a `nodeSelector`, node or pod affinity or anti-affinity, topology spread constraints, or a toleration other than the defaults Kubernetes adds |
| `deployments[].hpa` | A HorizontalPodAutoscaler (or KEDA ScaledObject) targets it |
| `pods[].owner` | Resolved through ReplicaSets: `{kind: Deployment\|DaemonSet\|StatefulSet\|Job\|Other\|None, name}` |
| `pods[].localStorage` | The pod mounts an `emptyDir` volume |
| `pdbs[]` | Each PDB whose selector matches exactly the pods of one Deployment: `deployment`, and one of `minAvailable` or `maxUnavailable` as an integer. Otherwise the fetcher writes `deployment: null` and `unresolved: "<reason>"` |

The snapshot includes every node, every pod on every node, every deployment in managed namespaces, and every deployment that owns a pod on a node any plan step names. It must be at most 4 MiB.

## The abstract state

The kernel turns the snapshot into a state and changes that state step by step. The state has two layers.

**Desired fields**, compared with the goal by the `frame` rule:

| Object | Fields |
| --- | --- |
| Deployment | `replicas`, `image` per container, `restarted` |
| Node | `unschedulable`, `drained` |

**Pods.** Each modelled pod belongs to a deployment (or is a DaemonSet, StatefulSet, Job or unowned pod, which the kernel tracks only on nodes it drains). A deployment pod is in one of three conditions:

| Condition | Meaning | Counts as available? |
| --- | --- | --- |
| `ready` at a known node | From the snapshot, unchanged since | Yes |
| `ready` at one of several candidate nodes | Moved or replaced, then waited for; exact node unknown | Yes |
| `pending` with candidate nodes | Created by a step; not yet waited for | No |
| `rolling` with candidate nodes | Part of a rolling update not yet waited for | See below |

**Available(d)** is the number of pods of deployment `d` that are `ready`, at the state in question. While `d` is mid rolling update, available(d) is `max(0, replicas − maxUnavailable)`, the floor Kubernetes keeps during a rollout (0 for `Recreate`).

**Candidate nodes.** When the model creates a pod (eviction, rolling update, scale up), the pod's candidate set is the set of nodes that are `ready`, not `unschedulable` and not tainted, at that moment. The model never guesses where the scheduler put it.

**Worst case for disruption.** When a step disrupts a node, every pod that *might* be on it (a known node equal to it, or a candidate set containing it) is treated as being on it. This keeps budget checks sound without modelling the scheduler. The simplest way to make it exact is to cordon every node you will drain before the first drain, so moved pods can't be candidates for those nodes.

**Capacity model.** To check that pods fit, the kernel places them with first-fit-decreasing: pods by CPU request (largest first, then by name), nodes by name, each pod on the first candidate node with enough free CPU, memory and pod slots. Free is allocatable minus the requests of every modelled pod charged to the node (known pods, and pending or candidate-placed pods at the node first-fit chose for them). DaemonSet pods are charged and never move.

## Assumptions

Each is an "axiom": the kernel relies on it, can't check it, and lists it in the packet's "Not checked" section when the plan depends on it.

| ID | Assumption | When it's listed | How it's tested or guarded |
| --- | --- | --- | --- |
| A1 `ready-stays-ready` | Pods ready in the snapshot stay ready unless a step disrupts them | Always | Executor's live `pre` checks; wait steps |
| A2 `new-pods-start` | Pods the plan creates become ready by the next `wait` that names their deployment | Any step that creates pods (the fact names those steps) | `wait` timeout; executor stops the run |
| A3 `scheduler-fit` | If first-fit finds room, the real scheduler places the pods too | Any step that creates pods | Model-fidelity tests; `wait` timeout |
| A4 `no-other-actors` | Nothing else changes the modelled objects during the run | Always | `resourceVersion` preconditions; drift check before each step |
| A5 `snapshot-true` | The snapshot describes the cluster correctly | Always | Fetcher tests against known states |

## Unknown triggers

The kernel returns Unknown, naming the trigger, whenever the plan depends on something outside the model. "Depends on" means: the object is the target of a step, or its pods are on a node a step disrupts, or its pods must be placed.

| Trigger | Condition |
| --- | --- |
| `hpa` | A deployment the plan depends on has an autoscaler |
| `pdb-unresolved` | A PDB the plan depends on has `unresolved` set, or two PDBs cover one deployment |
| `placement-rules` | A pod the model must place has `placementRules: true` |
| `tainted-nodes` | A pod must be placed and some node is `ready`, not `unschedulable`, but tainted |
| `stateful-owner` | A node the plan drains has a pod owned by a StatefulSet, Job or `Other` |
| `local-storage` | A node the plan drains has a pod with `localStorage: true` |
| `node-not-ready` | A node the plan names is not `ready` |
| `unsettled-start` | A deployment the plan depends on has `readyReplicas` or `updatedReplicas` below `replicas` in the snapshot |
| `overlapping-change` | A step touches a deployment, or disrupts its pods, while that deployment is mid rolling update (add a `wait` first) |

Unowned pods (`owner.kind: None`) on a drained node are not Unknown: draining would delete them for good, so the plan is Refuted (rule `no-orphans`).
