| Seeded plan | Should be caught by | Kernel | Caught by |
| --- | --- | --- | --- |
| drain-without-cordon | rule:cordon-first | Refuted (cordon-first) | kernel |
| second-drain-without-wait | rule:budget | Refuted (budget, never-zero) | kernel |
| cordon-the-second-node-late | rule:budget | Refuted (budget, never-zero) | kernel |
| scale-without-wait | rule:settled | Refuted (settled) | kernel |
| scale-past-the-limit | rule:limits | Refuted (limits) | kernel |
| image-by-tag | rule:limits | Refuted (limits) | kernel |
| digest-one-character-too-long | rule:limits | Refuted (limits) | kernel |
| change-a-protected-deployment | rule:protected | Refuted (protected) | kernel |
| three-cordoned | rule:limits | Refuted (limits) | kernel |
| no-room | rule:capacity | Refuted (capacity) | kernel |
| autoscaled | trigger:hpa | Unknown (hpa) | kernel |
| stale-from | rule:frame | Refuted (frame) | kernel |
| scale-checkout-to-zero | rule:never-zero | Refuted (budget, never-zero) | kernel |
| restart-without-wait | rule:settled | Refuted (settled) | kernel |
| rollback-forgets-a-node | rule:rollback | Refuted (rollback) | kernel |
| unknown-node | rule:scope | Refuted (scope) | kernel |
| false-assertion | rule:assertions | Refuted (assertions) | kernel |
| scales-checkout-too | frame | Refuted (frame) | kernel |
| drain-and-quietly-update-cart | frame | Refuted (frame) | kernel |
| image-and-a-restart | frame | Refuted (frame) | kernel |
| cart-to-five | ticket | Proved | grader (goal vs ticket) |
| drains-n3-not-n2 | ticket | Proved | grader (goal vs ticket) |
| restarts-checkout-instead | ticket | Proved | grader (goal vs ticket) |
| scales-cart-down-instead | ticket | Proved | grader (goal vs ticket) |
| new-image-on-the-wrong-deployment | ticket | Proved | grader (goal vs ticket) |

25 seeded plans: 20 caught by the kernel, 5 only by checking the goal against the ticket, 0 missed.
