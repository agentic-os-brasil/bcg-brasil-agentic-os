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
x-bcgos-source-fingerprint: 79e719006e45e5213ba879c48f152c9294efe45c3895c78ffcf7673dc305591c
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: bcgos-managed-wiki/0.1
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `docs/install-update.md`. The source remains authoritative.

## Related

- [Maestro release and distribution](/concepts/release-distribution.md)

## Source content

# Verified installation and rollback

Maestro separates three authorities:

- the release verifier authenticates an exact manifest and every listed file;
- the updating CLI may prepare a transaction, but it cannot replace itself;
- the stable `bcgos-bootstrap` process waits for the launching CLI to exit,
  activates the transaction and restores last-known-good state on failure.

## Managed and owner-data roots

The roots must be absolute, canonical and non-overlapping.

```text
managed-root/
  bin/bcgos[.exe]
  bundles/<bundle-version>/
  recovery/cli/<previous-version>-<transaction>/
  bcgos-bootstrap[.exe]       # seeded separately; not self-updated in v1

owner-data-root/
  config/install-state.json
  updates/tx-*/activation-plan.json
  workspaces/                 # never read or written by activation
```

For first installation, an approved operating-system installer places the
native-signed stable bootstrapper and the exact public authority registry under
the protected managed root. The bootstrapper takes only the already downloaded
signed release directory and owner-data root, derives the managed root from its
own path, verifies the registry seed and complete release, creates the exact
activation plan itself and activates it. No caller-supplied first-install plan
or managed root is accepted.

The exact signed CLI and compressed base bundle are copied into the
transaction with their authenticated sizes and SHA-256 digests. The stable
bootstrapper re-verifies the signed release and the complete activation
contract. Activation then hashes the exact CLI bytes it copies and hashes the
exact archive stream it extracts, with traversal, links, special file types,
duplicate paths, file-count and expanded-size limits.

## Signed local-release boundary

`releaseverify.VerifyDirectory` requires:

- the exact `release-manifest.json` and raw 64-byte detached signature;
- a currently active key found in the approved local Maestro
  product/issuer/key registry;
- matching size, SHA-256 and Ed25519 signature for every artifact;
- matching release-notes digest;
- no missing, extra, directory or symlink entry.

Authenticated provider discovery and download terminate at this same local
release boundary and cannot bypass the verifier. `bcgos update --check` emits
one exact, schema-versioned plan and persists its signed payload. It remains
unavailable unless the build contains an approved provider registration,
authority-registry digest and usable native credential store.

## Activation and recovery

For updates, the bootstrapper takes only the exact durable confirmation plan ID
and owner-data root. After the launching CLI exits, it derives the managed root
from its own protected installed path, loads
`managed-root/trust/release-authority-registry.json`, reloads the matching
pending envelope and repeats confirmation. The registry bytes must match the
SHA-256 seed embedded in that exact bootstrapper build; copying the executable
beside a different registry cannot establish a new authority. Provider
directories, activation paths, target and trust registry are not
caller-selected flags. Under the activation lock it rejects any
installed-state change since confirmation,
revalidates all signed release, plan and staged-byte bindings, preserves the
current CLI under `managed-root/recovery`, installs the immutable bundle,
activates the new CLI and runs `bcgos version`.

If file activation, self-check or durable state commit fails, the new CLI and
bundle are removed and the previous CLI is restored. Explicit rollback uses
the same lock and self-check. Local configuration, workspaces and user memory
are outside every activation path.

`bcgos update --confirm <plan-id>` accepts only the exact pending plan, derives
the managed root from its own protected `bin` path and launches the fixed
independently seeded bootstrapper with its own PID. Bootstrapper output goes to
a new owner-data log rather than sharing the CLI JSON stream. The bootstrapper
waits for that process before touching the active executable, so neither
Windows nor macOS asks `bcgos` to replace itself.

The bootstrapper seed also establishes the initial release-authority registry
at the fixed protected path above.
The bootstrapper build defaults to no registry seed and therefore remains
unavailable until release authority approves and injects the exact digest.

For updates, activation writes a strict intent receipt before moving managed
payloads. If the process stops at any later boundary, the next invocation
reclaims only a lock whose recorded process has exited, then re-verifies the
signed source, receipt, active CLI, backup and complete installed bundle tree.
An exact activated payload can finish the state commit only after the same
`bcgos version` self-check succeeds during recovery. A failed self-check or
partial payload restores the source CLI and removes the target bundle before a
clean retry. An exact committed target consumes the pending plan as already
successful; divergent state fails closed.
The registry contains public keys only and cannot be replaced by an
unauthenticated provider response or by the managed bundle it is used to
verify. Production key custody and the signed seed delivery mechanism remain
external release-environment approvals.

## Still unavailable

- approved operating-system installation directories;
- production signing keys and native code-signing identities;
- approved production provider registration and native-store use;
- a signed bootstrapper seed channel;
- approved corporate-device acceptance of the operator evidence.
