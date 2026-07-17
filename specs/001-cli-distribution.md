# Spec 001 - CLI distribution contract

Status: direction accepted; implementation pending.

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
