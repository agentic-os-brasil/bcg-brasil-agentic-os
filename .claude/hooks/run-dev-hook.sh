#!/usr/bin/env sh
set -eu

event="${1:-}"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
cd "$repo_root"

if ! command -v go >/dev/null 2>&1; then
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"Go ainda nao esta instalado. Use $start-contributing e guie o contribuidor sem assumir conhecimento de Git."}}'
  exit 0
fi

exec go run ./dev/harness claude "$event"
