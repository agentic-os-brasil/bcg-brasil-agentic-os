#!/usr/bin/env bash
# Opt-in model-backed canary. Sends only a fixed synthetic prompt plus the
# distributable Maestro workspace instructions to the configured Claude
# service. It must be run attended on each target platform.

set -euo pipefail

usage() {
  echo "usage: live-agent-canary.sh --zip PATH --trace TRACE.jsonl --receipt RECEIPT.json" >&2
  exit 2
}

ZIP_PATH=""
TRACE=""
RECEIPT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --zip) ZIP_PATH="${2:-}"; shift 2 ;;
    --trace) TRACE="${2:-}"; shift 2 ;;
    --receipt) RECEIPT="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[ -f "$ZIP_PATH" ] && [ -n "$TRACE" ] && [ -n "$RECEIPT" ] || usage
[ ! -e "$TRACE" ] && [ ! -e "$RECEIPT" ] || {
  echo "trace or receipt already exists" >&2
  exit 1
}
command -v claude >/dev/null 2>&1 || { echo "Claude Code is unavailable" >&2; exit 1; }

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
. "$REPO_ROOT/installers/zip/user-template/.claude/hooks/lib/python.sh"
maestro_python >/dev/null 2>&1 || { echo "Python 3 is unavailable" >&2; exit 1; }

SCRATCH=$(mktemp -d -t maestro-live-agent-XXXXXX)
trap 'rm -rf "$SCRATCH"' EXIT
unzip -q "$ZIP_PATH" -d "$SCRATCH"

PROMPT='Valida este canario com exatamente uma chamada real da ferramenta Agent. Chama Yoda e pede apenas CANARIO_YODA_OK. Nao simule nem substitua a chamada. Responde CANARIO_HUB_OK somente depois de receber o retorno real.'

set +e
(
  cd "$SCRATCH/Maestro"
  claude -p \
    --setting-sources project \
    --output-format stream-json \
    --verbose \
    --include-hook-events \
    --forward-subagent-text \
    --no-session-persistence \
    --model opus \
    --effort xhigh \
    --max-budget-usd 0.50 \
    --permission-mode dontAsk \
    --allowedTools=Agent \
    -- "$PROMPT"
) > "$TRACE"
CLAUDE_RC=$?
set -e

PLATFORM=$(uname -s)
ZIP_SHA=$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')
CLAUDE_VERSION=$(claude --version 2>/dev/null || printf 'unavailable')

maestro_py "$REPO_ROOT/acceptance/zip-update/eval_live_agent_trace.py" \
  --trace "$TRACE" \
  --receipt "$RECEIPT" \
  --platform "$PLATFORM" \
  --architecture "$(uname -m)" \
  --release-sha256 "$ZIP_SHA" \
  --claude-version "$CLAUDE_VERSION" \
  --claude-rc "$CLAUDE_RC"
