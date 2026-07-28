#!/usr/bin/env bash
# Deterministic, network-free PR freshness guard.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
printf '%s' "$COMMAND" | grep -qE '(^|[;&|[:space:]])gh[[:space:]]+pr[[:space:]]+create([[:space:]]|$)' || exit 0

BASE=$(printf '%s' "$COMMAND" | sed -nE 's/.*--base[=[:space:]]+([^[:space:]]+).*/\1/p' | head -1)
[ -n "$BASE" ] || BASE=main
REF="origin/${BASE}"
if ! git rev-parse --verify --quiet "$REF^{commit}" >/dev/null 2>&1; then
  MSG="PR creation blocked: local ${REF} is unavailable, so branch freshness cannot be qualified. Fetch the base ref, then retry. This hook never performs network I/O."
  jq -n --arg c "$MSG" \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
  exit 2
fi

BEHIND=$(git rev-list --count "HEAD..${REF}" 2>/dev/null || printf '%s' "unknown")
if [ "$BEHIND" != "0" ]; then
  MSG="PR creation blocked: branch is ${BEHIND} commit(s) behind ${REF}. Update and recheck before opening the PR."
  jq -n --arg c "$MSG" \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
  exit 2
fi
