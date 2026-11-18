#!/bin/bash
set -euo pipefail
umask 077
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
[[ $# -eq 9 && "$2" == --experimental-installed-seed ]] || {
  echo 'Uso: build-kit.sh destino --experimental-installed-seed instalador role.json role.json.sig registro.json sha256-registro release-inicial fonte-assinada' >&2
  exit 1
}
cd "$SOURCE"
exec go run ./dev/pilot/installed-seed-factory --out "$1" --installer "$3" --role "$4" --role-signature "$5" --registry "$6" --registry-sha256 "$7" --initial-release "$8" --source "$9" --templates "$SOURCE/dev/pilot/templates/installed-seed"
