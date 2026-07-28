#!/usr/bin/env bash
# Advisory signal after a failed test command. It never blocks or retries.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
TOOL=$(printf '%s' "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
OUTPUT=$(printf '%s' "$INPUT" | jq -r '.tool_response.output // empty' 2>/dev/null || true)
[ "$TOOL" = "Bash" ] || exit 0
printf '%s' "$COMMAND" | grep -qE '(^|[;&|[:space:]])(pytest|py\.test|go[[:space:]]+test|cargo[[:space:]]+test|npm[[:space:]]+test|pnpm[[:space:]]+test|yarn[[:space:]]+test)([[:space:]]|$)' || exit 0

FAILURE_SIGNALS=$(printf '%s' "$OUTPUT" | grep -Eic '(^|[[:space:]])([0-9]+[[:space:]]+failed|FAIL|FAILED|ERROR)' || true)
[ "$FAILURE_SIGNALS" -gt 0 ] || exit 0
MSG="Tests reported a failure; do not push until it is classified and resolved."
MSG="${MSG}"$'\n'"failure_signals: ${FAILURE_SIGNALS}"
MSG="${MSG}"$'\n'"Next: reproduce narrowly, classify the failure, then rerun the affected checks."
jq -n --arg c "$MSG" \
  '{hookSpecificOutput:{hookEventName:"PostToolUse",additionalContext:$c}}'
