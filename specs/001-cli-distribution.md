# Spec 001 - CLI distribution contract

Status: direction accepted; CLI entrypoint, initial memory bridge, workspace init, status and doctor implemented; install, update and release distribution pending.

## User journey

The pilot user installs `bcgos`, initializes any approved work folder, validates the installation and receives signed updates without using Git.

## Initial commands

```text
bcgos init [path]
bcgos doctor
bcgos status
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

`capture`, `status` and `context` call the runtime-neutral memory core. Capture content enters through bounded standard input rather than process arguments and still requires an adapter sanitization attestation. Until an approved synthesis and eligibility adapter is installed, `dream` returns the machine-readable capability state `unavailable` and performs no emulation. The local data directory and context budgets remain explicit arguments until `bcgos init` owns approved per-platform configuration.

## Current bootstrap behavior

`bcgos init [path]` is idempotent and creates only a minimal user-visible
surface: `.bcgos/workspace.json` and `brain/README.md`. It preserves an
existing brain README and never creates a client, project or people taxonomy
before that taxonomy is accepted. Private configuration, memory, scheduler
state and logs are created under the local data root, not under the workspace.

`bcgos status [path]` returns machine-readable workspace state, version and
declared capability availability. `bcgos doctor [path]` returns actionable
checks for workspace integrity, local-data separation and Claude Code/Codex
presence. A missing runtime is reported, not silently installed; unavailable
bundles and updates are declared rather than emulated.

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
