#!/usr/bin/env bash
# generate-portable-zip: orquestra candidate → bootstrappers → release-export
# para produzir o(s) ZIP(s) portable canary em releases/<versão>/.
#
# Não é o pipeline de produção. Não publica no GitHub. Para publicação, chamar
# dev/skills/release-export/scripts/export-release.sh direto.

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: generate.sh \
  --version MAJOR.MINOR.PATCH \
  --authority-registry ABS_FILE \
  --authority-registry-sha256 LOWERCASE_SHA256 \
  [--targets macos,windows] \
  [--output-directory ABS_DIR] \
  [--work-directory ABS_DIR]

Se --targets é omitido, o padrão é "macos,windows".
Se --output-directory é omitido, releases/<versão>/ é usado.
Se --work-directory é omitido, um tmpdir é criado e removido ao final.
EOF
}

die() {
  echo "generate-portable-zip: $*" >&2
  exit 1
}

version=""
authority_registry=""
authority_registry_sha256=""
targets="macos,windows"
output_directory=""
work_directory=""

while (($# > 0)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --authority-registry) authority_registry="${2:-}"; shift 2 ;;
    --authority-registry-sha256) authority_registry_sha256="${2:-}"; shift 2 ;;
    --targets) targets="${2:-}"; shift 2 ;;
    --output-directory) output_directory="${2:-}"; shift 2 ;;
    --work-directory) work_directory="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; die "unknown flag: $1" ;;
  esac
done

[[ -n "$version" ]] || { usage; die "--version required"; }
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be MAJOR.MINOR.PATCH"
[[ -n "$authority_registry" ]] || { usage; die "--authority-registry required"; }
[[ -f "$authority_registry" ]] || die "authority-registry not found: $authority_registry"
[[ -n "$authority_registry_sha256" ]] || { usage; die "--authority-registry-sha256 required"; }
[[ "$authority_registry_sha256" =~ ^[a-f0-9]{64}$ ]] || die "sha256 must be 64 lowercase hex chars"

repo_root="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$repo_root"

want_macos=0
want_windows=0
IFS=',' read -ra target_list <<<"$targets"
for t in "${target_list[@]}"; do
  case "$t" in
    macos) want_macos=1 ;;
    windows) want_windows=1 ;;
    "") ;;
    *) die "unknown target: $t (allowed: macos, windows)" ;;
  esac
done
(( want_macos + want_windows > 0 )) || die "at least one target required"

if [[ -z "$output_directory" ]]; then
  output_directory="$repo_root/releases/$version"
fi

if [[ -e "$output_directory" && -n "$(ls -A "$output_directory" 2>/dev/null | grep -v .gitkeep || true)" ]]; then
  die "output directory not empty: $output_directory (factory refuses overwrite)"
fi

cleanup_workdir=0
if [[ -z "$work_directory" ]]; then
  work_directory="$(mktemp -d -t generate-portable-zip.XXXXXX)"
  cleanup_workdir=1
fi
trap '[[ $cleanup_workdir -eq 1 ]] && rm -rf "$work_directory"' EXIT

echo "==> [1/4] harness validate --full"
go run ./dev/harness validate --full >/dev/null

echo "==> [2/4] release candidate (canary) → $work_directory/signed-release"
mkdir -p "$work_directory/signed-release"
go run ./dev/release candidate \
  --version "$version" \
  --channel canary \
  --output "$work_directory/signed-release"

echo "==> [3/4] bootstrappers"
sha_of() { shasum -a 256 "$1" | awk '{print $1}'; }

export_args=(
  --version "$version"
  --release-directory "$work_directory/signed-release"
  --authority-registry "$authority_registry"
  --authority-registry-sha256 "$authority_registry_sha256"
  --output-directory "$output_directory"
)

if (( want_windows )); then
  win_bin="$work_directory/bcgos-bootstrap_${version}_windows_amd64.exe"
  go run ./dev/release binary --version "$version" --os windows --arch amd64 --output "$win_bin"
  win_sha="$(sha_of "$win_bin")"
  export_args+=(--bootstrapper "$win_bin" --bootstrapper-sha256 "$win_sha")
fi

if (( want_macos )); then
  mac_bin="$work_directory/bcgos-bootstrap_${version}_darwin_arm64"
  go run ./dev/release binary --version "$version" --os darwin --arch arm64 --output "$mac_bin"
  mac_sha="$(sha_of "$mac_bin")"
  export_args+=(--macos-bootstrapper "$mac_bin" --macos-bootstrapper-sha256 "$mac_sha")
fi

echo "==> [4/4] release-export → $output_directory"
dev/skills/release-export/scripts/export-release.sh "${export_args[@]}"

echo "==> verify"
shopt -s nullglob
for zip in "$output_directory"/Maestro-Portable-*.zip; do
  echo "  unzip -t $zip"
  unzip -tq "$zip"
  sha_file="${zip}.sha256"
  if [[ -f "$sha_file" ]]; then
    expected="$(awk '{print $1}' "$sha_file")"
    actual="$(sha_of "$zip")"
    [[ "$expected" == "$actual" ]] || die "sha256 mismatch for $zip"
    echo "  sha256 OK ($actual)"
  fi
done
shopt -u nullglob

echo
echo "done. artifacts in: $output_directory"
