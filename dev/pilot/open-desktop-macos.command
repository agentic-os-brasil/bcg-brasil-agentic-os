#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
printf 'Confirme a extensão somente se ela não substituir outro ensaio Maestro. Use owner e workspace de COMECE-AQUI.md.\n'
open "$ROOT/exports/maestro-product.mcpb"
