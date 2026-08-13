# Maestro canary release: external trust-gate runbook

The complete cross-track operating plan is
[`docs/canary-operating-plan.md`](canary-operating-plan.md). Use it to classify
Maintenance Canary, native qualification, Release Canary and pilot evidence
before applying the external controls in this runbook. This document owns the
release trust gate; it does not qualify the maintenance runtime.

This runbook covers controls that deliberately live outside the repository. It
is a prerequisite for a real canary run, not authorization to publish and not
evidence that the pilot gates in `specs/022-guided-pilot-release.md` have
passed.

## Status boundary

The repository can build and verify deterministic unsigned candidates; however,
the release workflow files are currently disabled, so GitHub dispatch is
unavailable. The signed prerelease path must also fail closed when production
signing, provider or GitHub governance authorities are absent.

The last administrative diagnosis on 2026-07-26 reported:

- GitHub Actions jobs could not start because account billing or the Actions
  spending limit blocked runner execution;
- the private repository plan did not permit the required protection for
  `main`;
- the `maestro-prerelease` protected environment and its reviewers were not
  configured;
- repository-level SHA-pinning enforcement and immutable GitHub releases were
  disabled.

Those observations are external state and can change without a commit. They
were not reverified while preparing this runbook because the available local
GitHub credentials were invalid. An administrator must recheck each item in the
GitHub UI or authenticated API and attach current evidence to the release
ticket. Until then, treat every item above as unresolved.

Production release keys, native signing identities, approved provider
registration, managed-device evidence, a support owner and an incident path
also remain external gates. A green repository harness cannot substitute for
them.

## Required GitHub controls

Keep the repository private. Do not make it public to obtain a plan feature, and
do not weaken source protection, environment approval, immutable releases,
action pinning or signing to make the workflow run.

Before dispatching a release:

1. Restore GitHub Actions billing and configure an approved non-zero spending
   limit. Prove that one harmless workflow job advances from `queued` to
   `in_progress`.
2. Select a GitHub plan or equivalent enterprise control plane that supports
   required reviews for both protected branches and a protected environment in
   this private repository. Recheck current GitHub plan capabilities before
   purchase because they are provider policy, not a repository contract.
3. Protect `main`, or create an equivalent ruleset, with:
   - pull-request-only changes;
   - at least one independent approval;
   - stale-review dismissal and required conversation resolution;
   - the three required checks named `development harness (ubuntu-latest)`,
     `development harness (windows-latest)` and
     `development harness (macos-latest)`;
   - enforcement for administrators;
   - no force push, deletion or bypass actor.
4. Create the `maestro-prerelease` environment:
   - allow deployments only from protected `main`;
   - make an independent release or security custodian (or a tightly scoped
     custodian team) the required reviewer;
   - prevent self-review;
   - restrict workflow dispatch authority so the initiator and approver are
     different people;
   - disable administrator bypass.

   GitHub's standard required-reviewer list is an any-one gate; listing Daniel
   and a custodian does not require both approvals. If policy requires two
   approvals, enforce that with an approved custom deployment protection rule
   or equivalent external control and retain its evidence.
5. Restrict Actions to the approved allowlist and enable full-SHA pinning at
   the organization or repository level. The development harness is the
   repository-level backstop: it parses every
   `.github/workflows/**/*.{yml,yaml}` and every
   `.github/actions/**/action.{yml,yaml}` file. External `owner/repository`
   actions must use a full 40-hex commit SHA. Local `./` actions are accepted
   only as clean repository-relative paths, and `docker://` actions only with a
   lowercase `sha256` digest.
6. Enable immutable releases. Confirm the signed workflow rejects both an
   existing version tag and a changed asset set.

Record the ruleset, environment, Actions policy and immutable-release settings
in the release ticket without copying secrets.

## Protected environment inputs

Store these values only in the `maestro-prerelease` environment. Never commit
them, paste them into a ticket or move them to repository-wide secrets.

| Type | Exact name | Expected custody or content |
| --- | --- | --- |
| Variable | `MAESTRO_PROVIDER_CONFIG_B64` | Base64 approved public provider configuration, bound to the exact private release repository. |
| Variable | `MAESTRO_AUTHORITY_REGISTRY_B64` | Base64 approved public Maestro authority registry with validity and revocation state. |
| Variable | `MAESTRO_RELEASE_ISSUER` | Production Maestro release issuer identifier. |
| Variable | `MAESTRO_RELEASE_KEY_ID` | Production Ed25519 public-key identifier. |
| Secret | `MAESTRO_WINDOWS_PFX_B64` | Base64 Authenticode certificate bundle under Windows-signing custody. |
| Secret | `MAESTRO_WINDOWS_PFX_PASSWORD` | Password for the Windows signing bundle. |
| Secret | `MAESTRO_MACOS_P12_B64` | Base64 Developer ID certificate bundle under macOS-signing custody. |
| Secret | `MAESTRO_MACOS_P12_PASSWORD` | Password for the macOS signing bundle. |
| Secret | `MAESTRO_MACOS_SIGNING_IDENTITY` | Exact approved Developer ID identity. |
| Secret | `MAESTRO_MACOS_KEYCHAIN_PASSWORD` | Password for the workflow's ephemeral keychain. |
| Secret | `MAESTRO_MACOS_NOTARY_KEY_B64` | Base64 App Store Connect API private key (`.p8`) held by the notarization custodian. |
| Secret | `MAESTRO_MACOS_NOTARY_KEY_ID` | App Store Connect API key ID authorized for notarization. |
| Secret | `MAESTRO_MACOS_NOTARY_ISSUER_ID` | App Store Connect API issuer ID authorized for notarization. |
| Secret | `MAESTRO_ED25519_SEED_B64` | Production Maestro release-signing seed; migrate to an approved hardware-backed signer when available. |
| Secret | `MAESTRO_RELEASE_POLICY_TOKEN` | Least-privileged installation token used only to read immutable-release policy. |

An administrator must confirm that each input exists without revealing its
value. Environment secrets must remain unavailable until independent approval.

## Release Canary execution

Use a reviewed commit on protected `main` and an unused canonical semantic
version.

1. Confirm the Maintenance Canary and native-qualification evidence are either
   explicitly out of scope or attached as separate ledger entries. Never use a
   maintenance wake receipt as release evidence.
2. Confirm the three required CI checks passed for the exact source commit.
3. Check that the release-candidate workflow is actually enabled. In the
   current checkout, `.github/workflows/release-candidate.yml` is absent and
   only `.github/workflows/release-candidate.yml.disabled` exists. Record
   `unavailable`/STOP and do not continue while that is true. Re-enable the
   workflow only through a reviewed protected-branch change, then verify the
   enabled path before dispatching.
4. After the enabled-path check passes, dispatch **release candidate** from
   `main` with that version and the `canary` channel. Treat its artifact as
   unsigned engineering output only. The future dispatch syntax is:

   ```text
   gh workflow run release-candidate.yml --ref main \
     -f version=VERSION -f channel=canary
   ```

5. Verify candidate closure with:

   ```text
   go run ./dev/release verify --directory dist/release-candidate
   ```

   Also run readiness against the exact public inputs and candidate. Exit code
   `0` means no blocked/unavailable checks; exit code `1` is blocked and exit
   code `3` is unavailable or not evaluated:

   ```text
   go run ./dev/release readiness \
     --provider-config dist/release-authority/provider.json \
     --authority-registry dist/release-authority/registry.json \
     --authority-registry-sha256 AUTHORITY_REGISTRY_SHA256 \
     --candidate dist/release-candidate
   ```

6. Check that the signed-prerelease workflow is actually enabled. In the
   current checkout, `.github/workflows/signed-prerelease.yml` is absent and
   only `.github/workflows/signed-prerelease.yml.disabled` exists. Record
   `unavailable`/STOP and do not continue while that is true. After a reviewed
   re-enable and independent environment approval, the future dispatch syntax
   is:

   ```text
   gh workflow run signed-prerelease.yml --ref main \
     -f version=VERSION -f channel=canary \
     -f publish_confirmation=publish-maestro-prerelease
   ```

   The workflow executes the authoritative closure commands
   `go run ./dev/release sign ...` and
   `go run ./dev/release verify-signed ...` with the protected
   `MAESTRO_ED25519_SEED_B64` secret. The local equivalent is only permitted
   inside the approved signing custody and must receive the seed on stdin:

   ```text
   printf '%s' "$MAESTRO_ED25519_SEED_B64" | \
     go run ./dev/release sign \
       --candidate dist/release-candidate \
       --output dist/signed-release \
       --authority-registry dist/release-authority/registry.json \
       --issuer RELEASE_ISSUER --key-id RELEASE_KEY_ID
   go run ./dev/release verify-signed \
     --directory dist/signed-release \
     --authority-registry dist/release-authority/registry.json
   ```

   Never put the seed in the ledger, shell history, ticket or artifact.
7. Capture the workflow URL, immutable release URL and tag, source commit,
   manifest digest, signed asset checksums, native-signing evidence and GitHub
   attestation result.
8. Through the approved OS installation channel, seed the platform-signed
   bootstrapper and authority registry on one clean managed Windows device and
   one clean managed macOS device.
9. Follow `acceptance/clean-device/README.md` to record install, update and
   rollback receipts for the same run ID, assemble sanitized device reports and
   obtain the approved external countersignatures.

Neither publication nor two device reports automatically make the release
pilot-ready. Cohort progression remains a human decision under Spec 022. The
exact state, evidence and stop/rollback criteria are maintained in the
[canonical operating plan](canary-operating-plan.md).

## Stop and rollback

Stop immediately on signature or key mismatch, native-signing warning,
credential leakage, unexpected bundle content, workspace or owner-data
mutation, failed restoration, missing authority, or a severity-1/2 incident.
Preserve immutable evidence and last-known-good artifacts. Do not retag,
replace release assets, force-update or use an unsigned override.

Rollback is an authenticated activation of the previously approved release,
bound to its provider release ID, manifest digest and activation receipt. It is
not deletion or asset replacement. The repository does not expose a generic
user-facing rollback slash-command. The only currently executable rollback path is the
clean-device acceptance script, which invokes the approved bootstrapper:

```text
bash acceptance/clean-device/macos.sh --phase rollback ... \
  --activation-receipt /evidence/update-activation-receipt.json \
  --output /evidence/rollback.json
powershell -File acceptance/clean-device/windows.ps1 -Phase rollback ... \
  -ActivationReceipt C:\\evidence\\update-activation-receipt.json \
  -Output C:\\evidence\\rollback.json
```

The omitted arguments are mandatory identity, signer, managed-root, data-root,
workspace and sentinel arguments documented in
`acceptance/clean-device/README.md`. A release-level rollback without that
approved bootstrapper/provider surface is `unavailable`/STOP and cannot close
the Release Canary or pilot gate.

## Administrator-owned open items

- restore and fund GitHub Actions, then prove runners start;
- procure and configure private-repository branch and environment controls;
- configure `main`, `maestro-prerelease`, action policy and immutable releases;
- appoint independent reviewers, signing custodians, support owner and incident
  owner;
- provision native signing, the production Maestro release authority, provider
  registration and least-privileged policy reader;
- provide clean managed Windows and macOS devices and external evidence
  countersignatures.

Until every item is currently evidenced and the signed pipeline plus
clean-device sequence succeeds, Maestro is not release-ready.
