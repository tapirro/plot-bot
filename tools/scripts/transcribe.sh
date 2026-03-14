#!/bin/bash
# transcribe.sh — Extract audio from video/audio file and transcribe to text
# Usage: ./transcribe.sh <input_file> [output_name] [--model small|medium|large-v3]
#
# Examples:
#   ./transcribe.sh /path/to/video.mov
#   ./transcribe.sh /path/to/audio.mp3 "meeting-name"
#   ./transcribe.sh /path/to/video.mov "standup" --model small
#
# Output: transcripts/<date>_<name>.md with timestamps

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TRANSCRIPTS_DIR="$REPO_ROOT/transcripts"

# Defaults
MODEL="medium"
INPUT_FILE=""
OUTPUT_NAME=""

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --model)
      MODEL="$2"
      shift 2
      ;;
    -*)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
    *)
      if [[ -z "$INPUT_FILE" ]]; then
        INPUT_FILE="$1"
      elif [[ -z "$OUTPUT_NAME" ]]; then
        OUTPUT_NAME="$1"
      fi
      shift
      ;;
  esac
done

if [[ -z "$INPUT_FILE" ]]; then
  echo "Usage: $0 <input_file> [output_name] [--model small|medium|large-v3]"
  exit 1
fi

if [[ ! -f "$INPUT_FILE" ]]; then
  echo "ERROR: File not found: $INPUT_FILE"
  exit 1
fi

# Check dependencies
if ! command -v ffmpeg &>/dev/null; then
  echo "ERROR: ffmpeg not found. Install with: brew install ffmpeg"
  exit 1
fi

if ! python3 -c "from faster_whisper import WhisperModel" 2>/dev/null; then
  echo "ERROR: faster-whisper not found. Install with: pip install faster-whisper"
  exit 1
fi

# Extract date from filename or use today
FILE_DATE=$(echo "$(basename "$INPUT_FILE")" | grep -oE '20[0-9]{2}-[0-9]{2}-[0-9]{2}' | head -1 || true)
if [[ -z "$FILE_DATE" ]]; then
  FILE_DATE=$(date +%Y-%m-%d)
fi

# Generate output name if not provided
if [[ -z "$OUTPUT_NAME" ]]; then
  OUTPUT_NAME=$(basename "$INPUT_FILE" | sed 's/\.[^.]*$//' | sed 's/Screen Recording //' | sed "s/$FILE_DATE//" | sed 's/^ *at *//' | sed 's/[^a-zA-Z0-9а-яА-Я]/-/g' | sed 's/-\+/-/g' | sed 's/^-//' | sed 's/-$//')
  if [[ -z "$OUTPUT_NAME" ]]; then
    OUTPUT_NAME="recording"
  fi
fi

OUTPUT_FILE="$TRANSCRIPTS_DIR/${FILE_DATE}_${OUTPUT_NAME}.md"
TEMP_AUDIO="/tmp/transcribe_audio_$$.mp3"

mkdir -p "$TRANSCRIPTS_DIR"

echo "=== Transcribe Pipeline ==="
echo "Input:  $INPUT_FILE"
echo "Output: $OUTPUT_FILE"
echo "Model:  $MODEL"
echo "Date:   $FILE_DATE"
echo ""

# Step 1: Extract audio (skip if already audio)
MIME=$(file --mime-type -b "$INPUT_FILE")
if [[ "$MIME" == audio/* ]]; then
  echo "[1/3] Input is audio, copying..."
  cp "$INPUT_FILE" "$TEMP_AUDIO"
else
  echo "[1/3] Extracting audio from video..."
  ffmpeg -i "$INPUT_FILE" -vn -acodec libmp3lame -q:a 4 "$TEMP_AUDIO" -y 2>/dev/null
fi

DURATION=$(ffprobe -v quiet -show_entries format=duration -of csv=p=0 "$TEMP_AUDIO" 2>/dev/null | cut -d. -f1)
DURATION_MIN=$((DURATION / 60))
echo "       Duration: ${DURATION_MIN} min"

# Step 2: Transcribe with faster-whisper
echo "[2/3] Transcribing with whisper ($MODEL)..."

python3 << PYEOF
import sys, time
from faster_whisper import WhisperModel

model = WhisperModel("$MODEL", device="cpu", compute_type="int8")
segments, info = model.transcribe(
    "$TEMP_AUDIO",
    language="ru",
    beam_size=3,
    word_timestamps=False,
    vad_filter=True
)

lines = []
for seg in segments:
    mm_s, ss_s = divmod(int(seg.start), 60)
    mm_e, ss_e = divmod(int(seg.end), 60)
    ts = f"[{mm_s:02d}:{ss_s:02d} \u2192 {mm_e:02d}:{ss_e:02d}]"
    lines.append(f"{ts} {seg.text.strip()}")

with open("$TEMP_AUDIO.txt", "w") as f:
    f.write("\n\n".join(lines))

print(f"Segments: {len(lines)}")
PYEOF

# Step 3: Assemble markdown
echo "[3/3] Assembling markdown..."

cat > "$OUTPUT_FILE" << HEADER
# ${OUTPUT_NAME}

**Дата:** ${FILE_DATE}
**Длительность:** ${DURATION_MIN} мин
**Источник:** $(basename "$INPUT_FILE") (Whisper STT, ${MODEL})

---

## Транскрипт

HEADER

cat "$TEMP_AUDIO.txt" >> "$OUTPUT_FILE"

# Cleanup
rm -f "$TEMP_AUDIO" "$TEMP_AUDIO.txt"

echo ""
echo "=== Done ==="
echo "Saved: $OUTPUT_FILE"
echo "Size:  $(wc -c < "$OUTPUT_FILE" | tr -d ' ') bytes"
echo "Lines: $(wc -l < "$OUTPUT_FILE" | tr -d ' ')"
