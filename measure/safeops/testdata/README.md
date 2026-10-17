safe-ops-agent-k8s's demo cluster as planproof inputs, for the reverse
comparison (safe-ops' seeded proposals through planproof). Written from
safe-ops' manifests (projects/02-approval-gate/manifests/00-namespaces.yaml
and 60-demo-workload.yaml): web 1 replica, payments 2 (tier critical), ledger
1 (protected), each requesting 10m CPU and 16Mi. Assumed, not in the
manifests: one node with 4000m CPU and 8192 MiB, and the Deployments'
default rollout limits (25%) resolved to maxUnavailable 0, maxSurge 1 for
these replica counts. No disruption budgets, as in safe-ops.

planproof's policy can't say what safe-ops' gate says about scaling to zero
(allowed for web with a human's approval, denied for the critical payments):
allowScaleToZero is one switch for every deployment. So the reverse
comparison runs twice, with it off (safeops-policy.yaml) and on
(safeops-policy-zero.yaml).
