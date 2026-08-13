---
type: User Playbook
title: Install, update and rollback
description: The user-facing installation and reversible update contract.
resource: repo://docs/install-update.md
tags:
    - install
    - update
    - rollback
sources:
    - id: install-update
      resource: repo://docs/install-update.md
      title: Install, update and rollback
status: stable
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/install-update
x-bcgos-scope: managed
x-bcgos-source-fingerprint: eaaee7072b9aa1d0faf0279c3b9ae26e9200e7cd0bb6d2193e55b9995e8ccd5f
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: maestro-managed-wiki/0.2
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `docs/install-update.md`. The source remains authoritative.

## Related

- [Maestro release and distribution](/concepts/release-distribution.md)

## Source content

# Verified installation and update

Maestro is distributed as a signed ZIP bundle. The release manifest and every
artifact are signed with the approved Maestro Ed25519 identity. Installation is
manual for the current distribution model.

## Distribution model

```text
maestro-base_<version>.zip       # verified base bundle
release-manifest.json            # signed manifest with per-artifact SHA-256
release-manifest.json.sig        # detached Ed25519 signature
```

Users receive the signed release ZIP and extract it to a local workspace folder.
There is no installer binary. No CLI binary is installed or activated. All
Maestro capabilities are delivered through the bundle and accessed through
Claude Code slash-commands.

## Managed and owner-data roots

The roots must be absolute, canonical and non-overlapping.

```text
workspace-root/          # extracted ZIP contents
  CLAUDE.md              # Claude Code configuration
  bundles/               # capability catalog
  skills/                # slash-command definitions
  owner/                 # owner-private data (created at first run)

owner-data-root/
  config/                # owner profile and workspace state
  workspaces/            # never read or written by activation
```

## Signed release boundary

`releaseverify.VerifyDirectory` requires:

- the exact `release-manifest.json` and raw 64-byte detached signature;
- a currently active key found in the approved local Maestro
  product/issuer/key registry;
- matching size, SHA-256 and Ed25519 signature for every artifact;
- matching release-notes digest;
- no missing, extra, directory or symlink entry.

The local readiness report (`go run ./dev/release readiness`) validates the
managed schemas, provider configuration and candidate closure without contacting
any provider, signing, or installing anything.

## Update path

For the current release, update is performed manually:

1. Download the new signed release ZIP from the private release channel.
2. Verify the manifest signature against the approved authority registry.
3. Extract the ZIP to the workspace, replacing the previous bundle contents.
4. Open the workspace in Claude Code. Run `/maestro-doctor` to confirm state.

The automated update path (bootstrapper with authenticated provider, activation
transaction and rollback) is an architecture-complete contract that remains
unavailable until production signing authorities, a signed bootstrapper seed
channel and approved corporate-device acceptance are provisioned. See
`docs/releasing.md` for gate status.

## Still unavailable

- approved operating-system installation directories;
- production signing keys and native code-signing identities;
- approved production provider registration and native-store use;
- a signed bootstrapper seed channel;
- automated activation, rollback and recovery via the bootstrapper;
- approved corporate-device acceptance of the operator evidence.
