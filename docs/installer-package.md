# Maestro installer package contract

The visual wizard is only an interface. A real user-space installer package
must carry the exact trust-bearing inputs that the bridge is authorized to
consume:

- the native `maestro-installer` bridge for the target operating-system
  architecture;
- the exact signed release directory;
- the approved authority registry;
- the independently signed native `bcgos-bootstrap` seed;
- the deterministic wizard tree and approved Maestro platform icon (`.icns` or
  `.ico`).

For the controlled Canary, the user distributables are target-specific:
`Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip` and
`Maestro-Portable-<version>-macos-arm64-local-beta-unsigned.zip`, produced by
`go run ./dev/release portable-windows` and `portable-macos`. Each contains the
exact signed release, the pinned native bootstrapper and authority registry,
one fixed workspace, provenance and a seed `maestro-os/CLAUDE.md`. It
deliberately contains no user-facing activation command, wizard or
`maestro-installer.exe`. The runtime file
`bcgos_<version>_windows_amd64.exe` remains inside the signed `release/` tree;
the stable bootstrapper verifies and activates it as
`managed/bin/bcgos.exe`, so it must never be sent as a standalone download.

The owner extracts the package, opens only its `maestro-os/` folder in Claude
Code and sends a natural-language message such as “Quero começar”. The seed
`CLAUDE.md` makes Claude explain the preparation, request the one-and-done
confirmation and execute the internal activator itself. The owner does not use
a terminal or run a command; native Claude Code or Windows permission prompts
can still require a click. The existing adapter then preserves the seed text,
projects the managed orientation and skills into that workspace and binds hooks
to the exact absolute installed CLI path. The extracted directory therefore
must not move after activation. Owner
data remains in `%LOCALAPPDATA%\BCGOS`, outside the portable product root.

### Script-only portable beta — no Go on the endpoint

Decisions `SHLL`, `SSPF` and Spec 053 define two distinct reduced-capability
ZIPs:

- `Maestro-Portable-<version>-macos-shell-local-beta.zip`;
- `Maestro-Portable-<version>-windows-powershell-local-beta.zip`.

They contain no `bcgos`, bootstrapper, Go source/toolchain, Mach-O, PE, ELF,
object or bytecode payload. macOS uses `install.sh`; Windows uses
`Install-Maestro.ps1`. Optional double-click delegators ask for confirmation
and invoke the same scripts without an execution-policy bypass, but they are
not the primary or universally supported enterprise path.

The normal quick journey is extract, double-click the target `Start Maestro`
launcher, confirm once, then open the permanent `maestro-os` revealed in Finder
or Explorer so a fresh Claude session loads hooks and agents. The launcher
never opens Claude automatically and reveal failure does not fail an otherwise
committed installation. If double-click is blocked, the fallback remains open
the seeded `maestro-os` in Claude Code, speak and confirm once. Installation stages versioned
managed content in the conventional user application root, creates the stable
workspace under the user's home rather than Downloads, projects Maestro-owned
compatible skills into it and retains one previous version for rollback. The
ZIP is disposable after handoff. An internal SHA-256
inventory is checked before product mutation and unexpected files are rejected.
Runtime version staging is transactional. Workspace projection is journaled
and recoverable but not physically atomic: rerunning install/update/rollback
restores the previous known Maestro-owned projection before retrying, while
unknown live bytes remain a preserved conflict. macOS failure injection covers
this recovery; native Windows and real power-loss evidence remain acceptance
gates. `doctor` verifies the runtime, projection completion
receipt, active-version agreement, seven hook bindings and handler identity,
managed skills and agents, capability matrix and managed `CLAUDE.md` block.
That is configured-on-disk evidence, not proof that Claude invoked every hook.
The bounded projection receipt also binds the exact managed block digest, so a
future script-only update can distinguish a known prior Maestro block from an
owner edit while preserving all content outside the markers.
On Windows, an existing workspace `CLAUDE.md` must be UTF-8 without BOM; an
unsupported encoding stops before workspace mutation and leaves the file
byte-for-byte unchanged.

This profile retains all seven Claude lifecycle bindings through readable
shell/PowerShell handlers, the five canonical specialists as operational Claude
project agents, compatible managed skills, orientation, a script-specific
onboarding, atlas, policies, content update/rollback and `continuity-lite-v1`.
The continuity profile keeps reviewed task/checkpoint bodies in Markdown and
injects only a bounded logical pointer, state and checkpoint-presence flag on
`SessionStart`; it is not the authenticated native execution ledger. The
separate `session-profile-lite-v1` preserves explicitly reviewed working
style through a validated local pointer and injects only
`standard|advanced|power`, revision and relative pointer—not the profile body.
Its separate consent explains that Claude may later open only a relevant
section of the reviewed Markdown; revocation deletes the pointer state while
preserving the Markdown unless the owner requests deletion separately.
It is not native SELF, memory or an authenticated Session Context packet.
`agent-route-lite-v1` adds bounded metadata-only sequence assurance for the
five specialists, including the strategic Client Account–Case–Client Account
round trip. It remains best-effort hook enforcement without signed packets or
authenticated native receipts.
Skills whose state machine requires the native CLI are listed as unavailable
rather than installed in a broken form. It intentionally reports the native CLI,
signed-provider verification, secure credentials, authenticated native hook
receipts, external-mutation challenges, native signed specialist-route
authority, schedulers, background maintenance, native ingestion and CLI
ledgers as unavailable. This beta installs only the Claude runtime projection;
the canonical Codex adapter remains unavailable in this profile. The inventory is integrity evidence, not publisher authentication;
deliver the ZIP SHA-256 independently. Shell/PowerShell/AppLocker/EDR policy can
still block the scripts, and the package never attempts to weaken those controls.
The recipient can read the shell/PowerShell and managed skills/content. This
route keeps the Go implementation and repository private, but is not a promise
of zero readable product logic.

The self-contained installer factory is the single-file option over the same
validated package contract. Run `go run ./dev/release self-contained` with the
complete conventional package directory as `--source` and its
`maestro-installer.exe` as `--base`; do not pass the signed release directory
or the ZIP directly. Both the wrapper and the embedded bridge must be compiled
as Windows GUI executables (`-H=windowsgui`); the factory rejects a console
subsystem base instead of emitting an EXE that only flashes a terminal. The
output appends a bounded, digest-bound payload and remains an
`unsigned-candidate` until Authenticode is applied.

The self-contained wrapper verifies its payload footer and SHA-256 before
extracting. Archive entries must be regular files or directories with safe
relative names; traversal, symlink/hardlink entries, duplicates and size-limit
violations are rejected. It launches the existing bridge with the extracted
conventional package root and removes the temporary directory after the bridge
exits. This wrapper does not replace release-manifest or native-signature
verification. Its output is still `unsigned-candidate` until the approved
Authenticode step is completed.

For an explicitly owner-directed `canary-simple` test, the platform's native
signature status is diagnostic and does not block the installation. Empty or
malformed status output is still rejected. On macOS, the profile also allows
the workspace handoff to proceed while hook evidence is still
`native_qualification_pending`; the installer reports `native_qualified: false`
instead of promoting that evidence. The `strict` profile continues to require
native verification and native qualification, and the pinned
`windows-local-beta` profile continues to require its exact `NotSigned`
exception and package digests.

### Controlled Windows local beta

Decision `CARY` permits one narrower pre-pilot package profile while the
organization-owned Authenticode identity is unavailable. The profile is
selected only by the Windows factory with `-LocalBeta`; there is no installer
runtime flag that can enable it. The factory requires and compiles into the
inner bridge all four public bindings:

- the exact test-only release issuer and key ID;
- the SHA-256 of the separate beta authority registry;
- the SHA-256 of the versioned Windows bootstrapper;
- the authenticated release channel `canary`.

The portable factories may run on macOS, Linux or Windows to assemble their
respective artifacts, but they never emit a universal executable package. The
owner opens only `maestro-os/` in Claude Code. The seeded `CLAUDE.md` checks
that the current host matches the archive, asks for one confirmation and
invokes only its matching internal bootstrapper. On Windows, the factory requires
`Get-AuthenticodeSignature` to report exactly `NotSigned` for the supplied
bootstrapper. A present/malformed certificate table, `HashMismatch`,
`NotTrusted`, `UnknownError` and every other status fail closed. The release
manifest and artifacts still require their normal Ed25519 verification, and
the copied installed bootstrapper is verified again by the bridge against the
compiled profile.

The controlled-Canary output is exactly
`Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip`, accompanied
by its provenance JSON and a `<zip>.sha256` file. The archive and every native
executable remain unsigned: factory pins plus the independently delivered ZIP
checksum establish the bounded package identity, not publisher identity.
SmartScreen, WDAC or AppLocker may block execution before Maestro starts.

The macOS ZIP is a controlled local-beta handoff, not a replacement for a
production macOS installer. It uses `~/Library/Application Support/BCGOS` and
fails on a non-arm64 host. A production macOS release still requires Developer
ID signing, notarization and clean-device acceptance; neither Claude nor the
package removes Gatekeeper.

The signed base bundle carried in `release/` includes the
`maestro-setup-update` skill. It is installed as part of the bundle and is not
exported as a loose skill file.

`dev/release/build-macos-installer.sh` packages those inputs into
`Maestro Installer.app` and an unsigned DMG. It rejects missing or symlinked
inputs, checks the requested architecture with `lipo`, validates the release
manifest identity against the requested version, re-hashes every staged
bridge/registry/bootstrapper/icon input, records file/tree digests and passes
explicit paths to the bridge. The bridge therefore keeps its normal per-user
managed and owner-data roots without requiring administrator permission or
changing the global `PATH`.

The output is deliberately marked `unsigned-candidate`. Its provenance record
also includes the release manifest and detached-signature digests. The script
does not
create keys, sign bytes, notarize the app, contact a provider or weaken the
bridge's release verification. Developer ID, notarization, Ed25519 authority,
provider publication and clean-device acceptance remain protected release
gates.

The separate `dev/release/maestro-rehearsal-dmg.sh` remains the safe local
technical rehearsal: it launches `--simulate` and contains no signed release
inputs. Do not use the rehearsal artifact as a production installer.

Personal Apple or Windows signing identities are prohibited, including for
beta; technical beta remains unsigned. A beta Ed25519 key, if needed, belongs in
a separate test registry and may verify only isolated beta artifacts. The
production registry and workflow must reject its issuer/key ID; a retained beta
public key is archival evidence, never installer/update trust. Production
installers require the organization-owned authorities and custody described in
[`docs/releasing.md`](releasing.md).
