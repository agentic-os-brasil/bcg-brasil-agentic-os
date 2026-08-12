#!/usr/bin/env bash
# check-rollup-size.sh — sensor gate on the SessionStart skills rollup.
#
# Why: the rollup is emitted as additionalContext on EVERY session for every
# Maestro beta user. A single bloated SKILL.md description ships instantly to
# ~40 people. This gate catches accidental growth before push.
#
# What it checks:
#   1. Byte count of the generated rollup vs baseline (dev/skills/.rollup-baseline)
#   2. Skill count vs baseline
#   3. Per-entry byte deltas > threshold flagged
#
# Delta rules:
#   - Skills added: baseline expected to grow ~120 bytes/skill (name + desc line).
#   - Absolute cap: 140 chars per description line (matches emit_skills_rollup).
#   - Growth without new skills: > 500 bytes OR > 20% → FAIL.
#
# Update baseline: run with `--write` after intentional skill additions.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SKILLS_DIR="$REPO_ROOT/bundles/base/skills"
BASELINE="$REPO_ROOT/dev/skills/.rollup-baseline"

if [ ! -d "$SKILLS_DIR" ]; then
  echo "ERROR: skills dir not found: $SKILLS_DIR" >&2
  exit 2
fi

# Generate rollup using the SAME awk logic as the hook.
generate_rollup() {
  awk '
    FNR == 1 { in_fm = 0; fm_count = 0; name = ""; desc = "" }
    /^---[[:space:]]*$/ {
      fm_count++
      if (fm_count == 1) { in_fm = 1; next }
      if (fm_count == 2) {
        in_fm = 0
        if (name != "" && desc != "") {
          sub(/\. .*$/, ".", desc)
          if (length(desc) > 140) desc = substr(desc, 1, 137) "..."
          printf "- **%s** — %s\n", name, desc
        }
        nextfile
      }
    }
    in_fm && /^name:[[:space:]]/    { sub(/^name:[[:space:]]*/, ""); name = $0 }
    in_fm && /^description:[[:space:]]/ { sub(/^description:[[:space:]]*/, ""); desc = $0 }
  ' "$SKILLS_DIR"/*/SKILL.md 2>/dev/null | sort
}

ROLLUP="$(generate_rollup)"
CUR_BYTES=$(printf '%s' "$ROLLUP" | wc -c | tr -d ' ')
CUR_SKILLS=$(printf '%s\n' "$ROLLUP" | grep -c '^- \*\*' || true)

if [ "${1:-}" = "--write" ]; then
  printf 'bytes=%s\nskills=%s\n' "$CUR_BYTES" "$CUR_SKILLS" > "$BASELINE"
  echo "baseline updated: bytes=$CUR_BYTES skills=$CUR_SKILLS"
  exit 0
fi

if [ ! -f "$BASELINE" ]; then
  echo "NOTICE: no baseline — creating (bytes=$CUR_BYTES skills=$CUR_SKILLS)"
  printf 'bytes=%s\nskills=%s\n' "$CUR_BYTES" "$CUR_SKILLS" > "$BASELINE"
  exit 0
fi

BASE_BYTES=$(awk -F= '/^bytes=/ {print $2}' "$BASELINE")
BASE_SKILLS=$(awk -F= '/^skills=/ {print $2}' "$BASELINE")

DELTA=$(( CUR_BYTES - BASE_BYTES ))
SKILL_DELTA=$(( CUR_SKILLS - BASE_SKILLS ))
# Expected growth: ~120 bytes per new skill (name + description line).
EXPECTED_GROWTH=$(( SKILL_DELTA * 120 ))
UNEXPLAINED=$(( DELTA - EXPECTED_GROWTH ))

# Percentage delta (integer math, guarded against zero).
if [ "$BASE_BYTES" -gt 0 ]; then
  PCT=$(( UNEXPLAINED * 100 / BASE_BYTES ))
else
  PCT=0
fi

echo "rollup: bytes=$CUR_BYTES (base=$BASE_BYTES  Δ=$DELTA) skills=$CUR_SKILLS (base=$BASE_SKILLS  Δ=$SKILL_DELTA)"
echo "unexplained growth: $UNEXPLAINED bytes ($PCT%)"

# FAIL rules.
if [ "$UNEXPLAINED" -gt 500 ]; then
  echo "FAIL: unexplained growth exceeds 500 bytes." >&2
  echo "  If intentional (e.g., description improvements), rerun with --write." >&2
  exit 1
fi
if [ "$PCT" -gt 20 ]; then
  echo "FAIL: unexplained growth exceeds 20%." >&2
  echo "  If intentional, rerun with --write." >&2
  exit 1
fi

echo "OK"
exit 0
