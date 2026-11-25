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

prepare_initial_server() {
  local manifest="$STAGE/initial-release/release-manifest.json" index=0 matches=0 name="" expected="" size="" manifest_sha="" manifest_size="" server walk mode
  # The factory pins this manifest only after original signed-release/role checks.
  # Revalidate its staged inventory identity before reading a component selection.
  while /usr/bin/plutil -extract "files.$index.path" raw -o - "$META" >/dev/null 2>&1; do
    if [[ "$(jget "$META" "files.$index.path")" == initial-release/release-manifest.json ]]; then
      ((matches+=1)); manifest_sha="$(jget "$META" "files.$index.sha256")"; manifest_size="$(jget "$META" "files.$index.size")"
    fi
    ((index+=1))
  done
  [[ "$matches" == 1 && -f "$manifest" && ! -L "$manifest" && "$(stat -f '%l' "$manifest")" == 1 && "$(stat -f '%u' "$manifest")" == "$(id -u)" && "$(stat -f '%z' "$manifest")" == "$manifest_size" && "$(sha "$manifest")" == "$manifest_sha" ]] || fail 'Manifesto inicial privado divergente; preserve a preparação.'
  index=0; matches=0
  while /usr/bin/plutil -extract "artifacts.$index.kind" raw -o - "$manifest" >/dev/null 2>&1; do
    if [[ "$(jget "$manifest" "artifacts.$index.kind")" == mcp_server && "$(jget "$manifest" "artifacts.$index.os")" == darwin && "$(jget "$manifest" "artifacts.$index.arch")" == "$ARCH" ]]; then
      ((matches+=1)); name="$(jget "$manifest" "artifacts.$index.name")"; expected="$(jget "$manifest" "artifacts.$index.sha256")"; size="$(jget "$manifest" "artifacts.$index.size")"
    fi
    ((index+=1))
  done
  [[ "$matches" == 1 && "$name" =~ ^maestro-mcp_[A-Za-z0-9_.-]+_darwin_(arm64|amd64)$ && "$name" != *..* && "$expected" =~ ^[0-9a-f]{64}$ && "$size" =~ ^[0-9]+$ ]] || fail 'Componente inicial não corresponde ao alvo exato deste Mac.'
  [[ "$expected" == "$(jget "$META" server_hashes.0)" || "$expected" == "$(jget "$META" server_hashes.1)" ]] || fail 'Componente inicial fora das versões admitidas.'
  index=0; matches=0
  while /usr/bin/plutil -extract "files.$index.path" raw -o - "$META" >/dev/null 2>&1; do
    if [[ "$(jget "$META" "files.$index.path")" == "initial-release/$name" ]]; then
      [[ "$(jget "$META" "files.$index.sha256")" == "$expected" && "$(jget "$META" "files.$index.size")" == "$size" ]] || fail 'Inventário do componente inicial divergente.'
      ((matches+=1))
    fi
    ((index+=1))
  done
  server="$STAGE/initial-release/$name"; walk="$(dirname "$server")"
  while [[ "$walk" != "$STAGE" ]]; do [[ ! -L "$walk" ]] || fail 'Link na preparação do componente inicial.'; walk="$(dirname "$walk")"; done
  [[ "$matches" == 1 && -d "$STAGE" && ! -L "$STAGE" && -f "$server" && ! -L "$server" && "$(stat -f '%l' "$server")" == 1 && "$(stat -f '%u' "$server")" == "$(id -u)" && "$(stat -f '%z' "$server")" == "$size" && "$(sha "$server")" == "$expected" ]] || fail 'Componente inicial privado divergente; nenhum outro arquivo será ajustado.'
  mode="$(stat -f '%Lp' "$server")"; (( (8#$mode & 077) == 0 )) || fail 'Componente inicial permite acesso a outros usuários.'
  chmod -h 700 "$server" || fail 'Não foi possível preparar o componente inicial autenticado.'
  [[ -f "$server" && ! -L "$server" && "$(stat -f '%l' "$server")" == 1 && "$(stat -f '%u' "$server")" == "$(id -u)" && "$(stat -f '%Lp' "$server")" == 700 && "$(stat -f '%z' "$server")" == "$size" && "$(sha "$server")" == "$expected" ]] || fail 'Componente inicial mudou durante a preparação; preserve os arquivos.'
}
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
prepare_initial_server
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
    printf '\nCriar perfil Code privado em %s, sem copiar login nem alterar seu perfil normal.\nSomente neste perfil novo: memória automática e sincronização de skills e plugins da conta serão desativadas; hooks e plugins próprios do Maestro passam pela qualificação normal.\nDigite YES para criar; Enter deixa pendente: ' "$profile" >&2
    read -r answer
    [[ "$answer" == YES ]] || fail 'Criação do perfil deixada pendente.'
    private_dir "$profile"
    (set -o noclobber; printf '%s\n' '{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false,"syncClaudeAiSkills":false}' > "$profile/settings.json") || fail 'Perfil mudou durante a preparação; preserve-o.'
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
  [[ "${TARGET:-}" != desktop ]] || fail 'Este alvo Desktop não configura login Code.'
  local answer
  if [[ "${1:-}" != force ]] && /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$CODE" auth status > "$RUN/code-auth-private.json" 2> "$RUN/code-auth-private.log" && [[ "$(jget "$RUN/code-auth-private.json" loggedIn 2>/dev/null || true)" == true ]]; then return; fi
  printf '\nO perfil Code do piloto precisa de login oficial. Nenhuma credencial será copiada. Abrir login agora? Digite YES; Enter deixa pendente: '
  read -r answer
  if [[ "$answer" != YES ]]; then printf '\nCode continua pendente; nenhuma chamada de login foi iniciada.\n'; return 1; fi
  /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$CODE" auth login || fail 'Login não concluído; use o fluxo oficial deste perfil e retome.'
  /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$CODE" auth status > "$RUN/code-auth-private.json" 2> "$RUN/code-auth-private.log" || fail 'Login ainda pendente.'
  [[ "$(jget "$RUN/code-auth-private.json" loggedIn 2>/dev/null || true)" == true ]] || fail 'Login ainda pendente.'
}
CFG="$BASE/participant.json"
if [[ ! -e "$CFG" ]]; then
  [[ "${1:-}" != --check-only ]] || fail 'Ainda não preparado. Abra Maestro.command uma vez para escolher seu workspace e revisar a instalação.'
  printf '\nMaestro MCP — piloto experimental\nO programa e os relatórios ficam em Developer. Seu brain existente será preservado.\n'
  printf 'Clientes: both para Code e Desktop, ou desktop sem configurar Code [both]: '; read -r TARGET
  case "$TARGET" in ''|both) TARGET="";; desktop) ;; *) fail 'Escolha both ou desktop; nenhuma seleção foi salva.';; esac
  CODE=""; CODE_CONFIG=""
  printf 'Diretório do owner existente em Developer [%s; Enter se nunca usou Maestro]: ' "$DEV"; read -r OWNER; OWNER="${OWNER:-$DEV}"
  [[ "$OWNER" == "$DEV" || "$OWNER" == "$DEV/"* ]] || fail 'Selecione o owner existente em Developer; não crie outro para contornar uma instalação.'
  [[ -d "$OWNER" && ! -L "$OWNER" ]] || fail 'O owner deve ser um diretório local existente.'
  OWNER="$(cd "$OWNER" && pwd -P)"
  [[ "$OWNER" == "$DEV" || "$OWNER" == "$DEV/"* ]] || fail 'O owner físico fica fora de Developer. Preserve o vínculo existente.'
  printf 'Workspace em Developer [%s/Maestro-Workspace]: ' "$DEV"; read -r WORK; WORK="${WORK:-$DEV/Maestro-Workspace}"
  [[ "$WORK" == /* && ! -L "$WORK" && -d "$(dirname "$WORK")" ]] || fail 'Escolha uma pasta local com o diretório pai existente.'
  WORK="$(cd "$(dirname "$WORK")" && pwd -P)/$(basename "$WORK")"; [[ "$WORK" == "$DEV/"* ]] || fail 'O workspace deve ficar em Developer.'
  if [[ "$TARGET" != desktop ]]; then
  default_code="$(command -v claude || true)"
  if [[ -z "$default_code" ]]; then
    for candidate in "$HOME/.local/bin/claude" /opt/homebrew/bin/claude /usr/local/bin/claude; do
      if [[ -x "$candidate" ]]; then default_code="$candidate"; break; fi
    done
  fi
  if [[ -n "$default_code" ]]; then default_code="$(resolve_code_executable "$default_code" || true)"; fi
  printf 'Executável absoluto do Claude Code [%s]: ' "$default_code"; read -r CODE; CODE="${CODE:-$default_code}"
  CODE="$(resolve_code_executable "$CODE")" || fail 'Instale o Claude Code pelo fluxo normal e escolha o executável absoluto.'
  fi
  BRAIN="${MAESTRO_HOME:-}"
  if [[ -z "$BRAIN" && -e "$OWNER/.maestro-home.json" ]]; then BRAIN="$(jget "$OWNER/.maestro-home.json" home)"; fi
  if [[ -z "$BRAIN" && -e "$OWNER/.maestro" ]]; then BRAIN="$OWNER/.maestro"; fi
  if [[ -z "$BRAIN" ]]; then
    printf 'Novo home do Maestro em Developer [%s/Maestro]: ' "$DEV"; read -r BRAIN; BRAIN="${BRAIN:-$DEV/Maestro}"
  fi
  [[ "$BRAIN" == "$DEV/"* ]] || fail 'O brain selecionado fica fora de Developer. Preserve-o e peça orientação ao time; este piloto não migra dados.'
  CLIENTS="$(prepare_client_parent)"
  if [[ "$TARGET" != desktop ]]; then
  printf 'Perfil Code privado [%s/Maestro-Code-Profile]: ' "$OWNER"; read -r CODE_CONFIG; CODE_CONFIG="${CODE_CONFIG:-$OWNER/Maestro-Code-Profile}"
  CODE_CONFIG="$(prepare_code_configuration "$CODE_CONFIG")"
  fi
  CARRIER="$(/usr/bin/uuidgen | tr -d '-' | tr '[:upper:]' '[:lower:]')"
  if [[ "$TARGET" == desktop ]]; then
    json "$CFG" admission_sha256 "$ROLE" user_home "$OWNER" workspace "$WORK" carrier_id "$CARRIER" brain_home "$BRAIN" client_parent "$CLIENTS" client_target desktop
  else
  json "$CFG" admission_sha256 "$ROLE" user_home "$OWNER" workspace "$WORK" code_executable "$CODE" carrier_id "$CARRIER" brain_home "$BRAIN" client_parent "$CLIENTS" code_configuration_directory "$CODE_CONFIG"
  fi
fi
[[ -f "$CFG" && ! -L "$CFG" && "$(stat -f '%l' "$CFG")" == 1 && "$(jget "$CFG" admission_sha256)" == "$ROLE" ]] || fail 'Configuração local divergente.'
OWNER="$(jget "$CFG" user_home)"; WORK="$(jget "$CFG" workspace)"; CARRIER="$(jget "$CFG" carrier_id)"
TARGET="$(jget "$CFG" client_target 2>/dev/null || true)"
case "$TARGET" in ''|desktop) ;; *) fail 'Alvo local desconhecido; preserve participant.json.';; esac
CLIENTS="$(jget "$CFG" client_parent 2>/dev/null || true)"; CODE_CONFIG="$(jget "$CFG" code_configuration_directory 2>/dev/null || true)"
CODE="$(jget "$CFG" code_executable 2>/dev/null || true)"
[[ -n "$CLIENTS" && "$CLIENTS" == "$OWNER/"* && -d "$CLIENTS" ]] || fail 'Seleção privada dos clientes ausente ou fora do owner; preserve a configuração.'
private_dir "$CLIENTS"
if [[ "$TARGET" == desktop ]]; then
  [[ -z "$CODE" && -z "$CODE_CONFIG" ]] || fail 'A seleção Desktop contém configuração Code; preserve a configuração.'
else
  [[ -n "$CODE_CONFIG" && "$CODE_CONFIG" == "$OWNER/"* && -d "$CODE_CONFIG" ]] || fail 'Esta preparação é anterior ao perfil isolado, ou o perfil está ausente. Preserve participant.json e o brain; envie o relatório.'
  private_dir "$CODE_CONFIG"
fi
BRAIN="$(jget "$CFG" brain_home)"
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
  if [[ "$TARGET" == desktop ]]; then
    json "$RUN/setup-request.json" release_directory "$STAGE/initial-release" managed_root "$PRODUCT" user_home "$OWNER" workspace "$WORK" target desktop
  else
  json "$RUN/setup-request.json" release_directory "$STAGE/initial-release" managed_root "$PRODUCT" user_home "$OWNER" workspace "$WORK" code_packet "$BASE/initial-code" desktop_packet "$BASE/initial-desktop.mcpb"
  fi
  "$BOOTSTRAP" review < "$RUN/setup-request.json" > "$RUN/setup-review.json" || fail 'A revisão recusou os dados. O brain e a instalação anterior foram preservados.'
  review_confirm "$RUN/setup-review.json"
  digest="$(jget "$RUN/setup-review.json" review_digest)"
  "$BOOTSTRAP" apply --review-digest "$digest" --confirm < "$RUN/setup-request.json" > "$RUN/setup-result.json" || fail 'Preparação parcial. Preserve esta pasta e envie o relatório; nenhuma nova tentativa foi aplicada automaticamente.'
fi
verify_installer "$INSTALLED"
manage() {
  verify_installer "$INSTALLED"
  if [[ "$TARGET" == desktop ]]; then
    "$INSTALLED" manage "${COMMON[@]}" --client-target desktop --client-desktop-path "$CLIENTS/maestro-product.mcpb" "$@"
  else
    local intent="" pending_intent=prepare-desktop answer status_file intent_args=()
    if [[ $# -gt 0 ]]; then
      [[ $# -eq 2 && "$1" == --management-intent && "$2" == update ]] || fail 'Escolha de gerenciamento não reconhecida; nenhuma preparação foi iniciada.'
      intent=update; pending_intent=update-desktop
    fi
    printf '\nPreparar ou retomar o Code agora? Digite YES; Enter prepara o Desktop e mantém o Code pendente, sem consultar login nem executar o Code: '
    read -r answer || answer=""
    if [[ "$answer" != YES ]] || ! ensure_code_login; then
      PILOT_OBSERVATION_CLIENT=desktop
      printf '\nPreparando somente a parte Desktop da seleção ambos. Code, hooks e qualificação continuam pendentes.\n'
      "$INSTALLED" manage "${COMMON[@]}" --client-code-root "$CLIENTS/code" --client-desktop-path "$CLIENTS/maestro-product.mcpb" --client-code-executable "$CODE" --client-code-config "$CODE_CONFIG" --client-scope user --management-intent "$pending_intent"
      return
    fi
    PILOT_OBSERVATION_CLIENT=""
    if [[ -z "$intent" ]]; then
      status_file="$(mktemp "$RUN/manage-clients-XXXXXXXX")"
      if "$INSTALLED" clients-status "${COMMON[@]}" > "$status_file" 2> "$status_file.log" && [[ "$(jget "$status_file" state 2>/dev/null || true)" == partial_preparation ]] && desktop_clients_available "$status_file"; then intent=resume-code; fi
    fi
    if [[ -n "$intent" ]]; then intent_args=(--management-intent "$intent"); fi
    /usr/bin/env "CLAUDE_CONFIG_DIR=$CODE_CONFIG" "$INSTALLED" manage "${COMMON[@]}" --client-code-root "$CLIENTS/code" --client-desktop-path "$CLIENTS/maestro-product.mcpb" --client-code-executable "$CODE" --client-code-config "$CODE_CONFIG" --client-scope user ${intent_args[@]+"${intent_args[@]}"}
  fi
}
CLIENT_STATUS=("${COMMON[@]}")
if [[ "$TARGET" == desktop ]]; then CLIENT_STATUS+=(--client-target desktop); fi
desktop_clients_available() {
  local file="$1" state
  state="$(jget "$file" state 2>/dev/null || true)"
  [[ "$state" != current_materialization ]] || return 0
  [[ "$state" == partial_preparation && "$(jget "$file" preparation.phase 2>/dev/null || true)" == desktop_prepared_code_pending && "$(jget "$file" preparation.selected_target 2>/dev/null || true)" == both && "$(jget "$file" preparation.desktop 2>/dev/null || true)" == prepared && "$(jget "$file" preparation.code 2>/dev/null || true)" == pending_qualification ]]
}
current_clients=false
if "$INSTALLED" clients-status "${CLIENT_STATUS[@]}" > "$RUN/clients-before-private.json" 2> "$RUN/clients-before-private.log"; then
  if desktop_clients_available "$RUN/clients-before-private.json"; then
    if [[ "$(jget "$RUN/clients-before-private.json" state)" == partial_preparation ]]; then
      [[ -z "$TARGET" ]] || fail 'A preparação parcial difere da seleção salva; preserve o carrier.'
    else
      [[ "$(jget "$RUN/clients-before-private.json" record.materialization.target 2>/dev/null || true)" == "$TARGET" ]] || fail 'O alvo atual difere da seleção salva; preserve o carrier e revise pelo gerenciador.'
    fi
    current_clients=true
  fi
fi
if [[ "${1:-}" != --check-only && "$current_clients" != true ]]; then
  json "$RUN/source-request.json" managed_root "$PRODUCT" user_home "$OWNER"
  # This hint controls only first-choice UI; the installed owner validates state and binds the source.
  source_state="$OWNER/.maestro-install/config/install-state.json"
  first_source=false
  if [[ -f "$source_state" && ! -L "$source_state" && "$(stat -f '%l' "$source_state")" == 1 && "$(stat -f '%u' "$source_state")" == "$(id -u)" ]] && /usr/bin/plutil -convert json -o - "$source_state" >/dev/null 2>&1 && [[ "$(/usr/bin/plutil -type schema_version "$source_state")" == integer && "$(jget "$source_state" schema_version)" == 4 && "$(/usr/bin/plutil -type managed_root "$source_state")" == string && "$(jget "$source_state" managed_root)" == "$PRODUCT" ]]; then
    if source_kind="$(/usr/bin/plutil -type source_control "$source_state" 2>/dev/null)"; then
      [[ "$source_kind" != null ]] || first_source=true
    else
      first_source=true
    fi
  fi
  if [[ "$first_source" == true ]]; then
    printf '\nSelecione a pasta SOURCE sincronizada do time e disponível offline, separada do ZIP local.\nDigite TESTE-LOCAL somente para ensaio com a amostra deste kit; ela não acompanha a origem do time.\nPasta SOURCE (Enter deixa pendente): '
    read -r SHARED
    [[ -n "$SHARED" ]] || fail 'Escolha da origem deixada pendente; nenhuma origem foi inscrita.'
    if [[ "$SHARED" == TESTE-LOCAL ]]; then SHARED="$ROOT/source"; printf '\nEnsaio local, sem acompanhar outra pasta compartilhada: %s\n' "$SHARED"; fi
    /usr/bin/plutil -insert source_root -string "$SHARED" "$RUN/source-request.json"
    /usr/bin/plutil -insert source -dictionary "$RUN/source-request.json"
    for field in source_id channel os arch; do /usr/bin/plutil -insert "source.$field" -string "$(jget "$META" "identity.source.$field")" "$RUN/source-request.json"; done
  fi
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
"$INSTALLED" clients-status "${CLIENT_STATUS[@]}" > "$RUN/clients-status.json" || true
SERVER="$PRODUCT/bin/maestro-mcp"
[[ -f "$SERVER" && ! -L "$SERVER" && "$(stat -f '%l' "$SERVER")" == 1 ]] || fail 'Backend atual indisponível.'
server_sha="$(sha "$SERVER")"
[[ "$server_sha" == "$(jget "$META" server_hashes.0)" || "$server_sha" == "$(jget "$META" server_hashes.1)" ]] || fail 'Backend fora das duas versões admitidas. Nada será executado.'
DIAGNOSTIC_ROOT="$RUN"
refresh_report() {
  local pair current_sha mode="${1:---check-only}" scope=()
  private_dir "$DIAGNOSTIC_ROOT"
  private_dir "$RUN"
  if [[ -e "$RUN/report.json" || -L "$RUN/report.json" ]]; then RUN="$(mktemp -d "$DIAGNOSTIC_ROOT/recheck-XXXXXXXX")"; fi
  private_dir "$RUN"
  verify_installer "$INSTALLED"
  "$INSTALLED" clients-status "${CLIENT_STATUS[@]}" > "$RUN/clients-status.json" || true
  [[ -f "$SERVER" && ! -L "$SERVER" && "$(stat -f '%l' "$SERVER")" == 1 ]] || fail 'Backend atual indisponível.'
  current_sha="$(sha "$SERVER")"
  [[ "$current_sha" == "$(jget "$META" server_hashes.0)" || "$current_sha" == "$(jget "$META" server_hashes.1)" ]] || fail 'Backend fora das versões admitidas. Nada será executado.'
  if [[ "${PILOT_OBSERVATION_CLIENT:-}" == desktop ]]; then scope=(--observation-client desktop); fi
  status=0
  "$SERVER" installed-pilot-check "${COMMON[@]}" --installer-version "$VERSION" --experimental-seed-sha256 "$ROLE" --experimental-installed-root "$PRODUCT" "$mode" ${scope[@]+"${scope[@]}"} > "$RUN/report.json" 2> "$RUN/check-private.log" || status=$?
  [[ -f "$RUN/report.json" && "$(jget "$RUN/report.json" schema_version)" == 2 && "$(jget "$RUN/report.json" evidence_class)" == engineering_only ]] || fail 'Verificador não produziu um relatório reconhecido.'
  for pair in 'markdown|RELATORIO.md' 'code_prompt|CODE.txt' 'code_readback_prompt|CODE-RELEITURA.txt' 'desktop_prompt|DESKTOP.txt' 'code_ready_prompt|CODE-CONEXAO.txt' 'desktop_ready_prompt|DESKTOP-CONEXAO.txt'; do
    /usr/bin/plutil -extract "${pair%%|*}" raw -o "$RUN/${pair#*|}" "$RUN/report.json" || fail 'Relatório incompleto.'
  done
  CODE_PREPARATION_PENDING=false
  if [[ "$(jget "$RUN/report.json" client_preparation 2>/dev/null || true)" == desktop_prepared_code_pending ]]; then CODE_PREPARATION_PENDING=true; fi
  printf '\nRelatório atual: %s/RELATORIO.md\nOs relatórios anteriores foram preservados.\n' "$RUN"
  if [[ "$mode" == --functional ]]; then
    [[ "$status" == 0 && "$(jget "$RUN/report.json" check_mode 2>/dev/null || true)" == functional && "$(jget "$RUN/report.json" functional_check 2>/dev/null || true)" == passed ]]
    return $?
  fi
}
refresh_report
printf '\nRelatório para compartilhar: %s/RELATORIO.md\nOs JSONs e logs são privados; não envie a pasta inteira.\n' "$RUN"
[[ "${1:-}" != --check-only ]] || exit "$status"
functional_test() {
  printf '\nEste teste MCP cria um documento sintético e registra um teste no histórico, concluído quando passa; clientes, hooks, memória e agendamento têm avaliações separadas.\n'
  if ( refresh_report --functional ); then
    printf 'Teste MCP encerrado. Confira as etapas no novo RELATORIO.md; o uso real no Code e no Desktop continua separado.\n'
  else
    printf '\nTeste MCP incompleto; os diagnósticos e relatórios anteriores estão preservados em %s. Confira a etapa pendente no relatório; use a opção 5 para revisar a preparação ou compartilhe RELATORIO.md com o time antes de tentar novamente.\n' "$DIAGNOSTIC_ROOT"
  fi
}
attest() {
  local label answer
  printf '\nObservações suas (não são provas automáticas). Use passou, falhou ou nao-testei.\n'
  printf '\n## Observações do participante\n\nAutodeclarações; não substituem qualificação nativa.\n' >> "$RUN/RELATORIO.md"
  for label in 'Code: identidade/skills' 'Desktop: conexão/skills' 'Onboarding e mesmo workspace' 'Memória: salvar e reler' 'Hooks e delegação' 'Conversão de documento'; do
    if [[ "${TARGET:-}" == desktop && ( "$label" == 'Code: identidade/skills' || "$label" == 'Hooks e delegação' ) ]]; then
      printf '\n- %s: not_applicable (alvo Desktop)\n' "$label" >> "$RUN/RELATORIO.md"
      continue
    fi
    if [[ ( "${PILOT_OBSERVATION_CLIENT:-}" == desktop || "${CODE_PREPARATION_PENDING:-false}" == true ) && ( "$label" == 'Code: identidade/skills' || "$label" == 'Hooks e delegação' ) ]]; then
      printf '\n- %s: nao-testei (fora do escopo Desktop desta tentativa)\n' "$label" >> "$RUN/RELATORIO.md"
      continue
    fi
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
  [[ "$prompt" == CODE-CONVERSAO.txt || "$prompt" == DESKTOP-CONVERSAO.txt ]] || fail 'Documento disponível somente na etapa de documento.'
  guard_test_prompt "$prompt"
  state="$(jget "$RUN/report.json" conversion.state)"
  if [[ "$state" == pending_runtime ]]; then printf 'Conversão pendente: o runtime atual não está pronto. O restante do ensaio pode continuar; nenhum documento será criado.\n'; return 0; fi
  [[ "$state" == ready_for_participant_test ]] || fail 'Identidade do conversor divergente ou indisponível; a jornada foi interrompida.'
  expected_manifest="$(jget "$RUN/report.json" conversion.expected_manifest_sha256)"
  expected_pack="$(jget "$RUN/report.json" conversion.expected_pack_sha256)"
  desktop_clients_available "$current" || fail 'Desktop ainda não está preparado; preserve o diagnóstico.'
  [[ "$expected_manifest" =~ ^[0-9a-f]{64}$ && "$expected_pack" =~ ^[0-9a-f]{64}$ && "$(jget "$current" record.binding.workspace)" == "$WORK" && "$(jget "$current" record.materialization.manifest_sha256)" == "$expected_manifest" ]] || fail 'O workspace ou a versão atual mudou desde o diagnóstico. Reabra o teste.'
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
  [[ "$TARGET" != desktop ]] || fail 'Este alvo Desktop não seleciona Code.'
  guard_test_prompt "$1"
  "$INSTALLED" clients-status "${CLIENT_STATUS[@]}" > "$RUN/clients-status.json" || fail 'Registro atual indisponível.'
  [[ "$(jget "$RUN/clients-status.json" state)" == current_materialization && "$(jget "$RUN/clients-status.json" record.code.state)" == registered && "$(jget "$RUN/clients-status.json" record.code.enabled)" == true ]] || fail 'Code ainda não está registrado e habilitado; conclua pelo gerenciador.'
  "$INSTALLED" client-plugin-path "${COMMON[@]}" > "$RUN/current-plugin.txt" || fail 'Plugin atual não admitido.'
  local executable expected observed_profile
  executable="$(jget "$RUN/clients-status.json" record.code.executable)"; expected="$(jget "$RUN/clients-status.json" record.code.executable_sha256)"
  [[ "$executable" == /* && -f "$executable" && "$(sha "$executable")" == "$expected" ]] || fail 'O Code selecionado mudou; revise o registro pelo gerenciador.'
  observed_profile="$(jget "$RUN/clients-status.json" record.code.configuration_directory 2>/dev/null || true)"
  [[ "$observed_profile" == "$CODE_CONFIG" ]] || fail 'O perfil Code registrado divergiu; preserve o registro e confira pelo gerenciador.'
  if [[ "$1" == CODE-CONVERSAO.txt ]]; then prepare_conversion_document "$RUN/clients-status.json" "$1"; fi
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
  local packet methods method_version copied=false
  guard_test_prompt "$1"
  "$INSTALLED" clients-status "${CLIENT_STATUS[@]}" > "$RUN/clients-status.json" || fail 'Registro atual indisponível.'
  desktop_clients_available "$RUN/clients-status.json" || fail 'Desktop ainda não está preparado; conclua pelo gerenciador.'
  packet="$(jget "$RUN/clients-status.json" record.materialization.desktop_path)"
  methods="$(jget "$RUN/clients-status.json" record.materialization.desktop_methods.archive)"
  method_version="$(jget "$RUN/clients-status.json" record.materialization.desktop_methods.version)"
  if [[ "$1" == DESKTOP-CONVERSAO.txt ]]; then prepare_conversion_document "$RUN/clients-status.json" "$1"; fi
  if /usr/bin/pbcopy < "$RUN/$1" 2>/dev/null; then
    copied=true
  else
    printf '\nCópia do prompt pendente. Abra o arquivo desta etapa indicado abaixo e copie o texto manualmente.\n'
  fi
  if desktop_install_is_current "$RUN/clients-status.json"; then
    printf '\nA instalação deste pacote/contrato já foi confirmada por você. O instalador da extensão não será reaberto; conexão e funcionamento continuam pendentes de teste.\n'
  else
    printf '\nA instalação deste pacote/contrato ainda precisa de confirmação atual. Instale/ative somente esta extensão e confirme o pacote pelo gerenciador (opção 5).\n'
    if ! /usr/bin/open "$packet" 2>/dev/null; then
      printf 'Abertura do pacote pendente. Abra manualmente o pacote Desktop indicado abaixo pelo fluxo normal do cliente.\n'
    fi
  fi
  printf 'Pacote Desktop: %s\nPrompt desta etapa: %s\nRelatório preservado: %s/RELATORIO.md\nA conexão e o funcionamento continuam pendentes de teste.\n' "$packet" "$RUN/$1" "$RUN"
  printf 'Biblioteca de skills %s: %s\nA importação no Desktop não foi observada pelo menu. Se a biblioteca ainda não estiver disponível no cliente, importe esse arquivo pelo fluxo normal.\n' "$method_version" "$methods"
  if [[ "$copied" == true ]]; then
    printf 'Prompt copiado. Abra sua conversa no Desktop, use somente o Maestro deste ensaio, cole uma vez e execute. Depois registre o resultado no menu 7.\n'
  else
    printf 'Após copiar o texto manualmente, abra sua conversa no Desktop, use somente o Maestro deste ensaio, cole uma vez e execute. Depois registre o resultado no menu 7.\n'
  fi
}
pilot_steps() {
  local client="$1" index=0 title key prompt_file action
  while true; do
    title="$(jget "$RUN/report.json" "${client}_steps.$index.title" 2>/dev/null)" || fail 'Etapas indisponíveis. Reabra o diagnóstico com a versão atual.'
    printf '\nEtapa atual: %s\nEnvie somente esta etapa. Avance após conferir o resultado no cliente.\n1 — Enviar esta etapa\n2 — Próxima etapa\n3 — Etapa anterior\n4 — Conferir após interrupção\nEnter — Voltar\nEscolha: ' "$title"
    read -r action
    case "$action" in
      1)
        key="$(jget "$RUN/report.json" "${client}_steps.$index.id")"
        if [[ "$client" == code ]]; then prompt_file=CODE-ETAPA.txt; else prompt_file=DESKTOP-ETAPA.txt; fi
        if [[ "$key" == conversion ]]; then
          if [[ "$client" == code ]]; then prompt_file=CODE-CONVERSAO.txt; else prompt_file=DESKTOP-CONVERSAO.txt; fi
        fi
        /usr/bin/plutil -extract "${client}_steps.$index.prompt" raw -o "$RUN/$prompt_file" "$RUN/report.json" || fail 'Etapa indisponível.'
        if [[ "$client" == code ]]; then open_code "$prompt_file"; else open_desktop "$prompt_file"; fi
        ;;
      2)
        if /usr/bin/plutil -extract "${client}_steps.$((index+1)).title" raw -o - "$RUN/report.json" >/dev/null 2>&1; then index=$((index+1)); else printf 'Última etapa. Registre o resultado no menu 7.\n'; fi
        ;;
      3) if (( index > 0 )); then index=$((index-1)); fi;;
      4) index=0;;
      '') return;;
      *) printf 'Escolha uma opção da lista.\n';;
    esac
  done
}
while [[ -t 0 ]]; do
  if [[ "$TARGET" == desktop ]]; then
    printf '\n2 — Testar ou continuar no Desktop\n4 — Conferir conexão Desktop\n5 — Gerenciar Desktop e métodos\n7 — Registrar resultados e abrir relatório\n8 — Atualizar da pasta compartilhada\n'
  elif [[ "${PILOT_OBSERVATION_CLIENT:-}" == desktop || "${CODE_PREPARATION_PENDING:-false}" == true ]]; then
    printf '\nEscopo atual: Desktop. Code e hooks não foram observados nesta tentativa.\n2 — Testar ou continuar no Desktop\n4 — Conferir conexão Desktop\n5 — Preparar ou retomar clientes\n7 — Registrar resultados e abrir relatório\n8 — Atualizar da pasta compartilhada\n9 — Renovar login do perfil Code do piloto\n'
  else
  printf '\n1 — Testar ou continuar no Code\n2 — Continuar no Desktop\n3 — Conferir conexão Code\n4 — Conferir conexão Desktop\n5 — Gerenciar/qualificar hooks e skills\n6 — Conferir persistência no Code\n7 — Registrar resultados e abrir relatório\n8 — Atualizar da pasta compartilhada\n9 — Renovar login do perfil Code do piloto\n'
  fi
  printf '10 — Testar MCP automaticamente (documento sintético e histórico de teste)\nEnter — Sair\nEscolha: '
  read -r choice
  case "$choice" in
    1) pilot_steps code;; 2) pilot_steps desktop;; 3) open_code CODE-CONEXAO.txt;; 4) open_desktop DESKTOP-CONEXAO.txt;;
    5) manage || printf '\nGerenciamento pendente. Preserve o relatório.\n'; refresh_report;;
    6) open_code CODE-RELEITURA.txt;; 7) attest; /usr/bin/open -t "$RUN/RELATORIO.md";;
    8) manage --management-intent update || printf '\nAtualização pendente. Preserve o relatório.\n'; refresh_report;;
    9) if ! ensure_code_login force; then printf '\nLogin Code permanece pendente. O Desktop preparado foi preservado.\n'; fi;;
    10) functional_test;;
    '') break;; *) printf 'Escolha uma opção da lista.\n';;
  esac
done
