# Spec 001 - CLI distribution contract

Status: direction accepted; CLI entrypoint and initial memory bridge implemented; install, init, doctor, update and release distribution pending.

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
