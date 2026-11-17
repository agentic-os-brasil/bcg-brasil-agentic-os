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
RUNTIME_MODE=false
PILOT_VERSION=0.0.0
BINARY_RELATIVE=product/maestro-mcp
if [[ -e "$KIT/RUNTIME.json" ]]; then
  [[ -f "$KIT/RUNTIME.json" && ! -L "$KIT/RUNTIME.json" ]] || fail 'Metadado do runtime inválido; preserve o kit.'
  [[ "$(/usr/bin/plutil -extract schema_version raw -o - "$KIT/RUNTIME.json")" == 1 && "$(/usr/bin/plutil -extract profile raw -o - "$KIT/RUNTIME.json")" == engineering-runtime-pilot-v1 ]] || fail 'Perfil do runtime desconhecido.'
  [[ "$(/usr/bin/plutil -extract target raw -o - "$KIT/RUNTIME.json")" == "$TARGET" ]] || fail 'Este kit completo contém somente o runtime Mac Apple Silicon. Outros alvos continuam em validação.'
  PILOT_VERSION="$(/usr/bin/plutil -extract version raw -o - "$KIT/RUNTIME.json")"
  [[ "$PILOT_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Versão experimental inválida.'
  RUNTIME_MODE=true
  BINARY_RELATIVE=product/server/maestro-mcp
fi
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
printf '%s\n' 'Somente ao criar este perfil Code novo: autoMemoryEnabled:false e syncClaudeAiPlugins:false desativam a memória automática do cliente e o download/carregamento de plugins sincronizados da conta. Perfis existentes e suas escolhas são preservados.'
printf '%s\n' 'O pacote Desktop seleciona revisão local no navegador mesmo quando houver formulário nativo; cada alteração exige sua aprovação exata e expira em 150 segundos.'
if [[ "$RUNTIME_MODE" == true ]]; then
  printf '\nEste perfil inclui o conversor local verificado, sem Python, downloads ou serviço. O preparador copia um DOCX sintético para o workspace novo. Após criar o workspace, mostra uma segunda revisão completa para você aprovar a ativação do conversor.\nVersão experimental: %s\nEmissor: %s\nExpira em: %s\nManifesto: %s\nRuntime: %s\n' "$PILOT_VERSION" "$(/usr/bin/plutil -extract issuer raw -o - "$KIT/RUNTIME.json")" "$(/usr/bin/plutil -extract expires_at raw -o - "$KIT/RUNTIME.json")" "$(/usr/bin/plutil -extract manifest_sha256 raw -o - "$KIT/RUNTIME.json")" "$(/usr/bin/plutil -extract pack_sha256 raw -o - "$KIT/RUNTIME.json")"
fi
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
inventory() { (cd "$ROOT" && LC_ALL=C find -s product exports launchers -path exports/code-native-qualified -prune -o -type f -exec shasum -a 256 '{}' +); }
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
  # The sentinel preserves exact trailing newlines while the producer status is checked.
  current_inventory="$(inventory && printf .)" || fail 'Não foi possível conferir o material gerenciado; o kit preserva os arquivos.'
  cmp -s "$ROOT/managed-files.sha256" <(printf '%s' "${current_inventory%.}") || fail 'Material gerenciado mudou; o kit preserva os arquivos.'
  "$ROOT/launchers/Testar Maestro.command" --check-only
  printf 'Ensaio conferido. Para continuar, abra %s/launchers/Testar Maestro.command\n' "$ROOT"
  exit 0
fi
mkdir "$ROOT"
(set -o noclobber; printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-owned") || fail 'Outra preparação já reivindicou o destino.'
set -o noclobber
mkdir "$ROOT/product" "$ROOT/owner" "$ROOT/exports" "$ROOT/launchers" "$ROOT/client" "$ROOT/client/code"
if [[ "$RUNTIME_MODE" == true ]]; then
  mkdir "$ROOT/product/server" "$ROOT/product/managed" "$ROOT/product/managed/trust"
  cp -n "$KIT/runtime-registry.json" "$ROOT/product/managed/trust/release-authority-registry.json"
  expected_registry="$(/usr/bin/plutil -extract registry_sha256 raw -o - "$KIT/RUNTIME.json")"
  copied_registry="$(shasum -a 256 "$ROOT/product/managed/trust/release-authority-registry.json")"
  [[ "$expected_registry" =~ ^[0-9a-f]{64}$ && "${copied_registry%% *}" == "$expected_registry" ]] || fail 'O registro público mudou; preserve a preparação.'
fi
printf '%s\n' '{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}' > "$ROOT/client/code/settings.json"
cp -n "$KIT/bin/maestro-mcp-$TARGET" "$ROOT/$BINARY_RELATIVE"
expected_binary="$(awk -v target="./bin/maestro-mcp-$TARGET" '$2==target { print $1 }' "$KIT/SHA256SUMS")"
actual_binary="$(shasum -a 256 "$ROOT/$BINARY_RELATIVE")"
[[ "$expected_binary" =~ ^[0-9a-f]{64}$ && "${actual_binary%% *}" == "$expected_binary" ]] || fail 'O executável copiado mudou; preserve o ensaio.'
BIN="$ROOT/$BINARY_RELATIVE"
printf '%s  %s\n' "$expected_binary" "$BINARY_RELATIVE" > "$ROOT/pilot-binary.sha256"
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
if [[ "$RUNTIME_MODE" == true ]]; then
  cp -n "$KIT/test-documents/maestro-pilot.docx" "$ROOT/workspace/maestro-pilot.docx"
  source_sample="$(shasum -a 256 "$KIT/test-documents/maestro-pilot.docx")"
  copied_sample="$(shasum -a 256 "$ROOT/workspace/maestro-pilot.docx")"
  [[ "${source_sample%% *}" == "${copied_sample%% *}" ]] || fail 'O DOCX sintético está ocupado ou mudou; preserve os arquivos.'
  "$BIN" pilot-runtime-review --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --release-directory "$KIT/runtime-release" > "$ROOT/runtime-review.json" || diagnose_failure 'A revisão do conversor não foi concluída.'
  cat "$ROOT/runtime-review.json"
  runtime_review="$(/usr/bin/plutil -extract review_sha256 raw -o - "$ROOT/runtime-review.json")"
  [[ "$runtime_review" =~ ^[0-9a-f]{64}$ ]] || fail 'Revisão do conversor incompleta; preserve a preparação.'
  printf '\nLeia a revisão completa acima. Para ativar somente este conversor neste ensaio, digite ATIVAR CONVERSOR: '
  read -r runtime_reply || diagnose_failure 'Ativação não aprovada; a preparação parcial foi preservada.'
  [[ "$runtime_reply" == 'ATIVAR CONVERSOR' ]] || diagnose_failure 'Ativação recusada; o workspace inicializado foi preservado e o conversor não foi colocado.'
  "$BIN" pilot-runtime-apply --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --release-directory "$KIT/runtime-release" --review-digest "$runtime_review" --confirm > "$ROOT/runtime-placement.json" || diagnose_failure 'A ativação do conversor não foi concluída.'
fi
build_code() { "$BIN" plugin --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/code" --version "$PILOT_VERSION" "$@"; }
case "$(cat "$KIT/LIFECYCLE-CONTEXT.txt")" in enabled) build_code --lifecycle-context || diagnose_failure 'O pacote Code não foi concluído.';; disabled) build_code || diagnose_failure 'O pacote Code não foi concluído.';; *) fail 'Seleção de contexto desconhecida.';; esac
build_desktop() { "$BIN" mcpb --desktop-pilot --owner-review-channel local-browser --product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --server "$BIN" --destination "$ROOT/exports/maestro-product.mcpb" --version "$PILOT_VERSION" "$@"; }
if [[ "$RUNTIME_MODE" == true ]]; then build_desktop --engineering-runtime-pilot || diagnose_failure 'O pacote Desktop não foi concluído.'; else build_desktop || diagnose_failure 'O pacote Desktop não foi concluído.'; fi
{
  printf '# Maestro MCP — teste guiado\n\nAbra **launchers/Testar Maestro.command**. Para conferir a conexão sem alterar dados, escolha **3 — Conferir conexão Code** ou **4 — Conferir conexão Desktop**. No Code, a opção abre a conversa com o texto exato deste relatório, sem copiar ou colar. No Desktop, o texto é copiado: cole uma vez em uma conversa nova. O estado mostrado é a saúde da conexão, não o estado dos trabalhos.\n\nPara o ensaio completo, escolha **1 — Ensaio completo no Code** e depois **2 — Retomar no Desktop**. O Code recebe o prompt inicial automaticamente; no Desktop, cole o texto copiado. Cada etapa preserva as confirmações normais.\n\nO perfil Code é separado e pode pedir login normal nessa janela. Ao criar um perfil novo, este ensaio desativa a memória automática do cliente e os plugins sincronizados da conta somente nesse perfil. Repetir a preparação preserva suas escolhas existentes. O plugin e a extensão já usam as pastas aprovadas; não há diretórios para copiar ou preencher. Se o cliente pedir configurar outros caminhos, preserve a mensagem e pare.\n\n'
  printf 'Para Desktop, escolha 4 para conferir somente a conexão ou 2 para retomar o ensaio completo em launchers/Testar Maestro.command e confirme a instalação da extensão Maestro MCP — Ensaio indicada no relatório. Ela já usa as pastas aprovadas; não há campos de caminho para salvar. Antes de colar, abra Configurações > Extensões > a extensão deste ensaio e, somente na linha Maestro context (maestro_context), selecione ✓ / Sempre permitir. Preserve as demais permissões. ✓ / Sempre permitir libera só a leitura; gravações de conteúdo exigem revisão exata no formulário nativo ou no navegador local selecionado neste ensaio. Abra uma conversa nova e cole o texto copiado. Se a extensão estiver desativada ou ausente, confira Configurações > Extensões > Configurar > Ativado nessa extensão. Preserve outras extensões Maestro; se o cliente propuser substituir outra extensão, pare.\n\nRevisão do dono: O Desktop deste ensaio e o Code do kit completo selecionam explicitamente a revisão local no navegador para gravações protegidas. Uma solicitação interativa abre o registro completo no navegador deste computador, mesmo quando o cliente oferece formulário nativo. Leia o conteúdo, marque a caixa inicialmente desmarcada e escolha Aprovar alteração exata ou Recusar. Fechar a aba, cancelar ou ultrapassar 150 segundos não aprova. Este canal local não qualifica formulários MCP, hooks ou bloqueio nativo. Se receber confirmation_unavailable, deixe a ação pendente, preserve a mensagem e envie o RELATORIO.md ao time. Não repita a chamada, não contorne o bloqueio nem use um sim no chat como aprovação. A permissão de leitura não aprova gravações.\n\nO workspace_id esperado vem desta criação real:\n\n'
  cat "$ROOT/preparation.json"
  printf '\n\nOs relatórios ficam em diagnostics/, fora dos dados do Maestro. Compartilhe somente RELATORIO.md após conferir seu conteúdo. As chamadas reais nos clientes, login, confirmações e hooks continuam sendo verificados por você.\n'
  if [[ "$RUNTIME_MODE" == true ]]; then printf 'Este ensaio usa o runtime local e uma autoridade experimental com validade curta. Consulte content.status para os formatos realmente disponíveis. O perfil não configura instalação ou atualização de produção. Ao expirar, preserve seus dados e solicite um kit novo; não altere datas nem permissões.\n'; else printf 'O servidor é de engenharia sem versão de release; 0.0.0 é o rótulo dos pacotes.\n'; fi
} > "$ROOT/COMECE-AQUI.md"
printf '\nPara testar os sete hooks nativos, primeiro use a opção 3 e conclua o login normal no perfil Code. Depois escolha **5 — Qualificar os sete hooks do Code**. Essa ação separada usa modelo e pode gerar cobrança: escolha sua estimativa por sessão, leia o plano completo e confirme QUALIFICAR somente se concordar. Preparar, abrir Code e conferir conexão não executam a qualificação. Após sucesso, o launcher seleciona o pacote qualificado e as duas pastas automaticamente. Resultado existente é preservado, sem repetição automática. Desktop continua como extensão MCP e não instala os sete hooks nativos do Code.\n' >> "$ROOT/COMECE-AQUI.md"
inventory > "$ROOT/managed-files.sha256"
printf '%s\n' "$KIT_ID" > "$ROOT/.maestro-pilot-complete"
"$ROOT/launchers/Testar Maestro.command" --check-only
printf '\nPreparado e conferido. Abra %s/launchers/Testar Maestro.command para testar nos clientes.\n' "$ROOT"
