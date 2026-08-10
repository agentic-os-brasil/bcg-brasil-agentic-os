#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: export-release.sh \
  --version MAJOR.MINOR.PATCH \
  --release-directory ABS_DIR \
  --authority-registry ABS_FILE \
  --authority-registry-sha256 SHA256 \
  --bootstrapper ABS_FILE \
  --bootstrapper-sha256 SHA256 \
  [--output-directory ABS_DIR] [--github-repository OWNER/REPO] \
  [--publish-github --confirm-publish]
EOF
}

die() {
  echo "release export: $*" >&2
  exit 1
}

version=""
release_directory=""
authority_registry=""
authority_registry_sha256=""
bootstrapper=""
bootstrapper_sha256=""
output_directory=""
github_repository=""
publish_github=0
confirm_publish=0

while (($# > 0)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --release-directory) release_directory="${2:-}"; shift 2 ;;
    --authority-registry) authority_registry="${2:-}"; shift 2 ;;
    --authority-registry-sha256) authority_registry_sha256="${2:-}"; shift 2 ;;
    --bootstrapper) bootstrapper="${2:-}"; shift 2 ;;
    --bootstrapper-sha256) bootstrapper_sha256="${2:-}"; shift 2 ;;
    --output-directory) output_directory="${2:-}"; shift 2 ;;
    --github-repository) github_repository="${2:-}"; shift 2 ;;
    --publish-github) publish_github=1; shift ;;
    --confirm-publish) confirm_publish=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; die "opção desconhecida: $1" ;;
  esac
done

[[ -n "$version" && -n "$release_directory" && -n "$authority_registry" &&
  -n "$authority_registry_sha256" && -n "$bootstrapper" &&
  -n "$bootstrapper_sha256" ]] || { usage; die "todos os inputs da factory são obrigatórios"; }
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "versão deve ser MAJOR.MINOR.PATCH"
[[ "$authority_registry_sha256" =~ ^[a-f0-9]{64}$ ]] || die "authority registry SHA-256 inválido"
[[ "$bootstrapper_sha256" =~ ^[a-f0-9]{64}$ ]] || die "bootstrapper SHA-256 inválido"

root="$(git rev-parse --show-toplevel 2>/dev/null)" || die "execute na raiz de um checkout Git"
cd "$root"
[[ -z "$(git status --porcelain)" ]] || die "árvore Git suja; preserve as mudanças antes de exportar"
branch="$(git branch --show-current)"
[[ "$branch" = "main" ]] || die "exportação de release deve partir da branch main"
source_commit="$(git rev-parse HEAD)"

for input in "$release_directory" "$authority_registry" "$bootstrapper"; do
  [[ "$input" = /* ]] || die "input precisa ser path absoluto: $input"
  [[ -f "$input" || -d "$input" ]] || die "input ausente: $input"
done

if [[ -z "$output_directory" ]]; then
  output_directory="$root/releases/$version"
fi
[[ "$output_directory" = /* ]] || die "output directory precisa ser absoluto"
if [[ -e "$output_directory" ]]; then
  [[ -d "$output_directory" ]] || die "output directory não é um diretório"
  [[ -z "$(find "$output_directory" -mindepth 1 -maxdepth 1 -print -quit)" ]] || die "output directory já contém artefatos: $output_directory"
else
  mkdir -p "$output_directory"
fi

zip_path="$output_directory/Maestro-Portable-${version}-windows-amd64-local-beta-unsigned.zip"
expected_checksum="$zip_path.sha256"
expected_provenance="$zip_path.provenance.json"

go run ./dev/harness validate --full
go run ./dev/release portable-windows \
  --version "$version" \
  --release-directory "$release_directory" \
  --authority-registry "$authority_registry" \
  --authority-registry-sha256 "$authority_registry_sha256" \
  --bootstrapper "$bootstrapper" \
  --bootstrapper-sha256 "$bootstrapper_sha256" \
  --output "$zip_path"

[[ -f "$zip_path" && -f "$expected_checksum" && -f "$expected_provenance" ]] || die "factory não produziu o conjunto completo"

if command -v unzip >/dev/null 2>&1; then
  unzip -t "$zip_path" >/dev/null || die "ZIP inválido"
else
  echo "release export: unzip indisponível; validação estrutural do ZIP ficou unavailable" >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  actual_zip_sha256="$(shasum -a 256 "$zip_path" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  actual_zip_sha256="$(sha256sum "$zip_path" | awk '{print $1}')"
else
  die "shasum/sha256sum indisponível"
fi
grep -F "${actual_zip_sha256}  $(basename "$zip_path")" "$expected_checksum" >/dev/null || die "checksum emitido não corresponde ao ZIP"

metadata="$output_directory/EXPORT-METADATA.txt"
{
  printf 'schema_version=1\n'
  printf 'source_commit=%s\n' "$source_commit"
  printf 'version=%s\n' "$version"
  printf 'tag=maestro-v%s\n' "$version"
  printf 'artifact=%s\n' "$(basename "$zip_path")"
  printf 'artifact_sha256=%s\n' "$actual_zip_sha256"
  printf 'authority_registry_sha256=%s\n' "$authority_registry_sha256"
  printf 'bootstrapper_sha256=%s\n' "$bootstrapper_sha256"
  printf 'status=unsigned-controlled-canary\n'
} > "$metadata"

if ((publish_github)); then
  ((confirm_publish)) || die "publicação exige --confirm-publish"
  command -v gh >/dev/null 2>&1 || die "gh CLI indisponível para publicar"
  gh auth status >/dev/null 2>&1 || die "gh não está autenticado"
  if [[ -z "$github_repository" ]]; then
    github_repository="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
  fi
  [[ "$github_repository" =~ ^[^/]+/[^/]+$ ]] || die "GitHub repository inválido: $github_repository"
  tag="maestro-v$version"
  if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
    die "tag já existe e não pode ser reutilizada: $tag"
  fi
  if gh release view "$tag" --repo "$github_repository" >/dev/null 2>&1; then
    die "GitHub Release já existe e não pode ser substituído: $tag"
  fi
  notes_file="$release_directory/release-notes-$version.md"
  args=(release create "$tag" "$zip_path" "$expected_checksum" "$expected_provenance" \
    --repo "$github_repository" --target "$source_commit" --title "Maestro $version" \
    --prerelease)
  if [[ -f "$notes_file" ]]; then
    args+=(--notes-file "$notes_file")
  else
    args+=(--notes "Unsigned controlled Canary artifact. Pilot and production gates remain separate.")
  fi
  gh "${args[@]}"
  printf 'github_release=https://github.com/%s/releases/tag/%s\n' "$github_repository" "$tag" >> "$metadata"
elif ((confirm_publish)); then
  die "--confirm-publish só pode ser usado com --publish-github"
fi

cat "$metadata"
