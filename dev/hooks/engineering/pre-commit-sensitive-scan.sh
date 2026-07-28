#!/usr/bin/env bash
# Advisory scan for obvious secrets or sensitive material in staged additions.
# Repository-specific PII rules belong outside this portable pack.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
printf '%s' "$COMMAND" | grep -qE '(^|[;&|[:space:]])git[[:space:]]+commit([[:space:]]|$)' || exit 0

ADDED=$(git diff --staged --unified=0 2>/dev/null | grep '^+' | grep -v '^+++' || true)
KEY_COUNT=$(printf '%s' "$ADDED" | grep -Eic 'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY' || true)
TOKEN_COUNT=$(printf '%s' "$ADDED" | grep -Eic 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{20,}' || true)
[ "$KEY_COUNT" -gt 0 ] || [ "$TOKEN_COUNT" -gt 0 ] || exit 0

MSG="Sensitive-looking material found in staged additions. Pause and validate before committing; this neutral scan is advisory and may have false positives."
MSG="${MSG}"$'\n'"private_key_signal_count: ${KEY_COUNT}"
MSG="${MSG}"$'\n'"token_signal_count: ${TOKEN_COUNT}"
jq -n --arg c "$MSG" \
  '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
