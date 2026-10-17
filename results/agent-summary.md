| Model | Runs | Solvable tasks solved | Infeasible tasks declined | First plan Proved | Proved after repair | Submissions per run | Unknown seen | Proved, wrong goal | "No safe plan" on a solvable task | Errors | Tokens in / out (cached) | Cost |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| claude-opus-5-5/high | 13 | 9/9 (100%) | 4/4 (100%) | 9/13 (69%) | 0/13 (0%) | 0.8 | 1 | 0 | 0 | 0 | 66287 / 8063 (29230) | $0.35 |
| qwen2.5-1.5b-instruct | 13 | 0/9 (0%) | 4/4 (100%) | 0/13 (0%) | 0/13 (0%) | 1.0 | 1 | 0 | 9 | 0 | 109212 / 4609 (0) | - |

| Task | claude-opus-5-5/high | qwen2.5-1.5b-instruct |
| --- | --- | --- |
| drain-two | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| drain-n3 | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| upgrade-three | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| scale-cart | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| checkout-image | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| restart-cart | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| drain-then-image | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| checkout-down | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| tight-drain | ✓ proved-correct (1) | ✗ declared-but-solvable (1) |
| three-cordoned | ✓ declared-no-safe-plan (0) | ✓ declared-no-safe-plan (1) |
| tight-two | ✓ declared-no-safe-plan (0) | ✓ declared-no-safe-plan (1) |
| autoscaled-cart | ✓ declared-no-safe-plan (1) | ✓ declared-no-safe-plan (1) |
| payments-up | ✓ declared-no-safe-plan (0) | ✓ declared-no-safe-plan (1) |
