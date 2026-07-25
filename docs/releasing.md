# Maestro release lifecycle

This repository is the product factory. Pilot users receive immutable release
artifacts; they do not clone the source repository.

## State model

1. **Source snapshot** - a reviewed commit that passes the full development
   harness.
2. **Unsigned candidate** - deterministic CLI binaries, base bundle, manifest
   and release notes produced by the manual `release candidate` workflow.
3. **Signed release** - candidate bytes signed by approved Maestro release and
   native platform identities. This authority is not configured in the
   repository.
4. **Published release** - the signed set is uploaded through an authenticated
   private provider without renaming or replacing manifest entries.
5. **Pilot-ready release** - clean corporate Windows and macOS install, update
   and rollback evidence exists.

The workflow introduced here stops at state 2. Its GitHub token is read-only,
it cannot create a Release, and its seven-day artifact is explicitly named
`unsigned`.

## Build an unsigned candidate

From a clean source snapshot:

```text
go run ./dev/release candidate \
  --version 0.1.0 \
  --channel canary \
  --output dist/release-candidate
```

The output directory must not already exist. The factory cross-builds:

- `bcgos_<version>_windows_amd64.exe`
- `bcgos_<version>_darwin_amd64`
- `bcgos_<version>_darwin_arm64`
- `maestro-base_<version>.tar.gz`
- `release-manifest.json`
- `release-notes-<version>.md`

The base bundle is assembled only from
`bundles/base/distribution.json`. The allowlist uses exact source and target
paths; repository-wide globs, symlinks, developer files and user/workspace data
are rejected.

Verify an existing directory with:

```text
go run ./dev/release verify --directory dist/release-candidate
```

Verification rejects missing, extra, non-regular or digest-mismatched files.
It proves candidate closure and integrity, not authenticity.

## Authorities still required

- Maestro Ed25519 production release key and custody process.
- Windows Authenticode identity and macOS Developer ID/notarization.
- Authenticated private-release provider registration.
- Immutable publication ledger across release versions and channels.
- Clean corporate-device acceptance and incident/support ownership.

No production path may convert `unavailable` into an unsigned override.
