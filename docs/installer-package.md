# Maestro installer package contract

The visual wizard is only an interface. A real user-space installer package
must carry the exact trust-bearing inputs that the bridge is authorized to
consume:

- the native `maestro-installer` bridge for the target macOS architecture;
- the exact signed release directory;
- the approved authority registry;
- the independently signed native `bcgos-bootstrap` seed;
- the deterministic wizard tree and approved Maestro `.icns`.

On Windows, the distributable handoff is the complete
`Maestro-Windows-Installer` folder (normally sent as one archive), whose only
user-facing entrypoint is `maestro-installer.exe`. The runtime file
`bcgos_<version>_windows_amd64.exe` may exist inside the signed `release/`
tree, but it is never the installer and must never be sent as a standalone
download. The installer bridge fails closed if `wizard/`, `release/`,
`authority-registry.json` or the single versioned Windows bootstrapper is
missing.

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
