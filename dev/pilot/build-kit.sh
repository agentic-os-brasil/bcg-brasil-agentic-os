#!/bin/bash
set -euo pipefail
umask 077
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
OUT="${1:?Uso: build-kit.sh /caminho/Developer/pasta-do-kit}"
[[ $# -le 2 && ( $# -lt 2 || "${2:-}" == --lifecycle-context ) ]] || { echo 'Use build-kit.sh destino [--lifecycle-context].' >&2; exit 1; }
[[ "$OUT" == /* && "$OUT" == */Developer/* && ! -L "$OUT" ]] || { echo 'Use um destino absoluto sob Developer.' >&2; exit 1; }
[[ "$(uname -s)" == Darwin ]] || { echo 'Este construtor usa o produtor Mac; os alvos cruzados continuam sem execução nativa inferida.' >&2; exit 1; }
[[ ! -e "$OUT" ]] || { echo 'O destino já existe; use uma pasta nova para evitar misturar artefatos.' >&2; exit 1; }
[[ -d "$HOME/Developer" && ! -L "$HOME/Developer" ]] || { echo 'Use uma pasta local Developer no seu perfil.' >&2; exit 1; }
DEV="$(cd "$HOME/Developer" && pwd -P)"
PARENT="$(cd "$(dirname "$OUT")" && pwd -P)"
case "$PARENT/" in "$DEV/"|"$DEV/"*) ;; *) echo 'O destino físico deve permanecer dentro de ~/Developer.' >&2; exit 1;; esac
OUT="$PARENT/$(basename "$OUT")"
[[ ! -e "$OUT.zip" ]] || { echo 'O ZIP já existe; preserve-o e escolha outro destino.' >&2; exit 1; }

[[ -z "$(git -C "$SOURCE" status --porcelain --untracked-files=all)" ]] || { echo 'Use uma revisão commitada e limpa; preserve o trabalho local.' >&2; exit 1; }
mkdir -p "$OUT/bin"
(cd "$SOURCE/adapters/mcp" && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-darwin-arm64" ./cmd/maestro-mcp)
(cd "$SOURCE/adapters/mcp" && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-darwin-amd64" ./cmd/maestro-mcp)
(cd "$SOURCE/adapters/mcp" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-windows-amd64.exe" ./cmd/maestro-mcp)
chmod 700 "$OUT/bin/maestro-mcp-windows-amd64.exe"
chmod 700 "$OUT/bin/maestro-mcp-darwin-arm64"
chmod 700 "$OUT/bin/maestro-mcp-darwin-amd64"
cp "$SOURCE/dev/pilot/setup-macos.command" "$OUT/Preparar Maestro.command"
mkdir "$OUT/templates"
cp "$SOURCE/dev/pilot/open-code-macos.command" "$OUT/templates/open-code-macos.command"
cp "$SOURCE/dev/pilot/open-desktop-macos.command" "$OUT/templates/open-desktop-macos.command"
chmod 700 "$OUT/templates/"*.command
python3 -c 'from pathlib import Path; import sys; Path(sys.argv[2]).write_text(Path(sys.argv[1]).read_text(), encoding="utf-8-sig")' "$SOURCE/dev/pilot/setup-windows.ps1" "$OUT/Preparar Maestro.ps1"
python3 -c 'from pathlib import Path; import sys; Path(sys.argv[2]).write_bytes(Path(sys.argv[1]).read_text().replace("\n", "\r\n").encode("ascii"))' "$SOURCE/dev/pilot/prepare-windows.cmd" "$OUT/Preparar Maestro.cmd"
printf 'disabled\n' > "$OUT/LIFECYCLE-CONTEXT.txt"
if [[ "${2:-}" == --lifecycle-context ]]; then
  help="$("$OUT/bin/maestro-mcp-darwin-$( [[ "$(uname -m)" == arm64 ]] && printf arm64 || printf amd64 )" plugin --help 2>&1 || true)"
  [[ "$help" == *--lifecycle-context* ]] || { echo 'O servidor atual não oferece o contexto passivo; preserve este artefato parcial.' >&2; exit 1; }
  printf 'enabled\n' > "$OUT/LIFECYCLE-CONTEXT.txt"
fi
cp "$SOURCE/dev/pilot/LEIA-ME.md" "$OUT/LEIA-ME.md"
cp "$SOURCE/dev/pilot/ROTEIRO.md" "$OUT/ROTEIRO.md"
cp "$SOURCE/LICENSE.md" "$OUT/LICENSE.md"
chmod 700 "$OUT/Preparar Maestro.command"
SOURCE_COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"
SOURCE_TREE="$(git -C "$SOURCE" rev-parse HEAD^{tree})"
{
  printf 'mode=ordinary-unversioned-developer-rehearsal\nbinary_source_base_commit=%s\n' "$SOURCE_COMMIT"
  printf 'source_state=committed_clean_snapshot_sha256_identifies_exact_bytes\n'
  printf 'source_tree=%s\ntrust=unsigned_engineering_candidate\npublisher_authentication=unavailable_checksums_are_integrity_only\n' "$SOURCE_TREE"
  printf 'server_version=unavailable\npacket_label=0.0.0\nplatforms=darwin-arm64,darwin-amd64,windows-amd64\n'
  printf 'darwin_arm64_evidence=built_actual_native_execution_requires_retained_receipt\ndarwin_amd64_evidence=cross_build_only_native_execution_unobserved\n'
  printf 'native_client_loading=not-yet-observed\nwindows_binary_evidence=cross_build_only_this_kit_requires_native_rehearsal\n'
  printf 'signed_install_update=unavailable\ndocling_unseeded=unavailable\nnative_agents_hooks=unavailable_without_attended_qualification\nwindows_darwin_jobs=unavailable\n'
} > "$OUT/ARTIFACT.txt"
(cd "$OUT"; LC_ALL=C find -s . -type f ! -name SHA256SUMS ! -name KIT-ID.txt -exec shasum -a 256 '{}' \;) > "$OUT/SHA256SUMS"
CONTENT_ID="$(shasum -a 256 "$OUT/SHA256SUMS")"
printf '%s\n' "${CONTENT_ID%% *}" > "$OUT/KIT-ID.txt"
(cd "$OUT" && shasum -a 256 -c SHA256SUMS)
ZIP="${OUT%/}.zip"
[[ ! -e "$ZIP" ]] || { echo 'O arquivo ZIP de destino já existe; preserve-o e use um destino novo.' >&2; exit 1; }
COPYFILE_DISABLE=1 /usr/bin/ditto -c -k --norsrc --keepParent "$OUT" "$ZIP"
(cd "$(dirname "$ZIP")" && shasum -a 256 "$(basename "$ZIP")") > "$ZIP.sha256"
printf '\nKit: %s\nChecksum do ZIP: %s.sha256\n' "$ZIP" "$ZIP"
