#!/usr/bin/env bash
# Native managed guard for Claude. Shares policy and bounded JSON parsing with
# Codex/PowerShell; stock macOS needs neither Python nor third-party packages.
# This covers file-tool calls, not arbitrary shell/network effects.
set -u
ROOT="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)}"
. "$(dirname "${BASH_SOURCE[0]}")/lib/maestro-runtime.sh" || exit 2
RESULT=$(maestro_runtime "$ROOT" codex-hook PreToolUse --root "$ROOT") || {
  printf 'Cross-case write blocked: verified runtime unavailable. Resolve the managed installation before retrying.\n' >&2
  exit 2
}
case "$RESULT" in
  *'"permissionDecision":"deny"'*)
    printf 'Cross-case write blocked: target, active case or filesystem aliases could not be verified. Confirm the active account/case and retry.\n' >&2
    exit 2 ;;
esac
printf '%s\n' "$RESULT"
