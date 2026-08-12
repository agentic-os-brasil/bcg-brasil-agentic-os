#!/usr/bin/env bash
# DEPRECATED: This script builds `cmd/bcgos`, which PR #302 deleted as part
# of the surgical Go delete / ZIP-only distribution pivot. It cannot be run
# as-is and will fail closed until it is either repointed at the ZIP
# installer path (`installers/zip/`) or removed. Kept in place so operators
# grepping `dev/` see the deprecation notice instead of a stale success path.
echo "DEPRECATED: dev/trial-install-smoke.sh depends on cmd/bcgos, which was removed in PR #302 (ZIP-only pivot). Repoint at installers/zip/ or delete before use." >&2
exit 1
# Runs a real macOS/Linux trial install in an isolated temporary home.
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
trial_root="$(mktemp -d)"
cleanup_trial_root() {
  # Go's module cache makes downloaded sources read-only. Restore write
  # permission so cleanup remains reliable on macOS as well as Linux.
  chmod -R u+w "$trial_root" 2>/dev/null || true
  rm -rf "$trial_root"
}
trap cleanup_trial_root EXIT

artifact_dir="$trial_root/artifact"
install_root="$trial_root/install"
workspace="$trial_root/workspace"
task_cache="$trial_root/go-cache"
task_modcache="$trial_root/go-mod-cache"
mkdir -p "$artifact_dir"

GOCACHE="$task_cache" GOMODCACHE="$task_modcache" go build -trimpath -o "$artifact_dir/bcgos" "$project_root/cmd/bcgos"
shasum -a 256 "$artifact_dir/bcgos" | awk '{print $1 "  bcgos"}' > "$artifact_dir/bcgos.sha256"

printf '%064d  bcgos\n' 0 > "$artifact_dir/invalid.sha256"
if bash "$project_root/installers/trial/install.sh" \
  --artifact "$artifact_dir/bcgos" \
  --checksum "$artifact_dir/invalid.sha256" \
  --install-root "$trial_root/rejected-install" \
  --allow-unsigned-trial; then
  echo "installer accepted an invalid checksum" >&2
  exit 1
fi
if [[ -e "$trial_root/rejected-install" ]]; then
  echo "installer left an activation directory after checksum rejection" >&2
  exit 1
fi

bash "$project_root/installers/trial/install.sh" \
  --artifact "$artifact_dir/bcgos" \
  --checksum "$artifact_dir/bcgos.sha256" \
  --install-root "$install_root" \
  --allow-unsigned-trial

"$install_root/bin/bcgos" version | grep -q '^bcgos '
if bash "$project_root/installers/trial/install.sh" \
  --artifact "$artifact_dir/bcgos" \
  --checksum "$artifact_dir/bcgos.sha256" \
  --install-root "$install_root" \
  --allow-unsigned-trial; then
  echo "installer replaced an existing trial installation" >&2
  exit 1
fi
"$install_root/bin/bcgos" version | grep -q '^bcgos '
HOME="$trial_root/home" "$install_root/bin/bcgos" init "$workspace" | grep -q '"state": "initialized"'
HOME="$trial_root/home" "$install_root/bin/bcgos" doctor "$workspace" | grep -q '"state": '

echo "[ok] local trial installation"
