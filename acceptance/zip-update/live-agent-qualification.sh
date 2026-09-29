#!/usr/bin/env bash
# Model-backed release qualification. Sends only a fixed controlled prompt plus the
# distributable Maestro workspace instructions to the configured Claude
# service. It must be run attended on each target platform.

set -euo pipefail

usage() {
  echo "usage: live-agent-qualification.sh --zip PATH --trace TRACE.jsonl --receipt RECEIPT.json" >&2
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

PROMPT='Executa este teste de qualificacao de release com exatamente uma chamada real da ferramenta Agent. Chama Yoda e pede apenas QUALIFICACAO_YODA_OK. Nao simule nem substitua a chamada. Responde QUALIFICACAO_MAESTRO_OK somente depois de receber o retorno real.'

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
    --permission-mode dontAsk \
    --allowedTools=Agent \
    -- "$PROMPT"
) > "$TRACE"
CLAUDE_RC=$?
set -e

PLATFORM="macOS"
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
