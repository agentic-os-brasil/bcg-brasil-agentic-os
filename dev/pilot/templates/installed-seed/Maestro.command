#!/bin/bash
set -euo pipefail
umask 077
# Public distribution input can be synced. Only checked bytes enter private staging.
fail() {
  printf '\nMaestro: %s\nNenhum arquivo existente foi apagado. Preserve o diagnóstico e envie RELATORIO.md ao time.\n' "$*" >&2
  if [[ -n "${RUN:-}" && -f "$RUN/RELATORIO.md" && ! -L "$RUN/RELATORIO.md" ]]; then printf '\nEtapa pendente: %s\n' "$*" >> "$RUN/RELATORIO.md"; fi
  exit 1
}
[[ $# -eq 0 || ( $# -eq 1 && "$1" == --check-only ) ]] || fail 'Use este arquivo sem argumentos, ou --check-only.'
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
META="$ROOT/DISTRIBUTION.json"
sha() { local value; value="$(/usr/bin/shasum -a 256 "$1")"; printf '%s' "${value%% *}"; }
[[ -f "$META" && "$(sha "$META")" == '@@DISTRIBUTION_SHA256@@' ]] || fail 'O kit está incompleto ou mudou. Marque a pasta como Sempre manter neste dispositivo e tente novamente.'
jget() { /usr/bin/plutil -extract "$2" raw -o - "$1"; }
[[ "$(jget "$META" profile)" == engineering-installed-seed-v1 && "$(jget "$META" evidence_class)" == engineering_only ]] || fail 'Perfil de distribuição diferente deste piloto.'
ROLE="$(jget "$META" identity.admission_sha256)"
VERSION="$(jget "$META" identity.installer_version)"
[[ "$(jget "$META" identity.target_os)" == darwin ]] || fail 'Este kit é para outro sistema.'
ARCH="$(uname -m)"; [[ "$ARCH" != x86_64 ]] || ARCH=amd64
[[ "$ARCH" == "$(jget "$META" identity.target_arch)" ]] || fail 'Este kit é para outra arquitetura de Mac.'
DEV="$HOME/Developer"
if [[ ! -e "$DEV" && ! -L "$DEV" ]]; then mkdir "$DEV"; fi
[[ -d "$DEV" && ! -L "$DEV" ]] || fail 'Crie uma pasta local Developer no seu perfil e execute novamente.'
DEV="$(cd "$DEV" && pwd -P)"
private_dir() {
  local path="$1" walk mode
  [[ "$path" == "$DEV/"* ]] || fail 'A pasta privada deve ficar em Developer.'
  walk="$path"
  while [[ "$walk" != "$DEV" ]]; do [[ ! -L "$walk" ]] || fail 'Link em pasta privada; preserve o local.'; walk="$(dirname "$walk")"; done
  if [[ ! -e "$path" ]]; then mkdir "$path"; fi
  [[ -d "$path" && "$(stat -f '%u' "$path")" == "$(id -u)" ]] || fail 'Pasta privada pertence a outro usuário.'
  mode="$(stat -f '%Lp' "$path")"; (( (8#$mode & 077) == 0 )) || fail 'A pasta do piloto permite acesso a outros usuários. Use uma pasta privada nova.'
}
private_dir "$DEV/Maestro-Pilot"
BASE="$DEV/Maestro-Pilot/$ROLE"; private_dir "$BASE"
private_dir "$BASE/stage"
STAGE="$BASE/stage"
# plutil raw arrays are not portable; count the closed array by indexed extraction.
i=0
while /usr/bin/plutil -extract "files.$i.path" raw -o - "$META" >/dev/null 2>&1; do
  path="$(jget "$META" "files.$i.path")"; expected="$(jget "$META" "files.$i.sha256")"; size="$(jget "$META" "files.$i.size")"
  [[ "$path" =~ ^(bootstrap|initial-release|test-documents)/[A-Za-z0-9_./-]+$ && "$path" != *..* && "$expected" =~ ^[0-9a-f]{64}$ && "$size" =~ ^[0-9]+$ ]] || fail 'Inventário de preparação inválido.'
  source="$ROOT/$path"; destination="$STAGE/$path"
  [[ -f "$source" && ! -L "$source" && "$(stat -f '%z' "$source")" == "$size" && "$(sha "$source")" == "$expected" ]] || fail "Sincronização incompleta em $path. Mantenha a pasta disponível offline e tente de novo."
  parent="$(dirname "$destination")"; walk="$parent"
  while [[ "$walk" != "$STAGE" ]]; do [[ ! -L "$walk" ]] || fail 'Link na preparação privada.'; walk="$(dirname "$walk")"; done
  mkdir -p "$parent"
  if [[ ! -e "$destination" && ! -L "$destination" ]]; then (set -o noclobber; cat "$source" > "$destination"); fi
  [[ -f "$destination" && ! -L "$destination" && "$(stat -f '%l' "$destination")" == 1 && "$(stat -f '%z' "$destination")" == "$size" && "$(sha "$destination")" == "$expected" ]] || fail "A preparação privada divergiu em $path. Preserve-a e envie o diagnóstico."
  ((i+=1))
done
[[ "$i" -ge 6 ]] || fail 'Inventário incompleto.'
BOOTSTRAP="$STAGE/$(jget "$META" installer)"; chmod 700 "$BOOTSTRAP"
verify_installer() { [[ -f "$1" && ! -L "$1" && "$(stat -f '%l' "$1")" == 1 && "$(sha "$1")" == "$(jget "$META" identity.installer_sha256)" ]] || fail 'O instalador mudou. Nada será executado.'; }
verify_installer "$BOOTSTRAP"
# The verified tagged entry independently rechecks the signed role and its expiry.
private_dir "$BASE/diagnostics"
RUN="$(mktemp -d "$BASE/diagnostics/teste-$(date +%Y%m%d-%H%M%S).XXXXXXXX")"
printf '# Maestro MCP — preparação pendente\n\nPiloto experimental (`engineering_only`).\nA preparação ou qualificação ainda não foi concluída. Os JSONs e logs são privados.\n' > "$RUN/RELATORIO.md"
printf '\nRelatório desta tentativa: %s/RELATORIO.md\n' "$RUN"
"$BOOTSTRAP" seed-status > "$RUN/seed-status-private.json" || fail 'A admissão experimental expirou ou não foi aceita. Peça um kit revisado ao time.'
[[ "$(jget "$RUN/seed-status-private.json" schema_version)" == 3 && "$(jget "$RUN/seed-status-private.json" experimental_seed.admission_sha256)" == "$ROLE" && "$(jget "$RUN/seed-status-private.json" experimental_seed.profile)" == engineering-installed-seed-v1 ]] || fail 'O instalador retornou outro perfil ou outra admissão.'
json() {
  local file="$1" key value; shift
  [[ ! -L "$file" ]] || fail 'Link em arquivo de configuração.'
  /usr/bin/plutil -create xml1 "$file"
  while [[ $# -gt 0 ]]; do key="$1"; value="$2"; shift 2; /usr/bin/plutil -insert "$key" -string "$value" "$file"; done
  /usr/bin/plutil -convert json "$file"
}
# Resolve normal installer symlinks before the original no-follow native review.
# This never executes the selected path or relaxes the native verifier.
resolve_code_executable() {
  local selected="$1" target count=0
  [[ "$selected" == /* && "$selected" != *$'\n'* && "$selected" != *$'\r'* ]] || return 1
  while :; do
    selected="$(cd "$(dirname "$selected")" && pwd -P)/$(basename "$selected")" || return 1
    if [[ ! -L "$selected" ]]; then break; fi
    ((count+=1)); [[ "$count" -le 32 ]] || return 1
    target="$(/usr/bin/readlink "$selected")" || return 1
    [[ -n "$target" && "$target" != *$'\n'* && "$target" != *$'\r'* ]] || return 1
    if [[ "$target" == /* ]]; then selected="$target"; else selected="$(dirname "$selected")/$target"; fi
  done
  [[ -f "$selected" && -x "$selected" && ! -L "$selected" ]] || return 1
  printf '%s' "$selected"
}
private_selection_path() {
  local selected="$1" walk
  [[ "$selected" == "$DEV/"* && "$selected" != *$'\n'* && "$selected" != *$'\r'* ]] || return 1
  case "$selected" in */../*|*/..|*/./*|*/.|*//*|*/) return 1;; esac
  walk="$selected"
  while [[ "$walk" != "$DEV" ]]; do [[ ! -L "$walk" ]] || return 1; walk="$(dirname "$walk")"; done
  printf '%s' "$selected"
}
require_disjoint() {
  local candidate="$1" reserved
  shift
  for reserved in "$@"; do
    reserved="$(private_selection_path "$reserved")" || fail 'Seleção privada tem link ou caminho ambíguo; preserve-a e confira o vínculo.'
    [[ "$candidate" != "$reserved" && "$candidate" != "$reserved/"* && "$reserved" != "$candidate/"* ]] || fail 'Escolha pastas separadas para perfil, clientes, brain, workspace e instalação.'
  done
}
prepare_code_configuration() {
  local profile="$1" answer
  [[ "$profile" == "$OWNER/"* && "$profile" != *$'\n'* && "$profile" != *$'\r'* ]] || fail 'O perfil Code deve ficar dentro do owner selecionado.'
  [[ -d "$(dirname "$profile")" && ! -L "$profile" ]] || fail 'Escolha um perfil privado com diretório pai existente; links são preservados.'
  profile="$(cd "$(dirname "$profile")" && pwd -P)/$(basename "$profile")"
  [[ "$profile" == "$OWNER/"* ]] || fail 'O perfil físico fica fora do owner selecionado.'
  profile="$(private_selection_path "$profile")" || fail 'Perfil com link ou caminho ambíguo; preserve-o.'
  require_disjoint "$profile" "$BRAIN" "$WORK" "$BASE" "$OWNER/.maestro-install" "$OWNER/Maestro-Clients"
  if [[ ! -e "$profile" ]]; then
    printf '\nCriar perfil Code privado em %s, sem copiar login nem alterar seu perfil normal.\nSomente neste perfil novo: memória automática e plugins sincronizados serão desativados; hooks e plugins próprios do Maestro passam pela qualificação normal.\nDigite YES para criar; Enter deixa pendente: ' "$profile" >&2
    read -r answer
    [[ "$answer" == YES ]] || fail 'Criação do perfil deixada pendente.'
    private_dir "$profile"
    (set -o noclobber; printf '%s\n' '{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}' > "$profile/settings.json") || fail 'Perfil mudou durante a preparação; preserve-o.'
  else
    private_dir "$profile"
  fi
  printf '%s' "$profile"
}
prepare_client_parent() {
  local parent="$OWNER/Maestro-Clients"
  require_disjoint "$parent" "$BRAIN" "$WORK" "$BASE" "$OWNER/.maestro-install"
  private_dir "$parent"
  private_dir "$parent/$ROLE"
  printf '%s' "$parent/$ROLE"
}
ensure_code_login() {
  local answer
  if [[ "${1:-}" != force ]] && /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$CODE" auth status > "$RUN/code-auth-private.json" 2> "$RUN/code-auth-private.log" && [[ "$(jget "$RUN/code-auth-private.json" loggedIn 2>/dev/null || true)" == true ]]; then return; fi
  printf '\nO perfil Code do piloto precisa de login oficial. Nenhuma credencial será copiada. Abrir login agora? Digite YES; Enter deixa pendente: '
  read -r answer
  [[ "$answer" == YES ]] || fail 'Login do perfil de teste pendente. Reabra este programa quando quiser continuar.'
  /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$CODE" auth login || fail 'Login não concluído; use o fluxo oficial deste perfil e retome.'
  /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$CODE" auth status > "$RUN/code-auth-private.json" 2> "$RUN/code-auth-private.log" || fail 'Login ainda pendente.'
  [[ "$(jget "$RUN/code-auth-private.json" loggedIn 2>/dev/null || true)" == true ]] || fail 'Login ainda pendente.'
}
CFG="$BASE/participant.json"
if [[ ! -e "$CFG" ]]; then
  [[ "${1:-}" != --check-only ]] || fail 'Ainda não preparado. Abra Maestro.command uma vez para escolher seu workspace e revisar a instalação.'
  printf '\nMaestro MCP — piloto experimental\nO programa e os relatórios ficam em Developer. Seu brain existente será preservado.\n'
  printf 'Diretório do owner existente em Developer [%s; Enter se nunca usou Maestro]: ' "$DEV"; read -r OWNER; OWNER="${OWNER:-$DEV}"
  [[ "$OWNER" == "$DEV" || "$OWNER" == "$DEV/"* ]] || fail 'Selecione o owner existente em Developer; não crie outro para contornar uma instalação.'
  [[ -d "$OWNER" && ! -L "$OWNER" ]] || fail 'O owner deve ser um diretório local existente.'
  OWNER="$(cd "$OWNER" && pwd -P)"
  [[ "$OWNER" == "$DEV" || "$OWNER" == "$DEV/"* ]] || fail 'O owner físico fica fora de Developer. Preserve o vínculo existente.'
  printf 'Workspace em Developer [%s/Maestro-Workspace]: ' "$DEV"; read -r WORK; WORK="${WORK:-$DEV/Maestro-Workspace}"
  [[ "$WORK" == /* && ! -L "$WORK" && -d "$(dirname "$WORK")" ]] || fail 'Escolha uma pasta local com o diretório pai existente.'
  WORK="$(cd "$(dirname "$WORK")" && pwd -P)/$(basename "$WORK")"; [[ "$WORK" == "$DEV/"* ]] || fail 'O workspace deve ficar em Developer.'
  default_code="$(command -v claude || true)"
  if [[ -z "$default_code" ]]; then
    for candidate in "$HOME/.local/bin/claude" /opt/homebrew/bin/claude /usr/local/bin/claude; do
      if [[ -x "$candidate" ]]; then default_code="$candidate"; break; fi
    done
  fi
  if [[ -n "$default_code" ]]; then default_code="$(resolve_code_executable "$default_code" || true)"; fi
  printf 'Executável absoluto do Claude Code [%s]: ' "$default_code"; read -r CODE; CODE="${CODE:-$default_code}"
  CODE="$(resolve_code_executable "$CODE")" || fail 'Instale o Claude Code pelo fluxo normal e escolha o executável absoluto.'
  BRAIN="${MAESTRO_HOME:-}"
  if [[ -z "$BRAIN" && -e "$OWNER/.maestro-home.json" ]]; then BRAIN="$(jget "$OWNER/.maestro-home.json" home)"; fi
  if [[ -z "$BRAIN" && -e "$OWNER/.maestro" ]]; then BRAIN="$OWNER/.maestro"; fi
  if [[ -z "$BRAIN" ]]; then
    printf 'Novo home do Maestro em Developer [%s/Maestro]: ' "$DEV"; read -r BRAIN; BRAIN="${BRAIN:-$DEV/Maestro}"
  fi
  [[ "$BRAIN" == "$DEV/"* ]] || fail 'O brain selecionado fica fora de Developer. Preserve-o e peça orientação ao time; este piloto não migra dados.'
  CLIENTS="$(prepare_client_parent)"
  printf 'Perfil Code privado [%s/Maestro-Code-Profile]: ' "$OWNER"; read -r CODE_CONFIG; CODE_CONFIG="${CODE_CONFIG:-$OWNER/Maestro-Code-Profile}"
  CODE_CONFIG="$(prepare_code_configuration "$CODE_CONFIG")"
  CARRIER="$(/usr/bin/uuidgen | tr -d '-' | tr '[:upper:]' '[:lower:]')"
  json "$CFG" admission_sha256 "$ROLE" user_home "$OWNER" workspace "$WORK" code_executable "$CODE" carrier_id "$CARRIER" brain_home "$BRAIN" source_root "$ROOT/source" client_parent "$CLIENTS" code_configuration_directory "$CODE_CONFIG"
fi
[[ -f "$CFG" && ! -L "$CFG" && "$(stat -f '%l' "$CFG")" == 1 && "$(jget "$CFG" admission_sha256)" == "$ROLE" ]] || fail 'Configuração local divergente.'
OWNER="$(jget "$CFG" user_home)"; WORK="$(jget "$CFG" workspace)"; CARRIER="$(jget "$CFG" carrier_id)"
CLIENTS="$(jget "$CFG" client_parent 2>/dev/null || true)"; CODE_CONFIG="$(jget "$CFG" code_configuration_directory 2>/dev/null || true)"
[[ -n "$CLIENTS" && -n "$CODE_CONFIG" ]] || fail 'Esta preparação é anterior ao perfil isolado. Preserve participant.json e o brain; envie o relatório para revisar a continuação do mesmo owner.'
[[ "$CLIENTS" == "$OWNER/"* && "$CODE_CONFIG" == "$OWNER/"* ]] || fail 'Seleção dos clientes difere do owner; preserve a configuração.'
[[ -d "$CLIENTS" && -d "$CODE_CONFIG" ]] || fail 'Seleção privada ausente; preserve a configuração e peça orientação, sem recriar perfil ou clientes.'
private_dir "$CLIENTS"; private_dir "$CODE_CONFIG"
BRAIN="$(jget "$CFG" brain_home)"; CODE="$(jget "$CFG" code_executable)"; SHARED="$(jget "$CFG" source_root)"
[[ -z "${MAESTRO_HOME:-}" || "$MAESTRO_HOME" == "$BRAIN" ]] || fail 'MAESTRO_HOME mudou; preserve o brain e confira a configuração.'
export MAESTRO_HOME="$BRAIN"
PRODUCT="$BASE/product"
COMMON=(--product-root "$PRODUCT" --user-home "$OWNER" --workspace "$WORK" --carrier-id "$CARRIER")
INSTALLED="$PRODUCT/maestro-install"
review_confirm() {
  local file="$1" answer
  /usr/bin/plutil -p "$file"
  printf '\nConfira os efeitos acima. Digite YES para aplicar; Enter deixa pendente: '; read -r answer
  [[ "$answer" == YES ]] || fail 'Operação deixada pendente por sua escolha.'
}
if [[ ! -e "$INSTALLED" ]]; then
  [[ "${1:-}" != --check-only ]] || fail 'Instalação pendente. Abra Maestro.command para revisar.'
  json "$RUN/setup-request.json" release_directory "$STAGE/initial-release" managed_root "$PRODUCT" user_home "$OWNER" workspace "$WORK" code_packet "$BASE/initial-code" desktop_packet "$BASE/initial-desktop.mcpb"
  "$BOOTSTRAP" review < "$RUN/setup-request.json" > "$RUN/setup-review.json" || fail 'A revisão recusou os dados. O brain e a instalação anterior foram preservados.'
  review_confirm "$RUN/setup-review.json"
  digest="$(jget "$RUN/setup-review.json" review_digest)"
  "$BOOTSTRAP" apply --review-digest "$digest" --confirm < "$RUN/setup-request.json" > "$RUN/setup-result.json" || fail 'Preparação parcial. Preserve esta pasta e envie o relatório; nenhuma nova tentativa foi aplicada automaticamente.'
fi
verify_installer "$INSTALLED"
manage() {
  verify_installer "$INSTALLED"
  ensure_code_login
  /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$INSTALLED" manage "${COMMON[@]}" --client-code-root "$CLIENTS/code" --client-desktop-path "$CLIENTS/maestro-product.mcpb" --client-code-executable "$CODE" --client-code-config "$CODE_CONFIG" --client-scope user "$@"
}
current_clients=false
if "$INSTALLED" clients-status "${COMMON[@]}" > "$RUN/clients-before-private.json" 2> "$RUN/clients-before-private.log"; then
  [[ "$(jget "$RUN/clients-before-private.json" state)" != current_materialization ]] || current_clients=true
fi
if [[ "${1:-}" != --check-only && "$current_clients" != true ]]; then
  json "$RUN/source-request.json" managed_root "$PRODUCT" user_home "$OWNER" source_root "$SHARED"
  /usr/bin/plutil -insert source -dictionary "$RUN/source-request.json"
  for field in source_id channel os arch; do /usr/bin/plutil -insert "source.$field" -string "$(jget "$META" "identity.source.$field")" "$RUN/source-request.json"; done
  "$INSTALLED" source-review < "$RUN/source-request.json" > "$RUN/source-review.json" || fail 'Fonte compartilhada indisponível ou divergente. Mantenha a pasta offline e tente novamente.'
  review_confirm "$RUN/source-review.json"
  digest="$(jget "$RUN/source-review.json" digest)"
  "$INSTALLED" source-check --review-digest "$digest" --confirm < "$RUN/source-request.json" > "$RUN/source-result.json" || fail 'Fonte não foi aceita; nada foi atualizado automaticamente.'
  state="$(jget "$RUN/source-result.json" state)"
  if [[ "$state" == candidate_staged_activation_unconfirmed ]]; then
    plan="$(jget "$RUN/source-result.json" plan_id)"
    "$INSTALLED" update-review --plan-id "$plan" < "$RUN/source-request.json" > "$RUN/update-review.json" || fail 'Atualização pendente de revisão.'
    review_confirm "$RUN/update-review.json"
    "$INSTALLED" update --plan-id "$plan" --confirm < "$RUN/source-request.json" > "$RUN/update-result.json" || fail 'Atualização parcial; preserve o diagnóstico e use o gerenciador para reconciliar.'
  fi
  printf '\nO próximo passo prepara e registra os clientes com o perfil completo. Se faltar login, prova nativa ou permissão, continuará pendente; não escolha resources-only para contornar.\n'
  if ! manage; then printf '\nRegistro/qualificação pendente. O relatório abaixo preserva esse estado.\n'; fi
fi
# Current installer owns path, generation and registration verification.
"$INSTALLED" clients-status "${COMMON[@]}" > "$RUN/clients-status.json" || true
SERVER="$PRODUCT/bin/maestro-mcp"
[[ -f "$SERVER" && ! -L "$SERVER" && "$(stat -f '%l' "$SERVER")" == 1 ]] || fail 'Backend atual indisponível.'
server_sha="$(sha "$SERVER")"
[[ "$server_sha" == "$(jget "$META" server_hashes.0)" || "$server_sha" == "$(jget "$META" server_hashes.1)" ]] || fail 'Backend fora das duas versões admitidas. Nada será executado.'
status=0
"$SERVER" installed-pilot-check "${COMMON[@]}" --installer-version "$VERSION" --experimental-seed-sha256 "$ROLE" --experimental-installed-root "$PRODUCT" --check-only > "$RUN/report.json" 2> "$RUN/check-private.log" || status=$?
[[ -f "$RUN/report.json" && "$(jget "$RUN/report.json" schema_version)" == 2 && "$(jget "$RUN/report.json" evidence_class)" == engineering_only ]] || fail 'Verificador não produziu um relatório reconhecido.'
for pair in 'markdown|RELATORIO.md' 'code_prompt|CODE.txt' 'code_readback_prompt|CODE-RELEITURA.txt' 'desktop_prompt|DESKTOP.txt' 'code_ready_prompt|CODE-CONEXAO.txt' 'desktop_ready_prompt|DESKTOP-CONEXAO.txt'; do
  /usr/bin/plutil -extract "${pair%%|*}" raw -o "$RUN/${pair#*|}" "$RUN/report.json" || fail 'Relatório incompleto.'
done
printf '\nRelatório para compartilhar: %s/RELATORIO.md\nOs JSONs e logs são privados; não envie a pasta inteira.\n' "$RUN"
[[ "${1:-}" != --check-only ]] || exit "$status"
attest() {
  local label answer
  printf '\nObservações suas (não são provas automáticas). Use passou, falhou ou nao-testei.\n'
  printf '\n## Observações do participante\n\nAutodeclarações; não substituem qualificação nativa.\n' >> "$RUN/RELATORIO.md"
  for label in 'Code: identidade/skills' 'Desktop: conexão/skills' 'Onboarding e mesmo workspace' 'Memória: salvar e reler' 'Hooks e delegação' 'Conversão de documento'; do
    printf '%s: ' "$label"; read -r answer
    case "$answer" in passou|falhou|nao-testei) ;; *) answer=nao-testei;; esac
    printf '\n- %s: %s (participante)\n' "$label" "$answer" >> "$RUN/RELATORIO.md"
  done
}
guard_test_prompt() {
  [[ "$(jget "$RUN/report.json" status)" != blocked ]] || fail 'O diagnóstico bloqueou a jornada. Preserve o relatório; nenhum cliente será aberto.'
  [[ -f "$RUN/$1" && -n "$(cat "$RUN/$1")" ]] || fail 'O prompt deste teste está indisponível. Reabra o diagnóstico antes de continuar.'
}
prepare_conversion_document() {
  local current="$1" prompt="$2" state expected_manifest expected_pack result server_sha
  [[ "$prompt" == CODE.txt || "$prompt" == DESKTOP.txt ]] || fail 'Documento disponível somente nas opções completas 1 e 2.'
  guard_test_prompt "$prompt"
  state="$(jget "$RUN/report.json" conversion.state)"
  if [[ "$state" == pending_runtime ]]; then printf 'Conversão pendente: o runtime atual não está pronto. O restante do ensaio pode continuar; nenhum documento será criado.\n'; return 0; fi
  [[ "$state" == ready_for_participant_test ]] || fail 'Identidade do conversor divergente ou indisponível; a jornada foi interrompida.'
  expected_manifest="$(jget "$RUN/report.json" conversion.expected_manifest_sha256)"
  expected_pack="$(jget "$RUN/report.json" conversion.expected_pack_sha256)"
  [[ "$expected_manifest" =~ ^[0-9a-f]{64}$ && "$expected_pack" =~ ^[0-9a-f]{64}$ && "$(jget "$current" state)" == current_materialization && "$(jget "$current" record.binding.workspace)" == "$WORK" && "$(jget "$current" record.materialization.manifest_sha256)" == "$expected_manifest" ]] || fail 'O workspace ou a versão atual mudou desde o diagnóstico. Reabra o teste.'
  [[ "$(jget "$RUN/report.json" conversion.source)" == maestro-pilot.docx && "$(jget "$RUN/report.json" conversion.source_sha256)" == 6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81 && -f "$STAGE/test-documents/maestro-pilot.docx" && ! -L "$STAGE/test-documents/maestro-pilot.docx" && "$(sha "$STAGE/test-documents/maestro-pilot.docx")" == 6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81 ]] || fail 'O documento sintético divergiu do ensaio. Nenhum documento foi preparado.'
  [[ -f "$SERVER" && ! -L "$SERVER" && "$(stat -f '%l' "$SERVER")" == 1 ]] || fail 'Backend atual indisponível.'
  server_sha="$(sha "$SERVER")"
  [[ "$server_sha" == "$(jget "$META" server_hashes.0)" || "$server_sha" == "$(jget "$META" server_hashes.1)" ]] || fail 'O backend mudou desde o diagnóstico. Reabra o teste.'
  result="$(mktemp "$RUN/prepare-document.XXXXXXXX")"
  "$SERVER" installed-pilot-prepare-document "${COMMON[@]}" --installer-version "$VERSION" --experimental-seed-sha256 "$ROLE" --experimental-installed-root "$PRODUCT" > "$result" || fail 'O documento não foi preparado. Preserve qualquer arquivo existente e o diagnóstico; nada será sobrescrito.'
  state="$(jget "$result" state)"
  [[ "$(jget "$result" schema_version)" == 1 && ( "$state" == fixture_prepared || "$state" == fixture_already_present ) && "$(jget "$result" source)" == maestro-pilot.docx && "$(jget "$result" source_sha256)" == 6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81 ]] || fail 'O preparador não devolveu o documento esperado. A jornada foi interrompida.'
  printf 'Documento sintético preparado ou já presente sem sobrescrita. Conversão e execução nativa ainda não foram testadas; siga o prompt no cliente.\n'
}
open_code() {
  guard_test_prompt "$1"
  "$INSTALLED" clients-status "${COMMON[@]}" > "$RUN/clients-status.json" || fail 'Registro atual indisponível.'
  [[ "$(jget "$RUN/clients-status.json" state)" == current_materialization && "$(jget "$RUN/clients-status.json" record.code.state)" == registered && "$(jget "$RUN/clients-status.json" record.code.enabled)" == true ]] || fail 'Code ainda não está registrado e habilitado; conclua pelo gerenciador.'
  "$INSTALLED" client-plugin-path "${COMMON[@]}" > "$RUN/current-plugin.txt" || fail 'Plugin atual não admitido.'
  local executable expected observed_profile
  executable="$(jget "$RUN/clients-status.json" record.code.executable)"; expected="$(jget "$RUN/clients-status.json" record.code.executable_sha256)"
  [[ "$executable" == /* && -f "$executable" && "$(sha "$executable")" == "$expected" ]] || fail 'O Code selecionado mudou; revise o registro pelo gerenciador.'
  observed_profile="$(jget "$RUN/clients-status.json" record.code.configuration_directory 2>/dev/null || true)"
  [[ "$observed_profile" == "$CODE_CONFIG" ]] || fail 'O perfil Code registrado divergiu; preserve o registro e confira pelo gerenciador.'
  if [[ "$1" == CODE.txt ]]; then prepare_conversion_document "$RUN/clients-status.json" "$1"; fi
  (cd "$WORK" && /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$executable" "$(cat "$RUN/$1")")
}
desktop_install_is_current() {
  local file="$1" confirmation evidence observed_package observed_contract package contract
  confirmation="$(jget "$file" record.desktop.install_confirmation 2>/dev/null || true)"
  evidence="$(jget "$file" record.desktop.evidence_class 2>/dev/null || true)"
  observed_package="$(jget "$file" record.desktop.package_sha256 2>/dev/null || true)"
  observed_contract="$(jget "$file" record.desktop.native_contract_sha256 2>/dev/null || true)"
  package="$(jget "$file" record.materialization.desktop_sha256)"
  contract="$(jget "$file" record.materialization.desktop_native_contract_sha256)"
  [[ "$confirmation" == user_confirmed_install && "$evidence" == operator_attested && "$package" =~ ^[0-9a-f]{64}$ && "$contract" =~ ^[0-9a-f]{64}$ && "$observed_package" == "$package" && "$observed_contract" == "$contract" ]]
}
open_desktop() {
  local packet methods method_version
  guard_test_prompt "$1"
  "$INSTALLED" clients-status "${COMMON[@]}" > "$RUN/clients-status.json" || fail 'Registro atual indisponível.'
  [[ "$(jget "$RUN/clients-status.json" state)" == current_materialization ]] || fail 'Pacotes ainda não estão atuais.'
  packet="$(jget "$RUN/clients-status.json" record.materialization.desktop_path)"
  methods="$(jget "$RUN/clients-status.json" record.materialization.desktop_methods.archive)"
  method_version="$(jget "$RUN/clients-status.json" record.materialization.desktop_methods.version)"
  if [[ "$1" == DESKTOP.txt ]]; then prepare_conversion_document "$RUN/clients-status.json" "$1"; fi
  /usr/bin/pbcopy < "$RUN/$1"
  if desktop_install_is_current "$RUN/clients-status.json"; then
    printf '\nA instalação deste pacote/contrato já foi confirmada por você. O instalador da extensão não será reaberto; conexão e funcionamento continuam pendentes de teste.\n'
  else
    printf '\nA instalação deste pacote/contrato ainda precisa de confirmação atual. Instale/ative somente esta extensão e confirme o pacote pelo gerenciador (opção 5).\n'
    /usr/bin/open "$packet"
  fi
  printf 'Biblioteca de skills %s: %s\nA importação no Desktop não foi observada pelo menu. Se a biblioteca ainda não estiver disponível no cliente, importe esse arquivo pelo fluxo normal.\n' "$method_version" "$methods"
  printf 'Prompt copiado. Abra sua conversa no Desktop, use somente o Maestro deste ensaio, cole uma vez e execute. Depois registre o resultado no menu 7.\n'
}
while [[ -t 0 ]]; do
  printf '\n1 — Ensaio completo no Code\n2 — Retomar no Desktop (um prompt)\n3 — Conferir conexão Code\n4 — Conferir conexão Desktop\n5 — Gerenciar/qualificar hooks e skills\n6 — Conferir persistência no Code\n7 — Registrar resultados e abrir relatório\n8 — Atualizar da pasta compartilhada\n9 — Renovar login do perfil Code do piloto\nEnter — Sair\nEscolha: '
  read -r choice
  case "$choice" in
    1) open_code CODE.txt;; 2) open_desktop DESKTOP.txt;; 3) open_code CODE-CONEXAO.txt;; 4) open_desktop DESKTOP-CONEXAO.txt;;
    5) manage; printf 'Após concluir, reabra Maestro.command para gerar um diagnóstico atual.\n'; break;;
    6) open_code CODE-RELEITURA.txt;; 7) attest; /usr/bin/open -t "$RUN/RELATORIO.md";;
    8) manage --management-intent update; printf 'Reabra Maestro.command para verificar a nova geração.\n'; break;;
    9) ensure_code_login force;;
    '') break;; *) printf 'Escolha uma opção da lista.\n';;
  esac
done
