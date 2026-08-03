#!/bin/sh
set -eu

# Build a macOS user-space installer package from explicit release inputs.
# This factory never signs, notarizes or substitutes unsigned release bytes.

usage() {
  cat >&2 <<'EOF'
usage: build-macos-installer.sh \
  --version MAJOR.MINOR.PATCH --arch amd64|arm64 \
  --bridge FILE --wizard-dir DIR --release-dir DIR \
  --authority-registry FILE --bootstrapper FILE --icon FILE \
  --icon-sha256 SHA256 --output FILE.dmg
EOF
  exit 2
}

version=""
arch=""
bridge=""
wizard_dir=""
release_dir=""
authority_registry=""
bootstrapper=""
icon=""
icon_sha256=""
output=""

while (($#)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --arch) arch="${2:-}"; shift 2 ;;
    --bridge) bridge="${2:-}"; shift 2 ;;
    --wizard-dir) wizard_dir="${2:-}"; shift 2 ;;
    --release-dir) release_dir="${2:-}"; shift 2 ;;
    --authority-registry) authority_registry="${2:-}"; shift 2 ;;
    --bootstrapper) bootstrapper="${2:-}"; shift 2 ;;
    --icon) icon="${2:-}"; shift 2 ;;
    --icon-sha256) icon_sha256="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done

[[ "$(uname -s)" = "Darwin" ]] || {
  echo "error: macOS installer packaging requires Darwin tooling." >&2
  exit 2
}
for command in hdiutil lipo shasum SetFile plutil; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "error: missing required command: $command" >&2
    exit 2
  }
done
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || usage
[[ "$arch" = "amd64" || "$arch" = "arm64" ]] || usage
[[ "$icon_sha256" =~ ^[a-f0-9]{64}$ ]] || usage

for path in "$bridge" "$wizard_dir" "$release_dir" "$authority_registry" "$bootstrapper" "$icon" "$output"; do
  case "$path" in
    /*) ;;
    *) echo "error: all package paths must be absolute: $path" >&2; exit 2 ;;
  esac
done

regular_file() {
  [ -f "$1" ] && [ ! -L "$1" ] || {
    echo "error: expected a regular non-symlink file: $1" >&2
    exit 1
  }
}

regular_tree() {
  [ -d "$1" ] && [ ! -L "$1" ] || {
    echo "error: expected a regular non-symlink directory: $1" >&2
    exit 1
  }
  if find "$1" -type l -print -quit | grep -q .; then
    echo "error: package tree contains a symlink: $1" >&2
    exit 1
  fi
}

regular_file "$bridge"
regular_tree "$wizard_dir"
regular_tree "$release_dir"
regular_file "$authority_registry"
regular_file "$bootstrapper"
regular_file "$icon"
release_manifest="$release_dir/release-manifest.json"
release_signature="$release_dir/release-manifest.json.sig"
regular_file "$release_manifest"
regular_file "$release_signature"
[ ! -e "$output" ] || {
  echo "error: output already exists: $output" >&2
  exit 1
}

actual_icon_sha256="$(shasum -a 256 "$icon" | awk '{print $1}')"
[ "$actual_icon_sha256" = "$icon_sha256" ] || {
  echo "error: icon SHA-256 does not match the approved input." >&2
  exit 1
}

manifest_size="$(stat -f%z "$release_manifest")"
[ "$manifest_size" -le 1048576 ] || {
  echo "error: release manifest exceeds the 1 MiB packaging bound." >&2
  exit 1
}
plutil -convert xml1 -o /dev/null "$release_manifest" >/dev/null 2>&1 || {
  echo "error: release manifest is not valid property-list/JSON data." >&2
  exit 1
}
manifest_product="$(plutil -extract product raw -o - "$release_manifest" 2>/dev/null || true)"
manifest_release="$(plutil -extract release raw -o - "$release_manifest" 2>/dev/null || true)"
[ "$manifest_product" = "maestro" ] || {
  echo "error: release manifest product must be maestro." >&2
  exit 1
}
[ "$manifest_release" = "$version" ] || {
  echo "error: release manifest version does not match --version $version." >&2
  exit 1
}

assert_arch() {
  local path="$1"
  local expected="$2"
  local arches
  arches="$(lipo -archs "$path" 2>/dev/null || true)"
  case " $arches " in
    *" $expected "*) ;;
    *) echo "error: $path does not contain the requested $expected architecture." >&2; exit 1 ;;
  esac
}

assert_arch "$bridge" "$arch"
assert_arch "$bootstrapper" "$arch"

tree_digest() {
  root="$1"
  (
    cd "$root"
    find . -type f -print | LC_ALL=C sort | while IFS= read -r relative; do
      digest="$(shasum -a 256 "$relative" | awk '{print $1}')"
      size="$(stat -f%z "$relative")"
      printf 'F|%s|%s|%s\n' "${relative#./}" "$size" "$digest"
    done
  ) | shasum -a 256 | awk '{print $1}'
}

bridge_sha256="$(shasum -a 256 "$bridge" | awk '{print $1}')"
registry_sha256="$(shasum -a 256 "$authority_registry" | awk '{print $1}')"
bootstrapper_sha256="$(shasum -a 256 "$bootstrapper" | awk '{print $1}')"
release_manifest_sha256="$(shasum -a 256 "$release_manifest" | awk '{print $1}')"
release_signature_sha256="$(shasum -a 256 "$release_signature" | awk '{print $1}')"
wizard_tree_sha256="$(tree_digest "$wizard_dir")"
release_tree_sha256="$(tree_digest "$release_dir")"

output_parent="$(dirname "$output")"
mkdir -p "$output_parent"
staging="$(mktemp -d "$output_parent/.maestro-installer-${version}.XXXXXX")"
succeeded=0
cleanup() {
  rm -rf "$staging"
  if [ "$succeeded" -ne 1 ]; then
    rm -f "$output"
  fi
}
trap cleanup EXIT

app="$staging/Maestro Installer.app"
resources="$app/Contents/Resources"
mkdir -p "$app/Contents/MacOS" "$resources"
cp "$bridge" "$resources/maestro-installer"
chmod 755 "$resources/maestro-installer"
cp -R "$wizard_dir" "$resources/wizard"
cp -R "$release_dir" "$resources/release"
cp "$authority_registry" "$resources/authority-registry.json"
cp "$bootstrapper" "$resources/$(basename "$bootstrapper")"
cp "$icon" "$resources/maestro.icns"
cp "$icon" "$staging/.VolumeIcon.icns"
SetFile -a V "$staging/.VolumeIcon.icns"
SetFile -a C "$staging"

cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleDisplayName</key><string>Maestro Installer</string>
<key>CFBundleExecutable</key><string>Maestro Installer</string>
<key>CFBundleIconFile</key><string>maestro.icns</string>
<key>CFBundleIdentifier</key><string>com.bcgbrasil.maestro.installer.runtime</string>
<key>CFBundleName</key><string>Maestro Installer</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>CFBundleVersion</key><string>${version}-unsigned-candidate</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>LSUIElement</key><false/>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
EOF

bootstrap_name="$(basename "$bootstrapper")"
cat > "$app/Contents/MacOS/Maestro Installer" <<EOF
#!/bin/sh
set -eu
contents_dir=\$(CDPATH= cd -- "\$(dirname -- "\$0")/.." && pwd)
exec "\$contents_dir/Resources/maestro-installer" \\
  --wizard-dir "\$contents_dir/Resources/wizard" \\
  --release-dir "\$contents_dir/Resources/release" \\
  --authority-registry "\$contents_dir/Resources/authority-registry.json" \\
  --bootstrapper "\$contents_dir/Resources/$bootstrap_name"
EOF
chmod 755 "$app/Contents/MacOS/Maestro Installer"

staged_wizard_sha256="$(tree_digest "$resources/wizard")"
staged_release_sha256="$(tree_digest "$resources/release")"
staged_bridge_sha256="$(shasum -a 256 "$resources/maestro-installer" | awk '{print $1}')"
staged_registry_sha256="$(shasum -a 256 "$resources/authority-registry.json" | awk '{print $1}')"
staged_bootstrapper_sha256="$(shasum -a 256 "$resources/$bootstrap_name" | awk '{print $1}')"
staged_icon_sha256="$(shasum -a 256 "$resources/maestro.icns" | awk '{print $1}')"
staged_volume_icon_sha256="$(shasum -a 256 "$staging/.VolumeIcon.icns" | awk '{print $1}')"
[ "$staged_wizard_sha256" = "$wizard_tree_sha256" ] || {
  echo "error: wizard changed while packaging." >&2
  exit 1
}
[ "$staged_release_sha256" = "$release_tree_sha256" ] || {
  echo "error: release inputs changed while packaging." >&2
  exit 1
}
[ "$staged_bridge_sha256" = "$bridge_sha256" ] || {
  echo "error: bridge changed while packaging." >&2
  exit 1
}
[ "$staged_registry_sha256" = "$registry_sha256" ] || {
  echo "error: authority registry changed while packaging." >&2
  exit 1
}
[ "$staged_bootstrapper_sha256" = "$bootstrapper_sha256" ] || {
  echo "error: bootstrapper changed while packaging." >&2
  exit 1
}
[ "$staged_icon_sha256" = "$actual_icon_sha256" ] || {
  echo "error: app icon changed while packaging." >&2
  exit 1
}
[ "$staged_volume_icon_sha256" = "$actual_icon_sha256" ] || {
  echo "error: volume icon changed while packaging." >&2
  exit 1
}

cat > "$staging/installer-provenance.json" <<EOF
{
  "product": "maestro-installer",
  "version": "$version",
  "target_os": "darwin",
  "target_arch": "$arch",
  "bridge_sha256": "$bridge_sha256",
  "release_manifest_sha256": "$release_manifest_sha256",
  "release_signature_sha256": "$release_signature_sha256",
  "wizard_tree_sha256": "$wizard_tree_sha256",
  "release_tree_sha256": "$release_tree_sha256",
  "authority_registry_sha256": "$registry_sha256",
  "bootstrapper_sha256": "$bootstrapper_sha256",
  "icon_sha256": "$actual_icon_sha256",
  "native_signature": "pending",
  "notarization": "pending",
  "status": "unsigned-candidate"
}
EOF

cat > "$staging/README-INSTALLER.md" <<EOF
# Maestro Installer $version — unsigned candidate

Open **Maestro Installer.app** to install the exact release bundled in this
package. It writes only to the current user's profile and does not require
administrator permission or change the global PATH.

The bridge will verify the release manifest, authority registry and native
bootstrapper before activation. This package has not been Developer ID signed,
notarized or promoted to a pilot release; it is an installer candidate only.

The exact package inputs are recorded in installer-provenance.json. A failed
verification stops the transaction and never falls back to unsigned bytes.
EOF

hdiutil create -volname "Maestro $version" -srcfolder "$staging" -format UDZO -ov "$output" >/dev/null
succeeded=1
echo "unsigned macOS installer candidate: $output"
echo "sha256: $(shasum -a 256 "$output" | awk '{print $1}')"
