# planproof specification, v0

This is the normative definition of what the kernel accepts and how it decides. The [PRD](../prd/README.md) says why; this says exactly what. If the code and this spec disagree, one of them is a bug.

| Document | Defines |
| --- | --- |
| [01-plan.md](01-plan.md) | The plan: goal, steps, assertions, rollback |
| [02-snapshot-and-model.md](02-snapshot-and-model.md) | The snapshot the fetcher writes; the kernel's abstract state; assumptions; Unknown triggers |
| [03-operations.md](03-operations.md) | What each of the seven operations does to the abstract state |
| [04-rules-and-policy.md](04-rules-and-policy.md) | The rules checked at every state and at the end; the policy file |
| [05-verdict.md](05-verdict.md) | The check algorithm, the verdict and the facts it carries |
| [06-review-packet.md](06-review-packet.md) | The packet a person reads: structure, writing rules, templates |
| [glossary.md](glossary.md) | Every term the packet uses, with one meaning each |

## Conventions

- "Must", "must not" and "may" are requirements on the kernel unless stated otherwise.
- All three inputs have `apiVersion: planproof/v0`. Any other version is rejected.
- The kernel reads JSON. The CLI also accepts YAML for plans and policies and converts it to compact JSON with sorted keys before calling the kernel; examples here use YAML for readability. The conversion is strict ([T-03](../prd/areas/t-cli.md)): one document; no anchors, aliases, merge keys, floats, timestamps or custom tags; no duplicate keys. Snapshots are JSON only.
- Hashes are SHA-256 of the exact bytes of the JSON the kernel received, written `sha256:<hex>`.
- Deployments are named `namespace/name`. Nodes are named by name.
- Quantities are integers: CPU in millicores (`cpuMilli`), memory in MiB (`memoryMiB`), pod slots (`pods`). The fetcher converts and rounds requests up.

## The guarantee

If the kernel returns **Proved** for `(snapshot, plan, policy)`, then, **under the assumptions in [02](02-snapshot-and-model.md#assumptions)**, running the steps in order from the state in the snapshot:

1. never breaks a rule in [04](04-rules-and-policy.md) at any state the model considers, including the worst case during each step; and
2. ends in a state whose changes, compared with the snapshot, are exactly the plan's goal.

Every assumption that matters for a given plan is listed in that plan's packet under "Not checked".
