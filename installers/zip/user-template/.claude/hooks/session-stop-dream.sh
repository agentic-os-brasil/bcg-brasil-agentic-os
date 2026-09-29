#!/usr/bin/env bash
# Drain authored checkpoints; only persisted work requests dreaming.
set +e
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
. "$(dirname "${BASH_SOURCE[0]}")/lib/maestro-runtime.sh"
maestro_runtime "$PROJECT_DIR" daily-stop --root "$PROJECT_DIR" || printf 'Maestro: daily checkpoint remains pending.\n' >&2
exit 0
