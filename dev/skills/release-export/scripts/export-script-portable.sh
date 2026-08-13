#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: export-script-portable.sh --version MAJOR.MINOR.PATCH [--output-directory ABS_DIR]" >&2
}

die() {
  echo "script portable export: $*" >&2
  exit 1
}

version=""
output_directory=""
while (($# > 0)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --output-directory) output_directory="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; die "opção desconhecida: $1" ;;
  esac
done

[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { usage; die "versão deve ser MAJOR.MINOR.PATCH"; }
root="$(git rev-parse --show-toplevel 2>/dev/null)" || die "execute em um checkout Git"
cd "$root"
[[ -z "$(git status --porcelain)" ]] || die "árvore Git suja; preserve as mudanças antes de exportar"
[[ "$(git branch --show-current)" = main ]] || die "exportação deve partir da branch main"

if [[ -z "$output_directory" ]]; then
  output_directory="$root/releases/$version/script-only"
fi
[[ "$output_directory" = /* ]] || die "output directory precisa ser absoluto"
if [[ -e "$output_directory" ]]; then
  [[ -d "$output_directory" && -z "$(find "$output_directory" -mindepth 1 -maxdepth 1 -print -quit)" ]] || die "output directory já contém artefatos"
else
  mkdir -p "$output_directory"
fi

macos_zip="$output_directory/Maestro-Portable-${version}-macos-shell-local-beta.zip"
windows_zip="$output_directory/Maestro-Portable-${version}-windows-powershell-local-beta.zip"

go run ./dev/harness validate --full
go run ./dev/release portable-script-macos --version "$version" --output "$macos_zip"
go run ./dev/release portable-script-windows --version "$version" --output "$windows_zip"

for archive in "$macos_zip" "$windows_zip"; do
  [[ -f "$archive" && -f "$archive.sha256" && -f "$archive.provenance.json" ]] || die "factory não produziu conjunto completo: $archive"
  unzip -t "$archive" >/dev/null || die "ZIP inválido: $archive"
  if command -v shasum >/dev/null 2>&1; then
    digest="$(shasum -a 256 "$archive" | awk '{print $1}')"
  else
    digest="$(sha256sum "$archive" | awk '{print $1}')"
  fi
  grep -F "$digest  $(basename "$archive")" "$archive.sha256" >/dev/null || die "checksum divergente: $archive"
  if [[ "$archive" = "$macos_zip" ]]; then
    macos_digest="$digest"
  else
    windows_digest="$digest"
  fi
done

cat > "$output_directory/EXPORT-METADATA.txt" <<EOF
schema_version=1
source_commit=$(git rev-parse HEAD)
version=$version
macos_artifact=$(basename "$macos_zip")
macos_sha256=$macos_digest
windows_artifact=$(basename "$windows_zip")
windows_sha256=$windows_digest
status=script-only-controlled-beta
publisher_authentication=unavailable
native_cli=unavailable
EOF

echo "script-only portable export complete: $output_directory"
