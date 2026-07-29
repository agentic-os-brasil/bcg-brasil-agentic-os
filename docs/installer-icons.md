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

## Windows PE packaging contract

The Windows installer executable is built with the verified `.ico` as a PE
resource, before Authenticode is applied:

```powershell
$iconManifest = Get-Content .\dist\native-icons\maestro-app-icon-manifest.json -Raw | ConvertFrom-Json
$iconPath = (Resolve-Path (Join-Path (Resolve-Path .\dist\native-icons).Path $iconManifest.ico)).Path
$windresSHA256 = $env:MAESTRO_WINDRES_SHA256
if ($windresSHA256 -notmatch '^[a-f0-9]{64}$') { throw "Approved windres fingerprint is missing." }
.\dev\release\build-windows-installer.ps1 `
  -Version 0.1.0 `
  -Icon $iconPath `
  -IconSHA256 $iconManifest.ico_sha256 `
  -ResourceCompilerSHA256 $windresSHA256 `
  -WizardDir (Resolve-Path .\installers\wizard) `
  -ReleaseDirectory (Resolve-Path .\dist\release) `
  -AuthorityRegistry (Resolve-Path .\dist\authority-registry.json) `
  -Bootstrapper (Resolve-Path .\dist\bcgos-bootstrap_0.1.0_windows_amd64.exe) `
  -OutputDirectory (Join-Path (Resolve-Path .\dist).Path "maestro-installer-windows")
```

This contract requires the approved MinGW `windres` executable on the Windows
release worker. It creates a temporary `.syso` resource object, builds the
installer and packages a self-contained directory with `maestro-installer.exe`,
the `wizard/` assets, the exact `release/` tree, `authority-registry.json`,
the native bootstrapper, the canonical `.ico`, `README-UNSIGNED.md` and the
`Run-Maestro-Rehearsal.cmd` launcher and the provenance file. Temporary
source/object files are removed in a `finally` block, and a partially created
output directory is removed if any step fails.
The provenance file records the icon, compiler, approved compiler fingerprint,
resource-object digests, the packaged wizard and release roots, the registry
and bootstrapper digests, the rehearsal launcher and the explicit
`unsigned-candidate` status. Missing `windres`, a changed input, an
unapproved compiler fingerprint or a failed resource build stops the process;
no unsigned bypass is accepted.

The package can therefore be copied to a clean Windows sandbox and launched by
double-clicking `Run-Maestro-Rehearsal.cmd`. The launcher passes `--simulate`
and the adjacent `wizard/` directory to the executable, creates a unique
user-profile sandbox and opens the visual installation flow. This is a
technical rehearsal only: the package remains unsigned until the approved
Authenticode step runs.

## Current evidence boundary

The visual branch proves the icon and theme render in the dependency-free
wizard. The local macOS rehearsal proves `.icns`/`.ico` generation and DMG
consumption. It does not yet prove execution of the Windows PE packaging script
on the approved Windows worker, native signing or acceptance on a clean device.
Those remain release-factory and external-authority gates.
