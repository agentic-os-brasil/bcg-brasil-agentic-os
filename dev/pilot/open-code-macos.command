#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
"$ROOT/exports/code/scripts/maestro-mcp" inspect --packet --product-root "$ROOT/exports/code" --user-home "$ROOT/owner" --workspace "$ROOT/workspace"
CODE="$(command -v claude || true)"
[[ -n "$CODE" ]] || { echo 'Disponibilize o comando claude pelo fluxo normal do Code.' >&2; exit 1; }
export CLAUDE_CONFIG_DIR="$ROOT/client/code"
cd "$ROOT/workspace"
exec "$CODE" --plugin-dir "$ROOT/exports/code"
