#!/bin/bash
set -euo pipefail
umask 077
fail() { printf '\nMaestro: %s\n' "$*" >&2; exit 1; }
KIT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
[[ "$(uname -s)" == Darwin ]] || fail 'Use o preparador Windows no outro sistema.'
case "$(uname -m)" in arm64) TARGET=darwin-arm64;; x86_64) TARGET=darwin-amd64;; *) fail 'Arquitetura indisponível.';; esac
[[ -z "${MAESTRO_HOME+x}" ]] || fail 'MAESTRO_HOME já está definido. O kit preserva esse redirecionamento e recusa.'
verify_kit() {
  [[ -f "$KIT/SHA256SUMS" && ! -L "$KIT/SHA256SUMS" && -f "$KIT/KIT-ID.txt" && ! -L "$KIT/KIT-ID.txt" ]] || fail 'Extraia o kit completo.'
  [[ -z "$(find "$KIT" -type l -print -quit)" ]] || fail 'O kit contém um link; os arquivos foram preservados.'
  (cd "$KIT" && shasum -a 256 -c SHA256SUMS >/dev/null) || fail 'Conteúdo diferente do kit; preserve-o e solicite outra cópia.'
  actual="$(shasum -a 256 "$KIT/SHA256SUMS")"; actual="${actual%% *}"
  KIT_ID="$(cat "$KIT/KIT-ID.txt")"
  [[ "$KIT_ID" == "$actual" && "$KIT_ID" =~ ^[0-9a-f]{64}$ ]] || fail 'Identidade do kit divergente.'
}
verify_kit
REVIEWED_KIT_ID="$KIT_ID"
ROOT="${1:-$HOME/Developer/MaestroPilot-${KIT_ID:0:12}}"
MODE="${2:-interactive}"
CONFIRM="${3:-}"
[[ "$MODE" == interactive || "$MODE" == --review || "$MODE" == --confirm ]] || fail 'Use [pasta-absoluta] [--review | --confirm digest].'
[[ "$ROOT" == /* && "$ROOT" != *$'\n'* && "$ROOT" != *$'\r'* ]] || fail 'Escolha um caminho absoluto sem controles.'
DEV="$HOME/Developer"
[[ -d "$DEV" && ! -L "$DEV" ]] || fail 'Crie uma pasta local ~/Developer antes de preparar.'
DEV="$(cd "$DEV" && pwd -P)"
case "$ROOT" in "$DEV"/*) ;; *) fail 'O ensaio fica somente dentro da pasta local ~/Developer.';; esac
case "$ROOT/" in *'/../'*|*'/./'*|*'//'*) fail 'Use um caminho físico, sem atalhos.';; esac
walk="$DEV"
IFS=/ read -r -a pieces <<< "${ROOT#"$DEV"/}"
for piece in "${pieces[@]}"; do
  walk="$walk/$piece"
  [[ ! -L "$walk" ]] || fail 'O caminho contém um link; nada foi redirecionado.'
  if [[ -e "$walk" ]]; then
    [[ -d "$walk" && "$(stat -f '%u' "$walk")" == "$(id -u)" ]] || fail 'Destino ocupado ou pertencente a outro usuário.'
  fi
done
[[ -d "$(dirname "$ROOT")" ]] || fail 'A pasta pai deve existir; escolha uma pasta nova.'
[[ -f "$KIT/bin/maestro-mcp-$TARGET" ]] || fail 'Este artefato não contém o alvo desta máquina.'
review="$(printf '%s\n' 'ordinary-engineering-rehearsal-v1' "$KIT_ID" "$TARGET" "$ROOT" | shasum -a 256)"; review="${review%% *}"
printf '\nEnsaio local de engenharia; sem instalação/atualização assinada.\nProduto: %s/product\nEstado privado: %s/owner\nWorkspace: %s/workspace\nPerfil Code isolado: %s/client/code\nreview_digest=%s\n' "$ROOT" "$ROOT" "$ROOT" "$ROOT" "$review"
[[ "$MODE" != --review ]] || exit 0
if [[ "$MODE" == --confirm ]]; then
  [[ "$CONFIRM" == "$review" ]] || fail 'Digest diferente; leia a revisão atual.'
else
  printf 'Para criar ou conferir este ensaio, digite PREPARAR: '
  read -r reply
  [[ "$reply" == PREPARAR ]] || fail 'Preparação recusada; nada foi criado.'
fi
verify_kit
[[ "$KIT_ID" == "$REVIEWED_KIT_ID" ]] || fail 'O kit mudou após a revisão; leia uma revisão nova.'
inventory() { (cd "$ROOT" && LC_ALL=C find -s product exports launchers -type f -exec shasum -a 256 '{}' \;); }
diagnose_failure() {
  "$ROOT/launchers/Testar Maestro.command" --check-only || true
  fail "$* Preparação preservada; consulte o relatório e não apague a pasta."
}
if ! command -v claude >/dev/null; then
  printf 'Code: comando claude indisponível. O relatório continuará; instale pelo fluxo normal do cliente para testar Code.\n'
fi
if [[ -e "$ROOT" ]]; then
  [[ -f "$ROOT/.maestro-pilot-owned" && ! -L "$ROOT/.maestro-pilot-owned" && "$(cat "$ROOT/.maestro-pilot-owned")" == "$KIT_ID" ]] || fail 'Pasta estrangeira preservada; selecione um destino novo.'
  [[ -f "$ROOT/.maestro-pilot-complete" && -f "$ROOT/managed-files.sha256" ]] || fail 'Preparação parcial preservada; abra launchers/Testar Maestro.command para obter o diagnóstico ou use outro destino.'
  [[ -z "$(find "$ROOT/product" "$ROOT/exports" "$ROOT/launchers" -type l -print -quit)" ]] || fail 'Material gerenciado contém link; preservado.'
  cmp -s "$ROOT/managed-files.sha256" <(inventory) || fail 'Material gerenciado mudou; o kit preserva os arquivos.'
  "$ROOT/launchers/Testar Maestro.command" --check-only
  printf 'Ensaio conferido. Para continuar, abra %s/launchers/Testar Maestro.command\n' "$ROOT"
  exit 0
fi
mkdir "$ROOT"
(set -o noclobber; printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-owned") || fail 'Outra preparação já reivindicou o destino.'
set -o noclobber
mkdir "$ROOT/product" "$ROOT/owner" "$ROOT/exports" "$ROOT/launchers" "$ROOT/client" "$ROOT/client/code"
cp -n "$KIT/bin/maestro-mcp-$TARGET" "$ROOT/product/maestro-mcp"
expected_binary="$(awk -v target="./bin/maestro-mcp-$TARGET" '$2==target { print $1 }' "$KIT/SHA256SUMS")"
actual_binary="$(shasum -a 256 "$ROOT/product/maestro-mcp")"
[[ "$expected_binary" =~ ^[0-9a-f]{64}$ && "${actual_binary%% *}" == "$expected_binary" ]] || fail 'O executável copiado mudou; preserve o ensaio.'
BIN="$ROOT/product/maestro-mcp"
printf '%s  product/maestro-mcp\n' "$expected_binary" > "$ROOT/pilot-binary.sha256"
for pair in 'open-code-macos.command|Abrir Code.command' 'open-desktop-macos.command|Abrir Desktop.command' 'test-macos.command|Testar Maestro.command'; do
  source_name="${pair%%|*}"; target_name="${pair#*|}"
  cp -n "$KIT/templates/$source_name" "$ROOT/launchers/$target_name"
  expected="$(awk -v target="./templates/$source_name" '$2==target { print $1 }' "$KIT/SHA256SUMS")"
  observed="$(shasum -a 256 "$ROOT/launchers/$target_name")"
  [[ "$expected" =~ ^[0-9a-f]{64}$ && "${observed%% *}" == "$expected" ]] || fail 'Launcher ocupado ou alterado; preserve o ensaio.'
done
if ! "$BIN" init --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" > "$ROOT/preparation.json"; then
  diagnose_failure 'A inicialização não foi concluída.'
fi
cat "$ROOT/preparation.json"
"$BIN" inspect --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" || diagnose_failure 'O diagnóstico recusou o vínculo.'
build_code() { "$BIN" plugin --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/code" --version 0.0.0 "$@"; }
case "$(cat "$KIT/LIFECYCLE-CONTEXT.txt")" in enabled) build_code --lifecycle-context || diagnose_failure 'O pacote Code não foi concluído.';; disabled) build_code || diagnose_failure 'O pacote Code não foi concluído.';; *) fail 'Seleção de contexto desconhecida.';; esac
"$BIN" mcpb --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/maestro-product.mcpb" --version 0.0.0 || diagnose_failure 'O pacote Desktop não foi concluído.'
{
  printf '# Maestro MCP — teste guiado\n\nAbra **launchers/Testar Maestro.command**. Ele confere o ensaio, abre o relatório e permite escolher Code ou Desktop; o texto com o ID correto é copiado automaticamente. Cole uma vez na conversa escolhida.\n\nO perfil Code é separado e pode pedir login normal. Ao habilitar o plugin, preencha os campos com estes valores literais:\n\nOwner directory:\n\n    %s/owner\n\nWorkspace:\n\n    %s/workspace\n\n' "$ROOT" "$ROOT"
  printf 'Depois abra launchers/Abrir Desktop.command e confirme os mesmos campos na extensão. Se já houver outra extensão maestro-product, pare antes de substituir.\n\nO workspace_id esperado vem desta criação real:\n\n'
  cat "$ROOT/preparation.json"
  printf '\n\nOs relatórios ficam em diagnostics/, fora dos dados do Maestro. Compartilhe somente RELATORIO.md após conferir seu conteúdo. As chamadas reais nos clientes, login, confirmações e hooks continuam sendo verificados por você. O servidor é de engenharia sem versão de release; 0.0.0 é o rótulo dos pacotes.\n'
} > "$ROOT/COMECE-AQUI.md"
inventory > "$ROOT/managed-files.sha256"
printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-complete"
"$ROOT/launchers/Testar Maestro.command" --check-only
printf '\nPreparado e conferido. Abra %s/launchers/Testar Maestro.command para testar nos clientes.\n' "$ROOT"
