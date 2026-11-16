#!/bin/bash
set -euo pipefail
umask 077
fail() { printf '\nMaestro: %s\n' "$*" >&2; exit 1; }
[[ $# -eq 0 || ( $# -eq 1 && "$1" == --check-only ) ]] || fail 'Abra este arquivo sem argumentos.'
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
walk="$ROOT"
while [[ "$walk" != / ]]; do
  [[ ! -L "$walk" ]] || fail 'Caminho com link preservado; use a pasta local preparada.'
  walk="$(dirname "$walk")"
done
[[ -f "$ROOT/.maestro-pilot-owned" && ! -L "$ROOT/.maestro-pilot-owned" ]] || fail 'Este launcher requer uma pasta preparada pelo kit.'
[[ -f "$ROOT/pilot-binary.sha256" && ! -L "$ROOT/pilot-binary.sha256" ]] || fail 'Falta a identidade do executável; preserve a preparação.'
[[ -z "$(find "$ROOT/product" "$ROOT/launchers" "$ROOT/exports" -type l -print -quit)" ]] || fail 'Material gerenciado com link; preserve a preparação.'
identity="$(cat "$ROOT/pilot-binary.sha256")"
[[ "$identity" =~ ^([0-9a-f]{64})\ \ product/maestro-mcp$ ]] || fail 'Identidade do executável inválida.'
expected="${BASH_REMATCH[1]}"
actual="$(shasum -a 256 "$ROOT/product/maestro-mcp")"
[[ "${actual%% *}" == "$expected" ]] || fail 'O executável mudou; preserve a preparação.'
if [[ -f "$ROOT/.maestro-pilot-complete" ]]; then
  [[ -f "$ROOT/managed-files.sha256" && ! -L "$ROOT/managed-files.sha256" ]] || fail 'Falta o inventário gerenciado.'
  # The sentinel preserves exact trailing newlines while the producer status is checked.
  current_inventory="$(cd "$ROOT" && LC_ALL=C find -s product exports launchers -type f -exec shasum -a 256 '{}' + && printf .)" || fail 'Não foi possível conferir o material gerenciado; preserve a preparação.'
  cmp -s "$ROOT/managed-files.sha256" <(printf '%s' "${current_inventory%.}") || fail 'Material gerenciado mudou; preserve a preparação.'
fi
[[ ! -L "$ROOT/diagnostics" ]] || fail 'A pasta de diagnóstico contém link; preserve a preparação.'
if [[ ! -e "$ROOT/diagnostics" ]]; then mkdir "$ROOT/diagnostics"; fi
[[ -d "$ROOT/diagnostics" && "$(stat -f '%u' "$ROOT/diagnostics")" == "$(id -u)" ]] || fail 'Pasta de diagnóstico indisponível.'
RUN="$(mktemp -d "$ROOT/diagnostics/teste-$(date +%Y%m%d-%H%M%S).XXXXXXXX")"
status=0
"$ROOT/product/maestro-mcp" pilot-check --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" > "$RUN/report.json" 2>/dev/null || status=$?
if ! /usr/bin/plutil -extract schema_version raw -o - "$RUN/report.json" 2>/dev/null | /usr/bin/grep -qx 1; then
  printf '# Maestro MCP — diagnóstico incompleto\n\nO verificador não produziu um relatório reconhecido. Preserve esta pasta e envie este arquivo ao time. Nenhum cliente foi aberto.\n' > "$RUN/RELATORIO.md"
  printf 'Relatório: %s/RELATORIO.md\n' "$RUN"
  exit 1
fi
for pair in 'markdown|RELATORIO.md' 'code_prompt|CODE.txt' 'desktop_prompt|DESKTOP.txt' 'code_ready_prompt|CODE-CONEXAO.txt' 'desktop_ready_prompt|DESKTOP-CONEXAO.txt'; do
  /usr/bin/plutil -extract "${pair%%|*}" raw -o "$RUN/${pair#*|}" "$RUN/report.json" || fail "Relatório incompleto; preserve $RUN."
done
printf '\nRelatório: %s/RELATORIO.md\n' "$RUN"
[[ "${1:-}" != --check-only ]] || exit "$status"
open -t "$RUN/RELATORIO.md"
if [[ "$status" -ne 0 ]]; then fail 'Preparação bloqueada. Envie RELATORIO.md ao time; os arquivos foram preservados.'; fi
[[ -t 0 ]] || exit 0
printf '\n1 — Ensaio completo no Code\n2 — Retomar no Desktop\n3 — Conferir conexão Code (somente leitura)\n4 — Conferir conexão Desktop (somente leitura)\nEnter — Sair\nEscolha: '
read -r choice
case "$choice" in
  1) [[ -s "$RUN/CODE.txt" ]] || fail 'O relatório ainda não liberou o ensaio no cliente.'
     command -v claude >/dev/null || fail 'Disponibilize o comando claude pelo fluxo normal do Code e abra Testar Maestro novamente.'
     prompt="$(cat "$RUN/CODE.txt" && printf .)" || fail 'Não foi possível ler o prompt deste relatório.'
     printf 'Abrindo o Code com o ensaio completo. Conclua o login normal se solicitado.\n'
     exec "$ROOT/launchers/Abrir Code.command" "${prompt%.}";;
  2) [[ -s "$RUN/DESKTOP.txt" ]] || fail 'O relatório ainda não liberou o ensaio no cliente.'
     pbcopy < "$RUN/DESKTOP.txt"
     printf 'Prompt copiado. Confirme a instalação da extensão deste ensaio. Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e, somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Depois cole uma vez em uma conversa nova do Desktop.\n'
     exec "$ROOT/launchers/Abrir Desktop.command";;
  3) [[ -s "$RUN/CODE-CONEXAO.txt" ]] || fail 'O relatório ainda não liberou a conferência no Code.'
     command -v claude >/dev/null || fail 'Disponibilize o comando claude pelo fluxo normal do Code e abra Testar Maestro novamente.'
     prompt="$(cat "$RUN/CODE-CONEXAO.txt" && printf .)" || fail 'Não foi possível ler o prompt deste relatório.'
     printf 'Abrindo o Code com a conferência de conexão. Conclua o login normal se solicitado.\n'
     exec "$ROOT/launchers/Abrir Code.command" "${prompt%.}";;
  4) [[ -s "$RUN/DESKTOP-CONEXAO.txt" ]] || fail 'O relatório ainda não liberou a conferência no Desktop.'
     pbcopy < "$RUN/DESKTOP-CONEXAO.txt"
     printf 'Conferência copiada. Confirme a instalação da extensão deste ensaio. Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e, somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Depois cole uma vez em uma conversa nova do Desktop.\n'
     exec "$ROOT/launchers/Abrir Desktop.command";;
  '') ;;
  *) fail 'Escolha 1, 2, 3, 4 ou Enter.';;
esac
