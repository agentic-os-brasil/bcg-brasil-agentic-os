#!/usr/bin/env bash
# Fail-closed merge guard. Human approval belongs to the repository's canonical
# protected workflow; this neutral template cannot authenticate an override.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
printf '%s' "$COMMAND" | grep -qE '(^|[;&|[:space:]])gh[[:space:]]+pr[[:space:]]+merge([[:space:]]|$)' || exit 0
MSG="Merge blocked: this neutral hook cannot authenticate human approval. Hand the current head to the repository's canonical protected merge workflow; required checks and branch protection still apply."
jq -n --arg c "$MSG" \
  '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
exit 2
