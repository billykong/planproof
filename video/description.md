# YouTube upload

Times below come from `out/video/timings.json`; update them if the script changes. Upload each `captions.srt` (English, "with timing") rather than relying on auto-captions.

**Altered or synthetic content:** Yes, for the video and every Short. The narration is an AI voice; nothing shown is generated footage.

## The video

**File:** `out/video/video.mp4` (10:47, 1920x1080). **Thumbnail:** `out/video/thumbnail.png`.

**Title:**

    Don't trust the AI's plan. Trust a checker small enough to read.

**Description:**

    AI agents can now write the plans that change production systems. planproof checks each plan with a small, rule-based checker (no AI, about 1,700 lines of Go) and explains the result in plain English. It's a demo of how such a check can work, not a production system. What it is, why it can be trusted, the evidence, and the decades of public work it builds on.

    00:00 Who checks the AI's plan?
    01:09 A student and a strict examiner
    01:36 How it fits together
    02:15 How the checker thinks
    03:07 A plan that looks fine, and one that does too much
    04:04 Why trust the checker
    05:31 What the person still does
    06:25 The evidence: AI models, bad plans, a policy gate, AI reviewers
    08:44 What it doesn't do
    09:23 Built on decades of public work
    09:54 The short version

    Code, specification, tests and every result (MIT): https://github.com/billykong/planproof
    The full write-up, with every source: https://billykong.github.io/planproof/
    The previous project, safe-ops-agent-k8s: https://youtu.be/pDVeg_Gb_i8

    How this video was made: the script was drafted with AI (Claude) from the write-up, then checked and edited by me; the narration is an AI voice (Kokoro, running locally); the slides are generated from code. All checker output is real, from runs of planproof at the commit the post describes, replayed at the narration's pace. The reviewer results are from AI reviewers, not people.

**Tags (optional):** AI agents, Kubernetes, AI safety, platform engineering, formal methods.

## The Shorts

Upload each as a normal video; a vertical video under three minutes becomes a Short. In each one's settings, set **Related video** to the main video, so the Short links to it.

| File | Title |
| --- | --- |
| `out/short-looks-fine/short-looks-fine.mp4` (0:53) | This AI plan looks fine. It isn't. #shorts |
| `out/short-wrong-goal/short-wrong-goal.mp4` (0:56) | My checker proved the wrong plan #shorts |
| `out/short-reviewers/short-reviewers.mp4` (0:49) | AI reviewers caught 12 of 18 bad plans. Then 18. #shorts |

**Description (same for all three):**

    planproof: a demo of a small checker, with no AI in it, for AI-written change plans. Not a production system. Full video: [video link]
    Write-up: https://billykong.github.io/planproof/
    Code (MIT): https://github.com/billykong/planproof
    Script drafted with AI and checked by me; AI voice; all checker output is from real runs.
