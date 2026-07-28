#!/usr/bin/env bash
# Runtime-neutral SessionStart snapshot. No network, model, worker or source read.
set -uo pipefail

command -v jq >/dev/null 2>&1 || exit 0
ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
BRANCH=$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || printf '%s' "unknown")
COMMIT=$(git -C "$ROOT" log -1 --pretty=format:%h\ %s 2>/dev/null || printf '%s' "unknown")
CHANGED=$(git -C "$ROOT" status --porcelain 2>/dev/null | wc -l | tr -d ' ')

MSG="Engineering session snapshot"
MSG="${MSG}"$'\n'"branch: ${BRANCH}"
MSG="${MSG}"$'\n'"last_commit: ${COMMIT}"
MSG="${MSG}"$'\n'"changed_paths: ${CHANGED}"
MSG="${MSG}"$'\n'"next pointers: \$qa-gate · \$unit-test-wave · \$pr-quality-loop"
MSG="${MSG}"$'\n'"source context remains outside the hook; inspect local instructions only when the task requires it."

jq -n --arg c "$MSG" \
  '{hookSpecificOutput:{hookEventName:"SessionStart",additionalContext:$c}}'
