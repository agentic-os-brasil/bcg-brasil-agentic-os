# Installer bridge contract

`cmd/maestro-installer` is the executable bridge behind the Maestro visual
wizard. It is designed for a user who can write to the corporate user profile
but cannot elevate to device administrator.

## First install

### Windows user-level boundary

The Windows installer, `bcgos init`, `bcgos setup apply` and the stable
bootstrapper are user-level operations. They must be launched from the logged-in
user's normal token; the product refuses an elevated `Run as administrator`
process before creating or updating managed state. This keeps `.bcgos` and
`%LOCALAPPDATA%\\BCGOS` owned by the same user that will later run Maestro.

If a prior elevated attempt already created state owned by
`BUILTIN\\Administrators`, the refusal is intentional: Maestro does not silently
take ownership, reset ACLs, delete the workspace or overwrite owner data.
`bcgos doctor` reports this as an actionable ownership repair condition. Use a
bounded support repair or recreate only the Maestro-owned state after a backup;
do not apply `icacls /reset` to the workspace tree.

On Windows, run the supported installer flow by double-clicking the package or
from native PowerShell/cmd. Git Bash/MSYS can rewrite Windows paths and produce
a misleading reparse-point error before the binary receives the intended path;
retry the same flow natively before treating that message as a Maestro storage
failure.

1. The package supplies the exact signed release directory, a native-signed
   seeded `bcgos-bootstrap` and the public authority registry.
2. The bridge verifies the Ed25519-signed manifest and every artifact through
   `releaseverify.VerifyDirectory`.
3. It verifies the native bootstrapper identity (`codesign` on macOS or
   Authenticode through PowerShell on Windows).
4. It calls `bcgos-bootstrap seed-status` and requires its embedded registry
   digest and release version to match the supplied inputs.
5. It creates a new user-level managed root, installs the exact registry and
   bootstrapper, and delegates activation to `bcgos-bootstrap install`.
6. It runs `bcgos version` from the activated path before returning success.

The bridge never replaces a healthy existing installation and never treats a
regular CLI file by itself as installation health. It requires the exact
managed-root binding in `install-state.json`, the expected CLI version and the
managed bootstrapper, authority registry and bundle structure. A healthy
installation of the same release is preserved idempotently; a different
healthy release must use the signed update flow.

An interrupted installer-owned root or a valid state bound to an unhealthy or
missing root is moved to a plan-digest-bound recovery location before a clean
retry. The install state moves first, so a crash cannot leave an authoritative
state pointing at a missing managed root. Recovery never overwrites an earlier
quarantine, and an invalid state, symlinked path or unrecognized top-level file
fails closed without changing either root. The bridge never mutates the global
`PATH`, never accepts an unsigned override and never deletes owner data.

The only exception to strict native trust is the factory-compiled
`windows-local-beta` profile defined by decision `CARY`. It is not a
command-line option. The compiled bridge pins the exact beta issuer/key ID,
registry SHA-256 and bootstrapper SHA-256, then accepts `NotSigned` only for an
Ed25519-authenticated `canary` manifest with those same bindings. A normal
build continues to require Authenticode `Valid`. A partial profile, a different
channel or authority, any digest drift, or any invalid Authenticode status
fails closed before installation. The same native check is repeated after the
bootstrapper is copied into the managed root.

The bootstrapper's activation success is not accepted on process exit alone.
The bridge repeats the CLI version diagnostic and then requires a coherent
durable state plus the expected managed structure. A failure after state commit
is quarantined, not recursively deleted, and is explicitly reported as safe to
reinstall rather than ready. Owner data remains a separate root.

## Visual mode and tests

Visual mode serves the dependency-free wizard on loopback and exposes only
typed `/api/state`, `POST /api/verify`, `POST /api/install`,
`POST /api/open-data` and `POST /api/close` endpoints. Mutating calls require the unguessable
per-session header and an exact `plan_digest` returned by `/api/verify`; this
prevents a random local page from triggering installation or racing a changed
release. The verify endpoint runs the complete read-only plan
(`installer.Prepare`) so the wizard can show a real green check before the user
confirms installation; it does not create directories or copy files. When the package contains
the conventional `release/`, `wizard/`, `authority-registry.json` and one
versioned native bootstrapper beside the executable, the user can launch it
without flags. `--headless` is available for clean-device automation and does
not skip any verification. No telemetry, runtime hook or model request is
started by the installer.

The connected wizard creates the canonical local workspace only after core
installation. Its response says `workspace_created`, reports the exact path
and a workspace-bound `doctor` command, and keeps `ready_for_runtime=false`
while the adapter is only configured/unverified and readiness or scheduler
checks have not run. Later installer layers may promote those individual
fields only from their own observed checks. A preview never claims a workspace
or runtime is ready.

For a local unsigned visual test, `--preview` intentionally skips all release
inputs and serves only the static wizard. It cannot verify or install anything;
the footer remains in presentation mode. For an end-to-end local technical
rehearsal, use `--simulate`: it creates an isolated sandbox, returns a
simulation plan, exercises the confirmation-bound install transaction and
opens the resulting data directory. Simulation labels itself as rehearsal and
never presents its files as signed release bytes. Neither mode is evidence of
a signed release or pilot readiness.

The close action is also session-bound: the wizard posts to `/api/close`, the
bridge returns the closing acknowledgement and then shuts down its loopback
server. This avoids depending on the browser's `window.close()` policy and
does not expose a public unauthenticated shutdown route.

The actual signed package must still be assembled by the release workflow and
must carry native signing/notarization evidence before it can be called
pilot-ready.

### Cross-platform rehearsal commands

The same bridge contract can be exercised without administrator permission on
both pilot targets:

```powershell
.\maestro-installer.exe --simulate --wizard-dir .\wizard --simulation-root $env:TEMP\maestro-rehearsal
```

```bash
./maestro-installer --simulate --wizard-dir ./wizard --simulation-root "$(mktemp -d)"
```

The Windows and macOS binaries are built for the release matrix (`windows/
amd64`, `darwin/amd64`, `darwin/arm64`). The rehearsal writes only beneath the
explicit sandbox root; it never requests elevation or changes the global
`PATH`.

On macOS, the executable
[`maestro-rehearsal-dmg.sh`](../dev/release/maestro-rehearsal-dmg.sh) creates a
local-only DMG containing `Maestro Installer Rehearsal.app`. Double-clicking
that app starts the same `--simulate` wizard flow; the DMG README repeats the
three user actions and the no-administrator boundary. This artifact is still
unsigned and is not a pilot release.

The Windows visual installer candidate uses the separate
[`build-windows-installer.ps1`](../dev/release/build-windows-installer.ps1)
factory step. It embeds the hash-verified `.ico` as a PE resource through the
approved `windres` tool and writes unsigned provenance; Authenticode remains a
later protected-environment step.

The visual single-file installer remains available as an optional wrapper via
[`build-windows-singlefile-installer.ps1`](../dev/release/build-windows-singlefile-installer.ps1).
It first creates the same validated complete package, builds a thin wrapper
with the Maestro icon, appends a deterministic digest-bound payload and emits
one `Maestro-Installer-<version>-windows-amd64.exe`. Double-clicking that file
extracts only to a private temporary directory, launches the existing bridge,
propagates its exit status and cleans the temporary package. The wrapper is
not a second installer implementation and does not bypass release or
Authenticode verification.

For the current controlled Canary handoff, use `go run ./dev/release
portable-windows` with the exact signed release, registry/bootstrapper SHA-256
pins and versioned seeded Windows bootstrapper. The mandatory output basename
is `Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip`. It emits
provenance and an adjacent SHA-256 file, contains no wizard or installer bridge,
and delegates activation to the stable bootstrapper before applying the normal
one-and-done Claude setup. This package remains engineering/local-beta evidence,
not a signed release or a completed pilot gate.

## External CI unblock

The validation workflow is enabled and the PR jobs are configured for hosted
`ubuntu-latest`, `windows-latest` and `macos-latest` runners. If all three
checks fail with zero steps, inspect the check-run annotation before changing
code. The observed blocker is an account payment failure or spending limit;
an administrator must fix **Settings → Billing & plans** for the GitHub
account/organization and then rerun the failed workflow. Success means each
job has started and reports its actual harness steps; a green local rehearsal
cannot substitute for that remote evidence.
