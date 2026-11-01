#!/usr/bin/env bash
# Model-backed release qualification. Sends only a fixed controlled prompt plus the
# distributable Maestro workspace instructions to the configured Claude
# service. It must be run attended on each target platform.

set -euo pipefail

usage() {
  echo "usage: live-agent-qualification.sh --zip PATH --profile ABSOLUTE_TEST_PROFILE --trace TRACE.jsonl --receipt RECEIPT.json" >&2
  exit 2
}

ZIP_PATH=""
PROFILE=""
TRACE=""
RECEIPT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --zip) ZIP_PATH="${2:-}"; shift 2 ;;
    --profile) PROFILE="${2:-}"; shift 2 ;;
    --trace) TRACE="${2:-}"; shift 2 ;;
    --receipt) RECEIPT="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[ -f "$ZIP_PATH" ] && [ -n "$PROFILE" ] && [ -n "$TRACE" ] && [ -n "$RECEIPT" ] || usage
[ ! -e "$TRACE" ] && [ ! -e "$RECEIPT" ] || {
  echo "trace or receipt already exists" >&2
  exit 1
}
# The operator authenticates this dedicated profile; never copy real credentials.
case "$PROFILE" in /*) ;; *) echo "test profile must be an absolute real directory" >&2; exit 2 ;; esac
[ -d "$PROFILE" ] && [ ! -L "$PROFILE" ] || { echo "test profile must be a real directory" >&2; exit 2; }
PROFILE="${PROFILE%/}"
[ "$(cd "$PROFILE" && pwd -P)" = "$PROFILE" ] || { echo "test profile must not contain directory aliases" >&2; exit 2; }
OPERATOR_PROFILE=$(cd "${HOME:?operator profile unavailable}" && pwd -P)
[ "$PROFILE" != "$OPERATOR_PROFILE" ] || { echo "use a dedicated test profile, not the operator profile" >&2; exit 2; }
if [ -n "${CLAUDE_CONFIG_DIR:-}" ] && [ "${CLAUDE_CONFIG_DIR%/}" != "$PROFILE/.claude" ]; then
  echo "CLAUDE_CONFIG_DIR conflicts with the selected test profile" >&2
  exit 2
fi
if [ -L "$PROFILE/.claude" ] || { [ -e "$PROFILE/.claude" ] && [ ! -d "$PROFILE/.claude" ]; }; then
  echo "test profile Claude configuration must be a real directory" >&2
  exit 2
fi
command -v claude >/dev/null 2>&1 || { echo "Claude Code is unavailable" >&2; exit 1; }

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# Qualification tooling must not borrow the owner-state interpreter API.
PYTHON_LABEL=$(command -v python3) || { echo "Python 3 tooling is unavailable" >&2; exit 1; }
"$PYTHON_LABEL" -c 'import sys; raise SystemExit(sys.version_info[0] != 3)' || exit 1

SCRATCH=$(mktemp -d -t maestro-live-agent-XXXXXX)
trap 'rm -rf "$SCRATCH"' EXIT
unzip -q "$ZIP_PATH" -d "$SCRATCH"
. "$REPO_ROOT/installers/zip/user-template/.claude/hooks/lib/maestro-runtime.sh"
HOME="$PROFILE" USERPROFILE="$PROFILE" MAESTRO_HOME="$PROFILE/.maestro" \
  maestro_runtime "$SCRATCH/Maestro" home init --product-root "$SCRATCH/Maestro" --user-home "$PROFILE" >/dev/null
qualification_claude() {
  env -u CLAUDE_CONFIG_DIR HOME="$PROFILE" USERPROFILE="$PROFILE" MAESTRO_HOME="$PROFILE/.maestro" \
    CLAUDE_PROJECT_DIR="$SCRATCH/Maestro" claude "$@"
}

PROMPT='Executa este teste de qualificacao de release com exatamente uma chamada real da ferramenta Agent. Chama Yoda e pede apenas QUALIFICACAO_YODA_OK. Nao simule nem substitua a chamada. Responde QUALIFICACAO_MAESTRO_OK somente depois de receber o retorno real.'

set +e
(
  cd "$SCRATCH/Maestro"
  qualification_claude -p \
    --setting-sources project \
    --output-format stream-json \
    --verbose \
    --include-hook-events \
    --forward-subagent-text \
    --no-session-persistence \
    --model opus \
    --effort xhigh \
    --permission-mode dontAsk \
    --allowedTools=Agent \
    -- "$PROMPT"
) > "$TRACE"
CLAUDE_RC=$?
set -e

PLATFORM="macOS"
ZIP_SHA=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
CLAUDE_VERSION=$(qualification_claude --version 2>/dev/null || printf 'unavailable')

"$PYTHON_LABEL" "$REPO_ROOT/acceptance/zip-update/eval_live_agent_trace.py" \
  --trace "$TRACE" \
  --receipt "$RECEIPT" \
  --platform "$PLATFORM" \
  --architecture "$(uname -m)" \
  --release-sha256 "$ZIP_SHA" \
  --claude-version "$CLAUDE_VERSION" \
  --claude-rc "$CLAUDE_RC"
