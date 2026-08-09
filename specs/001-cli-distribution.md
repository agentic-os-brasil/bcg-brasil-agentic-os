# Spec 001 - CLI distribution contract

Status: direction and release-manifest contract accepted; CLI entrypoint,
initial memory bridge, workspace init, status and doctor implemented; packaging,
install, update, production signing and release publication pending.

## User journey

The pilot user installs `bcgos`, initializes any approved work folder, validates the installation and receives signed updates without using Git.

## Initial commands

```text
bcgos init [path]
bcgos doctor
bcgos status
bcgos skills index
bcgos update
bcgos version
```

The current source build also exposes an incremental memory bridge:

```text
bcgos memory capture
bcgos memory status
bcgos memory context
bcgos memory dream <daily|weekly>
```

`capture`, `status` and `context` call the runtime-neutral memory core. Capture content enters through bounded standard input rather than process arguments and still requires an adapter sanitization attestation. `dream daily` runs the bundled deterministic L1 synthesizer with the bounded managed runtime configuration; `dream weekly` remains machine-readably unavailable until qualified deep synthesis and lifetime eligibility adapters exist. The local data directory remains explicit for manual commands; the enrolled local maintenance adapter resolves the approved user-local data root.

## Current bootstrap behavior

`bcgos init [path]` is idempotent and creates only a minimal user-visible
surface: `.bcgos/workspace.json` and `brain/README.md`. It preserves an
existing brain README and never creates a client, project or people taxonomy
before that taxonomy is accepted. Private configuration, memory, scheduler
state and logs are created under the local data root, not under the workspace.

`bcgos status [path-or-workspace-id]` returns machine-readable workspace state, version and
declared capability availability. `bcgos doctor [path-or-workspace-id]` returns actionable
checks for workspace integrity, local-data separation and Claude Code/Codex
presence. A missing runtime is reported, not silently installed; unavailable
bundles and updates are declared rather than emulated.

Opaque workspace IDs resolve only through the private per-user binding written
by `bcgos init`. Resolution never scans the user's filesystem and revalidates
the path-bound workspace manifest before returning status. Binding directories
and files use the shared descriptor-anchored no-follow private-path boundary;
symlinks/reparse points, relaxed Unix permissions and tampered bindings fail
closed. An older workspace without a binding must be initialized once by path
before its ID can be used.

`bcgos skills index` returns the compact managed skills catalog used for
capability discovery. It contains navigation pointers only; an agent reads a
canonical skill on demand rather than loading all procedures into a session.

## Local installation trial

Before signed private releases exist, a trial may install a locally supplied
binary through `installers/trial/install.sh` (macOS/Linux) or
`installers/trial/install.ps1` (Windows). Both require an artifact, a SHA-256
checksum file, an explicit `allow unsigned trial` acknowledgement and a clean
target directory. They stage and self-check the binary before activation, do
not replace an existing trial install, alter PATH, download anything or claim
signature trust. CI runs the same flow in an isolated temporary environment on
Windows, macOS and Linux.

## Distribution source

The pilot release provider is the GitHub API for private releases in this repository. The implementation must hide that provider behind an interface so a future BCG artifact source can replace it.

The provider is not the release trust root. `specs/020-release-distribution.md`
and `schemas/release-manifest.schema.json` define a portable manifest whose
issuer and signing-key identity remain stable across a repository transfer.
Provider metadata is untrusted until the detached manifest signature and exact
artifact identities are verified.

## Release contents

- CLI artifacts per supported platform.
- OS bundle.
- Release manifest.
- Checksums and independent signatures.
- Migration metadata.
- Human-readable release notes.

## Update guarantees

- Validate before activation.
- Stage downloads outside the active installation.
- Never partially replace an active bundle.
- Preserve local data and configuration.
- Roll back automatically on failed validation.
- Keep CLI and bundle versions independently observable.
