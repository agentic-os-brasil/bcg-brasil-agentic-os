#!/bin/bash
# macOS Bash 3.2; shared verifier checks the platform helper and manifest.
set -eu
ROOT="$(cd "$(dirname "$0")/../.." && pwd -P)"
. "$ROOT/.claude/hooks/lib/maestro-runtime.sh"
maestro_runtime "$ROOT" codex-hook "$1" --root "$ROOT"
