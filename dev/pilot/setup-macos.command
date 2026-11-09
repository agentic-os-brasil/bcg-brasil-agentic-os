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
if [[ -e "$ROOT" ]]; then
  [[ -f "$ROOT/.maestro-pilot-owned" && ! -L "$ROOT/.maestro-pilot-owned" && "$(cat "$ROOT/.maestro-pilot-owned")" == "$KIT_ID" ]] || fail 'Pasta estrangeira preservada; selecione um destino novo.'
  [[ -f "$ROOT/.maestro-pilot-complete" && -f "$ROOT/managed-files.sha256" ]] || fail 'Preparação parcial preservada; selecione uma pasta nova para recuperar.'
  [[ -z "$(find "$ROOT/product" "$ROOT/exports" "$ROOT/launchers" -type l -print -quit)" ]] || fail 'Material gerenciado contém link; preservado.'
  cmp -s "$ROOT/managed-files.sha256" <(inventory) || fail 'Material gerenciado mudou; o kit preserva os arquivos.'
  "$ROOT/product/maestro-mcp" inspect --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace"
  printf 'Ensaio já preparado. Abra %s/COMECE-AQUI.md\n' "$ROOT"
  exit 0
fi
mkdir "$ROOT"
(set -o noclobber; printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-owned") || fail 'Outra preparação já reivindicou o destino.'
mkdir "$ROOT/product" "$ROOT/owner" "$ROOT/exports" "$ROOT/launchers" "$ROOT/client" "$ROOT/client/code"
cp -n "$KIT/bin/maestro-mcp-$TARGET" "$ROOT/product/maestro-mcp"
expected_binary="$(awk -v target="./bin/maestro-mcp-$TARGET" '$2==target { print $1 }' "$KIT/SHA256SUMS")"
actual_binary="$(shasum -a 256 "$ROOT/product/maestro-mcp")"
[[ "$expected_binary" =~ ^[0-9a-f]{64}$ && "${actual_binary%% *}" == "$expected_binary" ]] || fail 'O executável copiado mudou; preserve o ensaio.'
chmod 700 "$ROOT/product/maestro-mcp"
BIN="$ROOT/product/maestro-mcp"
"$BIN" init --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" | tee "$ROOT/preparation.json"
"$BIN" inspect --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace"
build_code() { "$BIN" plugin --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/code" --version 0.0.0 "$@"; }
case "$(cat "$KIT/LIFECYCLE-CONTEXT.txt")" in enabled) build_code --lifecycle-context;; disabled) build_code;; *) fail 'Seleção de contexto desconhecida.';; esac
"$BIN" mcpb --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/maestro-product.mcpb" --version 0.0.0
cat > "$ROOT/launchers/Abrir Code.command" <<'CODE'
#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
"$ROOT/exports/code/scripts/maestro-mcp" inspect --packet --product-root "$ROOT/exports/code" --user-home "$ROOT/owner" --workspace "$ROOT/workspace"
CODE="$(command -v claude || true)"
[[ -n "$CODE" ]] || { echo 'Disponibilize o comando claude pelo fluxo normal do Code.' >&2; exit 1; }
export CLAUDE_CONFIG_DIR="$ROOT/client/code"
cd "$ROOT/workspace"
exec "$CODE" --plugin-dir "$ROOT/exports/code"
CODE
cat > "$ROOT/launchers/Abrir Desktop.command" <<'DESKTOP'
#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
printf 'Confirme a extensão somente se ela não substituir outro ensaio Maestro. Use owner e workspace de COMECE-AQUI.md.\n'
open "$ROOT/exports/maestro-product.mcpb"
DESKTOP
chmod 700 "$ROOT/launchers/"*.command
{
  printf '# Maestro — ensaio local\n\nAbra launchers/Abrir Code.command. O perfil Code é separado e pode pedir login normal. Ao habilitar o plugin, preencha os campos com estes valores literais:\n\nOwner directory:\n\n    %s/owner\n\nWorkspace:\n\n    %s/workspace\n\n' "$ROOT" "$ROOT"
  printf 'Depois abra launchers/Abrir Desktop.command e confirme os mesmos campos na extensão. Se já houver outra extensão maestro-product, pare antes de substituir.\n\nO workspace_id esperado vem desta criação real:\n\n'
  cat "$ROOT/preparation.json"
  printf '\n\nUse o ROTEIRO.md do kit; confirme esse ID nos dois clientes antes de escrever. Código/versão e checksums estão em ARTIFACT.txt do kit. O servidor é de engenharia sem versão de release; 0.0.0 é o rótulo dos pacotes.\n'
} > "$ROOT/COMECE-AQUI.md"
inventory > "$ROOT/managed-files.sha256"
printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-complete"
printf '\nPreparado. Abra %s/COMECE-AQUI.md e use o ROTEIRO.md do kit.\n' "$ROOT"
