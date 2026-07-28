#!/usr/bin/env bash
# Remind the author to run the quality loop after a PR is created.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
INPUT=$(cat 2>/dev/null || true)
TOOL=$(printf '%s' "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)
COMMAND=$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
OUTPUT=$(printf '%s' "$INPUT" | jq -r '.tool_response.output // empty' 2>/dev/null || true)
[ "$TOOL" = "Bash" ] || exit 0
printf '%s' "$COMMAND" | grep -qE '(^|[;&|[:space:]])gh[[:space:]]+pr[[:space:]]+create([[:space:]]|$)' || exit 0

PR_NUMBER=$(printf '%s' "$OUTPUT" | grep -oE '/pull/[0-9]+' | head -1 | grep -oE '[0-9]+' || true)
[ -n "$PR_NUMBER" ] || PR_NUMBER=$(printf '%s' "$OUTPUT" | grep -oE '#[0-9]+' | head -1 | grep -oE '[0-9]+' || true)
if [ -n "$PR_NUMBER" ]; then
  MSG="PR #${PR_NUMBER} created. Next: run \$pr-quality-loop ${PR_NUMBER}. It refreshes head-bound evidence, QA and review; human approval remains separate."
else
  MSG="PR created. Next: run \$pr-quality-loop <number> once the number is known."
fi
jq -n --arg c "$MSG" \
  '{hookSpecificOutput:{hookEventName:"PostToolUse",additionalContext:$c}}'
