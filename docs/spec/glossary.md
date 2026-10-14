# Glossary

Each term has one meaning in the packet, the spec and the code. Words the packet uses that are not here have their ordinary dictionary meaning, and must be in the renderer's word list (W4).

| Term | Meaning | Not to be confused with |
| --- | --- | --- |
| agent | The AI system that writes a plan. Untrusted | |
| assumption | Something the kernel relies on but can't check (A1-A5) | |
| autoscaler | A HorizontalPodAutoscaler or similar controller that changes a deployment's number of pods | |
| check | (noun) One rule, applied by the kernel. (verb) Apply a rule | "verify": not used |
| deployment | A Kubernetes Deployment | |
| disruption budget | A PodDisruptionBudget: the minimum number of ready pods a deployment keeps during voluntary changes | |
| drain (code only) | Move all pods off a node. The packet says "move all pods off node X" | |
| cordon (code only) | Stop new pods on a node. The packet says "stop new pods on node X" | |
| goal | The exact list of changes a plan says it makes. The plan must make these and nothing else | The ticket or request |
| kernel | The small trusted program that checks a plan | The Linux kernel |
| node | A Kubernetes node | |
| owner | The controller that re-creates a pod (Deployment, DaemonSet, StatefulSet, Job) | |
| plan | The goal, the steps and the rollback the agent submits | |
| pod | A Kubernetes pod | |
| policy | The human-reviewed file of limits and scope the kernel applies | |
| Proved | The kernel found no rule broken at any state of its model | "safe": not used |
| ready | A pod passes its readiness check; for a deployment, all its pods are ready and up to date | |
| Refuted | The kernel found a rule broken; the packet says where | |
| rollback | Steps that undo the goal's reversible changes | |
| rule | One condition the kernel checks (`budget`, `frame`, …) | |
| runner | The executor: the component that runs a Proved plan | |
| snapshot | The state of the cluster that the fetcher recorded, which the kernel checks against | |
| step | One operation in the plan | |
| Unknown | The kernel can't check the plan, because it depends on something outside the model | |
