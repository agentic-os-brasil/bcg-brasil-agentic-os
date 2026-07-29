# Installer bridge contract

`cmd/maestro-installer` is the executable bridge behind the Maestro visual
wizard. It is designed for a user who can write to the corporate user profile
but cannot elevate to device administrator.

## First install

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

The bridge refuses existing non-empty managed roots, never mutates the global
`PATH`, never accepts an unsigned override and removes only a newly-created
managed root if first activation fails. Owner data remains a separate root.

## Visual mode and tests

Visual mode serves the dependency-free wizard on loopback and exposes only
typed `/api/state`, `POST /api/verify`, `POST /api/install` and
`POST /api/open-data` endpoints. Mutating calls require the unguessable
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

The final action opens the installed user-data directory, not a made-up
workspace. A workspace is chosen and initialized by the person after install;
the bridge never silently creates one or claims that a preview has one.

For a local unsigned visual test, `--preview` intentionally skips all release
inputs and serves only the static wizard. It cannot verify or install anything;
the footer remains in presentation mode. For an end-to-end local technical
rehearsal, use `--simulate`: it creates an isolated sandbox, returns a
simulation plan, exercises the confirmation-bound install transaction and
opens the resulting data directory. Simulation labels itself as rehearsal and
never presents its files as signed release bytes. Neither mode is evidence of
a signed release or pilot readiness.

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

## External CI unblock

The validation workflow is enabled and the PR jobs are configured for hosted
`ubuntu-latest`, `windows-latest` and `macos-latest` runners. If all three
checks fail with zero steps, inspect the check-run annotation before changing
code. The observed blocker is an account payment failure or spending limit;
an administrator must fix **Settings → Billing & plans** for the GitHub
account/organization and then rerun the failed workflow. Success means each
job has started and reports its actual harness steps; a green local rehearsal
cannot substitute for that remote evidence.
