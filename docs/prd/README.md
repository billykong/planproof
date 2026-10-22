# Product requirements: planproof

This folder is the product requirements document (PRD): what we're building, why, the requirements each part must meet, and how we'll know it works. The exact formats and semantics are in the [spec](../spec/README.md); requirements here cite it.

## Documents

| Document | Contents |
| --- | --- |
| [01-overview.md](01-overview.md) | Problem, product, goals and non-goals, design principles, success measures |
| [02-architecture.md](02-architecture.md) | Components, trust boundaries, trusted computing base, data flow |
| [03-security.md](03-security.md) | Threat model, cross-cutting security requirements, residual risks |
| [04-roadmap.md](04-roadmap.md) | Phases, milestones, open questions |
| [areas/k-kernel.md](areas/k-kernel.md) | K: the kernel |
| [areas/r-review-packet.md](areas/r-review-packet.md) | R: the review packet and its controlled language |
| [areas/c-cluster.md](areas/c-cluster.md) | C: fetcher, executor and model-fidelity tests |
| [areas/e-agent-and-evals.md](areas/e-agent-and-evals.md) | E: the agent loop, seeded plans and measurement |
| [areas/t-cli.md](areas/t-cli.md) | T: the `planproof` command and strict YAML conversion |
| [areas/l-lean.md](areas/l-lean.md) | L: the Lean model, proofs and differential tests (phase 5) |

## Status at a glance

| Area | Status |
| --- | --- |
| Phase 0: PRD, spec, worked example | **Done** (2026-10-14) |
| Phase 1: kernel, renderer, CLI | **Done** (2026-10-16): exit criteria met |
| Phase 2: fetcher, executor, fidelity, translation check | **Done** (2026-10-16) |
| K Kernel | **Phase 1 milestones 1-3 done**: budgets and CI; strict parsing and fuzzing; model, operations, rules and verdict, with golden examples. Open: K-25, K-26 |
| R Review packet | **Phase 1 milestone 4 done**: renderer, word list, template lint, golden packets. Open: R-09, R-11 |
| T CLI | **Done**: `check`, `render`, `hash`; strict YAML conversion. Open: an automated check for T-07 |
| C Cluster | **Done for phase 2**: fetcher, executor, model fidelity (the predicted lowest matched the real cluster exactly in every case), translation check. Open: C-22 (2 of 8 triggers); gaps in [known-gaps.md](../known-gaps.md) |
| E Agent and evals | **Phase 3 done**: Claude Opus 5.5 and Sonnet 5.5 39/39 over three runs, twice; Claude Haiku 5.5 36/39 with 2 plans proved with the wrong goal (a mistyped image digest), then 37/39 with none once kernel 0.2.0 checked the digest's form; Qwen2.5-1.5B 0/9 solvable (not a fair comparison); 25 seeded plans, none missed. **Phase 4 done**: safe-ops comparison, generated results, AI reviewers, prior art, [blog post](https://billykong.github.io/planproof/), [video](https://youtu.be/7fQ-RVWXN7E) (E-27; Shorts not yet uploaded); E-22 (people) dropped |
| L Lean model and proofs | **Phase 5 done** (2026-10-22): `Planproof.sound` proves the five core rules (frame, cordon-first, limits, budget and never-zero over every real run, capacity by witness) for the Lean checker, with only Lean's standard axioms; the Go kernel agrees with it on 305 corpus and 10,000 generated plans (tests, not proof). Found and fixed three kernel soundness bugs (0.3.0 to 0.5.0). Open: L-36 (the other rules), L-41 in the packet |

## Conventions

- **Requirement IDs** take the form `<area>-<nn>`, e.g. `K-03` or `SEC-02`. IDs are stable: a dropped requirement is marked *Dropped*, never renumbered.
- **Priority:** **Must** (the project isn't done without it), **Should** (expected; omission needs a stated reason) or **Could** (nice to have).
- **Status:** *Done* (an automated test proves it, and the test is named), *Partial*, *Planned* or *Open question*.
- "Done" means demonstrated by a test in this repo. A requirement met only by something nothing checks is *Partial*.
- Update this PRD in the same commit as the change that alters a requirement's status. Generate counts and results from the tests; don't type them in.
