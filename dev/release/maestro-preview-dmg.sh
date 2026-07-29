#!/bin/sh
set -eu

# Build a local-only visual DMG. This is deliberately outside the signed
# release factory: it contains no release, authority registry or bootstrapper
# inputs and can only launch the non-mutating --preview mode.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
VERSION=${MAESTRO_PREVIEW_VERSION:-0.1.0}
OUTPUT=${MAESTRO_PREVIEW_OUTPUT:-"$ROOT/dist/Maestro-Installer-${VERSION}-unsigned.dmg"}
STAGING=${MAESTRO_PREVIEW_STAGING:-"$ROOT/dist/maestro-preview-dmg"}
WIZARD_DIR=${MAESTRO_PREVIEW_WIZARD_DIR:-"$ROOT/installers/wizard"}
APP_NAME="Maestro Installer Preview.app"
APP="$STAGING/$APP_NAME"
ICONSET="$STAGING/maestro.iconset"

case "$(uname -s)" in
  Darwin) ;;
  *) echo "error: preview DMG requires macOS tooling (hdiutil, iconutil, sips, qlmanage)" >&2; exit 2 ;;
esac
for command in go hdiutil iconutil sips qlmanage SetFile; do
  command -v "$command" >/dev/null 2>&1 || { echo "error: missing required command: $command" >&2; exit 2; }
done
[ -d "$WIZARD_DIR" ] || { echo "error: wizard directory not found: $WIZARD_DIR" >&2; exit 2; }
[ -f "$WIZARD_DIR/assets/maestro-app-icon.svg" ] || { echo "error: app icon source not found" >&2; exit 2; }

rm -rf "$STAGING" "$OUTPUT"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$ICONSET" "$STAGING/.icon-render"
cp -R "$WIZARD_DIR" "$APP/Contents/Resources/wizard"

go build -o "$APP/Contents/Resources/maestro-installer" ./cmd/maestro-installer

qlmanage -t -s 512 -o "$STAGING/.icon-render" "$WIZARD_DIR/assets/maestro-app-icon.svg" >/dev/null
rendered="$STAGING/.icon-render/maestro-app-icon.svg.png"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$rendered" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
done
sips -z 32 32 "$rendered" --out "$ICONSET/icon_16x16@2x.png" >/dev/null
sips -z 64 64 "$rendered" --out "$ICONSET/icon_32x32@2x.png" >/dev/null
sips -z 256 256 "$rendered" --out "$ICONSET/icon_128x128@2x.png" >/dev/null
sips -z 512 512 "$rendered" --out "$ICONSET/icon_256x256@2x.png" >/dev/null
sips -z 1024 1024 "$rendered" --out "$ICONSET/icon_512x512@2x.png" >/dev/null
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/maestro.icns"
cp "$APP/Contents/Resources/maestro.icns" "$STAGING/.VolumeIcon.icns"
SetFile -a V "$STAGING/.VolumeIcon.icns"
SetFile -a C "$STAGING"
rm -rf "$ICONSET" "$STAGING/.icon-render"

cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleDisplayName</key><string>Maestro Installer Preview</string>
<key>CFBundleExecutable</key><string>Maestro Preview</string>
<key>CFBundleIconFile</key><string>maestro.icns</string>
<key>CFBundleIdentifier</key><string>com.bcgbrasil.maestro.installer.preview</string>
<key>CFBundleName</key><string>Maestro Installer Preview</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$VERSION</string>
<key>CFBundleVersion</key><string>${VERSION}-unsigned</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSUIElement</key><true/>
</dict></plist>
EOF
cat > "$APP/Contents/MacOS/Maestro Preview" <<'EOF'
#!/bin/sh
set -eu
contents_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec "$contents_dir/Resources/maestro-installer" --preview --wizard-dir "$contents_dir/Resources/wizard"
EOF
chmod +x "$APP/Contents/MacOS/Maestro Preview"

cat > "$STAGING/README-UNSIGNED.md" <<EOF
# Maestro Installer Preview — unsigned

This DMG is a local visual preview for Maestro ${VERSION}.
It launches only the non-mutating --preview mode. It contains no signed
release, authority registry or bootstrapper and cannot install the product.

This artifact is not evidence of a technical rehearsal, signed release,
notarization or pilot readiness.
EOF

mkdir -p "$(dirname -- "$OUTPUT")"
hdiutil create -volname "Maestro Preview (unsigned)" -srcfolder "$STAGING" -format UDZO -ov "$OUTPUT" >/dev/null
echo "unsigned preview DMG: $OUTPUT"
shasum -a 256 "$OUTPUT"
