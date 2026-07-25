#!/usr/bin/env bash
# Installs one locally supplied BCGOS trial artifact. It does not download an
# artifact or establish signature trust; use is always explicit and local.
set -euo pipefail

artifact=""
checksum=""
install_root=""
allow_unsigned=false

usage() {
  echo "Usage: install.sh --artifact PATH --checksum PATH [--install-root PATH] --allow-unsigned-trial"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact) artifact="${2:-}"; shift 2 ;;
    --checksum) checksum="${2:-}"; shift 2 ;;
    --install-root) install_root="${2:-}"; shift 2 ;;
    --allow-unsigned-trial) allow_unsigned=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$artifact" || -z "$checksum" || "$allow_unsigned" != true ]]; then
  echo "--artifact, --checksum and --allow-unsigned-trial are required" >&2
  exit 2
fi
if [[ ! -f "$artifact" || ! -f "$checksum" ]]; then
  echo "Artifact or checksum file was not found" >&2
  exit 1
fi
if [[ -z "$install_root" ]]; then
  case "$(uname -s)" in
    Darwin) install_root="$HOME/Library/Application Support/BCGOS/trial" ;;
    *) install_root="${XDG_DATA_HOME:-$HOME/.local/share}/bcgos/trial" ;;
  esac
fi
if [[ -e "$install_root" ]]; then
  echo "Trial installation already exists at $install_root; it was not replaced" >&2
  exit 1
fi

expected_hash="$(awk 'NR == 1 {print $1}' "$checksum")"
if [[ ! "$expected_hash" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo "Checksum file does not begin with a SHA-256 digest" >&2
  exit 1
fi
if command -v shasum >/dev/null 2>&1; then
  actual_hash="$(shasum -a 256 "$artifact" | awk '{print $1}')"
else
  actual_hash="$(sha256sum "$artifact" | awk '{print $1}')"
fi
actual_hash="$(printf '%s' "$actual_hash" | tr '[:upper:]' '[:lower:]')"
expected_hash="$(printf '%s' "$expected_hash" | tr '[:upper:]' '[:lower:]')"
if [[ "$actual_hash" != "$expected_hash" ]]; then
  echo "Artifact checksum does not match; nothing was installed" >&2
  exit 1
fi

parent="$(dirname "$install_root")"
mkdir -p "$parent"
stage="$(mktemp -d "$parent/.bcgos-trial-stage.XXXXXX")"
cleanup() { rm -rf "$stage"; }
trap cleanup EXIT
mkdir -p "$stage/bin"
cp "$artifact" "$stage/bin/bcgos"
chmod 700 "$stage/bin/bcgos"
if ! "$stage/bin/bcgos" version >/dev/null; then
  echo "Artifact did not pass its version self-check; nothing was installed" >&2
  exit 1
fi
mv "$stage" "$install_root"
trap - EXIT
echo "BCGOS trial installed at $install_root/bin/bcgos"
echo "This unsigned local trial does not configure PATH or automatic updates."
