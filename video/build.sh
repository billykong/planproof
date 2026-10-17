#!/usr/bin/env bash
# Builds the video and the Shorts: narration, frames, then each MP4.
#   video/build.sh [OUT_DIR]          the video and all Shorts (default video/out)
#   ONLY=looks-fine video/build.sh    one of them ("video" for the video)
# Tools go in TOOLS_DIR (default OUT_DIR/tools) on first run: resvg-js and
# ffmpeg-static from npm, Kokoro in a uv venv, and its model files (~350 MB).
# VOICE picks the Kokoro voice (default am_michael).
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
OUT=$(mkdir -p "${1:-$HERE/out}" && cd "${1:-$HERE/out}" && pwd)
TOOLS=${TOOLS_DIR:-$OUT/tools}
VOICE=${VOICE:-am_michael}
mkdir -p "$TOOLS"

if [ ! -x "$TOOLS/node_modules/ffmpeg-static/ffmpeg" ]; then
  (cd "$TOOLS" && npm init -y >/dev/null && npm i --silent @resvg/resvg-js@2.6.2 ffmpeg-static@5.2.0)
fi
if [ ! -x "$TOOLS/tts/bin/python" ]; then
  uv venv -q "$TOOLS/tts"
  VIRTUAL_ENV="$TOOLS/tts" uv pip install -q kokoro-onnx==0.4.9 soundfile
fi
mkdir -p "$TOOLS/models"
for f in kokoro-v1.0.onnx voices-v1.0.bin; do
  [ -s "$TOOLS/models/$f" ] || curl -sSL -o "$TOOLS/models/$f" "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/$f"
done
FFMPEG=$TOOLS/node_modules/ffmpeg-static/ffmpeg

# build NAME [SHORT_ID]: writes OUT/NAME/NAME.mp4, captions.srt, timings.json
build() {
  local name=$1 short=${2:-} dir=$OUT/$1
  mkdir -p "$dir"
  echo "=== $name: narration ($VOICE)"
  "$TOOLS/tts/bin/python" -I "$HERE/tts.py" "$dir" "$TOOLS/models" "$VOICE" $short 2>&1 | grep -v -i 'warn'
  echo "=== $name: frames"
  rm -rf "$dir/frames"
  (cd "$TOOLS" && node "$HERE/render.js" "$dir" $short)
  echo "=== $name: mp4"
  "$FFMPEG" -y -loglevel error -f concat -safe 0 -i "$dir/frames.txt" -i "$dir/narration.wav" \
    -vf "fps=30,format=yuv420p" -c:v libx264 -preset medium -crf 18 -tune stillimage \
    -af "loudnorm=I=-16:TP=-1.5:LRA=11" -c:a aac -b:a 192k -ar 48000 -shortest -movflags +faststart "$dir/$name.mp4"
  echo "wrote $dir/$name.mp4"
}

for name in video $(node -e 'console.log(Object.keys(require(process.argv[1]).shorts).join(" "))' "$HERE/shorts.js"); do
  [ -n "${ONLY:-}" ] && [ "$ONLY" != "$name" ] && continue
  if [ "$name" = video ]; then build video; else build "short-$name" "$name"; fi
done
