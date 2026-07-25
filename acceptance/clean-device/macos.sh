#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: macos.sh --phase install|update|rollback --run-id ID --version MAJOR.MINOR.PATCH \
  --device-id-hash SHA256 --provider-release-id ID --release-tag TAG \
  --manifest-sha256 SHA256 --expected-signer-id ID --managed-root DIR --data-root DIR \
  --workspace DIR --sentinel FILE --sentinel-sha256 SHA256 --output FILE \
  [--signed-release DIR] [--plan-id ID] [--activation-receipt FILE]
EOF
  exit 2
}

phase=""
run_id=""
device_id_hash=""
version=""
provider_release_id=""
release_tag=""
manifest_sha256=""
expected_signer_id=""
managed_root=""
data_root=""
signed_release=""
plan_id=""
activation_receipt=""
workspace=""
sentinel=""
sentinel_sha256=""
output=""

while (($#)); do
  case "$1" in
    --phase) phase="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    --device-id-hash) device_id_hash="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    --provider-release-id) provider_release_id="${2:-}"; shift 2 ;;
    --release-tag) release_tag="${2:-}"; shift 2 ;;
    --manifest-sha256) manifest_sha256="${2:-}"; shift 2 ;;
    --expected-signer-id) expected_signer_id="${2:-}"; shift 2 ;;
    --managed-root) managed_root="${2:-}"; shift 2 ;;
    --data-root) data_root="${2:-}"; shift 2 ;;
    --signed-release) signed_release="${2:-}"; shift 2 ;;
    --plan-id) plan_id="${2:-}"; shift 2 ;;
    --activation-receipt) activation_receipt="${2:-}"; shift 2 ;;
    --workspace) workspace="${2:-}"; shift 2 ;;
    --sentinel) sentinel="${2:-}"; shift 2 ;;
    --sentinel-sha256) sentinel_sha256="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done

[[ "$phase" =~ ^(install|update|rollback)$ ]] || usage
[[ "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || usage
[[ "$device_id_hash" =~ ^[a-f0-9]{64}$ ]] || usage
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || usage
[[ "$provider_release_id" =~ ^[1-9][0-9]{0,19}$ ]] || usage
[[ "$release_tag" = "maestro-v$version" ]] || usage
[[ "$manifest_sha256" =~ ^[a-f0-9]{64}$ ]] || usage
[[ "$expected_signer_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || usage
[[ "$sentinel_sha256" =~ ^[a-f0-9]{64}$ ]] || usage
[[ "$managed_root" = /* && "$data_root" = /* && "$workspace" = /* &&
  "$sentinel" = /* && "$output" = /* ]] || usage
[[ ! -e "$output" ]] || { echo "Receipt output already exists." >&2; exit 1; }

temp_files=()
cleanup() {
  if ((${#temp_files[@]})); then
    rm -f "${temp_files[@]}"
  fi
}
trap cleanup EXIT

native_signer_id() {
  local signing_details signer
  signing_details="$(codesign -dv --verbose=4 "$1" 2>&1)"
  signer="$(printf '%s\n' "$signing_details" | sed -n 's/^TeamIdentifier=//p' | head -1)"
  [[ "$signer" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || {
    echo "Native signer identity is unavailable." >&2
    return 1
  }
  printf '%s' "$signer"
}

bootstrapper="$managed_root/bcgos-bootstrap"
registry="$managed_root/trust/release-authority-registry.json"
active_cli="$managed_root/bin/bcgos"
[[ -f "$bootstrapper" && ! -L "$bootstrapper" ]] || {
  echo "Approved bootstrapper is unavailable at the protected path." >&2
  exit 1
}
[[ -f "$registry" && ! -L "$registry" ]] || {
  echo "Approved authority registry is unavailable at the protected path." >&2
  exit 1
}
[[ -f "$sentinel" && ! -L "$sentinel" ]] || {
  echo "Sanitized owner-data sentinel is unavailable." >&2
  exit 1
}

codesign --verify --strict --verbose=2 "$bootstrapper"
spctl --assess --type execute --verbose=2 "$bootstrapper"
bootstrapper_signer_id="$(native_signer_id "$bootstrapper")"
[[ "$bootstrapper_signer_id" = "$expected_signer_id" ]] || {
  echo "Bootstrapper signer is not the approved native signer." >&2
  exit 1
}
bootstrapper_sha256="$(shasum -a 256 "$bootstrapper" | awk '{print $1}')"

seed_status="$(mktemp)"
temp_files+=("$seed_status")
"$bootstrapper" seed-status > "$seed_status"
seed_registry_sha256="$(plutil -extract authority_registry_sha256 raw -o - "$seed_status")"
actual_registry_sha256="$(shasum -a 256 "$registry" | awk '{print $1}')"
[[ "$seed_registry_sha256" = "$actual_registry_sha256" ]] || {
  echo "Bootstrapper and authority registry are not the same approved seed." >&2
  exit 1
}

before_sentinel="$(shasum -a 256 "$sentinel" | awk '{print $1}')"
[[ "$before_sentinel" = "$sentinel_sha256" ]] || {
  echo "Owner-data sentinel changed before the acceptance phase." >&2
  exit 1
}

checks=(
  native-bootstrapper-signature
  authority-seed-bound
  owner-data-sentinel-preserved
)
from_version=""
activation_receipt_sha256=""
state_path="$data_root/config/install-state.json"

case "$phase" in
  install)
    [[ -n "$signed_release" && "$signed_release" = /* ]] || usage
    [[ ! -e "$active_cli" && ! -e "$data_root/config/install-state.json" ]] || {
      echo "Install phase requires a device with no existing Maestro state." >&2
      exit 1
    }
    actual_manifest_sha256="$(shasum -a 256 "$signed_release/release-manifest.json" | awk '{print $1}')"
    [[ "$actual_manifest_sha256" = "$manifest_sha256" ]] || {
      echo "Signed release manifest does not match the approved acceptance identity." >&2
      exit 1
    }
    case "$(uname -m)" in
      arm64) machine_arch="arm64" ;;
      x86_64) machine_arch="amd64" ;;
      *)
        echo "Unsupported macOS acceptance architecture." >&2
        exit 1
        ;;
    esac
    release_cli="$signed_release/bcgos_${version}_darwin_${machine_arch}"
    codesign --verify --strict --verbose=2 "$release_cli"
    spctl --assess --type execute --verbose=2 "$release_cli"
    [[ "$(native_signer_id "$release_cli")" = "$expected_signer_id" ]] || {
      echo "Release CLI signer is not the approved native signer." >&2
      exit 1
    }
    "$bootstrapper" install --verified-directory "$signed_release" --data-root "$data_root"
    "$active_cli" init "$workspace" >/dev/null
    checks+=(signed-release-verified first-install-activated provider-release-operator-asserted)
    ;;
  update)
    [[ "$plan_id" =~ ^[a-f0-9]{32}$ ]] || usage
    from_version="$(plutil -extract release raw -o - "$state_path")"
    pending_path="$data_root/config/pending-update.json"
    pending_plan="$(plutil -extract plan.id raw -o - "$pending_path")"
    pending_manifest="$(plutil -extract plan.manifest_sha256 raw -o - "$pending_path")"
    pending_provider_release_id="$(plutil -extract plan.provider_release_id raw -o - "$pending_path")"
    pending_release="$(plutil -extract plan.to_release raw -o - "$pending_path")"
    activation_plan_path="$(plutil -extract activation_plan_path raw -o - "$pending_path")"
    [[ "$pending_plan" = "$plan_id" &&
      "$pending_manifest" = "$manifest_sha256" &&
      "$pending_provider_release_id" = "$provider_release_id" &&
      "$pending_release" = "$version" ]] || {
      echo "Pending update does not bind the approved plan, provider release and manifest." >&2
      exit 1
    }
    activation_receipt="$(dirname "$activation_plan_path")/activation-receipt.json"
    confirmation="$("$active_cli" update --confirm "$plan_id")"
    confirmation_file="$(mktemp)"
    temp_files+=("$confirmation_file")
    printf '%s' "$confirmation" > "$confirmation_file"
    confirmed_plan="$(plutil -extract plan_id raw -o - "$confirmation_file")"
    confirmation_state="$(plutil -extract state raw -o - "$confirmation_file")"
    rm -f "$confirmation_file"
    [[ "$confirmed_plan" = "$plan_id" && "$confirmation_state" = "activation_started" ]] || {
      echo "CLI did not start the exact confirmed update." >&2
      exit 1
    }
    for _ in $(seq 1 120); do
      if [[ -f "$data_root/config/install-state.json" ]] &&
        [[ "$(plutil -extract release raw -o - "$data_root/config/install-state.json")" = "$version" ]]; then
        break
      fi
      sleep 1
    done
    [[ "$(plutil -extract release raw -o - "$data_root/config/install-state.json")" = "$version" ]] || {
      echo "Confirmed update did not reach the expected release." >&2
      exit 1
    }
    [[ -f "$activation_receipt" && ! -L "$activation_receipt" ]] || {
      echo "Update activation receipt is unavailable." >&2
      exit 1
    }
    [[ "$(plutil -extract confirmation_plan_id raw -o - "$activation_receipt")" = "$plan_id" &&
      "$(plutil -extract release raw -o - "$activation_receipt")" = "$version" &&
      "$(plutil -extract manifest_sha256 raw -o - "$activation_receipt")" = "$manifest_sha256" ]] || {
      echo "Update activation receipt does not bind the confirmed release." >&2
      exit 1
    }
    activation_receipt_sha256="$(shasum -a 256 "$activation_receipt" | awk '{print $1}')"
    checks+=(activation-receipt-bound exact-plan-confirmed pending-provider-bound signed-update-activated)
    ;;
  rollback)
    [[ -n "$activation_receipt" && "$activation_receipt" = /* &&
      -f "$activation_receipt" && ! -L "$activation_receipt" ]] || usage
    from_version="$(plutil -extract release raw -o - "$state_path")"
    [[ "$(plutil -extract release raw -o - "$activation_receipt")" = "$from_version" ]] || {
      echo "Rollback evidence is not bound to the active release." >&2
      exit 1
    }
    activation_receipt_sha256="$(shasum -a 256 "$activation_receipt" | awk '{print $1}')"
    "$bootstrapper" rollback --data-root "$data_root"
    checks+=(activation-receipt-bound last-known-good-restored provider-release-operator-asserted)
    ;;
esac

[[ -f "$active_cli" && ! -L "$active_cli" ]] || {
  echo "Active CLI is unavailable after the acceptance phase." >&2
  exit 1
}
codesign --verify --strict --verbose=2 "$active_cli"
spctl --assess --type execute --verbose=2 "$active_cli"
[[ "$(native_signer_id "$active_cli")" = "$expected_signer_id" ]] || {
  echo "Active CLI signer is not the approved native signer." >&2
  exit 1
}
"$active_cli" version | grep -F "bcgos $version" >/dev/null
status_file="$(mktemp)"
doctor_file="$(mktemp)"
temp_files+=("$status_file" "$doctor_file")
"$active_cli" status "$workspace" > "$status_file"
"$active_cli" doctor "$workspace" > "$doctor_file"
[[ "$(plutil -extract workspace.state raw -o - "$status_file")" = "ready" ]] || {
  echo "Workspace status is not ready." >&2
  exit 1
}
[[ "$(plutil -extract capabilities.private_release_auth raw -o - "$status_file")" = "configured" &&
  "$(plutil -extract capabilities.updates raw -o - "$status_file")" = "configured" ]] || {
  echo "Private release capabilities are not configured." >&2
  exit 1
}
[[ "$(plutil -extract state raw -o - "$doctor_file")" = "ready" ]] || {
  echo "Maestro doctor is not ready." >&2
  exit 1
}
rm -f "$status_file" "$doctor_file"
after_sentinel="$(shasum -a 256 "$sentinel" | awk '{print $1}')"
[[ "$after_sentinel" = "$sentinel_sha256" ]] || {
  echo "Owner-data sentinel changed during the acceptance phase." >&2
  exit 1
}
checks+=(native-cli-signature active-cli-self-check status-verified doctor-verified)

recorded_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
checks_json=""
for check in "${checks[@]}"; do
  if [[ -n "$checks_json" ]]; then checks_json+=","; fi
  checks_json+="\"$check\""
done
output_parent="$(dirname "$output")"
mkdir -p "$output_parent"
umask 077
receipt_temp="$(mktemp "$output_parent/.maestro-receipt.XXXXXX")"
temp_files+=("$receipt_temp")
printf '{
  "schema_version": 1,
  "run_id": "%s",
  "device_id_hash": "%s",
  "platform": "macos",
  "phase": "%s",
  "from_version": "%s",
  "to_version": "%s",
  "provider_release_id": "%s",
  "release_tag": "%s",
  "manifest_sha256": "%s",
  "bootstrapper_sha256": "%s",
  "authority_registry_sha256": "%s",
  "native_signer_id": "%s",
  "activation_receipt_sha256": "%s",
  "state": "pass",
  "recorded_at": "%s",
  "checks": [%s]
}\n' \
  "$run_id" "$device_id_hash" "$phase" "$from_version" "$version" \
  "$provider_release_id" "$release_tag" "$manifest_sha256" \
  "$bootstrapper_sha256" "$actual_registry_sha256" "$expected_signer_id" \
  "$activation_receipt_sha256" "$recorded_at" "$checks_json" > "$receipt_temp"
chmod 600 "$receipt_temp"
ln "$receipt_temp" "$output"
rm -f "$receipt_temp"

echo "Sanitized macOS $phase receipt written to $output"
