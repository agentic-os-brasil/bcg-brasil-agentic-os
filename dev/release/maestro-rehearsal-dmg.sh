#!/bin/sh
set -eu

# Build a local-only technical rehearsal DMG. This is deliberately outside the
# signed release factory: it contains no release, authority registry or
# bootstrapper inputs. It exercises the wizard's verify -> install -> open
# flow in an isolated simulation sandbox and never claims signed trust.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
VERSION=${MAESTRO_REHEARSAL_VERSION:-0.1.0}
OUTPUT=${MAESTRO_REHEARSAL_OUTPUT:-"$ROOT/dist/Maestro-Installer-${VERSION}-rehearsal.dmg"}
STAGING=${MAESTRO_REHEARSAL_STAGING:-"$ROOT/dist/maestro-rehearsal-dmg"}
WIZARD_DIR=${MAESTRO_REHEARSAL_WIZARD_DIR:-"$ROOT/installers/wizard"}
APP_NAME="Maestro Installer Rehearsal.app"
APP="$STAGING/$APP_NAME"

case "$(uname -s)" in
  Darwin) ;;
  *) echo "error: rehearsal DMG requires macOS tooling (hdiutil, qlmanage, sips, iconutil)" >&2; exit 2 ;;
esac
for command in go hdiutil SetFile; do
  command -v "$command" >/dev/null 2>&1 || { echo "error: missing required command: $command" >&2; exit 2; }
done
[ -d "$WIZARD_DIR" ] || { echo "error: wizard directory not found: $WIZARD_DIR" >&2; exit 2; }
[ -f "$WIZARD_DIR/assets/maestro-app-icon.svg" ] || { echo "error: app icon source not found" >&2; exit 2; }

rm -rf "$STAGING" "$OUTPUT"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp -R "$WIZARD_DIR" "$APP/Contents/Resources/wizard"

go build -o "$APP/Contents/Resources/maestro-installer" ./cmd/maestro-installer

go run ./dev/release icons \
  --source "$WIZARD_DIR/assets/maestro-app-icon.svg" \
  --output "$STAGING" >/dev/null
cp "$STAGING/maestro-app-icon.icns" "$APP/Contents/Resources/maestro.icns"
cp "$STAGING/maestro-app-icon.icns" "$STAGING/.VolumeIcon.icns"
SetFile -a V "$STAGING/.VolumeIcon.icns"
SetFile -a C "$STAGING"

cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleDisplayName</key><string>Maestro Installer Rehearsal</string>
<key>CFBundleExecutable</key><string>Maestro Rehearsal</string>
<key>CFBundleIconFile</key><string>maestro.icns</string>
<key>CFBundleIdentifier</key><string>com.bcgbrasil.maestro.installer.rehearsal</string>
<key>CFBundleName</key><string>Maestro Installer Rehearsal</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$VERSION</string>
<key>CFBundleVersion</key><string>${VERSION}-unsigned</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSUIElement</key><true/>
</dict></plist>
EOF
cat > "$APP/Contents/MacOS/Maestro Rehearsal" <<'EOF'
#!/bin/sh
set -eu
contents_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec "$contents_dir/Resources/maestro-installer" --simulate --wizard-dir "$contents_dir/Resources/wizard"
EOF
chmod +x "$APP/Contents/MacOS/Maestro Rehearsal"

cat > "$STAGING/README-REHEARSAL.md" <<EOF
# Maestro Installer Rehearsal — unsigned

This DMG runs a technical installation rehearsal for Maestro ${VERSION}.
Double-click \`Maestro Installer Rehearsal.app\` to open the visual wizard. It
launches --simulate, which creates an isolated sandbox and exercises the same
verify -> install -> open flow without signed release inputs.

In the wizard, choose **Verificar release**, then **Instalar Maestro**. At the
end, **Abrir pasta do Maestro** opens the rehearsal data directory. No
administrator permission is requested.

The rehearsal does not install the product, does not assert Ed25519,
Authenticode or notarization, and does not change the global PATH.

The DMG includes the deterministic Maestro icon manifest and both native icon
formats generated from the canonical SVG. It does not embed or assert a
Windows executable signature.

This artifact is not a signed release, notarized package or pilot evidence.
EOF

mkdir -p "$(dirname -- "$OUTPUT")"
hdiutil create -volname "Maestro Rehearsal (unsigned)" -srcfolder "$STAGING" -format UDZO -ov "$OUTPUT" >/dev/null
echo "unsigned rehearsal DMG: $OUTPUT"
shasum -a 256 "$OUTPUT"
