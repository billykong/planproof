# 05 - The check and the verdict

## The algorithm

`Check(snapshot, plan, policy []byte) Verdict` is a pure, deterministic function. The same three inputs always give byte-identical output.

1. **Inputs.** Check sizes, then parse each input strictly ([01](01-plan.md#fields), [02](02-snapshot-and-model.md#the-snapshot), [04](04-rules-and-policy.md#policy)). On any error: **Refuted**, rule `input`.
2. **Binding.** Hash the three inputs. If `plan.snapshot` is not the snapshot's hash: **Refuted**, rule `input`.
3. **Static rules**: `scope`, `protected`, `limits` (static part), `frame` (static part). On any failure: **Refuted**, at step 0, with every failing static rule.
4. **Steps.** Build the state from the snapshot. For each step, in order:
   1. If an Unknown trigger applies: **Unknown**, naming the trigger, the step and the object. Stop.
   2. Check the operation's requirements and the `pre` assertions on the current state.
   3. Compute the During state; check the each-state rules on it.
   4. Compute the After state; check the each-state rules and the `post` assertions on it.
   5. If anything failed in 2-4: **Refuted** at this step, with every failing rule and its evidence. Stop.
5. **End rules**: `settled`, then `frame`. On failure: **Refuted** at the end.
6. **Rollback**, if present: run it from the final state as in step 4 (an Unknown trigger here also gives **Unknown**), then check that reversible goal fields are restored. On failure: **Refuted**, rule `rollback`, with the inner rule and the rollback step.
7. **Proved.** Work out whether approval is required ([04](04-rules-and-policy.md#policy)).

The kernel stops at the first failing step: the model after a failure isn't meaningful. An Unknown at an earlier step wins over a Refuted at a later one for the same reason.

## The verdict

```json
{
  "apiVersion": "planproof/v0",
  "kind": "Verdict",
  "result": "Refuted",
  "kernelVersion": "0.1.0",
  "planSha256": "sha256:…",
  "snapshotSha256": "sha256:…",
  "policySha256": "sha256:…",
  "approvalRequired": false,
  "facts": [ … ]
}
```

`result` is `Proved`, `Refuted` or `Unknown`. `approvalRequired` is meaningful only for Proved. The executor runs nothing unless `result` is `Proved` and all three hashes and the kernel version match what it was given.

## Facts

The facts are everything the kernel found, as typed records. The renderer turns each fact into sentences ([06](06-review-packet.md)); the agent reads them to repair a plan. Facts are listed in a fixed order: by type in the order below, then by step, then by object name.

| `type` | Fields | When |
| --- | --- | --- |
| `goal-change` | `object` (`deployment/ns/name` or `node/name`), `field`, `container`?, `from`, `to` | Every goal entry, in the plan's order |
| `step` | `index` (from 1), `op`, parameters | Every step |
| `rollback-step` | `index`, `op`, parameters | Every rollback step |
| `not-reversible` | `object`, `field` | Each `drained` or `restarted` goal field, when a rollback is given |
| `check` | `rule`, `outcome`, `evidence` | Every rule. `outcome` is `pass`, `fail`, `incomplete` (no failure up to the step where the check stopped, later steps unchecked), `not-reached` (an end rule the check never reached) or `not-applicable` (`rollback` when there is none) |
| `failure` | `rule`, `step` (0 static, `end`, or an index; `rollback:<index>` for rollback steps), `phase` (`static`, `pre`, `during`, `after`, `end`), `evidence` | Each failing rule (Refuted) |
| `unknown` | `trigger`, `step`, `object` | Unknown |
| `assumption` | `id`, `steps` | Each assumption the plan depends on ([02](02-snapshot-and-model.md#assumptions)) |
| `approval` | `required`, `reasons` | Proved |
| `unchecked-text` | `field` (`ticket`, `rationale`), `text` | Each free-text field present |

### Evidence

Evidence holds the numbers behind an outcome, so the packet can show them. Each rule has fixed keys:

| Rule | Evidence keys |
| --- | --- |
| `budget` | `deployment`, `lowestAvailable`, `needs`, `step`, `phase` (one entry per PDB, the first state with the lowest value; `step` is null if the value never drops below the snapshot's) |
| `never-zero` | `deployment`, `lowestAvailable`, `step`, `phase` (only on failure, for the deployments that reached 0) |
| `capacity` | `node`, `cpuFreeMilli`, `memoryFreeMiB`, `step` (the tightest node after first fit); on failure, `pod` that didn't fit |
| `limits` | `maxUnschedulable`, `limitUnschedulable`, `minSchedulable`, `limitSchedulable`, with the step of each; on a static failure, the field and value |
| `frame` | On failure: `extra` (changes not in the goal) and `missing` (goal changes that didn't happen), each as goal-change records |
| `assertions` | `step`, `key`, `expected`, `actual` |
| `scope`, `protected`, `no-orphans`, `cordon-first`, `settled`, `input` | `object` and a fixed reason code |
| `rollback` | `innerRule`, `rollbackStep`, plus the inner rule's evidence; or `notRestored` records |

## Example: a Refuted verdict (abridged)

From [examples/drain-two-nodes](../../examples/drain-two-nodes/), the plan that drains n2 without waiting for the pods moved off n1:

```json
{"type": "failure", "rule": "budget", "step": 4, "phase": "during",
 "evidence": {"deployment": "shop/checkout", "lowestAvailable": 1, "needs": 2}},
{"type": "failure", "rule": "budget", "step": 4, "phase": "during",
 "evidence": {"deployment": "shop/cart", "lowestAvailable": 0, "needs": 1}},
{"type": "failure", "rule": "never-zero", "step": 4, "phase": "during",
 "evidence": {"deployment": "shop/cart", "lowestAvailable": 0}}
```
