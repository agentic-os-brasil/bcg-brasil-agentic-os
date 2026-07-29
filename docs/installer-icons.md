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

## Current evidence boundary

The visual branch proves the icon and theme render in the dependency-free
wizard. It does not yet prove that a Windows `.ico` or macOS `.icns` has been
embedded, signed or accepted on a clean device. Those remain release-factory
and external-authority gates.
