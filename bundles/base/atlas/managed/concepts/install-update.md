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
x-maestro-profile-version: "1"
x-maestro-stable-id: managed/install-update
x-maestro-scope: managed
x-maestro-source-fingerprint: b54d1d52765dfc79441b02cc1b5a6fc87664c10ef62bcd7a4367ca64234b8974
x-maestro-freshness: fresh
x-maestro-status: active
x-maestro-generator-version: maestro-managed-wiki/0.2
x-maestro-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `docs/install-update.md`. The source remains authoritative.

## Related

- [Maestro release and distribution](/concepts/release-distribution.md)

## Source content

# Portable installation and update

Maestro is currently transported as two target-specific ZIPs:

```text
Maestro-Portable-<version>-macos-arm64-local-beta-unsigned.zip
Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip
```

Each archive has a SHA-256 sidecar and carries a platform-native bootstrapper,
the narrow installed `bcgos` control plane and a manifest that pins target,
version, path and CLI digest. These checks establish local package integrity;
the current `local-beta-unsigned` artifacts are not organization-signed,
notarized, release-ready or pilot-ready.

## Two entry modes

- **Hub:** extract `Maestro/`, open that folder in Claude Code and use the
  conversational product surface. Its first-session hook runs the transported
  bootstrapper before creating private `data/` state.
- **Repo/Worktree:** after activation, explicitly enroll an ordinary Git
  checkout with the installed CLI. Each linked worktree is enrolled
  separately and receives its own opaque `workspace_id` while sharing the
  repository's opaque `repository_id`. Claude and Codex may coexist in the
  same checkout: enroll each runtime once, and inspect, repair or remove each
  projection independently.

The installed CLI is not added to global `PATH` and is not the professional
work interface. It exposes only activation support, runtime hooks and
`workspace enroll|status|repair|remove`; ordinary work remains conversational.

## Managed, private and checkout roots

```text
Maestro/
  managed/                    # replaceable manifest, bootstrapper and CLI
    install-manifest.json
    bin/bcgos[.exe]
  bundles/                    # governed product content
  .claude/                    # Hub projection and first-session hooks
  data/                       # owner-private state, preserved across updates
    install.json              # exact activated package identity
    install.previous.json     # prior activation after a version update
    workspaces/<id>/          # private direct-worktree bindings/context

ordinary-git-worktree/
  CLAUDE.md or AGENTS.md      # user orientation plus managed block if untracked
  .claude/ or .codex/         # bounded, regenerable runtime projection
  .bcgos/                     # shared state plus runtime-scoped identities
```

`managed/`, `data/`, the Git repository and the exact worktree are distinct
authorities. Direct hooks invoke the exact verified CLI by absolute path and
pass all three roots explicitly. The projection never copies owner memory,
credentials, logs or client data into the checkout. Machine-local generated
files use exact Git `info/exclude` entries; broad ignores are forbidden.

## First activation

The normal Hub path is to open the extracted `Maestro/` folder once. The
first-session hook calls `managed/bcgos-bootstrap` (`.exe` on Windows), verifies
the manifest and CLI digest, and writes private `data/install.json`.

An attended technical operator can run the same bootstrapper explicitly:

```text
managed/bcgos-bootstrap activate --managed-root <Maestro/managed> --data-root <Maestro/data>
```

Activation is non-elevated, target-specific and idempotent. A digest mismatch,
same-version/different-bytes package, symlinked authority or overlapping
managed/private roots fails before activation.

## Direct repository/worktree lifecycle

After activation, use the binary inside the same extracted installation:

```text
managed/bin/bcgos workspace enroll --runtime claude|codex <repo-or-worktree>
managed/bin/bcgos workspace status --runtime claude|codex <repo-or-worktree>
managed/bin/bcgos workspace repair --runtime claude|codex <repo-or-worktree>
managed/bin/bcgos workspace remove --runtime claude|codex <repo-or-worktree>
```

Use `managed\bin\bcgos.exe` on Windows. Enrollment never creates or removes a
worktree and never changes branch, HEAD, index, remote, Git configuration or
Git hooks. To use both apps, run `enroll` once for `claude` and once for
`codex`; removing one leaves the other and their shared orchestration state
intact. A tracked runtime-local configuration, modified managed surface, unsafe
path or identity mismatch fails closed. See
[Spec 055](repo://specs/055-direct-repository-worktree-entry.md).

## Update path

For an authorized newer ZIP of the same platform:

1. Close the runtime and rename the current `Maestro/` to `Maestro-old/`.
2. Extract the new archive beside it as a fresh `Maestro/`.
3. **Copy**, do not move, `Maestro-old/data/` into the new folder.
4. Reopen the Hub so the new bootstrapper verifies and activates the package.
   A version change preserves the prior state as
   `data/install.previous.json`; the private profile, memory and workspace
   bindings remain outside the replacement.
5. Run `/maestro-doctor`. For each direct checkout, run `workspace status` and
   explicitly run `workspace repair` only when it reports
   `repair_required`.
6. Keep `Maestro-old/` until Hub and required direct projections are verified.

Do not extract over the existing folder. The rename/copy sequence keeps both
the old core and its private-data copy recoverable during verification.

## Evidence still unavailable

- organization-owned Apple Developer ID signing and notarization;
- Windows Authenticode and corporate-device SmartScreen/WDAC/AppLocker proof;
- authenticated publication/provider evidence;
- clean-device install, update and rollback acceptance on both targets; and
- support/incident ownership and the approved pilot gate.

Those gates remain defined in [the release checklist](repo://docs/release-gates-checklist.md)
and are not implied by a local build, checksum, bootstrap receipt or test run.
