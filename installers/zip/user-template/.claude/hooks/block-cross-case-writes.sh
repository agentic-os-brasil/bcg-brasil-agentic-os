#!/usr/bin/env bash
# block-cross-case-writes.sh — Case isolation PreToolUse hook
# Blocks Edit and Write tool calls that would write into a case workspace
# other than the currently active case.
#
# Scope: Edit and Write tools only. Bash is intentionally excluded —
# Bash detection via regex is structurally unreliable (bypass vectors:
# python scripts, variable-stored commands, heredocs). If Bash enforcement
# is needed, use a PostToolUse file-watcher on `git diff --name-only` instead.
#
# Scope bound: this hook exits immediately (exit 0) for any path that is not
# inside data/cases/. This prevents overhead on all ordinary OS-level writes
# and limits enforcement strictly to the case workspace.
#
# Bootstrap path: writes to data/cases/<pending-id>/ are allowed when
# data/cases/.pending exists and matches the target case. This prevents
# false blocks during case creation.
#
# Fail-open: any error (missing active file, unreadable path, malformed
# input) exits 0 and allows the tool call. This hook must never block
# Claude from working due to its own errors.
#
# Input: JSON on stdin matching Claude Code PreToolUse hook contract.
# Output: exit 0 = allow; exit 2 + JSON {"decision":"block","reason":"..."}
#         on stdout = block the tool call.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
CASES_DIR="$DATA_DIR/cases"
ACTIVE_FILE="$CASES_DIR/.active"
PENDING_FILE="$CASES_DIR/.pending"

# Parse stdin once into tool_name and file_path
read -r -d '' HOOK_INPUT || true
TOOL_NAME=$(printf '%s' "$HOOK_INPUT" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    print(data.get('tool_name', ''))
except Exception:
    print('')
" 2>/dev/null)

case "$TOOL_NAME" in
  Edit|Write) ;;
  *) exit 0 ;;
esac

# Read the file path from the already-parsed input
TARGET_PATH=$(printf '%s' "$HOOK_INPUT" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    inp = data.get('tool_input', {})
    # Edit and Write both use 'file_path'
    print(inp.get('file_path', ''))
except Exception:
    print('')
" 2>/dev/null)

[ -z "$TARGET_PATH" ] && exit 0

# Resolve to absolute path
case "$TARGET_PATH" in
  /*) ABS_PATH="$TARGET_PATH" ;;
  *)  ABS_PATH="$PROJECT_DIR/$TARGET_PATH" ;;
esac

# Scope bound: exit immediately if path is not inside data/cases/.
# This is the primary performance guard — no case-ID extraction happens
# for ordinary writes outside the case workspace.
CASES_ABS="$CASES_DIR"
case "$ABS_PATH" in
  "$CASES_ABS"/*) ;;
  *) exit 0 ;;
esac

# Extract the case-id from the path: data/cases/<case-id>/...
# Strip the cases dir prefix
REL="${ABS_PATH#"$CASES_ABS/"}"
# The first path component is the case-id (or a sentinel like .active, .pending)
TARGET_CASE="${REL%%/*}"

# Sentinel files (.active, .pending, .initialized, etc.) are always allowed
case "$TARGET_CASE" in
  .*) exit 0 ;;
esac

# Read active case
[ -f "$ACTIVE_FILE" ] || exit 0
ACTIVE_CASE=$(tr -d '[:space:]' < "$ACTIVE_FILE" 2>/dev/null)
[ -z "$ACTIVE_CASE" ] && exit 0

# If writing to the active case, allow immediately
[ "$TARGET_CASE" = "$ACTIVE_CASE" ] && exit 0

# Bootstrap path: allow if .pending matches the target case
if [ -f "$PENDING_FILE" ]; then
  PENDING_CASE=$(tr -d '[:space:]' < "$PENDING_FILE" 2>/dev/null)
  [ "$TARGET_CASE" = "$PENDING_CASE" ] && exit 0
fi

# Block: target case differs from active and is not the pending case
printf '{"decision":"block","reason":"Cross-case write blocked. Active case: %s. Target path is in case: %s. To switch cases, update data/cases/.active first via $case-agent-setup."}\n' \
  "$ACTIVE_CASE" "$TARGET_CASE"
exit 2
