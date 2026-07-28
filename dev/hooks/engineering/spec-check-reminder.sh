#!/usr/bin/env bash
# Advisory reminder for source edits when a nearby specification may exist.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
FILE_PATH=$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null || true)
[ -n "$FILE_PATH" ] || exit 0
printf '%s' "$FILE_PATH" | grep -qE '\.(go|py|js|ts|tsx|rs|java|kt|sql)$' || exit 0

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
SPEC_ROOT=${AGENTIC_OS_SPEC_ROOT:-}
if [ -z "$SPEC_ROOT" ]; then
  for candidate in "$ROOT/specs" "$ROOT/docs/specs" "$ROOT/docs/harness/specs"; do
    if [ -d "$candidate" ]; then SPEC_ROOT="$candidate"; break; fi
  done
fi
[ -n "$SPEC_ROOT" ] || exit 0

BASE=$(basename "$FILE_PATH")
STEM=${BASE%.*}
MATCH=$(find "$SPEC_ROOT" -type f \( -iname "*${STEM}*.md" -o -iname "*spec*.md" \) 2>/dev/null | head -1 || true)
if [ -n "$MATCH" ]; then
  MSG="Specification pointer: ${MATCH#"$ROOT/"}. Confirm acceptance criteria before marking the source change complete."
else
  MSG="Specification reminder: source edit detected, but no nearby specification matched ${BASE}. For non-trivial behavior, create or update a bounded spec."
fi
jq -n --arg c "$MSG" \
  '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
