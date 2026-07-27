# Technical release rehearsal — 0.0.1 canary candidate

This is a reproducible engineering record for the unsigned release-candidate
workflow. It is not a signed release, a pilot approval, or corporate-device
acceptance evidence.

## Run identity

- Workflow: [release candidate run 30231463294](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/actions/runs/30231463294)
- Source commit: `3e214f4676e8fb4c300316ce540f4031d26fb924`
- Candidate: `0.0.1`
- Channel: `canary`
- Result: success
- Executed: 2026-07-27 (UTC)

## Checks passed

The run completed all stages for the unsigned candidate:

- full source harness on Ubuntu;
- native Windows amd64 build;
- native macOS Intel build;
- native macOS Apple silicon build;
- native provenance generation;
- deterministic candidate assembly;
- candidate closure verification;
- artifact upload.

The downloaded macOS binaries were executed outside the runner and both
reported `bcgos 0.0.1`. The downloaded Windows artifact was identified as a
PE32+ x86-64 executable; it still requires execution on a clean Windows device
for acceptance.

## Candidate manifest

The unsigned candidate contained these four release artifacts:

| Artifact | Size | SHA-256 |
| --- | ---: | --- |
| `bcgos_0.0.1_windows_amd64.exe` | 8,952,320 | `e7b3a83609d6cc10fdfb1e0f4b93234a3e2703a52668e71bf3fb157b3cfa1b08` |
| `bcgos_0.0.1_darwin_amd64` | 8,644,120 | `d66f9e092d9fd422de09d66bd80087d1f602f964957b90336716057865c51844` |
| `bcgos_0.0.1_darwin_arm64` | 7,968,130 | `92bb30292fdc974d7b5d91e5d6cce115ead3d717e4c82d16ecb6c5cb2c8cfe5a` |
| `maestro-base_0.0.1.tar.gz` | 11,786 | `df4305c4217248a9e112644586e819fde4b27c5e53e86ac9ba806133d70513d6` |

The workflow's closure verifier accepted the candidate. These hashes describe
the unsigned rehearsal artifacts and must not be reused as signed-release
authority evidence.

## Explicit boundary

This rehearsal does not prove:

- Authenticode or Developer ID signing;
- private provider authentication or release publication;
- immutable signed-manifest attestation;
- install, update and rollback on a clean managed Windows device;
- install, update and rollback on a clean managed macOS device;
- corporate acceptance or pilot readiness.

The next evidence record must reference a signed workflow run and the three
sanitized receipts from each approved device. Until then, the release remains
an unsigned technical candidate.
