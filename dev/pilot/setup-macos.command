#!/bin/bash
set -euo pipefail
umask 077

fail() { printf '\nMaestro: %s\n' "$*" >&2; exit 1; }
KIT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
[[ "$(uname -s)" == Darwin ]] || fail 'Este preparador atende somente macOS. Windows precisa do preparador e da validação próprios.'
case "$(uname -m)" in
  arm64) TARGET=darwin-arm64 ;;
  x86_64) TARGET=darwin-amd64; printf 'Mac Intel: binário compilado e formato inspecionado; a execução nativa deste kit ainda precisa ser observada.\n' ;;
  *) fail 'Arquitetura Mac indisponível neste kit.' ;;
esac
[[ -f "$KIT/KIT-ID.txt" && -f "$KIT/SHA256SUMS" ]] || fail 'Kit incompleto; extraia o ZIP inteiro antes de abrir este arquivo.'
(cd "$KIT" && shasum -a 256 -c SHA256SUMS >/dev/null) || fail 'A verificação do kit falhou. Preserve os arquivos e solicite uma cópia íntegra.'
[[ -z "${MAESTRO_HOME+x}" ]] || fail 'Esta sessão já define MAESTRO_HOME. O ensaio não redireciona nem altera esse estado; use uma sessão sem esse redirecionamento explícito.'
mkdir -p "$HOME/Developer"
DEV="$(cd "$HOME/Developer" && pwd -P)"
ROOT="${1:-$DEV/MaestroPilot}"
case "$ROOT" in "$DEV"/*) ;; *) fail 'Escolha uma pasta dentro de ~/Developer.' ;; esac
case "$ROOT/" in *'/../'*|*'/./'*|*$'\n'*) fail 'O caminho precisa ser absoluto e sem atalhos .' ;; esac
walk="$DEV"
IFS=/ read -r -a pieces <<< "${ROOT#"$DEV"/}"
for piece in "${pieces[@]}"; do
  [[ -n "$piece" ]] || fail 'Caminho inválido.'
  walk="$walk/$piece"
  [[ ! -L "$walk" ]] || fail 'O destino contém um link; nada será redirecionado.'
  if [[ -e "$walk" ]]; then
    [[ -d "$walk" && "$(stat -f '%u' "$walk")" == "$(id -u)" ]] || fail 'O destino contém uma entrada que não pertence a este usuário.'
  fi
done
KIT_ID="$(cat "$KIT/KIT-ID.txt")"
inventory() {
  (cd "$ROOT"; [[ -z "$(find product exports -type l -print -quit)" ]] || exit 1
   LC_ALL=C find -s product exports -type f -exec shasum -a 256 '{}' \;)
}
if [[ -e "$ROOT" ]]; then
  [[ -f "$ROOT/.maestro-pilot-owned" && "$(cat "$ROOT/.maestro-pilot-owned")" == "$KIT_ID" ]] || fail 'A pasta já existe e não foi criada por este kit. Escolha outro destino; os arquivos existentes foram preservados.'
  [[ -f "$ROOT/.maestro-pilot-complete" && -f "$ROOT/kit-files.sha256" ]] || fail 'Há uma preparação incompleta preservada. Solicite suporte ou escolha uma pasta nova; este comando não apaga nem repara dados.'
  CURRENT="$ROOT/.kit-check.$$"
  trap 'rm -f "$CURRENT"' EXIT
  inventory > "$CURRENT" || fail 'Os arquivos gerados contêm uma entrada inesperada.'
  cmp -s "$CURRENT" "$ROOT/kit-files.sha256" || fail 'Os arquivos gerados mudaram; preserve a pasta para diagnóstico.'
  "$ROOT/product/maestro-mcp" inspect --product-root "$ROOT/product" --user-home "$ROOT/user" --workspace "$ROOT/workspace"
  printf '\nEste piloto já está preparado. As instruções estão em:\n%s\n' "$ROOT/COMECE-AQUI.txt"
  exit 0
fi
mkdir -p "$ROOT"
printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-owned"
mkdir "$ROOT/product" "$ROOT/user" "$ROOT/exports"
cp "$KIT/bin/maestro-mcp-$TARGET" "$ROOT/product/maestro-mcp"
chmod 700 "$ROOT/product/maestro-mcp"
BIN="$ROOT/product/maestro-mcp"
"$BIN" init --product-root "$ROOT/product" --user-home "$ROOT/user" --workspace "$ROOT/workspace" | tee "$ROOT/diagnostico-init.json"
"$BIN" inspect --product-root "$ROOT/product" --user-home "$ROOT/user" --workspace "$ROOT/workspace"
printf '# Exemplo do piloto\n\nEste arquivo é sintético. O objetivo é testar continuidade entre Code e Desktop.\n' > "$ROOT/workspace/EXEMPLO.md"
mkdir -p "$ROOT/exports/code-marketplace/plugins" "$ROOT/exports/code-marketplace/.claude-plugin"
"$BIN" plugin --product-root "$ROOT/product" --user-home "$ROOT/user" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/code-marketplace/plugins/maestro-product" --version 0.0.0
printf '%s\n' '{"name":"maestro-pilot-20261105","owner":{"name":"Maestro"},"plugins":[{"name":"maestro-product","source":"./plugins/maestro-product"}]}' > "$ROOT/exports/code-marketplace/.claude-plugin/marketplace.json"
"$BIN" mcpb --product-root "$ROOT/product" --user-home "$ROOT/user" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/maestro-pilot.mcpb" --version 0.0.0
inventory > "$ROOT/kit-files.sha256"
{
  printf 'MAESTRO — ENSAIO DE DESENVOLVIMENTO\n\nEste servidor não tem identidade de release qualificada: sua versão é unavailable.\n0.0.0 é somente o rótulo dos pacotes. Não há atualização assinada automática.\n\n'
  printf '1. Abra o Claude Code nesta pasta:\n   cd %q\n' "$ROOT/workspace"
  printf '2. Cadastre o marketplace local:\n   claude plugin marketplace add %q\n' "$ROOT/exports/code-marketplace"
  printf '3. Instale o plugin do ensaio:\n   claude plugin install maestro-product@maestro-pilot-20261105 --scope user\n'
  printf '4. Abra uma nova sessão do Code na pasta acima. Confira maestro_context antes de escrever.\n\n'
  printf '5. No Desktop, abra este arquivo e confirme a extensão local pelo fluxo normal do aplicativo:\n   %s\n\n' "$ROOT/exports/maestro-pilot.mcpb"
  printf 'Dados reais da criação; compare este workspace_id com maestro_context nos dois clientes:\n'
  cat "$ROOT/diagnostico-init.json"
  printf '\nUse o ROTEIRO.md do kit. Os dois clientes devem mostrar esse mesmo workspace_id.\nNão use outro conector Maestro por engano; confira o vínculo antes de confirmar efeitos.\n\n'
  printf 'Arquivos gerenciados: %s\nEstado privado do ensaio: %s\nWorkspace sintético: %s\n' "$ROOT/product" "$ROOT/user" "$ROOT/workspace"
  printf '\nSe a política do computador ou do cliente recusar o binário, pare e registre a mensagem. O kit não altera permissões de segurança nem autenticação.\n'
} > "$ROOT/COMECE-AQUI.txt"
printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-complete"
cat "$ROOT/COMECE-AQUI.txt"
