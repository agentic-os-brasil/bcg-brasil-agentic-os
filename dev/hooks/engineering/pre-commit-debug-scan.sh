#!/usr/bin/env bash
# Hard-block debugger artifacts in staged additions; warn on likely debug logs.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
printf '%s' "$COMMAND" | grep -qE '(^|[;&|[:space:]])git[[:space:]]+commit([[:space:]]|$)' || exit 0

ADDED=$(git diff --staged --unified=0 2>/dev/null | grep '^+' | grep -v '^+++' || true)
HARD_COUNT=$(printf '%s' "$ADDED" | grep -Eic 'pdb\.set_trace\(\)|pdb\.post_mortem\(\)|breakpoint\(\)|(^|[[:space:]])debugger[;[:space:]]' || true)
if [ "$HARD_COUNT" -gt 0 ]; then
  MSG="Commit blocked: staged additions contain debugger artifacts. Remove them before committing."
  MSG="${MSG}"$'\n'"debugger_signal_count: ${HARD_COUNT}"
  jq -n --arg c "$MSG" \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
  exit 2
fi

SOFT_COUNT=$(printf '%s' "$ADDED" | grep -Eic '(^|[[:space:]])(print|console\.log)\(' || true)
if [ "$SOFT_COUNT" -gt 0 ]; then
  MSG="Review before commit: staged additions contain likely debug logging. Confirm that it is intentional and uses the repository's logging convention."
  MSG="${MSG}"$'\n'"debug_logging_signal_count: ${SOFT_COUNT}"
  jq -n --arg c "$MSG" \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
fi
