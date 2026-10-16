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

## Status at a glance

| Area | Status |
| --- | --- |
| Phase 0: PRD, spec, worked example | **Drafted** (2026-10-14) |
| K Kernel | **In progress**: phase 1 milestones 1-2 (budgets and CI; strict parsing, hashing, fuzzing) |
| R Review packet | Planned |
| C Cluster | Planned |
| E Agent and evals | Planned |

## Conventions

- **Requirement IDs** take the form `<area>-<nn>`, e.g. `K-03` or `SEC-02`. IDs are stable: a dropped requirement is marked *Dropped*, never renumbered.
- **Priority:** **Must** (the project isn't done without it), **Should** (expected; omission needs a stated reason) or **Could** (nice to have).
- **Status:** *Done* (an automated test proves it, and the test is named), *Partial*, *Planned* or *Open question*.
- "Done" means demonstrated by a test in this repo. A requirement met only by something nothing checks is *Partial*.
- Update this PRD in the same commit as the change that alters a requirement's status. Generate counts and results from the tests; don't type them in.
