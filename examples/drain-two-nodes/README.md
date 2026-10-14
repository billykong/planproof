# Example: drain two nodes

A four-node cluster with three deployments in `shop`, and three plans an agent might write for "upgrade nodes n1 and n2" (or just n1). The expected verdicts and packets were worked out by hand from the [spec](../../docs/spec/README.md); in phase 1 they become golden tests for the kernel and the renderer.

## The cluster ([snapshot.json](snapshot.json))

| Node | Pods (requests: checkout 500m/512Mi, cart 250m/256Mi, payments 500m/512Mi, kube-proxy 100m/64Mi) |
| --- | --- |
| n1 | checkout-a, cart-a, kube-proxy |
| n2 | checkout-b, cart-b, kube-proxy |
| n3 | checkout-c, payments-a, kube-proxy |
| n4 | payments-b, kube-proxy |

Each node: 4000m CPU, 8192 MiB. Disruption budgets: checkout `minAvailable: 2`, cart 1, payments 1. The [policy](policy.yaml) makes payments protected, checkout critical, and requires approval for any drain.

## The plans

| Plan | Expected | Why |
| --- | --- | --- |
| [plan-proved.yaml](plan-proved.yaml) | **Proved**, approval required ([packet](packet-proved.txt)) | Cordons both nodes first, so pods moved off n1 can't be candidates for n2, then drains one node at a time with a wait in between. Lowest point: 2 checkout and 1 cart ready, exactly the budgets |
| [plan-no-wait.yaml](plan-no-wait.yaml) | **Refuted** at step 4 ([packet](packet-no-wait.txt)) | Drains n2 while the pods moved off n1 are still pending: 1 checkout ready (needs 2), 0 cart ready |
| [plan-extra-change.yaml](plan-extra-change.yaml) | **Refuted** after the last step ([packet](packet-extra-change.txt)) | Every step keeps every rule, but the plan also scales cart from 2 to 3, which the goal doesn't list. A per-change policy gate would allow both changes; the `frame` rule catches the mismatch |

## Working for the Proved plan

| Step | State (CPU used on schedulable nodes) | checkout ready | cart ready |
| --- | --- | --- | --- |
| 1 cordon n1 | n1 unschedulable | 3 | 2 |
| 2 cordon n2 | n1, n2 unschedulable; n3, n4 schedulable (limit 2 each way) | 3 | 2 |
| 3 drain n1, during | checkout-a, cart-a gone | 2 | 1 |
| 3 drain n1, after | both pending, candidates {n3, n4}, first fit puts both on n3 (n3: 1850m) | 2 | 1 |
| 4 wait | both ready | 3 | 2 |
| 5 drain n2, during | checkout-b, cart-b gone; the moved pods can't be on n2 | 2 | 1 |
| 5 drain n2, after | both pending, first fit on n3 (n3: 2600m used, 1400m and 5568 MiB free) | 2 | 1 |
| 6 wait | all ready | 3 | 2 |

The plan and policy hashes in the packets are written `<plan>` and `<policy>` until the CLI exists: they're hashes of the JSON the CLI makes from the YAML.
