---
name: maestro-local-beta-installer
description: Clean Maestro-owned installation surfaces, then build and verify a real local-beta installer from a merged source snapshot using a test-only Ed25519 authority and the native macOS packaging path instead of the rehearsal simulator.
---

# Maestro Local-Beta Installer

Use this skill when the owner asks to clean and produce the next local-beta
installer that must execute the real verify -> install -> workspace flow. This
is a packaging direction, not a production-release authorization and not a
substitute for Developer ID signing, notarization or Authenticode signing.

Resolve the canonical `interaction-profile` before presenting the procedure.
It controls how much implementation detail to show, but never weakens the
release, signing, consent or cleanup gates below.

## Operating contract

This is the single entry point for a **clean local-beta generation**. Unless
the owner explicitly chooses `preserve`, the workflow is always:

1. resolve the exact clean-install manifest;
2. show the manifest and obtain explicit owner confirmation;
3. snapshot and unload Maestro-owned maintenance surfaces;
4. move those surfaces to a recoverable backup and verify the boundary is clean;
5. build and verify the local-beta artifact;
6. hand off the DMG, checksum, receipts and backup location.

Do not start packaging while an old Maestro installation remains active. The
cleanup is part of generation, but it is never an implicit authorization to
delete or replace paths. If an exact path cannot be resolved, stop and report
it rather than broadening the target with a glob or `rm -rf`.

The skill has three explicit modes:

- `clean_standard`: clean the intended Maestro workspace, the Maestro
  application-support root and the Maestro LaunchAgent.
- `clean_migration`: `clean_standard` plus the legacy `BCGOS` application-
  support root, only when the owner explicitly includes that path.
- `preserve`: do not touch an existing installation; use only when the owner
  explicitly requests an in-place/update validation.

## Status vocabulary

- **Rehearsal** means `dev/release/maestro-rehearsal-dmg.sh`: it runs
  `--simulate` and does not contain release inputs.
- **Local-beta installer** means `dev/release/build-macos-installer.sh`: it
  contains a signed test-only release manifest, the real bridge, bootstrapper,
  wizard, registry and icon. The macOS app is still unsigned/notarization-free.
- **Production release** requires organization-owned Ed25519 custody, Apple
  Developer ID signing, notarization, publication and clean-device evidence.

Never call a local-beta artifact a production or pilot-ready release. Never
put a private key, seed, certificate or credential in Git, a bundle or a DMG.

## Preconditions

1. Start from the merged `main` snapshot and confirm a clean worktree.
2. Choose one immutable `MAJOR.MINOR.PATCH` version. Never reuse a version
   already present in `dist/`, a release manifest or a delivered handoff. The
   explicit CLI version, `VERSION`, candidate manifest and every output filename
   must agree before an official release is claimed. A repository placeholder
   such as `0.0.0` may be used only for a controlled local test with the chosen
   version passed explicitly; record the mismatch and fix it before publication.
3. Use a separate test-only authority registry and matching Ed25519 seed. The
   registry issuer/key ID must be marked beta/test-only and must never be used
   by a production workflow. Read the public registry; do not print or commit
   the seed.
4. Use the current source for the bridge and bootstrapper. Do not reuse old
   native binaries from a previous installer.

## Clean-install preparation

Remove only Maestro-owned active surfaces, preserving a recoverable backup.
This is a destructive-boundary operation and requires an explicit owner
confirmation naming the resolved targets before any `bootout` or move. A vague
request to "clean everything" is not enough.

Before touching anything:

1. Resolve and print the exact paths that exist; exclude the source repository,
   Canary evidence workspace, unrelated maintenance and client data.
2. Show the owner the target list, the selected mode and the legacy-root
   inclusion decision; obtain confirmation for that exact list.
3. Create a timestamped backup directory **outside every active target** and
   record the current LaunchAgent state (including `launchctl print` output)
   inside it. A backup nested under `~/Library/Application Support/Maestro`
   is invalid because it would be moved with the root it backs up.

After moving each target, verify that it exists inside the backup and that the
active path is absent. Stop and report if any verification fails. Keep the
backup path in the handoff so rollback is possible.

The Maestro-owned active surfaces are:

- the intended user workspace, for example `~/Developer/maestro-os`;
- `~/Library/Application Support/Maestro`;
- legacy `~/Library/Application Support/BCGOS` only when the owner explicitly
  wants a completely clean migration test;
- the Maestro LaunchAgent, for example
  `~/Library/LaunchAgents/com.bcg.maestro.maintenance.plist`;
- prior build outputs under the isolated release worktree.

Unload the LaunchAgent before moving its plist. Move each existing target to
the backup using an explicit path; never use a recursive wildcard. Do not touch
the source repository, the Canary evidence workspace, unrelated maintenance or
client/project data. Prefer moving targets to a timestamped temporary backup
over irreversible deletion.

### Clean manifest and receipt

Before confirmation, render a manifest with absolute paths and existence state,
for example:

```text
mode: clean_standard
workspace: /Users/<owner>/Developer/maestro-os [exists|absent]
maestro_root: /Users/<owner>/Library/Application Support/Maestro [exists|absent]
launch_agent: /Users/<owner>/Library/LaunchAgents/com.bcg.maestro.maintenance.plist [exists|absent]
legacy_bcgos: excluded (select clean_migration to include)
```

The cleanup receipt must contain: selected mode, exact targets, backup path,
LaunchAgent label, pre-clean `launchctl print` result, moved paths and
post-clean existence checks. The receipt is evidence of cleanup only; it does
not qualify hooks, signing, notarization or runtime behavior.

After the move, verify both sides for every target: the active path is absent,
the backup path exists, and the backup contains the expected metadata. If any
check fails, stop before candidate generation and restore from the backup.

## Build sequence

Set `ROOT` to the current merged worktree, `VERSION` to the chosen release,
`REGISTRY` to the test-only public registry and `SEED` to the protected local
seed. Set `PUBLICATION_REPOSITORY` only when using the protected seeded build
path. Keep all paths absolute when invoking the packaging script.

### 1. Candidate

```sh
set -euo pipefail

go run ./dev/release candidate \
  --version "$VERSION" --channel canary \
  --output "$ROOT/dist/release-candidate-$VERSION"
```

The candidate must report `unsigned release candidate` and contain the
platform CLIs, base bundle, manifest and release notes.

### 2. Test-only manifest signature

```sh
set -euo pipefail

go run ./dev/release sign \
  --candidate "$ROOT/dist/release-candidate-$VERSION" \
  --output "$ROOT/dist/signed-release-$VERSION" \
  --authority-registry "$REGISTRY" \
  --issuer "$BETA_ISSUER" --key-id "$BETA_KEY_ID" \
  < "$SEED"

go run ./dev/release verify-signed \
  --directory "$ROOT/dist/signed-release-$VERSION" \
  --authority-registry "$REGISTRY"
```

The second command must pass. If the seed does not match the registry, stop;
do not generate a new key silently and do not weaken verification.

### 3. Current native bridge and correctly seeded bootstrapper

```sh
set -euo pipefail

mkdir -p "$ROOT/dist/native-$VERSION"
REGISTRY_SHA256="$(shasum -a 256 "$REGISTRY" | awk '{print $1}')"
GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath \
  -o "$ROOT/dist/native-$VERSION/maestro-installer" \
  ./cmd/maestro-installer
GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath \
  -ldflags "-X main.Version=$VERSION -X main.AuthorityRegistrySHA256=$REGISTRY_SHA256" \
  -o "$ROOT/dist/native-$VERSION/bcgos-bootstrap_${VERSION}_darwin_arm64" \
  ./cmd/bcgos-bootstrap

BOOTSTRAPPER="$ROOT/dist/native-$VERSION/bcgos-bootstrap_${VERSION}_darwin_arm64"
SEED_STATUS="$($BOOTSTRAPPER seed-status)"
printf '%s\n' "$SEED_STATUS" | grep -F "\"bootstrapper_version\":\"$VERSION\"" >/dev/null
printf '%s\n' "$SEED_STATUS" | grep -F "\"authority_registry_sha256\":\"$REGISTRY_SHA256\"" >/dev/null
```

Do not replace this with a plain `go build ./cmd/bcgos-bootstrap`. The plain
build embeds `0.0.0-dev` and an empty registry digest; the real installer
rejects that bootstrapper against every non-placeholder signed manifest.
The protected `dev/release seeded-binaries` command is an alternative only
when an approved provider configuration is available; its unavailable state
must not be bypassed with guessed publication inputs.

### 4. Icon assets

```sh
set -euo pipefail

go run ./dev/release icons \
  --source "$ROOT/installers/wizard/assets/maestro-app-icon.svg" \
  --output "$ROOT/dist/icons-$VERSION"
ICON_SHA256="$(shasum -a 256 "$ROOT/dist/icons-$VERSION/maestro-app-icon.icns" | awk '{print $1}')"
```

### 5. Real macOS package

```sh
set -euo pipefail

sh "$ROOT/dev/release/build-macos-installer.sh" \
  --version "$VERSION" --arch arm64 \
  --bridge "$ROOT/dist/native-$VERSION/maestro-installer" \
  --wizard-dir "$ROOT/installers/wizard" \
  --release-dir "$ROOT/dist/signed-release-$VERSION" \
  --authority-registry "$REGISTRY" \
  --bootstrapper "$BOOTSTRAPPER" \
  --icon "$ROOT/dist/icons-$VERSION/maestro-app-icon.icns" \
  --icon-sha256 "$ICON_SHA256" \
  --output "$ROOT/dist/Maestro-Installer-${VERSION}-local-beta.dmg"
```

The factory rejects symlinks, mismatched manifest versions, missing detached
signatures, wrong architectures and changed package inputs. It emits an
`unsigned macOS installer candidate`; that label is correct even though the
release manifest itself is verified by the test-only Ed25519 registry.

### 6. Windows portable package (canonical Windows handoff)

The Windows factory must produce **one versioned portable ZIP** containing the
complete package. A bare executable copied beside `dist/` is not a valid
Windows installer: the bridge needs `wizard/`, `release/`, the authority
registry and the versioned bootstrapper next to it. Publishing that bare `.exe`
causes the exact “flash and close” failure when a user double-clicks it.

Run the factory with an explicit archive destination:

```powershell
.\dev\release\build-windows-installer.ps1 `
  -Version $Version `
  -Icon $Icon -IconSHA256 $IconSHA256 `
  -WizardDir $WizardDir `
  -ReleaseDirectory $ReleaseDirectory `
  -AuthorityRegistry $Registry `
  -Bootstrapper $Bootstrapper `
  -OutputDirectory "$Root\dist\maestro-installer-windows-$Version-portable-unsigned" `
  -ArchiveOutput "$Root\dist\Maestro-Installer-$Version-windows-amd64-portable-unsigned.zip" `
  -ResourceCompilerSHA256 $WindresSHA256
```

The archive contains `maestro-installer.exe`, the exact verified inputs and
two explicit launchers:

- `Install-Maestro.cmd` — real verify → install → workspace wizard;
- `Run-Maestro-Rehearsal.cmd` — `--simulate` only, never a product install.

The user-facing handoff is the ZIP. The user extracts it and double-clicks
`Install-Maestro.cmd`; they must not open `bcgos-bootstrap_*.exe` or binaries
inside `release/`. Do not publish a standalone copy named
`Maestro-Installer-<VERSION>-windows-amd64.exe` unless a future factory makes
it genuinely self-contained and records that contract. A Windows DMG is never
valid; DMG is macOS-only.

After generation, verify the archive contains exactly one top-level package
directory, the two launchers and the complete `wizard/` and `release/` trees.
Record the ZIP SHA-256 and label the candidate unsigned/not Authenticode-signed.

## Verification and handoff

1. Confirm the cleanup receipt is complete and keep the backup until the clean
   install has been validated or the owner explicitly releases it.
2. Verify the DMG with `hdiutil imageinfo` and record its SHA-256.
3. Verify the Windows portable ZIP with `unzip -t` (or `Expand-Archive`),
   record its SHA-256 and provide the extraction/launch instructions above.
4. Optionally copy the DMG to `~/Downloads` only after the checksum is
   recorded. Do not copy the seed or private key.
5. Open the DMG and run the installer on the clean user account. Confirm the
   wizard reaches the real install path, creates the new workspace, wires the
   selected runtime and starts the guided onboarding prompt.
6. On a Windows device, extract the ZIP and run `Install-Maestro.cmd`. Confirm
   the wizard remains open, reaches the real install path, creates the new
   workspace and preserves a visible error/log if any preflight fails.
7. Preserve the checksum, version, registry issuer/key ID and observed
   install receipts as local evidence. Do not call the result native-qualified,
   signed, notarized, published or pilot-ready without the corresponding
   attended evidence.

## Failure handling

- If signing fails, stop at the manifest stage and fix the registry/seed
  binding.
- If packaging fails, keep the candidate and signed-release directories for
  diagnosis; never bypass the script with ad-hoc copying.
- If installation fails, preserve the installer journal and restore the
  recoverable backup of the user-owned surfaces. Do not delete evidence before
  the failure is understood.
