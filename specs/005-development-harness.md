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
go run ./dev/harness doctor
go run ./dev/harness setup
go run ./dev/harness recover
```

The harness must remain cross-platform, idempotent and usable without an agent or skill.

## Evidence layers

The development harness is one layer of the repository's evidence model; a
passing command never promotes a capability beyond the layer it actually
checks.

| Layer | Command or source | What it proves | What it does not prove |
|---|---|---|---|
| Local fast gate | `go run ./dev/harness validate` | Structural contracts, projections, policies, managed-atlas freshness, formatting and development tests | Full repository tests, hosted CI, review, mergeability, release or runtime qualification |
| Local full gate | `go run ./dev/harness validate --full` | Fast gate plus `go vet ./...` and `go test ./...` | Hosted operating-system matrix, human review, protected-branch state, signed release or native runtime evidence |
| Exact-tree gate | `go run ./dev/harness guard pre-commit` and repository hooks | The staged snapshot is safe, fully validated and free of blocked file classes/secrets | Remote acceptance or another person's review |
| Hosted gate | `.github/workflows/validate.yml` | The configured Windows, macOS and Linux workflow ran for a specific commit | Approval, mergeability, publication or pilot readiness |
| Product/release gates | `docs/release-gates-checklist.md` and release workflows | Separate packaging, authority, signing, clean-device and pilot evidence | A local harness pass by itself |
| Runtime evidence | Adapter and attended-session receipts | A named runtime/platform actually exercised the contract | Contract implementation or source-level test coverage alone |

The repository distinguishes `implemented`, `locally validated`, `CI green`,
`reviewed`, `mergeable`, `merged` and `pilot-ready`. Reports and onboarding
must preserve those labels instead of collapsing them into a generic
"green" state.

The repository may use a bare Git repository as object storage with linked
worktrees. `doctor` treats the bare path as storage, reports that no files were
changed and points the contributor to `git worktree list`; Git status, hooks and
contribution commands belong in a real worktree.

## Skill contract

`start-contributing` performs progressive first-time setup. `start-work` creates a safe daily path. `develop-change` classifies and implements changes. `record-decision` records durable choices. `prepare-pr` validates and prepares human review. `recover-work` diagnoses Git state without discarding files.

All skills live under `dev/`, call deterministic repository commands and are excluded from product distribution. `.claude/` and `AGENTS.md` are thin runtime-specific entry points over the same canonical skills.

## Enforcement layers

| Layer | Responsibility |
|---|---|
| Go harness | Cross-platform diagnosis, full validation, secret/client-file checks and exact-tree proof |
| Repository Git hooks | Block commits on `main`, unvalidated commits and pushes that do not match the validated tree |
| Claude hooks | Block destructive commands and autonomous merge; guide session start and decision-log validation |
| CI and GitHub | Re-run the full gate; require human review and protected `main` when the repository plan supports branch protection |

Local hooks are installed per clone with `go run ./dev/harness setup`. They are safety rails, not the remote authority. Every block must state the reason, confirm that no work was discarded and provide one recovery command.

Current pilot limitation: GitHub branch protection is unavailable for this private repository on the account's present plan. Until that changes, CODEOWNERS and PR review express the intended workflow but cannot technically prevent a maintainer from pushing directly to `main`; local hooks and CI remain active backstops.

## Contributor bootstrap contract

- Before clone, a shareable prompt guides prerequisite checks, browser-based authentication and repository access without accepting credentials in chat.
- Software installation requires the contributor's confirmation and must use an inspectable approved package channel; the workflow never pipes remote scripts into a shell or bypasses corporate policy.
- After clone, repo-owned platform scripts perform deterministic local setup. They do not install software, request administrator access, modify remote history or discard files.
- Bootstrap ends with a clean clone, repository-local Git identity, installed hooks and a green fast validation. It does not begin feature work.
- Canonical skills remain under `dev/skills`; runtime directories contain only thin discovery projections validated by the harness.
- Windows is the first implemented contributor bootstrap because Marcelo uses Windows. A macOS counterpart remains required by the contributor roadmap; product pilot parity continues to be governed separately by `DUAL`.
