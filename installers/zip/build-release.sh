#!/usr/bin/env bash
# Maestro target-specific ZIP factory.
#
# Usage:
#   installers/zip/build-release.sh <version> [macos-arm64|windows-amd64]
#
# With no target, both platform archives are built. A compatibility
# Maestro-v<version>.zip alias of the macOS archive is also emitted for the
# existing Hub evaluation harness; it is transport, not a universal binary.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
TEMPLATE_DIR="$REPO_ROOT/installers/zip/user-template"
BUNDLES_DIR="$REPO_ROOT/bundles"

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "usage: build-release.sh <version> [macos-arm64|windows-amd64]" >&2
  exit 2
fi

VERSION="$1"
REQUESTED_TARGET="${2:-all}"

if ! printf '%s' "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "version must match X.Y.Z: got '$VERSION'" >&2
  exit 2
fi

case "$REQUESTED_TARGET" in
  all|macos-arm64|windows-amd64) ;;
  *) echo "unsupported target: $REQUESTED_TARGET" >&2; exit 2 ;;
esac

if { [ "$REQUESTED_TARGET" = "all" ] || [ "$REQUESTED_TARGET" = "macos-arm64" ]; } && ! command -v codesign >/dev/null 2>&1; then
  echo "FATAL: codesign is required to build macOS portable output" >&2
  exit 1
fi

mkdir -p "$DIST_DIR"
FACTORY_ROOT=$(mktemp -d)
trap 'rm -rf "$FACTORY_ROOT"' EXIT

sha256_file() {
  shasum -a 256 "$1" | awk '{print $1}'
}

stage_common() {
  local maestro_dir="$1"
  mkdir -p "$maestro_dir"
  cp -R "$TEMPLATE_DIR/." "$maestro_dir/"
  cp -R "$BUNDLES_DIR" "$maestro_dir/bundles"
  if [ -d "$REPO_ROOT/schemas" ]; then
    cp -R "$REPO_ROOT/schemas" "$maestro_dir/schemas"
  fi
  printf '%s\n' "$VERSION" > "$maestro_dir/VERSION"

  if [ ! -f "$maestro_dir/CLAUDE.md" ]; then
    echo "FATAL: CLAUDE.md missing from user template" >&2
    exit 1
  fi
  if [ -e "$maestro_dir/data" ]; then
    rm -rf "$maestro_dir/data"
  fi
  find "$maestro_dir" -name '.DS_Store' -delete 2>/dev/null || true
  find "$maestro_dir" -name '__pycache__' -type d -exec rm -rf {} + 2>/dev/null || true
  find "$maestro_dir" -name '*.pyc' -delete 2>/dev/null || true
  find "$maestro_dir/bundles" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -delete 2>/dev/null || true
  chmod +x "$maestro_dir/.claude/hooks/"*.sh 2>/dev/null || true
}

build_target() {
  local target="$1"
  local target_os target_arch cli_name bootstrap_name
  case "$target" in
    macos-arm64)
      target_os="darwin"
      target_arch="arm64"
      cli_name="bcgos"
      bootstrap_name="bcgos-bootstrap"
      ;;
    windows-amd64)
      target_os="windows"
      target_arch="amd64"
      cli_name="bcgos.exe"
      bootstrap_name="bcgos-bootstrap.exe"
      ;;
  esac

  local target_root="$FACTORY_ROOT/$target"
  local maestro_dir="$target_root/Maestro"
  local managed_root="$maestro_dir/managed"
  local bin_root="$managed_root/bin"
  mkdir -p "$bin_root"
  stage_common "$maestro_dir"

  echo "==> Building installed control plane for $target"
  GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 \
    go build -mod=readonly -buildvcs=false -trimpath \
      -ldflags "-s -w -X main.Version=$VERSION" \
      -o "$bin_root/$cli_name" "$REPO_ROOT/cmd/bcgos"
  GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 \
    go build -mod=readonly -buildvcs=false -trimpath \
      -ldflags "-s -w -X main.Version=$VERSION" \
      -o "$managed_root/$bootstrap_name" "$REPO_ROOT/cmd/bcgos-bootstrap"
  chmod 700 "$bin_root/$cli_name" "$managed_root/$bootstrap_name"

  if [ "$target_os" = "darwin" ]; then
    if ! command -v codesign >/dev/null 2>&1; then
      echo "FATAL: codesign is required to publish the macOS portable factory output" >&2
      exit 1
    fi
    for executable in "$bin_root/$cli_name" "$managed_root/$bootstrap_name"; do
      codesign --force --sign - --timestamp=none "$executable" >/dev/null
      if ! codesign -d --verbose=4 "$executable" 2>&1 | grep -q '^Signature=adhoc$'; then
        echo "FATAL: macOS portable executable is not ad-hoc signed: $executable" >&2
        exit 1
      fi
    done
  fi

  local cli_sha
  cli_sha=$(sha256_file "$bin_root/$cli_name")
  cat > "$managed_root/install-manifest.json" <<EOF
{
  "schema_version": 1,
  "version": "$VERSION",
  "target_os": "$target_os",
  "target_arch": "$target_arch",
  "cli_path": "bin/$cli_name",
  "cli_sha256": "$cli_sha"
}
EOF

  # Identical inputs produce stable archive metadata and file order.
  find "$maestro_dir" -exec touch -h -t 198001010000 {} +
  local zip_name="Maestro-Portable-${VERSION}-${target}-local-beta-unsigned.zip"
  local zip_path="$DIST_DIR/$zip_name"
  rm -f "$zip_path"
  (
    cd "$target_root"
    find Maestro -type f -print | LC_ALL=C sort | zip -X -q "$zip_path" -@
  )
  local archive_sha
  archive_sha=$(sha256_file "$zip_path")
  printf '%s  %s\n' "$archive_sha" "$zip_name" > "${zip_path%.zip}.sha256"
  echo "  $zip_path"

  if [ "$target" = "macos-arm64" ] && [ "$REQUESTED_TARGET" = "all" ]; then
    local compatibility="$DIST_DIR/Maestro-v${VERSION}.zip"
    cp "$zip_path" "$compatibility"
    printf '%s  %s\n' "$archive_sha" "$(basename "$compatibility")" > "$DIST_DIR/Maestro-v${VERSION}.sha256"
  fi
}

if [ "$REQUESTED_TARGET" = "all" ]; then
  build_target macos-arm64
  build_target windows-amd64
else
  build_target "$REQUESTED_TARGET"
fi

echo "Release ZIPs built. Local build evidence does not establish signing, native qualification, release readiness or pilot readiness."
