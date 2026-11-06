#!/bin/bash
set -euo pipefail
umask 077
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
OUT="${1:?Uso: build-kit.sh /caminho/Developer/pasta-do-kit}"
[[ "$OUT" == /* && "$OUT" == */Developer/* && ! -L "$OUT" ]] || { echo 'Use um destino absoluto sob Developer.' >&2; exit 1; }
[[ "$(uname -s)/$(uname -m)" == Darwin/arm64 ]] || { echo 'O produtor atual qualifica somente Darwin arm64.' >&2; exit 1; }
[[ ! -e "$OUT" ]] || { echo 'O destino já existe; use uma pasta nova para evitar misturar artefatos.' >&2; exit 1; }
mkdir -p "$OUT/bin"
(cd "$SOURCE/adapters/mcp" && go build -trimpath -o "$OUT/bin/maestro-mcp-darwin-arm64" ./cmd/maestro-mcp)
(cd "$SOURCE/adapters/mcp" && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-darwin-amd64" ./cmd/maestro-mcp)
chmod 700 "$OUT/bin/maestro-mcp-darwin-arm64"
chmod 700 "$OUT/bin/maestro-mcp-darwin-amd64"
cp "$SOURCE/dev/pilot/setup-macos.command" "$OUT/Preparar Maestro.command"
cp "$SOURCE/dev/pilot/LEIA-ME.md" "$OUT/LEIA-ME.md"
cp "$SOURCE/dev/pilot/ROTEIRO.md" "$OUT/ROTEIRO.md"
cp "$SOURCE/LICENSE.md" "$OUT/LICENSE.md"
chmod 700 "$OUT/Preparar Maestro.command"
SOURCE_COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"
{
  printf 'mode=ordinary-unversioned-developer-rehearsal\nbinary_source_base_commit=%s\n' "$SOURCE_COMMIT"
  printf 'source_state=development_snapshot_native_sha256_identifies_exact_bytes\n'
  printf 'server_version=unavailable\npacket_label=0.0.0\nplatforms=darwin-arm64,darwin-amd64\n'
  printf 'darwin_arm64_evidence=ordinary_cli_and_official_sdk_stdio\ndarwin_amd64_evidence=cross_build_and_native_format_only\n'
  printf 'native_client_loading=not-yet-observed\nwindows_fresh_admission=unavailable\n'
  printf 'signed_install_update=unavailable\ndocling_unseeded=unavailable\nnative_agents_hooks=unavailable\n'
  printf 'code_plugin_profile=mcp_lifecycle\nlifecycle_context_hooks=configured\nlifecycle_enforcement=unavailable\n'
} > "$OUT/ARTIFACT.txt"
CONTENT_ID="$(cd "$OUT"; LC_ALL=C find -s . -type f ! -name KIT-ID.txt ! -name SHA256SUMS -exec shasum -a 256 '{}' \; | shasum -a 256 | cut -d ' ' -f 1)"
printf 'maestro-developer-rehearsal-20261105-%s\n' "$CONTENT_ID" > "$OUT/KIT-ID.txt"
(cd "$OUT"; find -s . -type f ! -name SHA256SUMS -exec shasum -a 256 '{}' \;) > "$OUT/SHA256SUMS"
(cd "$OUT" && shasum -a 256 -c SHA256SUMS)
ZIP="${OUT%/}.zip"
[[ ! -e "$ZIP" ]] || { echo 'O arquivo ZIP de destino já existe; preserve-o e use um destino novo.' >&2; exit 1; }
COPYFILE_DISABLE=1 /usr/bin/ditto -c -k --norsrc --keepParent "$OUT" "$ZIP"
(cd "$(dirname "$ZIP")" && shasum -a 256 "$(basename "$ZIP")") > "$ZIP.sha256"
printf '\nKit: %s\nChecksum do ZIP: %s.sha256\n' "$ZIP" "$ZIP"
