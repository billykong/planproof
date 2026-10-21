# 03 - Operations

Each operation is a pure function on the abstract state ([02](02-snapshot-and-model.md#the-abstract-state)). For each one, this page gives:

- **Requires**: conditions on the state before the step. If one is false, the plan is Refuted at this step.
- **Unknown if**: triggers from [02](02-snapshot-and-model.md#unknown-triggers) that this operation can raise.
- **During**: the worst-case state while the step runs. Rules are checked on it.
- **After**: the state when the step has finished. Rules are checked on it, then the next step starts from it.
- **Assumes**: the assumptions the packet lists for this step.

The model's simplifications are deliberate. Every one is on the conservative side (it may reject a safe plan), except where an assumption says otherwise. Each operation has model-fidelity tests on kind (PRD area C).

## scale

Parameters: `deployment`, `replicas` (integer ≥ 0). Let `r` be the current replicas and `n` the new value.

- **Requires**: none beyond the static rules.
- **Unknown if**: `hpa`, `overlapping-change`, `unsettled-start`; if `n > r`: `placement-rules`.
- **During** = **After**:
  - `replicas := n`.
  - If `n > r`: `n − r` new `pending` pods, candidate set = schedulable nodes, placed by first fit. If any doesn't fit, rule `capacity` fails.
  - If `n < r`: available(d) becomes `min(available(d), n)`. The model doesn't know which pods Kubernetes removes, so each remaining pod's candidate set becomes the union of the locations the deployment's pods had. Capacity they used is not freed within the plan (conservative).
- **Assumes**: A2, A3 when `n > r`.

## set-image

Parameters: `deployment`, `container`, `image`.

- **Requires**: the container exists in the deployment; if the policy sets `requireImageDigest`, `image` contains `@sha256:`.
- **Unknown if**: `hpa`, `overlapping-change`, `unsettled-start`, `placement-rules`.
- **During**: the deployment is mid rolling update. Available(d) is `replicas − maxUnavailable` (0 for `Recreate`). `maxSurge` extra pods must fit by first fit, on top of the existing pods; if not, rule `capacity` fails. (For `Recreate`, no surge.)
- **After**: `image[container] := image`. The deployment stays mid rolling update until a `wait` names it. Its pods are `rolling`, candidate set = schedulable nodes. For capacity, the old pods' requests are released and the new pods are placed by first fit.
- **Assumes**: A2, A3.

## rollout-restart

Parameters: `deployment`. Identical to `set-image`, except that **After** sets `restarted := true` and changes no image.

## cordon

Parameters: `node`.

- **Requires**: none beyond the static rules.
- **Unknown if**: `node-not-ready`.
- **During** = **After**: `unschedulable := true`. Pods already on the node or with it in their candidate set are not affected; pods created from now on can't have it as a candidate.

## uncordon

Parameters: `node`. **During** = **After**: `unschedulable := false`. If the node is ready and untainted, every pod still pending (made or moved by the plan and not yet ready) gains it as a candidate: the scheduler may place a pending pod on a node that becomes schedulable while it waits (kernel 0.5.0, 2026-10-21; until then candidates were fixed when the pod was made, and a plan that can take a deployment to zero was proved). Unknown if `node-not-ready`.

## drain

Parameters: `node`. Models `kubectl drain --ignore-daemonsets` through the Eviction API.

- **Requires**: the node is `unschedulable` (rule `cordon-first`); no pod on the node has `owner.kind: None` (rule `no-orphans`).
- **Unknown if**: `node-not-ready`, `stateful-owner`, `local-storage`, `overlapping-change` (a deployment with pods on the node is mid rolling update, counting the old pods it is replacing, which may still be on their nodes until a `wait` names it; kernel 0.4.0, 2026-10-21: until then the old pods were forgotten when the rollout started, and a drain of their node was proved although it can take the deployment below its rollout floor); and, for the moved pods, `placement-rules`, `hpa`, `pdb-unresolved`.
- **Moved pods**: every deployment pod that might be on the node: its known node is this node, or its candidate set contains this node (worst case). DaemonSet pods stay.
- **During**: all moved pods are gone at once (not available). This is the worst case: the Eviction API may move them one at a time, but the model doesn't rely on it.
- **After**: moved pods are `pending`. A moved pod was either on this node, and is now on a schedulable node, or on one of its other candidate nodes, and is still there. So its candidate set becomes the schedulable nodes (this node is cordoned, so never included) together with its previous candidates other than this node. Moved pods are placed by first fit; if any doesn't fit, rule `capacity` fails. `drained := true`. `podsOnNode` for this node is 0. (Until kernel 0.3.0, 2026-10-21, the candidate set became only the schedulable nodes. That forgot a pod could still be on another candidate node, and proved a plan that can take a deployment to zero: see the case table's "a pod that may have stayed on another candidate node".)
- **Assumes**: A2, A3.

Note: the real Eviction API refuses an eviction that would break a PDB, so a plan the model rejects for `budget` usually stalls in practice rather than causing an outage. It is still a wrong plan, and the kernel rejects it.

## wait

Parameters: `deployments` (non-empty list), `timeoutSeconds` (1 to the policy's `maxWaitSeconds`).

- **Requires**: each listed deployment exists in the snapshot.
- **During**: unchanged from the previous state.
- **After**: for each listed deployment, `pending` and `rolling` pods become `ready` at their candidate sets, and any rolling update ends. Deployments not listed are unchanged.
- **Assumes**: A2, A3 for each listed deployment that had pods pending or rolling.

The executor waits until each listed deployment has `readyReplicas == updatedReplicas == replicas` and its observed generation is current, and stops the whole run if `timeoutSeconds` passes first.
