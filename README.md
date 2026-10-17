# planproof

planproof is a review kernel for AI-proposed operations plans: a small, trusted checker, with results in plain English.

An agent writes a plan (a goal and the steps to reach it). The kernel checks the plan against a snapshot of the cluster and a policy, simulating every step and checking rules such as disruption budgets and node capacity at every intermediate state. It also checks that the plan changes exactly what its goal says and nothing else. A person then reads a short review packet, written in a controlled language modelled on ASD-STE100, that says what was checked and what was not. A separate executor runs only plans the kernel proved.

The idea comes from proof assistants such as Lean: let the clever, untrusted part search, and trust only a checker small enough to read. Don't review the agent's plan; review the checker once, and the goal each time.

Kubernetes first. Status: **phases 1 to 3 done**. The kernel, the review-packet renderer and the CLI reproduce the verdicts and packets derived by hand before the code existed. On a kind cluster, a fetcher writes snapshots and an executor runs proved plans with live guards. Run for real, plans never did worse than the kernel predicted (in every case the real lowest matched the prediction exactly), and the API audit log shows the executor wrote exactly what the plan compiled to. AI models then wrote plans for 13 tickets through the kernel: Claude Opus 5.5 and Sonnet 5.5 solved every one, Haiku 5.5 most, and no unsafe plan was proved ([results](results/agent-trials.md)). Next: measurement against the safe-ops gate and with reviewers (phase 4). See the [PRD status](docs/prd/README.md#status-at-a-glance).

## Try it

Needs Go (see `go.mod`).

```bash
make build
bin/planproof check --snapshot examples/drain-two-nodes/snapshot.json --plan examples/drain-two-nodes/plan-no-wait.yaml --policy examples/drain-two-nodes/policy.yaml --output packet
```

`check` prints the verdict as JSON (or, with `--output packet`, the packet a person reads) and exits 0 for Proved, 1 for Refuted, 2 for Unknown. `render` turns a saved verdict into a packet; `hash` prints the hash the kernel computes for a file, to put a snapshot's hash in a plan.

```bash
make ci        # gofmt, vet, tests, kernel budgets and coverage, the examples end to end, 30 s of fuzzing
```

The fetcher and the cluster tests need a kind cluster. It gets its own kubeconfig in `.kube/`, so nothing touches `~/.kube/config` or another cluster:

```bash
make kind-up     # 1 control plane + 4 workers, and the components' RBAC
make kind-test   # fetcher and executor on the cluster: permissions both ways, a real drain,
                 # and every way a run must stop (drift, conflict, live assertion, timeout);
                 # model fidelity (predicted vs real, in results/kind-test.txt); the audit-log
                 # translation check
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

Results: [results/agent-trials.md](results/agent-trials.md) (the agent trials: four models, 247 runs, every kernel reply in [results/trials.html](results/trials.html)), [results/kind-test.txt](results/kind-test.txt) (model fidelity on a real cluster), [results/seeded.md](results/seeded.md) (seeded bad plans).

| Path | Contents |
| --- | --- |
| [docs/prd/](docs/prd/README.md) | Product requirements: problem, goals, architecture, threat model, roadmap, requirements by area. Start here |
| [docs/spec/](docs/spec/README.md) | The normative spec: plan format, snapshot and model, operations, rules and policy, verdict, review packet, glossary |
| [examples/drain-two-nodes/](examples/drain-two-nodes/README.md) | A worked example: one snapshot, three plans, the expected packets |

## Related

Follows [safe-ops-agent-k8s](https://github.com/billykong/safe-ops-agent-k8s), a reference implementation of a safe operations agent on Kubernetes (sandboxing, approval gate, policy on effects, evals). planproof goes after a gap that project found: rules on single changes can't see multi-step plans, or tell when a plan does more than it was asked.

## License

[MIT](LICENSE)
