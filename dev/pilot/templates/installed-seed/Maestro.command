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
CFG="$BASE/participant.json"
if [[ ! -e "$CFG" ]]; then
  [[ "${1:-}" != --check-only ]] || fail 'Ainda não preparado. Abra Maestro.command uma vez para escolher seu workspace e revisar a instalação.'
  printf '\nMaestro MCP — piloto experimental\nO programa e os relatórios ficam em Developer. Seu brain existente será preservado.\n'
  printf 'Diretório do owner existente em Developer (se nunca usou Maestro, selecione a própria pasta Developer): '; read -r OWNER
  [[ "$OWNER" == "$DEV" || "$OWNER" == "$DEV/"* ]] || fail 'Selecione o owner existente em Developer; não crie outro para contornar uma instalação.'
  [[ -d "$OWNER" && ! -L "$OWNER" ]] || fail 'O owner deve ser um diretório local existente.'
  OWNER="$(cd "$OWNER" && pwd -P)"
  [[ "$OWNER" == "$DEV" || "$OWNER" == "$DEV/"* ]] || fail 'O owner físico fica fora de Developer. Preserve o vínculo existente.'
  printf 'Workspace em Developer [%s/Maestro-Workspace]: ' "$DEV"; read -r WORK; WORK="${WORK:-$DEV/Maestro-Workspace}"
  [[ "$WORK" == /* && ! -L "$WORK" && -d "$(dirname "$WORK")" ]] || fail 'Escolha uma pasta local com o diretório pai existente.'
  WORK="$(cd "$(dirname "$WORK")" && pwd -P)/$(basename "$WORK")"; [[ "$WORK" == "$DEV/"* ]] || fail 'O workspace deve ficar em Developer.'
  default_code="$(command -v claude || true)"
  printf 'Executável absoluto do Claude Code [%s]: ' "$default_code"; read -r CODE; CODE="${CODE:-$default_code}"
  [[ "$CODE" == /* && -f "$CODE" && -x "$CODE" ]] || fail 'Instale o Claude Code pelo fluxo normal e escolha o executável absoluto.'
  BRAIN="${MAESTRO_HOME:-}"
  if [[ -z "$BRAIN" && -e "$OWNER/.maestro-home.json" ]]; then BRAIN="$(jget "$OWNER/.maestro-home.json" home)"; fi
  if [[ -z "$BRAIN" && -e "$OWNER/.maestro" ]]; then BRAIN="$OWNER/.maestro"; fi
  if [[ -z "$BRAIN" ]]; then
    printf 'Novo home do Maestro em Developer [%s/Maestro]: ' "$DEV"; read -r BRAIN; BRAIN="${BRAIN:-$DEV/Maestro}"
  fi
  [[ "$BRAIN" == "$DEV/"* ]] || fail 'O brain selecionado fica fora de Developer. Preserve-o e peça orientação ao time; este piloto não migra dados.'
  CARRIER="$(/usr/bin/uuidgen | tr -d '-' | tr '[:upper:]' '[:lower:]')"
  json "$CFG" admission_sha256 "$ROLE" user_home "$OWNER" workspace "$WORK" code_executable "$CODE" carrier_id "$CARRIER" brain_home "$BRAIN" source_root "$ROOT/source"
fi
[[ -f "$CFG" && ! -L "$CFG" && "$(stat -f '%l' "$CFG")" == 1 && "$(jget "$CFG" admission_sha256)" == "$ROLE" ]] || fail 'Configuração local divergente.'
OWNER="$(jget "$CFG" user_home)"; WORK="$(jget "$CFG" workspace)"; CARRIER="$(jget "$CFG" carrier_id)"
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
  "$INSTALLED" manage "${COMMON[@]}" --client-code-root "$BASE/clients/code" --client-desktop-path "$BASE/clients/maestro-product.mcpb" --client-code-executable "$CODE" --client-scope user "$@"
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
  local executable expected
  executable="$(jget "$RUN/clients-status.json" record.code.executable)"; expected="$(jget "$RUN/clients-status.json" record.code.executable_sha256)"
  [[ "$executable" == /* && -f "$executable" && "$(sha "$executable")" == "$expected" ]] || fail 'O Code selecionado mudou; revise o registro pelo gerenciador.'
  if [[ "$1" == CODE.txt ]]; then prepare_conversion_document "$RUN/clients-status.json" "$1"; fi
  (cd "$WORK" && "$executable" "$(cat "$RUN/$1")")
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
  printf '\n1 — Ensaio completo no Code\n2 — Retomar no Desktop (um prompt)\n3 — Conferir conexão Code\n4 — Conferir conexão Desktop\n5 — Gerenciar/qualificar hooks e skills\n6 — Conferir persistência no Code\n7 — Registrar resultados e abrir relatório\n8 — Atualizar da pasta compartilhada\nEnter — Sair\nEscolha: '
  read -r choice
  case "$choice" in
    1) open_code CODE.txt;; 2) open_desktop DESKTOP.txt;; 3) open_code CODE-CONEXAO.txt;; 4) open_desktop DESKTOP-CONEXAO.txt;;
    5) manage; printf 'Após concluir, reabra Maestro.command para gerar um diagnóstico atual.\n'; break;;
    6) open_code CODE-RELEITURA.txt;; 7) attest; /usr/bin/open -t "$RUN/RELATORIO.md";;
    8) manage --management-intent update; printf 'Reabra Maestro.command para verificar a nova geração.\n'; break;;
    '') break;; *) printf 'Escolha uma opção da lista.\n';;
  esac
done
