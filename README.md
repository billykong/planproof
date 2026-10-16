# planproof

planproof is a review kernel for AI-proposed operations plans: a small, trusted checker, with results in plain English.

An agent writes a plan (a goal and the steps to reach it). The kernel checks the plan against a snapshot of the cluster and a policy, simulating every step and checking rules such as disruption budgets and node capacity at every intermediate state. It also checks that the plan changes exactly what its goal says and nothing else. A person then reads a short review packet, written in a controlled language modelled on ASD-STE100, that says what was checked and what was not. A separate executor runs only plans the kernel proved.

The idea comes from proof assistants such as Lean: let the clever, untrusted part search, and trust only a checker small enough to read. Don't review the agent's plan; review the checker once, and the goal each time.

Kubernetes first. Status: **phase 1**. The kernel works: it checks plans against the model and rules, and its verdicts on the worked examples match the ones derived by hand. The review-packet renderer and the CLI are next. See the [PRD status](docs/prd/README.md#status-at-a-glance).

## Build and test

Needs Go (see `go.mod`).

```bash
make ci        # gofmt, vet, tests, the kernel's budgets, 30 s of fuzzing
make budget    # kernel size, import allowlist, no goroutines or I/O
```

## Example

The kernel's answer to a plan that drains two nodes without waiting in between ([full packet](examples/drain-two-nodes/packet-no-wait.txt)):

```text
CAUTION: DO NOT APPROVE THIS PLAN.
During step 4, no `shop/cart` pods are ready. The disruption budget for `shop/cart` needs at least 1.
During step 4, only 1 `shop/checkout` pod is ready. The disruption budget for `shop/checkout` needs at least 2.
```

And to a plan whose every step is fine, but which also scales a deployment its goal doesn't mention ([full packet](examples/drain-two-nodes/packet-extra-change.txt)):

```text
CAUTION: DO NOT APPROVE THIS PLAN.
The plan makes a change that the goal does not list.
The number of `shop/cart` pods changes from 2 to 3.
```

## Documents

| Path | Contents |
| --- | --- |
| [docs/prd/](docs/prd/README.md) | Product requirements: problem, goals, architecture, threat model, roadmap, requirements by area. Start here |
| [docs/spec/](docs/spec/README.md) | The normative spec: plan format, snapshot and model, operations, rules and policy, verdict, review packet, glossary |
| [examples/drain-two-nodes/](examples/drain-two-nodes/README.md) | A worked example: one snapshot, three plans, the expected packets |

## Related

Follows [safe-ops-agent-k8s](https://github.com/billykong/safe-ops-agent-k8s), a reference implementation of a safe operations agent on Kubernetes (sandboxing, approval gate, policy on effects, evals). planproof goes after a gap that project found: rules on single changes can't see multi-step plans, or tell when a plan does more than it was asked.

## License

[MIT](LICENSE)
