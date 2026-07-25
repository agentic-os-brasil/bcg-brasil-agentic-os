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

The base bundle is extracted into the transaction with traversal, links,
special file types, duplicate paths, file-count and expanded-size limits.
Activation copies only the staged CLI and bundle into the managed root.

## Signed local-release boundary

`releaseverify.VerifyDirectory` requires:

- the exact `release-manifest.json` and raw 64-byte detached signature;
- a currently active key found in the approved local Maestro
  product/issuer/key registry;
- matching size, SHA-256 and Ed25519 signature for every artifact;
- matching release-notes digest;
- no missing, extra, directory or symlink entry.

This PR intentionally starts from a local release directory. Authenticated
provider discovery and download are a separate adapter and cannot bypass this
verifier.

## Activation and recovery

The bootstrapper takes a validated plan inside
`owner-data-root/updates/<transaction>`. It acquires a fail-closed activation
lock, preserves the current CLI under `managed-root/recovery`, installs the
immutable bundle version, activates the new CLI and runs `bcgos version`.

If file activation, self-check or durable state commit fails, the new CLI and
bundle are removed and the previous CLI is restored. Explicit rollback uses
the same lock and self-check. Local configuration, workspaces and user memory
are outside every activation path.

On Windows, the CLI launches the independently seeded bootstrapper with its
own PID and exits. The bootstrapper waits for that process before touching
`bcgos.exe`; the active executable never attempts to replace itself.

The bootstrapper seed also establishes the initial release-authority registry.
The registry contains public keys only and cannot be replaced by an
unauthenticated provider response or by the managed bundle it is used to
verify. Production key custody and the signed seed delivery mechanism remain
external release-environment approvals.

## Still unavailable

- approved operating-system installation directories;
- production signing keys and native code-signing identities;
- private provider authentication and download;
- a signed bootstrapper seed channel;
- clean corporate-device acceptance.
