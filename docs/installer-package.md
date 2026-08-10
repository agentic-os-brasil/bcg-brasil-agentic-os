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

For the controlled Windows Canary, the user distributable is now
`Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip`, produced by
`go run ./dev/release portable-windows`. It contains the exact signed release,
the pinned stable bootstrapper and authority registry, one fixed workspace,
provenance and a one-time `Activate-Maestro.cmd`. It deliberately contains no
wizard or `maestro-installer.exe`. The runtime file
`bcgos_<version>_windows_amd64.exe` remains inside the signed `release/` tree;
the stable bootstrapper verifies and activates it as
`managed/bin/bcgos.exe`, so it must never be sent as a standalone download.

After activation, the owner opens only the archive's `workspace/` folder in
Claude Desktop. The existing adapter projects the managed orientation and
skills into that workspace and binds hooks to the exact absolute installed CLI
path. The extracted directory therefore must not move after activation. Owner
data remains in `%LOCALAPPDATA%\BCGOS`, outside the portable product root.

The self-contained installer factory remains available as an optional future
wrapper over the same signed release contract, but it is not the current
controlled-Canary handoff.

The self-contained wrapper verifies its payload footer and SHA-256 before
extracting. Archive entries must be regular files or directories with safe
relative names; traversal, symlink/hardlink entries, duplicates and size-limit
violations are rejected. It launches the existing bridge with the extracted
conventional package root and removes the temporary directory after the bridge
exits. This wrapper does not replace release-manifest or native-signature
verification. Its output is still `unsigned-candidate` until the approved
Authenticode step is completed.

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

On Windows, the factory requires `Get-AuthenticodeSignature` to report exactly
`NotSigned` for the supplied bootstrapper. On a non-Windows build host where
that cmdlet is unavailable, it parses a well-formed PE optional header and
accepts only a zero offset and zero size in the certificate-table data-directory
entry. A present/malformed certificate table, `HashMismatch`, `NotTrusted`,
`UnknownError` and every other status fail closed. The release manifest and
artifacts still require their normal Ed25519 verification, and the copied
installed bootstrapper is verified again by the bridge against the compiled
profile.

The controlled-Canary output is exactly
`Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip`, accompanied
by its provenance JSON and a `<zip>.sha256` file. The archive and every native
executable remain unsigned: factory pins plus the independently delivered ZIP
checksum establish the bounded package identity, not publisher identity.
SmartScreen, WDAC or AppLocker may block execution before Maestro starts.

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
