#!/usr/bin/env bash
# Maestro SessionEnd dream marker — writes a timestamped marker to
# data/memory/.dream-requested so the next session can detect that a
# dreaming cycle is due. Fail-open: never blocks Claude.
#
# The marker is consumed by the dream-memory skill on the next SessionStart
# when adapters are available. It is idempotent: multiple stops in the
# same session update the timestamp without creating duplicate requests.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
MEMORY_DIR="$DATA_DIR/memory"
MARKER="$MEMORY_DIR/.dream-requested"
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# If memory dir does not exist, nothing to do.
[ -d "$MEMORY_DIR" ] || exit 0

# So arma o marcador quando o dia AINDA NAO tem sinal consolidado.
#
# Antes escrevia sem condicao nenhuma. Como Stop dispara ao fim de cada turno do
# assistente — nao ao fim da sessao —, o marcador voltava minutos depois de a
# skill apaga-lo, e o bloco de "acao obrigatoria antes de responder ao dono" do
# SessionStart (session-start-memory-inject.sh) passava a estar sempre ligado. O
# unico canal reservado para "isto vem antes de tudo" carregava, em toda sessao,
# uma ordem de refazer trabalho ja feito — e o dono via o aviso duas vezes no
# mesmo dia depois de o ciclo ter rodado.
#
# A condicao erra de proposito para o lado de MANTER o pedido: perder um ciclo e
# pior que repetir um. Qualquer duvida — pasta ausente, arquivo ilegivel, data
# que nao bate — cai no ramo que escreve o marcador.
TODAY=$(date +%Y-%m-%d)
if [ -f "$MEMORY_DIR/recent/$TODAY.md" ]; then
  rm -f "$MARKER" 2>/dev/null
  exit 0
fi

printf '%s\n' "$TS" > "$MARKER" 2>/dev/null

exit 0
