#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
printf 'Confirme a instalação da extensão Maestro MCP — Ensaio indicada no relatório. Ela já usa as pastas aprovadas, sem campos de caminho. Abra uma conversa nova e cole o texto copiado por Testar Maestro. Se a extensão estiver desativada ou ausente, confira Configurações > Extensões > Configurar > Ativado. Preserve outras extensões Maestro.\n'
open "$ROOT/exports/maestro-product.mcpb"
