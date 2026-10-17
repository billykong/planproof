# The video and Shorts

A narrated video of the blog post, [Don't trust the AI's plan. Trust a checker small enough to read.](https://billykong.github.io/planproof/), about 10 and a half minutes at 1920x1080, and three Shorts (1080x1920, under a minute each) cut from the same script and captures. Same pipeline as the [safe-ops-agent-k8s video](https://github.com/billykong/safe-ops-agent-k8s/tree/main/video).

```bash
video/build.sh                  # writes video/out/<name>/<name>.mp4 and captions.srt, for the video and each Short
ONLY=video video/build.sh       # just one (video, looks-fine, wrong-goal or reviewers)
```

The first run installs its tools under `video/out/tools` (resvg-js and ffmpeg-static from npm, [Kokoro](https://github.com/thewh1teagle/kokoro-onnx) in a uv venv, and Kokoro's model files, about 350 MB); `TOOLS_DIR` points it at an existing copy. Nothing needs a GPU or a network service after that. `VOICE=bm_george` picks another Kokoro voice.

| File | What it is |
| --- | --- |
| `scenes.js` | The video's script: each beat's narration and what's on screen |
| `shorts.js` | The three Shorts, in the same format |
| `captures/` | Real `planproof` output, trimmed to fit the screen |
| `tts.py` | Narrates each beat; writes the timings and captions |
| `render.js` | Draws each frame (SVG, rendered with resvg), 16:9 or 9:16, and times them to the narration |
| `build.sh` | Runs the three and muxes each MP4 with ffmpeg |
| `description.md` | YouTube titles, descriptions, chapters and the disclosure |

## Editing

Change the words in `scenes.js` or `shorts.js` and rebuild. Keep the script saying what the post says, and check any new claim against `results/`. `tts.py` has a table of spellings for words the voice misreads; the captions keep the script's own spelling.

## The captures

Recorded on 2026-10-17 at commit `4bcd53f` (kernel 0.2.0), from `examples/drain-two-nodes/`: `cat` of each plan, then `planproof check ... --output packet`. Lines are trimmed, never edited; `#@ name` lines mark where a beat's reveal stops. The predicted-versus-real table is from `results/kind-test.txt`; the other numbers are from `results/`.
