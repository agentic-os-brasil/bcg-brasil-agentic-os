# Maestro canary release: external trust-gate runbook

This runbook configures the external controls that the repository intentionally
does not store. It is a prerequisite for one real canary pipeline run. It does
not authorize publication by itself.

## Current diagnosis (2026-07-26)

The current repository is private and GitHub reports all of the following:

- GitHub Actions jobs are not starting because recent account payments failed
  or the Actions spending limit must be increased. This blocks execution before
  any workflow step, including Windows and macOS tests.
- `main` has no branch-protection rule or ruleset. The API rejects configuration
  with `Upgrade to GitHub Pro or make this repository public to enable this
  feature.`
- No `maestro-prerelease` environment exists. Consequently it has no reviewers,
  branch policy, secrets or variables.
- Actions accepts all actions and does not require SHA pinning. This PR pins all
  current workflow actions and adds a CI check against future tag-based refs.
- Immutable GitHub releases are disabled.

These are external-state blockers, not a code-test failure. A CI run cannot be
used as evidence until Actions billing is restored.

## Decision required: preserve the private trust gate

Keep the repository private. Upgrade to GitHub Enterprise Cloud (or an
Enterprise Server deployment with equivalent private-environment reviewer and
no-bypass controls), restore GitHub Actions billing, and configure the controls
below. Do **not** make the repository public and do **not** bypass
`github.ref_protected`, environment approval, immutable releases, or signing.

This is the smallest path that satisfies the existing signed-prerelease
workflow without weakening it. GitHub documents protected branches for private
repositories on Pro, Team and Enterprise plans and environment secrets for
private repositories on paid plans. Its current plan documentation limits
required environment reviewers on Free, Pro and Team to public repositories;
the independent private-environment approval gate therefore requires Enterprise
capability, not merely Pro or Team.

## One-time GitHub administration

Perform these settings in this order, recording the resulting links and
screenshots in the release ticket.

1. Restore Actions billing and set a non-zero Actions spending limit. Confirm a
   trivial queued job transitions from `queued` to `in_progress`.
2. Keep the repository private and upgrade to Enterprise capability that
   enables private branch protection and protected-environment reviewers. Do
   not use public visibility as a feature workaround.
3. Protect `main` (or create an equivalent ruleset) with all of the following:
   - pull-request-only changes, at least one independent approval, stale-review
     dismissal, and required conversation resolution;
   - required, up-to-date status checks named `development harness
     (ubuntu-latest)`, `development harness (windows-latest)` and
     `development harness (macos-latest)`;
   - administrators included in enforcement; no force push or branch deletion;
   - no actor in a bypass list. Restrict direct push to the approved release
     maintainers only if operationally necessary.
4. Create the `maestro-prerelease` environment:
   - deployment branches: protected branches only, therefore `main` after step
     3;
   - required reviewers: Daniel and one independent release custodian/security
     reviewer; prevent self-review;
   - do not enable an administrator bypass. If the selected plan cannot enforce
     this for a private repository, stop: it does not meet this canary gate.
5. In **Actions > General**, switch the policy to the approved action allowlist
   and turn on SHA-pinning enforcement after verifying the repository's current
   workflows. The workflow check in this repository is the compatible
   repository-level backstop.
6. Enable **immutable releases**. The signed workflow verifies that setting
   before it creates the prerelease and rejects an existing version tag.

## Environment inputs

Store these only in the `maestro-prerelease` environment; never commit their
values, put them in repository-level secrets, or paste them into a ticket.

| Type | Exact name | Custody / expected content |
| --- | --- | --- |
| Variable | `MAESTRO_PROVIDER_CONFIG_B64` | Base64 of approved public private-provider config; owner and repository must exactly match this publication repository. |
| Variable | `MAESTRO_AUTHORITY_REGISTRY_B64` | Base64 of the approved public authority registry, including validity and revocation state. |
| Variable | `MAESTRO_RELEASE_ISSUER` | Production Maestro release issuer identifier. |
| Variable | `MAESTRO_RELEASE_KEY_ID` | Production Ed25519 public-key identifier. |
| Secret | `MAESTRO_WINDOWS_PFX_B64` | Base64 Authenticode certificate bundle, held by Windows-signing custodian. |
| Secret | `MAESTRO_WINDOWS_PFX_PASSWORD` | Password for the Windows signing bundle. |
| Secret | `MAESTRO_MACOS_P12_B64` | Base64 Developer ID certificate bundle, held by macOS-signing custodian. |
| Secret | `MAESTRO_MACOS_P12_PASSWORD` | Password for the macOS signing bundle. |
| Secret | `MAESTRO_MACOS_SIGNING_IDENTITY` | Exact approved Developer ID signing identity. |
| Secret | `MAESTRO_MACOS_KEYCHAIN_PASSWORD` | Ephemeral keychain password used by the workflow. |
| Secret | `MAESTRO_ED25519_SEED_B64` | Base64 production Maestro release signing seed; prefer replacing this with an approved hardware-backed signing service when available. |
| Secret | `MAESTRO_RELEASE_POLICY_TOKEN` | Fine-grained GitHub App installation token or PAT with only repository `Administration: read`, used solely to read immutable-release policy. |

Before dispatch, an administrator must prove that each value is present without
revealing it. Environment secrets become available only after environment
approval; retain that behavior.

## Executable canary sequence

Use a fresh, reviewed `main` commit and an unused semantic version. Do not
start this sequence while a preceding state is missing.

1. Merge the signed-release and clean-device PR stack only after its normal
   review and CI gates pass. Confirm `main` is protected and the current commit
   passes all three required harness checks.
2. Dispatch **release candidate** from `main` with the intended version and
   `canary` channel. It produces an unsigned artifact only; verify its closure
   with `go run ./dev/release verify --directory dist/release-candidate`.
3. Dispatch **signed Maestro prerelease** from protected `main`, with the same
   unused version, `canary`, and its exact required publication confirmation.
   The independent environment reviewer must approve it. Capture the workflow
   URL, release URL, tag, manifest digest, signed-asset checksums and GitHub
   attestation result.
4. Through the approved OS installation channel, seed the platform-signed
   bootstrapper and authority registry on one clean managed Windows amd64
   device and one clean managed macOS device (Intel or Apple silicon).
5. On each device run the relevant script in `acceptance/clean-device/` for
   `install` (release A), `update` (A to B) and `rollback` (B to A), using one
   shared run ID. Validate all three receipts, then build the sanitized
   corporate report exactly as documented in
   `acceptance/clean-device/README.md`.
6. Obtain the approved external countersignature for each device report.
   Publish neither the report nor the release as "pilot-ready" automatically;
   the two-device evidence and human release decision remain distinct gates.

## Rollback boundary

Rollback is an authenticated move from B back to A using the previously
approved release A identity, manifest digest, provider release ID and the
update activation receipt. It is not deletion, retagging or asset replacement.
Immutable releases must remain enabled throughout. If validation fails, stop
the canary, preserve evidence, revoke the affected authority when required, and
open an incident; do not overwrite the release.

## Items requiring Daniel or an administrator

- Resolve the failed Actions payment or spending limit and fund the first real
  Windows/macOS workflow run.
- Approve GitHub Enterprise Cloud (or Enterprise Server equivalent) for the
  private trust controls; Pro or Team alone does not satisfy the independent
  private-environment reviewer gate documented above.
- Configure branch protection/ruleset and `maestro-prerelease` exactly as
  above, including independent reviewer identities.
- Enable immutable releases and the Actions SHA-pinning policy.
- Appoint signing custodians; provision the Windows and macOS identities,
  production Ed25519 authority, provider configuration and authority registry.
- Create the least-privileged policy-reader token, add all environment inputs,
  and retain custody evidence outside the repository.
- Provide two clean managed devices, confirm the protected bootstrapper path,
  name the support owner, and arrange the external countersignature.

Until these actions are complete and the actual signed pipeline plus two-device
evidence run has succeeded, this repository is **not release-ready**.
