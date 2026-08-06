---
name: maestro-local-beta-installer
description: Build and verify a real local-beta Maestro installer from a merged source snapshot, using a test-only Ed25519 authority and the native macOS packaging path instead of the rehearsal simulator.
---

# Maestro Local-Beta Installer

Use this skill when the owner asks to produce the next local-beta installer
that must execute the real verify -> install -> workspace flow. This is a
packaging direction, not a production-release authorization and not a
substitute for Developer ID signing or notarization.

Resolve the canonical `interaction-profile` before presenting the procedure.
It controls how much implementation detail to show, but never weakens the
release, signing, consent or cleanup gates below.

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
2. Choose one immutable `MAJOR.MINOR.PATCH` version. The explicit CLI version,
   `VERSION`, candidate manifest and output filename must agree before an
   official release is claimed. A repository placeholder such as `0.0.0` may
   be used only for a controlled local test with the chosen version passed
   explicitly; record the mismatch and fix it before publication.
3. Use a separate test-only authority registry and matching Ed25519 seed. The
   registry issuer/key ID must be marked beta/test-only and must never be used
   by a production workflow. Read the public registry; do not print or commit
   the seed.
4. Use the current source for the bridge and bootstrapper. Do not reuse old
   native binaries from a previous installer.

## Clean-install preparation

Remove only Maestro-owned active surfaces, preserving a recoverable backup:

- the intended user workspace, for example `~/Developer/maestro-os`;
- `~/Library/Application Support/Maestro`;
- legacy `~/Library/Application Support/BCGOS` only when the owner explicitly
  wants a completely clean migration test;
- the Maestro LaunchAgent, for example
  `~/Library/LaunchAgents/com.bcg.maestro.maintenance.plist`;
- prior build outputs under the isolated release worktree.

Unload the LaunchAgent before moving its plist. Do not touch the source
repository, the Canary evidence workspace, unrelated Kowalski maintenance or
client/project data. Prefer moving targets to a timestamped temporary backup
over irreversible deletion.

## Build sequence

Set `ROOT` to the current merged worktree, `VERSION` to the chosen release,
`REGISTRY` to the test-only public registry and `SEED` to the protected local
seed. Keep all paths absolute when invoking the packaging script.

### 1. Candidate

```sh
go run ./dev/release candidate \
  --version "$VERSION" --channel canary \
  --output "$ROOT/dist/release-candidate-$VERSION"
```

The candidate must report `unsigned release candidate` and contain the
platform CLIs, base bundle, manifest and release notes.

### 2. Test-only manifest signature

```sh
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

### 3. Current native bridge and bootstrapper

```sh
mkdir -p "$ROOT/dist/native-$VERSION"
GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath \
  -o "$ROOT/dist/native-$VERSION/maestro-installer" \
  ./cmd/maestro-installer
GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath \
  -o "$ROOT/dist/native-$VERSION/bcgos-bootstrap_${VERSION}_darwin_arm64" \
  ./cmd/bcgos-bootstrap
```

### 4. Icon assets

```sh
go run ./dev/release icons \
  --source "$ROOT/installers/wizard/assets/maestro-app-icon.svg" \
  --output "$ROOT/dist/icons-$VERSION"
ICON_SHA256="$(shasum -a 256 "$ROOT/dist/icons-$VERSION/maestro-app-icon.icns" | awk '{print $1}')"
```

### 5. Real macOS package

```sh
sh "$ROOT/dev/release/build-macos-installer.sh" \
  --version "$VERSION" --arch arm64 \
  --bridge "$ROOT/dist/native-$VERSION/maestro-installer" \
  --wizard-dir "$ROOT/installers/wizard" \
  --release-dir "$ROOT/dist/signed-release-$VERSION" \
  --authority-registry "$REGISTRY" \
  --bootstrapper "$ROOT/dist/native-$VERSION/bcgos-bootstrap_${VERSION}_darwin_arm64" \
  --icon "$ROOT/dist/icons-$VERSION/maestro-app-icon.icns" \
  --icon-sha256 "$ICON_SHA256" \
  --output "$ROOT/dist/Maestro-Installer-${VERSION}-local-beta.dmg"
```

The factory rejects symlinks, mismatched manifest versions, missing detached
signatures, wrong architectures and changed package inputs. It emits an
`unsigned macOS installer candidate`; that label is correct even though the
release manifest itself is verified by the test-only Ed25519 registry.

## Verification and handoff

1. Verify the DMG with `hdiutil imageinfo` and record its SHA-256.
2. Optionally copy the DMG to `~/Downloads` only after the checksum is
   recorded. Do not copy the seed or private key.
3. Open the DMG and run the installer on the clean user account. Confirm the
   wizard reaches the real install path, creates the new workspace, wires the
   selected runtime and starts the guided onboarding prompt.
4. Preserve the checksum, version, registry issuer/key ID and observed
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
