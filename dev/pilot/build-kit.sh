#!/bin/bash
set -euo pipefail
umask 077
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
OUT="${1:?Uso: build-kit.sh /caminho/Developer/pasta-do-kit}"
RUNTIME_RELEASE=""; RUNTIME_REGISTRY=""; ACQUISITION=false
if [[ "${2:-}" == --engineering-runtime || "${2:-}" == --engineering-acquisition ]]; then
  [[ "${2:-}" != --engineering-acquisition ]] || ACQUISITION=true
  [[ $# -eq 4 ]] || { echo 'Use build-kit.sh destino --engineering-runtime release-assinado registro-publico.' >&2; exit 1; }
  RUNTIME_RELEASE="$3"; RUNTIME_REGISTRY="$4"
  [[ "$RUNTIME_RELEASE" == /* && "$RUNTIME_REGISTRY" == /* && ! -L "$RUNTIME_RELEASE" && ! -L "$RUNTIME_REGISTRY" ]] || { echo 'Selecione entradas públicas absolutas, sem links.' >&2; exit 1; }
else
  [[ $# -le 2 && ( $# -lt 2 || "${2:-}" == --lifecycle-context ) ]] || { echo 'Use build-kit.sh destino [--lifecycle-context].' >&2; exit 1; }
fi
[[ "$OUT" == /* && "$OUT" == */Developer/* && ! -L "$OUT" ]] || { echo 'Use um destino absoluto sob Developer.' >&2; exit 1; }
[[ "$(uname -s)" == Darwin ]] || { echo 'Este construtor usa o produtor Mac; os alvos cruzados continuam sem execução nativa inferida.' >&2; exit 1; }
[[ ! -e "$OUT" ]] || { echo 'O destino já existe; use uma pasta nova para evitar misturar artefatos.' >&2; exit 1; }
[[ -d "$HOME/Developer" && ! -L "$HOME/Developer" ]] || { echo 'Use uma pasta local Developer no seu perfil.' >&2; exit 1; }
DEV="$(cd "$HOME/Developer" && pwd -P)"
PARENT="$(cd "$(dirname "$OUT")" && pwd -P)"
case "$PARENT/" in "$DEV/"|"$DEV/"*) ;; *) echo 'O destino físico deve permanecer dentro de ~/Developer.' >&2; exit 1;; esac
OUT="$PARENT/$(basename "$OUT")"
ZIP="$OUT.zip"
[[ ! -e "$ZIP" && ! -L "$ZIP" ]] || { echo 'O ZIP já existe; preserve-o e escolha outro destino.' >&2; exit 1; }
[[ ! -e "$ZIP.sha256" && ! -L "$ZIP.sha256" ]] || { echo 'O checksum já existe; preserve-o e escolha outro destino.' >&2; exit 1; }

[[ -z "$(git -C "$SOURCE" status --porcelain --untracked-files=all)" ]] || { echo 'Use uma revisão commitada e limpa; preserve o trabalho local.' >&2; exit 1; }
if ! mkdir "$OUT"; then
  echo 'O destino foi ocupado antes da reivindicação exclusiva; preserve o vencedor e escolha outra pasta.' >&2
  exit 1
fi
mkdir "$OUT/bin"
if [[ -n "$RUNTIME_RELEASE" ]]; then
  RUNTIME_REGISTRY_SHA="$(shasum -a 256 "$RUNTIME_REGISTRY")"; RUNTIME_REGISTRY_SHA="${RUNTIME_REGISTRY_SHA%% *}"
  (cd "$SOURCE" && go run ./dev/release verify-signed --directory "$RUNTIME_RELEASE" --authority-registry "$RUNTIME_REGISTRY")
  python3 - "$RUNTIME_RELEASE" "$RUNTIME_REGISTRY" "$OUT" "$RUNTIME_REGISTRY_SHA" "$ACQUISITION" <<'RUNTIME_KIT_PY'
import datetime
import hashlib
import json
from pathlib import Path
import re
import shutil
import sys

release, registry, output = map(Path, sys.argv[1:4])
assert hashlib.sha256(registry.read_bytes()).hexdigest() == sys.argv[4], "O registro público mudou durante a verificação."
manifest = json.loads((release / "release-manifest.json").read_text())
authority = json.loads(registry.read_text())
keys = authority["authorities"]
assert len(keys) == 1, "O piloto completo requer uma autoridade experimental única."
key = keys[0]
now = datetime.datetime.now(datetime.timezone.utc)
parse = lambda value: datetime.datetime.fromisoformat(value.replace("Z", "+00:00"))
assert re.fullmatch(r"maestro-engineering-pilot-[0-9]{8}", key["issuer"])
assert re.fullmatch(r"pilot-[0-9]{8}", key["key_id"])
assert key["status"] == "active" and parse(key["valid_from"]) <= now < parse(key["valid_until"])
assert parse(key["valid_until"]) - parse(key["valid_from"]) <= datetime.timedelta(days=14)
assert manifest["schema_version"] == 2 and manifest["channel"] == "canary"
assert manifest["issuer"] == {"id": key["issuer"], "key_id": key["key_id"]}
acquisition = sys.argv[5] == "true"
target = ("windows", "amd64") if acquisition else ("darwin", "arm64")
assert all((a["os"], a["arch"]) == target for a in manifest["artifacts"] if a["kind"] == "runtime_pack"), "O kit contém somente o alvo de runtime explicitamente selecionado."
servers = [a for a in manifest["artifacts"] if a["kind"] == "mcp_server" and (a["os"], a["arch"]) == target]
packs = [a for a in manifest["artifacts"] if a["kind"] == "runtime_pack" and a.get("adapter") == "docling" and (a["os"], a["arch"]) == target]
assert len(servers) == len(packs) == 1, "O piloto requer exatamente um servidor e um runtime do alvo declarado."
server, pack = servers[0], packs[0]
shutil.copyfile(release / server["name"], output / ("bin/maestro-mcp-windows-amd64.exe" if acquisition else "bin/maestro-mcp-darwin-arm64"))
(output / ("bin/maestro-mcp-windows-amd64.exe" if acquisition else "bin/maestro-mcp-darwin-arm64")).chmod(0o700)
if acquisition:
    (output / "runtime-release").mkdir()
    for child in release.iterdir():
        if child.name != pack["name"]:
            assert child.is_file() and not child.is_symlink()
            shutil.copyfile(child, output / "runtime-release" / child.name)
    recipe = release.parent / "runtime-recipe"
    assert {p.name for p in recipe.iterdir()} == {"runtime-acquisition.json", "runtime-acquisition.json.sig"}
    assert all(p.is_file() and not p.is_symlink() for p in recipe.iterdir())
    shutil.copytree(recipe, output / "runtime-recipe", symlinks=False)
else:
    shutil.copytree(release, output / "runtime-release", symlinks=False)
shutil.copyfile(registry, output / "runtime-registry.json")
assert hashlib.sha256((output / "runtime-registry.json").read_bytes()).hexdigest() == sys.argv[4]
assert hashlib.sha256((output / ("bin/maestro-mcp-windows-amd64.exe" if acquisition else "bin/maestro-mcp-darwin-arm64")).read_bytes()).hexdigest() == server["sha256"]
info = {"schema_version": 1, "profile": "engineering-runtime-pilot-v1", "version": manifest["server"]["version"], "target": "windows-amd64" if acquisition else "darwin-arm64", "acquisition": acquisition, "registry_sha256": hashlib.sha256(registry.read_bytes()).hexdigest(), "manifest_sha256": hashlib.sha256((release / "release-manifest.json").read_bytes()).hexdigest(), "pack_sha256": pack["sha256"], "issuer": key["issuer"], "issued_at": key["valid_from"], "expires_at": key["valid_until"]}
(output / "RUNTIME.json").write_text(json.dumps(info, indent=2) + "\n")
RUNTIME_KIT_PY
  if [[ "$ACQUISITION" == true ]]; then
    (cd "$SOURCE" && go run ./dev/pilot/verify-carrier "$OUT" "$OUT/runtime-registry.json" "$PARENT/.unused-acquisition-review-cache")
  else
    (cd "$SOURCE" && go run ./dev/release verify-signed --directory "$OUT/runtime-release" --authority-registry "$OUT/runtime-registry.json")
  fi
  mkdir "$OUT/test-documents"
  cp "$SOURCE/internal/ingest/docling/testdata/sample.docx" "$OUT/test-documents/maestro-pilot.docx"
else
  (cd "$SOURCE/adapters/mcp" && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-darwin-arm64" ./cmd/maestro-mcp)
  (cd "$SOURCE/adapters/mcp" && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-darwin-amd64" ./cmd/maestro-mcp)
  (cd "$SOURCE/adapters/mcp" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/bin/maestro-mcp-windows-amd64.exe" ./cmd/maestro-mcp)
  chmod 700 "$OUT/bin/maestro-mcp-windows-amd64.exe" "$OUT/bin/maestro-mcp-darwin-arm64" "$OUT/bin/maestro-mcp-darwin-amd64"
fi
cp "$SOURCE/dev/pilot/setup-macos.command" "$OUT/Preparar Maestro.command"
mkdir "$OUT/templates"
cp "$SOURCE/dev/pilot/open-code-macos.command" "$OUT/templates/open-code-macos.command"
cp "$SOURCE/dev/pilot/open-desktop-macos.command" "$OUT/templates/open-desktop-macos.command"
cp "$SOURCE/dev/pilot/test-macos.command" "$OUT/templates/test-macos.command"
python3 -c 'from pathlib import Path; import sys; Path(sys.argv[2]).write_text(Path(sys.argv[1]).read_text(), encoding="utf-8-sig")' "$SOURCE/dev/pilot/test-windows.ps1" "$OUT/templates/test-windows.ps1"
python3 -c 'from pathlib import Path; import sys; Path(sys.argv[2]).write_bytes(Path(sys.argv[1]).read_text().replace("\n", "\r\n").encode("ascii"))' "$SOURCE/dev/pilot/test-windows.cmd" "$OUT/templates/test-windows.cmd"
chmod 700 "$OUT/templates/"*.command
python3 -c 'from pathlib import Path; import sys; Path(sys.argv[2]).write_text(Path(sys.argv[1]).read_text(), encoding="utf-8-sig")' "$SOURCE/dev/pilot/setup-windows.ps1" "$OUT/Preparar Maestro.ps1"
python3 -c 'from pathlib import Path; import sys; Path(sys.argv[2]).write_bytes(Path(sys.argv[1]).read_text().replace("\n", "\r\n").encode("ascii"))' "$SOURCE/dev/pilot/prepare-windows.cmd" "$OUT/Preparar Maestro.cmd"
printf 'disabled\n' > "$OUT/LIFECYCLE-CONTEXT.txt"
if [[ "${2:-}" == --lifecycle-context || -n "$RUNTIME_RELEASE" ]]; then
  if [[ "$ACQUISITION" != true ]]; then
  help="$("$OUT/bin/maestro-mcp-darwin-$( [[ "$(uname -m)" == arm64 ]] && printf arm64 || printf amd64 )" plugin --help 2>&1 || true)"
  [[ "$help" == *-lifecycle-context* ]] || { echo 'O servidor atual não oferece o contexto passivo; preserve este artefato parcial.' >&2; exit 1; }
  fi
  printf 'enabled\n' > "$OUT/LIFECYCLE-CONTEXT.txt"
fi
cp "$SOURCE/dev/pilot/LEIA-ME.md" "$OUT/LEIA-ME.md"
cp "$SOURCE/dev/pilot/ROTEIRO.md" "$OUT/ROTEIRO.md"
cp "$SOURCE/LICENSE.md" "$OUT/LICENSE.md"
chmod 700 "$OUT/Preparar Maestro.command"
SOURCE_COMMIT="$(git -C "$SOURCE" rev-parse HEAD)"
SOURCE_TREE="$(git -C "$SOURCE" rev-parse HEAD^{tree})"
{
  if [[ -n "$RUNTIME_RELEASE" ]]; then
    printf 'mode=explicit-engineering-runtime-rehearsal\nbuilder_source_commit=%s\nbinary_source=provided_verified_signed_release_exact_digest_in_RUNTIME.json\n' "$SOURCE_COMMIT"
  else
    printf 'mode=ordinary-unversioned-developer-rehearsal\nbinary_source_base_commit=%s\n' "$SOURCE_COMMIT"
  fi
  printf 'source_state=committed_clean_snapshot_sha256_identifies_exact_bytes\n'
  printf 'source_tree=%s\ntrust=unsigned_engineering_candidate\npublisher_authentication=unavailable_checksums_are_integrity_only\n' "$SOURCE_TREE"
  if [[ -n "$RUNTIME_RELEASE" ]]; then
    RUNTIME_VERSION="$(/usr/bin/plutil -extract version raw -o - "$OUT/RUNTIME.json")"
    printf 'server_version=%s\npacket_label=%s\nplatforms=%s\n' "$RUNTIME_VERSION" "$RUNTIME_VERSION" "$(/usr/bin/plutil -extract target raw -o - "$OUT/RUNTIME.json")"
  else
    printf 'server_version=unavailable\npacket_label=0.0.0\nplatforms=darwin-arm64,darwin-amd64,windows-amd64\n'
  fi
  if [[ "$ACQUISITION" == true ]]; then
    printf 'darwin_arm64_evidence=not_included\ndarwin_amd64_evidence=not_included\nwindows_binary_evidence=provided_verified_artifact_native_execution_unattested\n'
  elif [[ -n "$RUNTIME_RELEASE" ]]; then
    printf 'darwin_arm64_evidence=provided_verified_artifact_native_execution_requires_retained_receipt\ndarwin_amd64_evidence=not_included\nwindows_binary_evidence=not_included\n'
  else
    printf 'darwin_arm64_evidence=built_actual_native_execution_requires_retained_receipt\ndarwin_amd64_evidence=cross_build_only_native_execution_unobserved\nwindows_binary_evidence=cross_build_only_this_kit_requires_native_rehearsal\n'
  fi
  printf 'native_client_loading=not-yet-observed\n'
  printf 'signed_install_update=unavailable\nnative_agents_hooks=unavailable_without_attended_qualification\nwindows_darwin_jobs=unavailable\n'
  if [[ "$ACQUISITION" == true ]]; then printf 'docling=runtime_absent_requires_explicit_upstream_acquisition_then_placement_and_native_qualification\n'; elif [[ -n "$RUNTIME_RELEASE" ]]; then printf 'docling=verified_engineering_release_requires_explicit_local_placement\n'; else printf 'docling_unseeded=unavailable\n'; fi
} > "$OUT/ARTIFACT.txt"
if [[ -n "$RUNTIME_RELEASE" ]]; then
  printf 'runtime_profile=engineering-runtime-pilot-v1\nruntime_targets=RUNTIME.json\nruntime_identity=RUNTIME.json\nruntime_trust=short_lived_experimental_issuer_not_organizational_publisher\nwindows_runtime_distribution=absent_upstream_acquisition_requires_separate_local_consent_native_acceptance_pending\nintel_runtime=unavailable\nproduction_update=unavailable\n' >> "$OUT/ARTIFACT.txt"
fi
(cd "$OUT"; LC_ALL=C find -s . -type f ! -name SHA256SUMS ! -name KIT-ID.txt -exec shasum -a 256 '{}' +) > "$OUT/SHA256SUMS"
CONTENT_ID="$(shasum -a 256 "$OUT/SHA256SUMS")"
printf '%s\n' "${CONTENT_ID%% *}" > "$OUT/KIT-ID.txt"
(cd "$OUT" && shasum -a 256 -c SHA256SUMS)
STAGE="$(mktemp -d "$PARENT/.maestro-pilot-archive.XXXXXXXX")"
STAGED_ZIP="$STAGE/$(basename "$ZIP")"
printf 'Staging de publicação retido para inspeção: %s\n' "$STAGE"
COPYFILE_DISABLE=1 /usr/bin/ditto -c -k --norsrc --keepParent "$OUT" "$STAGED_ZIP"
python3 - "$STAGED_ZIP" "$ZIP" "$ZIP.sha256" <<'PUBLISH_ARCHIVE_PY'
import hashlib
import os
from pathlib import Path
import sys

staged, archive, sidecar = map(Path, sys.argv[1:])
def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            value.update(block)
    return value.hexdigest()

try:
    expected = digest(staged)
    checksum = expected + "  " + archive.name + "\n"
    staged_sidecar = staged.parent / "checksum.sha256"
    with staged_sidecar.open("x", encoding="utf-8", newline="\n") as stream:
        stream.write(checksum)
    # os.link has no directory-target fallback and never replaces an entry.
    os.link(staged, archive, follow_symlinks=False)
    os.link(staged_sidecar, sidecar, follow_symlinks=False)
    if digest(archive) != expected or sidecar.read_text(encoding="utf-8") != checksum:
        raise OSError("published bytes changed before readback")
except OSError as error:
    print("Publicação pendente: destinos existentes e bytes parciais foram preservados; escolha um destino novo. " + str(error), file=sys.stderr)
    print("Staging retido: " + str(staged.parent), file=sys.stderr)
    sys.exit(1)
PUBLISH_ARCHIVE_PY
printf '\nKit: %s\nChecksum do ZIP: %s.sha256\n' "$ZIP" "$ZIP"
