# Maestro release gates

This checklist is the release decision record for moving from an unsigned
candidate to a pilot-eligible release. It consumes, but does not replace, the
[canonical Canary operating plan](canary-operating-plan.md). A checked box means that the named
evidence exists for the exact source commit, version, channel and provider
release. It does not mean that a different run or a later mutable state is
covered.

## Gate definitions

| Gate | What it proves | Required evidence | Pilot claim |
| --- | --- | --- | --- |
| Maintenance Canary | An attended local maintenance rehearsal obeys the runtime-neutral catalog, authority, lease and metadata-only receipt contract. | `bcgos maintenance catalog`, `status`, bounded wake/lifecycle evidence, exact workspace/home identity and no content-bearing receipts. | Engineering/maintenance evidence only. No native or release claim. |
| Native qualification | A fresh target-runtime/platform session invokes the exact installed adapter and proves identity, grants, fencing, recovery and negative cases. | Runtime/platform identity, observed lifecycle event, qualification digest, metadata-only receipts and independent review. | Runtime-qualified only. No signed-release or pilot claim. |
| Technical rehearsal | The repository can produce and close a deterministic candidate on all supported targets. | Full development harness; Windows amd64 and macOS amd64/arm64 builds; native `version` smoke tests; candidate manifest/artifact closure; unsigned artifact digests. | Engineering evidence only. No authenticity, publication or pilot claim. |
| Signed release | Approved authorities produced and published one immutable, authenticated release set. | Protected `main`; approved `maestro-prerelease` environment; active Ed25519 key in the authority registry; Authenticode verification; Developer ID signing and notarization/assessment; authenticated private provider; immutable release/tag; exact asset closure and provider attestation. | Signed prerelease. Still not pilot-ready. |
| Pilot-ready release | The signed release works on managed devices and has accountable operations. | One Windows and one macOS corporate-device report, each proving install → update → rollback; operator attestation; external countersignature; support owner; incident owner and rollback path; two-user canary observation. | Eligible for the human pilot decision under Spec 022. |

## Objective checklist

### 1. Maintenance Canary (local, attended, non-release)

- [ ] `bcgos maintenance catalog` and `bcgos maintenance status` are recorded
  for the exact source/run identity.
- [ ] The catalog remains `catalog_only` unless separate qualification evidence
  has promoted the exact job tuple.
- [ ] Any macOS lifecycle install uses
  `bcgos maintenance canary install-macos --confirm`; fixture homes are labeled
  filesystem-only.
- [ ] `maintenance wake` is not run against a fixture-home enrollment because
  the current command has no `--home` selector.
- [ ] `unavailable`, `busy`, failed and quarantined outcomes remain explicit;
  no wake receipt is treated as durable subsystem success.
- [ ] Event-trigger evidence is not claimed: the current CLI has no
  `--event-id` and does not map a concrete event job. See the canonical plan.

### 2. Native qualification (fresh attended runtime evidence)

- [ ] The exact runtime/platform/OS/adapter tuple is recorded.
- [ ] A fresh attended session observes the lifecycle event invoking the worker;
  adapter files, unit fixtures and adapter-command receipts are insufficient.
- [ ] Identity, scoped grant, non-blocking lease, timeout, crash recovery,
  retry fencing, terminal receipt and negative cases pass.
- [ ] macOS `launchctl` state is identity-bound; a plist on disk is not enough.
- [ ] Claude and Codex evidence is collected separately where both are claimed.
- [ ] An independent reviewer signs the qualification evidence and digest.

### 3. Technical rehearsal (local/repository-deterministic)

- [ ] Source commit is reviewed and `go run ./dev/harness validate --full` passes.
- [ ] `release candidate` runs from that exact commit and version/channel.
- [ ] Windows amd64, macOS Intel and macOS arm64 binaries are built on their
  matching runners and report the requested version.
- [ ] `go run ./dev/release verify --directory <candidate>` passes.
- [ ] Candidate bytes, manifest and notes have recorded SHA-256 digests.
- [ ] For `0.2.0` or any update receiving a pre-boundary install, the manifest
  carries `practice-agent-to-pa-expert` with exact bundle, catalog and policy
  digests; direct prepared activation enforces the same source range, and
  rollback tests cover canonical post-boundary states with no migration marker.
- [ ] Evidence is labeled `technical rehearsal` or `engineering evidence only`;
  no unsigned output is installed through the production path.

### 4. Signed release (external authority + immutable publication)

- [ ] `main` is protected with the required pull-request checks and no bypass
  actor; `github.ref_protected` is `true` in the dispatch run.
- [ ] GitHub Actions billing/spending is active and a harmless job reached
  `in_progress` before the release workflow is dispatched.
- [ ] `maestro-prerelease` exists, is restricted to protected `main`, has an
  independent reviewer and prevents self-approval.
- [ ] `MAESTRO_PROVIDER_CONFIG_B64` parses as `approved` and its owner/repository
  exactly matches the publication repository.
- [ ] `MAESTRO_AUTHORITY_REGISTRY_B64` parses, has the selected active Ed25519
  issuer/key ID inside its validity window, and its bytes/digest are recorded.
- [ ] The production Ed25519 private key is held by the approved custody
  process; no key material is committed or copied into evidence.
- [ ] Windows Authenticode signing and verification pass with the approved
  certificate identity and timestamp.
- [ ] macOS Developer ID signing passes and notarization/assessment evidence is
  attached for both supported architectures.
- [ ] The least-privileged release-policy token confirms immutable releases are
  enabled and the version tag does not already exist.
- [ ] The signed workflow verifies manifest/artifact signatures, exact source
  commit, exact asset closure and provider attestation after publication.
- [ ] Release notes state that the result is a signed prerelease, not
  pilot-ready.

### 5. Pilot-ready (device + operating evidence)

- [ ] Q-011 first-use case contract is explicitly approved, with an approved
  target cohort (classic and technical consultants), one acceptance metric,
  stop criteria and a named product owner. A proposed contract or a generic
  “pilot value” statement does not close this gate.
- [ ] An approved installation channel has delivered the platform-signed
  bootstrapper and the exact authority-registry seed.
- [ ] A clean managed Windows device produces passing install, update and
  rollback receipts for the same run ID and release identities.
- [ ] A clean managed macOS device produces the same three passing receipts.
- [ ] Each schema-v2 corporate report binds provider release IDs/tags, manifest
  digests, bootstrapper and registry digests, native signer, activation receipt,
  operator and support owner.
- [ ] An approved external evidence owner countersigns both corporate reports;
  the countersignature is retained outside the repository with its decision ID.
- [ ] Support owner, incident owner, escalation channel and last-known-good
  retention are named before inviting pilot users.
- [ ] One Windows and one macOS user complete the guided flow without Git or
  developer dependencies; the five-business-day observation window has no
  severity-1/2 incident or data-boundary breach.
- [ ] The human release owner records the pilot decision. Device evidence alone
  never promotes a release automatically.

## Evidence ledger

Record these fields for every gate: source commit, release version, channel,
workflow run URL, provider release ID/tag (when applicable), manifest SHA-256,
artifact/signature digests, authority-registry SHA-256, signer identities,
operator/device report paths, countersignature decision ID, support owner and
incident owner. Never copy private keys, passwords, tokens, certificates or
raw device identifiers into the ledger.

## Current boundary

The repository already provides deterministic candidate packaging, manifest and
artifact closure checks, signed-release verification, provider/update planning,
transactional install/update/rollback and strict clean-device receipt schemas.
The signed-release and pilot-ready boxes remain unchecked until the external
authority, GitHub governance, native signing and managed-device evidence are
actually present for one real run. A failed CI start caused by billing is an
external blocker, not a release-contract defect.

### Beta ownership boundary

The technical rehearsal gate is intentionally available before paid corporate
signing is provisioned. Personal Apple or Windows signing credentials are
prohibited, including for beta, which remains unsigned. Any beta Ed25519
authority is test-only and must live in a separate test registry; it cannot sign
new production artifacts. The production registry and workflow must reject its
issuer/key ID (or mark it revoked), and a newly issued organization-controlled
key plus custody record is required before the signed-release gate can be
checked. Retained beta public keys are for archival verification only, never
installer/update trust.

### Explicit status map (2026-07-27)

| Class | Current finding | Evidence or owner |
| --- | --- | --- |
| Deterministic locally | Manifest schema/semantic checks, issuer registry validation, exact Ed25519 release closure, private-provider response validation, immutable update plans, transactional install/update/rollback and report validation are covered by Go tests. The candidate workflow is SHA-pinned and its Windows/macOS binaries have version smoke tests. | Repository tests and `go run ./dev/harness validate --full`; workflow files; merged PRs #79/#80. |
| Depends on external authority | Production Ed25519 custody, authority-registry approval/seed delivery, Authenticode certificate, Developer ID certificate and notarization, private provider registration/token, GitHub protected `main`, `maestro-prerelease` reviewers, immutable publication policy, managed devices, countersigning, support and incident ownership. | Administrator/security owner; values must remain outside Git. |
| No evidence yet | No signed publication run, no production authority registry/key, no native-signed bootstrapper or notarized release, no provider release ID/attestation, no real clean-device Windows/macOS receipts, no external countersignatures and no five-business-day two-user canary. | Must be attached to the exact release ledger before promotion. |
| Pilot blockers | The latest remote diagnosis records GitHub Actions billing/spending blocking job execution and the Free private-repository plan lacking the required branch/environment controls. The environment, secrets/variables and all authority/device evidence are still absent. | Issue #77 and the administrator runbook; recheck live before dispatch because external state can change. |

See [`docs/canary-operating-plan.md`](canary-operating-plan.md) for the
cross-track phases, exact CLI surface, evidence ledger, responsibilities and
pass/fail/stop/rollback criteria. See
[`docs/release-canary-admin-runbook.md`](release-canary-admin-runbook.md) for
administrator-owned release trust controls.
