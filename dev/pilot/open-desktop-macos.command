#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
printf 'Confirme a extensão Maestro MCP — Ensaio indicada no relatório e seus caminhos preenchidos. Preserve outras extensões Maestro.\n'
open "$ROOT/exports/maestro-product.mcpb"
