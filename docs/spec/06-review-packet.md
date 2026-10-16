# 06 - The review packet

The packet is what a person reads before approving a plan. Its job: let a person understand, in about a minute, what will change, what the kernel checked, and what it did not.

The renderer makes the packet from the verdict's facts with fixed templates. Nothing in it is written by a model. Free text from the plan appears only quoted, in a section that says the kernel did not check it.

## Writing rules

The packet follows the principles of ASD-STE100 (Simplified Technical English), the controlled language of aerospace maintenance manuals, with the project's own word list. The rules, in our words:

| ID | Rule |
| --- | --- |
| W1 | One fact or one instruction per sentence. |
| W2 | Instructions have at most 20 words. Other sentences have at most 25. |
| W3 | Use the active voice. Write instructions in the imperative ("Move all pods off node `n1`."). |
| W4 | Use only words in the word list, each with its listed meaning ([glossary](glossary.md)), plus object names, numbers and units. |
| W5 | Do not use verb forms that end in "-ing". |
| W6 | Write numbers as digits, with a unit when there is one. |
| W7 | Write object names exactly as in the cluster, in code font. |
| W8 | Use CAUTION for a risk to the service, and NOTE for information. Put the instruction first, then the reason. (STE keeps WARNING for risks to people; the packet doesn't use it.) |
| W9 | Do not use these words: safe, safely, guaranteed, verified, ensure, probably, should, simply, just, obviously, fine. |
| W10 | Say the same thing with the same words: one template per fact. |

"Proved" is never written as "safe". It is always "The kernel found no violation of the N rules in section 3." The "Not checked" section is always present.

## Structure

Always in this order. Sections with nothing to show still appear, with "None."

```text
PLAN CHECK: <plan name>
Plan <hash>. Snapshot <hash>. Policy <hash>. Kernel <version>.

[CAUTION block, for Refuted or Unknown only]

1. GOAL            what will change; always ends "The plan must change nothing else."
2. RESULT          one line, then the approval line
3. CHECKS DONE     each rule: title, outcome, the numbers behind it
4. NOT CHECKED     assumptions, the goal itself, unchecked text from the agent
5. PROCEDURE       the steps, as numbered instructions
6. ROLLBACK        the rollback steps, and what it does not undo
```

The CAUTION block comes first because STE puts a safety instruction before the text it applies to. Hashes show the first 12 hex characters.

## Templates

The authoritative list of every sentence is [render/templates.go](../../render/templates.go): one named template per sentence form, linted against the rules above. The tables here describe them.

`{d}` is a deployment (`ns/name`), `{n}` a node, `{s}` a step number. Plurals are chosen by the number ("1 pod is", "0 pods are").

### Goal

| Change | Sentence |
| --- | --- |
| node `unschedulable` false → true | Stop new pods on node `{n}`. |
| node `unschedulable` true → false | Allow new pods on node `{n}`. |
| node `drained` | Move all pods off node `{n}`. |
| deployment `replicas` | Change the number of `{d}` pods from {from} to {to}. |
| deployment `image` | Change the image of container `{c}` in `{d}` from `{from}` to `{to}`. |
| deployment `restarted` | Replace all `{d}` pods. |

Then: "The plan must change nothing else."

Inside `frame` evidence, changes are described, not instructed:

| Change | Sentence |
| --- | --- |
| node `unschedulable` false → true | Node `{n}` stops new pods. |
| node `unschedulable` true → false | Node `{n}` accepts new pods again. |
| node `drained` | The plan moves all pods off node `{n}`. |
| deployment `replicas` | The number of `{d}` pods changes from {from} to {to}. |
| deployment `image` | The image of container `{c}` in `{d}` changes from `{from}` to `{to}`. |
| deployment `restarted` | The plan replaces all `{d}` pods. |

### Result and approval

| Fact | Sentence |
| --- | --- |
| Proved | PROVED. The kernel found no violation of the {k} rules in section 3. |
| Refuted | REFUTED. The plan breaks {k} rules {where}. Do not run this plan. `{where}` is "at step {s}", "before the first step" (static rules), "after the last step" or "at rollback step {s}". ("1 rule" when {k} is 1.) |
| Refuted, input | REFUTED. The kernel cannot read the input files. Do not run this plan. (The title then reads "(the kernel cannot read the inputs)".) |
| Unknown | UNKNOWN. The kernel cannot check step {s}. A person must decide. |
| Approval required | A person must approve this plan before it runs. Then one sentence per reason (one per critical deployment, from the approval fact's `evidence.critical`): "Reason: the plan moves pods off nodes." / "Reason: the plan changes or moves pods of a critical deployment, `{d}`." / "Reason: the plan decreases the number of pods." / "Reason: the plan changes an image." / "Reason: the policy requires approval for all plans." |
| Approval not required | The policy lets this plan run without approval. |

### Checks done

One numbered item per rule, in the order of the table below: "{title}: {OUTCOME}." Then the evidence sentences on the next lines.

| Outcome | Written as |
| --- | --- |
| `pass` | PASS. |
| `fail` | FAIL. |
| `incomplete` | INCOMPLETE. The kernel stopped at step {s}. (No evidence lines.) |
| `not-reached` | NOT CHECKED. The kernel stopped before this check. |
| `not-applicable` | NOT APPLICABLE. The plan has no rollback. |

| Rule | Title | Evidence sentence |
| --- | --- | --- |
| `input` | Input files | The plan, snapshot and policy are complete and match. |
| `scope` | Plan scope | The plan names only managed nodes and deployments. |
| `protected` | Protected deployments | The plan names no protected deployment. |
| `limits` | Limits | At most {x} nodes stop new pods at the same time. The limit is {lim}. / At least {y} nodes accept new pods at all times. The limit is {lim}. ("1 node stops" when {x} is 1.) |
| `assertions` | Step checks | The {k} step checks are true. ("The 1 step check is true." / "The plan has no step checks.") |
| `cordon-first` | Stop new pods before you move pods | Each node stops new pods before the plan moves pods off it. |
| `no-orphans` | Pods with no owner | No step removes a pod that has no owner. |
| `budget` | Disruption budgets | The lowest number of ready `{d}` pods is {x}, {phase} step {s}. The budget needs at least {needs}. (If the number never drops: "The number of ready `{d}` pods stays at {x}.") One line per budget, by deployment name |
| `never-zero` | At least 1 ready pod | Each deployment keeps at least 1 ready pod. (On failure: "The lowest number of ready `{d}` pods is 0, {phase} step {s}.") |
| `capacity` | Space on nodes | Node `{n}` has the least free space after step {s}: {cpu} millicores of CPU and {mem} MiB of memory. |
| `settled` | All pods ready at the end | The plan ends with all pods ready. |
| `frame` | Only the goal changes | The plan changes the {k} items in the goal and nothing else. (On failure: the same sentences as the CAUTION block.) |
| `rollback` | Rollback | The rollback obeys all rules. It restores {k} items. |

`{phase}` is "during" or "after".

### CAUTION block

Refuted:

```text
CAUTION: DO NOT APPROVE THIS PLAN.
<one or two sentences per failure fact>
```

| Failure | Sentences |
| --- | --- |
| `budget` | {Phase} step {s}, only {x} `{d}` pods are ready. The disruption budget for `{d}` needs at least {needs}. ("only 1 `{d}` pod is ready"; "no `{d}` pods are ready" when {x} is 0.) |
| `never-zero` | {Phase} step {s}, `{d}` has no ready pods. Each deployment must keep at least 1 ready pod. |
| `capacity` | After step {s}, no node has space for a new `{d}` pod. |
| `frame`, extra | The plan makes a change that the goal does not list. Then one change sentence (below) per extra change. |
| `frame`, missing | The plan does not make a change that the goal lists. Then one change sentence per missing change. |
| `frame`, static | "The goal lists `{field}` of `{object}` more than 1 time." / "The goal says that `{field}` of `{object}` is {from} now. The snapshot shows {actual}." / "The goal changes `{field}` of `{object}` to the value that it has now." |
| `cordon-first` | Step {s} moves pods off node `{n}`. Node `{n}` still accepts new pods. |
| `no-orphans` | Step {s} removes pod `{p}`. This pod has no owner, so it does not come back. |
| `settled` | The plan ends before the new `{d}` pods are ready. Add a step that waits for `{d}`. |
| `assertions` | Step {s} expects `{key}` of `{object}` to be {expected}. The value is {actual}. |
| `scope` | Not managed: "The plan names `{object}`. The policy does not let plans change it." Not in the snapshot: "The plan names `{object}`. The snapshot does not have it." (or "container `{c}` in `{object}`") |
| `protected` | The plan names `{object}`. The policy does not let plans change it. |
| `limits` | "{When}, {x} nodes stop new pods. The limit is {lim}." / "{When}, only {x} nodes accept new pods. The limit is {lim}." / "The plan asks for {x} pods. The limit is {lim}." / "A wait step can take {x} seconds. The limit is {lim} seconds." / "The image `{image}` has no digest. The policy needs a digest." |
| `input` | The {plan / snapshot / policy} file is not correct. Problem: `{reason}` at `{path}`. (Without "at" when there is no path.) |
| `rollback` | At a rollback step: the inner rule's sentences, which already say "rollback step {s}". Fields not restored: "The rollback does not restore this item." then one change sentence per field. |

`{When}` is "During step {s}" or "After step {s}" (or "rollback step {s}", or "the last step").

Unknown:

```text
CAUTION: THE KERNEL CANNOT CHECK THIS PLAN.
DO NOT APPROVE IT WITHOUT A FULL REVIEW BY A PERSON.
<one sentence pair for the trigger>
```

| Trigger | Sentences |
| --- | --- |
| `hpa` | Step {s} affects `{d}`. An autoscaler controls `{d}`, and the kernel does not model autoscalers. |
| `pdb-unresolved` | Step {s} affects `{d}`. The kernel cannot read the disruption budget for `{d}`. |
| `placement-rules` | Step {s} makes new `{d}` pods. These pods have placement rules, and the kernel does not model them. |
| `tainted-nodes` | Step {s} makes new pods. Node `{n}` has a taint, and the kernel does not model taints. |
| `stateful-owner` | Step {s} moves pod `{p}`. The kernel does not model the owner of this pod. |
| `local-storage` | Step {s} moves pod `{p}`. This pod has local data, and the data does not move with it. |
| `node-not-ready` | Step {s} names node `{n}`. Node `{n}` is not ready. |
| `unsettled-start` | Step {s} affects `{d}`. In the snapshot, not all `{d}` pods are ready. |
| `overlapping-change` | Step {s} affects `{d}` before the last change to `{d}` is complete. Add a step that waits for `{d}`. |

### Not checked

Always first: "The kernel does not check that this goal is the correct goal for the request. You must check this."

Each assumption line starts with its ID ("A1.").

| Assumption | Sentences |
| --- | --- |
| A1 | The kernel thinks that pods that are ready now stay ready. Before each step, the runner examines the cluster again. |
| A2 | The kernel does not check that the new pods from steps {s} start. If a wait takes too long, the runner stops the plan. |
| A3 | The kernel uses a simple model of the scheduler. The scheduler can put pods on different nodes. |
| A4 | The kernel thinks that nothing else changes these objects during the plan. If an object changes, the runner stops. |
| A5 | The kernel thinks that the snapshot is correct. |

Unchecked text: "The agent wrote this text. The kernel did not check it." then each field quoted, prefixed by "Ticket:" or "Reason from the agent:".

### Procedure and rollback

| Step | Sentence |
| --- | --- |
| `cordon` | Stop new pods on node `{n}`. |
| `uncordon` | Allow new pods on node `{n}`. |
| `drain` | Move all pods off node `{n}`. |
| `scale` | Change the number of `{d}` pods to {r}. |
| `set-image` | Change the image of container `{c}` in `{d}` to `{image}`. |
| `rollout-restart` | Replace all `{d}` pods. |
| `wait` | Wait until `{d1}` and `{d2}` are ready. Stop the plan if this takes more than {t}. ({t} in minutes when it divides by 60, else in seconds.) |

A `pre` assertion adds an indented line: "First, the runner makes sure that {claim}." A `post` assertion adds "After this step, the kernel expects that {claim}." Claims: "the number of `{d}` pods is {x}", "container `{c}` in `{d}` has the image `{image}`", "node `{n}` accepts new pods", "node `{n}` does not accept new pods", "node `{n}` has {x} pods".

With no rollback: "None. The plan has no rollback." After the rollback steps, one NOTE per `not-reversible` fact: "NOTE: The rollback does not move pods back to node `{n}`." / "NOTE: The rollback does not bring back the old `{d}` pods."

## Requirements on the renderer

- **Pure and deterministic**: the same verdict gives the same bytes.
- **Complete**: every fact produces at least one sentence, and every sentence comes from a fact, except the fixed structure (`head.*` templates and "The plan must change nothing else."). Tested by tagging each line with its fact index or structure template (`render.Debug`).
- **Every template reached**: some verdict in the test corpus (the examples and every case in `internal/testcases`) uses each template, so the lint and golden tests cover everything a person can read.
- **No free text** except `unchecked-text` facts, which are quoted, stripped of control characters and cut to their maximum length.
- **Linted**: CI fills every template with sample values and checks W2 (length), W4 (word list: [render/words.txt](../../render/words.txt), which must also contain no unused word), W5 ("-ing", with an allowlist of words that are not verb forms: "bring", "during", "nothing"), W9 (banned words) and a passive-voice heuristic. A template that fails the lint fails the build.
- **Golden tests**: each example's expected packet is a file in the repo, compared byte for byte.
