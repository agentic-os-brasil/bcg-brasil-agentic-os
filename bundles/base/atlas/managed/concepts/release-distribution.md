---
type: Release Playbook
title: Maestro release and distribution
description: The signed release and publication flow for Maestro artifacts.
resource: repo://docs/releasing.md
tags:
    - release
    - distribution
    - trust
sources:
    - id: release-distribution
      resource: repo://docs/releasing.md
      title: Maestro release and distribution
status: stable
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/release-distribution
x-bcgos-scope: managed
x-bcgos-source-fingerprint: 5b3b09f72f7872c9e9b058aad080cc89018c0fa48a7317823edb74ade66d9314
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: bcgos-managed-wiki/0.1
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `docs/releasing.md`. The source remains authoritative.

## Related

- [Wiki update lifecycle and OKF profile](/concepts/wiki-okf.md)
- [Install, update and rollback](/concepts/install-update.md)

## Source content

# Maestro release lifecycle

This repository is the product factory. Pilot users receive immutable release
artifacts; they do not clone the source repository.

## State model

1. **Source snapshot** - a reviewed commit that passes the full development
   harness.
2. **Unsigned candidate** - deterministic CLI binaries, base bundle, manifest
   and release notes produced by the manual `release candidate` workflow.
3. **Signed release** - candidate bytes signed by approved Maestro release and
   native platform identities.
4. **Published release** - the signed set is uploaded through an authenticated
   private provider without renaming or replacing manifest entries.
5. **Pilot-ready release** - clean corporate Windows and macOS install, update
   and rollback evidence exists.

The read-only `release candidate` workflow stops at state 2. The separately
environment-protected `signed Maestro prerelease` workflow can reach state 4,
but only after an operator enters the exact publication confirmation and the
release environment supplies all approved public configuration, signing
identities, a read-only release-policy token and secret custody inputs. The
repository does not contain those authorities, so the workflow fails closed
until they are configured.

## Technical beta boundary

The beta can proceed as a local or controlled technical rehearsal without a
paid personal signing account. Its artifacts must remain labeled
`unsigned-candidate` or `technical rehearsal` and are engineering evidence only:
they do not establish authenticity, publication, corporate-device trust or
pilot readiness. A personal Apple Developer membership or Windows signing
identity is prohibited, including for beta; technical beta remains unsigned. A
production Ed25519 key must not be used as a bridge to the corporate release
path.

When the organization is ready to distribute, the release owner must provision
organization-owned Apple Developer ID signing and notarization, Windows
Authenticode, and a new organization-controlled Ed25519 production key with
approved custody and registry seeding. A beta Ed25519 key, if needed for
isolated testing, lives in a separate test registry and remains test-only. The
production authority registry and workflow must exclude its issuer/key ID (or
mark it revoked) and reject it for installer/update trust and all new production
artifacts. Its public key may be retained only for read-only archival verification
of beta artifacts, never as production trust during transition.

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

## Role migration at the 0.2.0 release boundary

Release `0.2.0` opens the migration boundary from the retired `practice_agent`
authority to canonical `pa_expert`. Its signed manifest, and later releases
that can still receive a pre-boundary installation, must carry the required
`practice-agent-to-pa-expert` migration with the exact
bundle artifact digest plus the SHA-256 identities of the shipped agent
catalog and agent-skill policy. The source range is `>=0.1.0 <0.2.0`; after
`0.2.0`, the legacy role and IDs are rejected and are never silently renamed.

The update plan preserves those catalog/policy bindings. Activation records
them in install state, and both normal planning and direct prepared activation
enforce the same bounded source range. A retry is idempotent and a rollback
after the boundary fails closed if it would reactivate an unbound legacy
authority, even when a later canonical release no longer carries the original
migration marker.
Once no pre-boundary installation is in scope, a post-migration release may
omit the migration record; it still cannot reintroduce the legacy authority.

## Assemble the macOS installer candidate

The visual installer is packaged separately from the CLI candidate because it
must carry the exact release directory, native bootstrapper and authority
registry that the bridge will verify. Use
`dev/release/build-macos-installer.sh` with explicit absolute paths to those
inputs, the target-architecture bridge and the hash-approved `.icns`. The
factory emits a DMG containing `Maestro Installer.app`, records tree/file
digests and labels the result `unsigned-candidate`. It never creates keys,
signs, notarizes or publishes the package. The protected release workflow must
repeat the same packaging with approved inputs before the package can be
called a signed release.

## Produce a local readiness report

Before requesting an external release run, use the read-only readiness report
with explicit paths:

```text
go run ./dev/release readiness \
  --provider-config bundles/base/release/provider.json \
  --authority-registry path/to/release-authority-registry.json \
  --authority-registry-sha256 <exact-lowercase-sha256> \
  --candidate dist/release-candidate
```

The report is deterministic JSON with stable check IDs. It validates the
managed schemas, provider configuration, pinned authority registry, candidate
closure and dispatchable release workflows. Missing inputs are
`unavailable`; malformed or tampered inputs are `blocked`. It never contacts a
provider, opens a credential store, signs or notarizes bytes, installs an
update, or promotes a release. Its claim is only
`local_contract_evidence`; signature, provider authentication and clean-device
acceptance remain explicit external gates.

## Build and publish a signed prerelease

The protected workflow:

1. requires the protected default branch and validates the complete source
   harness;
2. materializes the approved public provider and release-authority inputs;
   the provider owner/repository must exactly match the publication repository;
3. builds seeded native CLI and bootstrapper binaries on matching Windows and
   macOS runners;
4. applies and verifies Authenticode or Developer ID signatures;
5. assembles the final candidate from those exact CLI bytes;
6. signs each release artifact and the exact final manifest with the approved
   Maestro Ed25519 identity;
7. requires repository-level immutable releases and rejects an existing tag;
8. creates the GitHub prerelease, verifies its GitHub attestation, exact commit
   and asset closure, then preserves bootstrapper seed provenance as a separate
   workflow artifact.

The release assets exclude private keys, certificates and provider
credentials. Signed release notes explicitly say that corporate-device
acceptance and pilot readiness remain separate gates.

The 14-day bootstrapper workflow artifact also preserves the exact public
provider and authority-registry bytes whose identities were compiled into the
native binaries. It is short-lived custody evidence for the installer work; it
is not the independently signed bootstrapper seed channel and is not pilot
distribution.

After the approved installer channel turns those inputs into a platform-signed
seed package, `acceptance/clean-device/` verifies one real Windows or macOS
device through first install, signed update and rollback. Its three sanitized
receipts are digest-bound into a schema-v2 operator attestation. The repository
does not authenticate the operator or device, so an approved external
countersignature is still required for corporate acceptance. These files are
not release assets and cannot promote the release automatically.

## Authorities still required

- Maestro Ed25519 production release key and custody process.
- Approval of the environment-secret custody model for the Ed25519 seed,
  Authenticode certificate and Developer ID certificate, or replacement with
  an approved hardware-backed signing service.
- An approved `release-authority-registry` instance containing only the
  production public keys, validity windows and revocation state.
- Windows Authenticode identity and macOS Developer ID/notarization.
- Authenticated private-release provider registration.
- Immutable publication ledger across release versions and channels.
- Approved corporate-device countersignature/acceptance and incident/support
  ownership.

The repository validates the registry contract and test fixtures. It does not
contain a production registry or a private signing key. The independently
signed bootstrapper seed must install the approved registry before a provider
release can become trusted.

No production path may convert `unavailable` into an unsigned override.

## Release decision checklist

Use [`docs/release-gates-checklist.md`](release-gates-checklist.md) as the
evidence record. It separates a technical rehearsal, a signed release and a
pilot-ready release, and names the external owner for every gate. A candidate
that passes local closure verification remains unsigned engineering output.
