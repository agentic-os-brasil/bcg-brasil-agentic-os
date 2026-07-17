# Spec 005 - Development harness

Status: accepted for initial implementation.

## Boundary

The harness exists only to develop and govern this source repository. It is not part of the `bcgos` user CLI, OS bundle, runtime adapters or pilot installation.

The full gate rejects explicit development-harness references in distribution surfaces. Future bundle packaging must use an allowlist rather than copying the repository tree.

## Objective

Make project decisions and contract-focused tests the recoverable logic of development without turning every diff into bureaucracy.

## Change classification

| Change | Required path |
|---|---|
| Behavioral contract, architecture, security, data or runtime policy | Spec or durable decision, failing contract test when executable, implementation, full validation |
| Bug | Reproduction test that fails, minimal fix, full validation |
| Refactor with behavioral risk | Characterization tests as needed, refactor, full validation |
| Documentation, copy or mechanical tooling | Applicable validation only; no artificial decision or test |
| Time-boxed spike | Explicitly non-production; convert to the normal path before incorporation |

## Decision log contract

- IDs contain exactly four uppercase letters, for example `ABCD`, `XPTO` or `EUWH`.
- Codes should be memorable when possible, but are not sequential and are never reused.
- Entries require date, status, owner, context, decision, consequences, references and supersedes.
- Allowed statuses are `proposed`, `accepted`, `superseded` and `rejected`.
- Accepted history is append-only; a new entry supersedes an old decision.
- CI blocks malformed entries and duplicate codes.
- `Supersedes` must be `none` or reference an existing four-letter code.
- Parallel-branch collisions are resolved before merge.
- Status, tasks and release notes do not belong in the decision log.

## Test contract

- Unit tests cover pure logic and meaningful edge cases.
- Bugs begin with a test that reproduces the failure.
- CLI, manifests and runtime portability use contract or conformance tests.
- Tests assert observable behavior, not private implementation shape.
- A test added after implementation must not be represented as test-first evidence.
- Coverage percentage is diagnostic in v0, not a merge target.

## Development commands

```text
go run ./dev/harness validate
go run ./dev/harness validate --full
go run ./dev/harness decision check
go run ./dev/harness decision available ABCD
```

The harness must remain cross-platform, idempotent and usable without an agent or skill.

## Skill contract

`develop-change` classifies changes and orchestrates the appropriate development loop. `record-decision` records only durable decisions and validates the log. Both live under `dev/`, call deterministic repository commands and are excluded from product distribution.
