#!/bin/bash
set -euo pipefail
[[ $# -eq 0 || ( $# -eq 1 && -n "$1" ) ]] || { echo 'Use zero argumentos ou um único prompt.' >&2; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
"$ROOT/exports/code/scripts/maestro-mcp" inspect --packet --product-root "$ROOT/exports/code" --user-home "$ROOT/owner" --workspace "$ROOT/workspace"
CODE="$(command -v claude || true)"
[[ -n "$CODE" ]] || { echo 'Disponibilize o comando claude pelo fluxo normal do Code.' >&2; exit 1; }
export CLAUDE_CONFIG_DIR="$ROOT/client/code"
cd "$ROOT/workspace"
if [[ $# -eq 1 ]]; then set -- -- "$1"; fi
exec "$CODE" --plugin-dir "$ROOT/exports/code" "$@"
