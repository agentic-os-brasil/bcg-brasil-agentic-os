#!/bin/bash
set -euo pipefail
umask 077
[[ $# -eq 0 || ( $# -eq 1 && -n "$1" ) ]] || { echo 'Use zero argumentos ou um único prompt.' >&2; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
identity="$(cat "$ROOT/pilot-binary.sha256")"
[[ "$identity" =~ ^([0-9a-f]{64})\ \ (product/(server/)?maestro-mcp)$ ]] || { echo 'Identidade do executável inválida; preserve o ensaio.' >&2; exit 1; }
expected="${BASH_REMATCH[1]}"
BIN="$ROOT/${BASH_REMATCH[2]}"
actual_binary="$(/usr/bin/shasum -a 256 "$BIN")"
[[ "${actual_binary%% *}" == "$expected" ]] || { echo 'O executável mudou; preserve o ensaio.' >&2; exit 1; }
PACKET="$ROOT/exports/code"
if [[ -e "$ROOT/exports/code-native-qualified" || -L "$ROOT/exports/code-native-qualified" ]]; then
  inspection="$("$BIN" inspect --packet --product-root "$ROOT/exports/code-native-qualified" --user-home "$ROOT/owner" --workspace "$ROOT/workspace")"
  native_mode="$(/usr/bin/plutil -extract packet_mode raw -o - - <<< "$inspection")"
  actual="$(/usr/bin/plutil -extract packet_server_sha256 raw -o - - <<< "$inspection")"
  [[ "$actual" == "$expected" ]] || { echo 'O export nativo usa outro executável; preserve o ensaio.' >&2; exit 1; }
  case "$native_mode" in native) PACKET="$ROOT/exports/code-native-qualified";; mcp_only) ;; *) echo 'Export nativo não qualificado; preserve o ensaio.' >&2; exit 1;; esac
fi
inspection="$("$BIN" inspect --packet --product-root "$PACKET" --user-home "$ROOT/owner" --workspace "$ROOT/workspace")"
mode="$(/usr/bin/plutil -extract packet_mode raw -o - - <<< "$inspection")"
actual="$(/usr/bin/plutil -extract packet_server_sha256 raw -o - - <<< "$inspection")"
[[ "$actual" == "$expected" ]] || { echo 'O pacote selecionado usa outro executável; preserve o ensaio.' >&2; exit 1; }
check_profile() {
  local directory
  for directory in "$ROOT/client" "$ROOT/client/code"; do
    [[ ! -L "$directory" && -d "$directory" ]] || { echo 'Perfil Code ausente ou com link; preserve a preparação e o destino existente.' >&2; exit 1; }
  done
}
CODE="$(command -v claude || true)"
[[ -n "$CODE" ]] || { echo 'Disponibilize o comando claude pelo fluxo normal do Code.' >&2; exit 1; }
export CLAUDE_CONFIG_DIR="$ROOT/client/code"
cd "$ROOT/workspace"
if [[ $# -eq 1 ]]; then set -- -- "$1"; fi
check_profile
if [[ "$mode" == native ]]; then
  selection="$(mktemp "$ROOT/client/code/native-session.XXXXXXXX")"
  printf '%s\n' '{"pluginConfigs":{"maestro-product@inline":{"options":{}}}}' > "$selection"
  /usr/bin/plutil -insert 'pluginConfigs.maestro-product@inline.options.user_home' -string "$ROOT/owner" "$selection"
  /usr/bin/plutil -insert 'pluginConfigs.maestro-product@inline.options.workspace' -string "$ROOT/workspace" "$selection"
  check_profile
  exec "$CODE" --plugin-dir "$PACKET" --settings "$selection" "$@"
fi
exec "$CODE" --plugin-dir "$PACKET" "$@"
