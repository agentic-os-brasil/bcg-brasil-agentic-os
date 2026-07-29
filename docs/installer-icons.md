# Maestro installer icon contract

The Maestro mark is a first-class part of the installer experience, not a
decoration added after packaging. The source of truth for the native app icon
is `installers/wizard/assets/maestro-app-icon.svg`; it uses the presentation
palette: deep green-black, luminous mint and quiet white.

`maestro-mark.svg` remains the orbit/baton mark used inside the wizard. The
separate app icon keeps the conductor silhouette legible at Finder and Dock
sizes instead of shrinking the full presentation mark into an unreadable
miniature.

## Where the mark appears

- browser and package preview: SVG favicon and wizard header;
- visual wizard: brand mark, hero and completion state;
- Windows package: convert `maestro-app-icon.svg` to a multi-size `.ico` and embed it
  in the signed `maestro-installer.exe` resource before Authenticode signing;
- macOS package: convert `maestro-app-icon.svg` to a multi-size `.icns` and place it
  in the signed `Maestro Installer.app` bundle before Developer ID signing and
  notarization.
- macOS DMG: copy the same `.icns` to `.VolumeIcon.icns` and set the Finder
  custom-icon flag before creating the image, so the mounted volume also has a
  Maestro identity.

The conversion step belongs to the release factory. A hand-edited platform
icon, an unsigned wrapper or a different logo is not equivalent evidence.
The source SVG is deterministic and inspectable; the platform-specific
conversion must be hash-recorded in the release manifest and performed before
the native signature is applied.

## Reproducible factory command

On the macOS release worker, generate both native formats from the canonical
source with:

```bash
go run ./dev/release icons \
  --source installers/wizard/assets/maestro-app-icon.svg \
  --output dist/native-icons
```

The command requires the system `qlmanage`, `sips` and `iconutil` tools. It
emits `maestro-app-icon.icns`, `maestro-app-icon.ico` and
`maestro-app-icon-manifest.json`, which records the source and output SHA-256
digests plus the SHA-256 fingerprints of the three rasterization tools. The
`.ico` is PNG-backed and ready for a Windows resource compiler;
the command itself does not embed it in a PE file or apply Authenticode.

The rehearsal DMG uses this same command. The signed release factory must
consume the recorded assets before signing the final native installer, must
run with the approved tool fingerprints and must fail if the source or
generated digest changes between packaging and signing.

## Current evidence boundary

The visual branch proves the icon and theme render in the dependency-free
wizard. It does not yet prove that a Windows `.ico` or macOS `.icns` has been
embedded, signed or accepted on a clean device. Those remain release-factory
and external-authority gates.
