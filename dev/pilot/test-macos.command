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
  current_inventory="$(cd "$ROOT" && LC_ALL=C find -s product exports launchers -path exports/code-native-qualified -prune -o -type f -exec shasum -a 256 '{}' + && printf .)" || fail 'Não foi possível conferir o material gerenciado; preserve a preparação.'
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
qualify_code() {
  local destination="$ROOT/exports/code-native-qualified" scratch="$ROOT/owner/native-scratch" code link hops=0 budget review_digest state
  [[ ! -e "$destination" && ! -L "$destination" ]] || fail 'Já existe um resultado de qualificação. Ele foi preservado; abra Code para usar um resultado nativo válido ou envie o relatório ao time. Não há repetição automática.'
  code="$(command -v claude || true)"
  [[ "$code" == /* ]] || fail 'Disponibilize uma instalação local do Code pelo fluxo normal antes de qualificar.'
  while [[ -L "$code" ]]; do
    (( hops += 1 )); [[ "$hops" -le 16 ]] || fail 'Cadeia de links do Code excessiva; preserve a instalação.'
    link="$(readlink "$code")"
    if [[ "$link" == /* ]]; then code="$link"; else code="$(dirname "$code")/$link"; fi
  done
  code="$(cd "$(dirname "$code")" && pwd -P)/$(basename "$code")"
  printf '\nEsta ação executa testes sintéticos com modelo no Code e pode gerar cobrança. Preparar e abrir Code normalmente não executam estes testes.\nAté 3 sessões CLI verificam ações permitidas, bloqueios e falhas/timeout dos hooks, com 8 turnos de pai e 120 segundos por sessão. O comando exige sua estimativa escolhida por sessão (maior que zero e no máximo US$ 1) para registrar o plano que você vai aprovar; não calcula uma cotação de preço. O plano mostra a soma estimada das sessões. Estimativas e limites do provedor não são tetos rígidos de cobrança; agentes filhos podem fazer outras requisições. Leia o plano completo antes de confirmar.\nSua estimativa por sessão, em USD (ex.: 0.25; vazio cancela): '
  read -r budget
  [[ -n "$budget" ]] || return 0
  [[ ! -L "$scratch" ]] || fail 'Scratch com link; preserve a pasta.'
  if [[ ! -e "$scratch" ]]; then mkdir -m 700 "$scratch"; fi
  local common=(--product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$ROOT/product/maestro-mcp" --destination "$destination" --version 0.0.0 --native-executable "$code" --native-scratch-root "$scratch" --native-challenge-directory "$ROOT/workspace/native-qualification-controls" --max-budget-usd "$budget")
  export CLAUDE_CONFIG_DIR="$ROOT/client/code"
  "$ROOT/product/maestro-mcp" native-review "${common[@]}" > "$RUN/native-review.json" || fail 'A revisão recusou a qualificação. Nenhum teste com modelo foi iniciado; preserve o diagnóstico privado e confira o login normal do perfil Code.'
  cat "$RUN/native-review.json"
  state="$(/usr/bin/plutil -extract plan.state raw -o - "$RUN/native-review.json")"
  [[ "$state" == reviewable ]] || fail 'O plano está indisponível. Nenhuma qualificação foi iniciada; preserve o diagnóstico privado.'
  review_digest="$(/usr/bin/plutil -extract review_digest raw -o - "$RUN/native-review.json")"
  [[ "$review_digest" =~ ^[0-9a-f]{64}$ ]] || fail 'Revisão incompleta; nenhum teste com modelo foi iniciado.'
  printf '\nLi o plano completo e autorizo estes testes com modelo. Para executar, digite QUALIFICAR (qualquer outra resposta cancela): '
  read -r consent
  [[ "$consent" == QUALIFICAR ]] || return 0
  "$ROOT/product/maestro-mcp" native-qualify-and-build "${common[@]}" --review-digest "$review_digest" --confirm > "$RUN/native-result.json" || fail 'A qualificação não terminou. Preserve os diagnósticos privados e confira o relatório antes de voltar ao ensaio MCP comum.'
  state="$(/usr/bin/plutil -extract state raw -o - "$RUN/native-result.json")"
  [[ "$state" == qualified ]] || fail 'Os testes não qualificaram os sete hooks. O resultado foi preservado; o ensaio MCP comum continua disponível. Compartilhe somente RELATORIO.md após conferir seu conteúdo.'
  inspection="$("$ROOT/product/maestro-mcp" inspect --packet --product-root "$destination" --user-home "$ROOT/owner" --workspace "$ROOT/workspace")" || fail 'O pacote resultante não foi admitido; preserve os diagnósticos privados.'
  [[ "$(/usr/bin/plutil -extract packet_mode raw -o - - <<< "$inspection")" == native ]] || fail 'O pacote resultante não é nativo qualificado; nenhum cliente foi aberto.'
  printf 'Qualificação concluída. O launcher verificará o pacote e abrirá o Code com os sete hooks. Os diagnósticos native-*.json são privados; não os compartilhe com o relatório.\n'
  exec "$ROOT/launchers/Abrir Code.command"
}
printf '\n1 — Ensaio completo no Code\n2 — Retomar no Desktop\n3 — Conferir conexão Code (somente leitura)\n4 — Conferir conexão Desktop (somente leitura)\n5 — Qualificar os sete hooks do Code (testes com modelo; confirmação separada)\nEnter — Sair\nEscolha: '
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
  5) qualify_code;;
  '') ;;
  *) fail 'Escolha 1, 2, 3, 4, 5 ou Enter.';;
esac
