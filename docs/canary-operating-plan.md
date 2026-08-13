# Maestro Canary operating plan

## Canary 0.1.22 closure boundary

The P0/P1 closure preserves the evidence boundary used by the canary: durable
execution state is inspectable across process restarts, while lifecycle receipts
remain adapter diagnostics and never promote a runtime to native qualification.
The deterministic Maestro bridge accepts typed agent events for an explicit
account/case/Walter loop; absent model/runtime events remain an explicit pending
input rather than being synthesized by the CLI. This evidence state does not
disable the configured contract or block a later event-bearing retry.
The local gate remains separate from native qualification and release trust;
all three must be reported independently in the canary record.

This is the canonical operating plan for Canary evidence. It separates four
different claims that must never be collapsed into one status:

1. **Maintenance Canary** — an attended, local rehearsal of the runtime-neutral
   maintenance contract and its bounded Darwin worker.
2. **Native qualification** — observed evidence that a supported OS/runtime
   actually invokes the installed adapter with the required identity, grants,
   leases, receipts and recovery behavior.
3. **Release Canary** — a technical candidate or signed prerelease moving
   through the external release trust gates.
4. **Pilot readiness** — signed distribution, clean-device acceptance, support,
   incident ownership and a human pilot decision.

The repository is contract-validated. A catalog, unit test, plist, adapter
receipt, wake receipt or successful command in another job does not promote a
capability to native-qualified, release-ready or pilot-ready.

## State model

| State | Evidence claim | Does not prove |
|---|---|---|
| `contract_validated` | Runtime-neutral contracts, schemas, fixtures and local harness evidence exist. | Native invocation or distribution trust. |
| `maintenance_canary_local` | An attended local lifecycle is enrolled and bounded maintenance behavior is observed. | Native scheduler qualification or release authenticity. |
| `native_qualified` | A fresh attended target-runtime session proves the exact adapter/runtime/platform tuple. | Signed publication or pilot approval. |
| `technical_rehearsal` | An unsigned candidate is reproducible and closed on supported build targets. | Authenticity, publication or corporate-device trust. |
| `signed_release` | Approved authorities published one immutable authenticated release set. | Pilot readiness. |
| `pilot_ready` | Clean-device, support, incident and two-user evidence closes the pilot gate. | Production promotion. |

Only a genuinely absent or disabled capability may be `unavailable`; failed or
stopped runs remain explicit and recoverable. These are evidence outcomes, not
reasons to infer the next state or block an enabled path.

The state vocabulary and ledger fields in this document are currently an
operator contract, not a repository-emitted or repository-validated record.
`schemas/canary-report.schema.json` validates only the aggregate local report;
it does not validate run identity, phase transitions, stop decisions,
last-known-good identity or countersignatures. Until a schema validator is
added and run against every ledger entry, the ledger gate is a pre-Canary
blocker: record the run as `unavailable` or `stopped` and do not claim
machine-checked Canary evidence.

## Ownership and responsibilities

| Role | Responsibility | Required sign-off |
|---|---|---|
| Release owner | Owns the run ID, exact source/version/channel, gate decision and last-known-good release. | Final release and pilot decision. |
| Maintenance/runtime owner | Owns the catalog, activation, occurrence, lease, receipt and native-session evidence. | Maintenance and native gates. |
| Platform operator | Runs attended lifecycle and clean-device procedures; preserves raw evidence and produces sanitized reports. | Operator attestation. |
| GitHub administrator | Owns billing, branch/ruleset, environment, action policy and immutable-release controls. | External control evidence. |
| Signing custodian | Controls platform signing and Maestro release authority custody. | Signing and authority evidence. |
| Independent reviewer | Approves protected environment actions and reviews native evidence. | Independent approval; no self-review. |
| Evidence owner | Retains the ledger, receipts, report paths and external countersignature decision ID. | Evidence completeness. |
| Support owner | Owns pilot support channel, user communication and escalation. | Pilot gate. |
| Incident owner | Owns severity response, stop decision and rollback coordination. | Pilot gate and incident closure. |

No role may approve its own protected release action where the gate requires an
independent reviewer.

## Exact CLI surface

> **Superseded** — The `bcgos` CLI was removed in PR #349 (2026-08-13) and replaced by
> the ZIP + slash-command (Maestro) model. The command names below use the current
> slash-command form. For the authoritative current surface, see
> [MAESTRO-CANARY.md](MAESTRO-CANARY.md).

The following commands are the current CLI contract. Commands below are
documented for a future run; this document does not claim that they have been
executed for the release under review.

### Maintenance contract and local Canary

```text
/maintenance catalog
/maintenance status
/maintenance wake \
  --trigger presence|daily|weekly|monthly \
  [--workspace ID] [--attended]

/maintenance canary install-macos \
  --workspace-path PATH \
  --executable PATH \
  --confirm [--home PATH] [--launchctl]
/maintenance canary status [--home PATH] [--launchctl]
/maintenance canary pause --confirm [--home PATH] [--launchctl]
/maintenance canary resume --confirm [--home PATH] [--launchctl]
/maintenance canary uninstall --confirm [--home PATH] [--launchctl]
/maintenance canary recover-quarantine \
  --job-id ID \
  --scheduled-for RFC3339 \
  --reason operator_confirmed_process_gone \
  --confirm [--home PATH] [--workspace ID]
```

`--confirm` is required for every lifecycle mutation except `status`.
`install-macos` with a non-current `--home` is filesystem-only; it cannot load
the real user's LaunchAgent domain.

When the exact current-user enrollment records `mode: native`, `status` and the
aggregate maintenance projection inspect launchctl read-only without requiring
another flag. `--launchctl` remains mandatory for lifecycle mutations. A
`last_receipt: null` result is not a scheduler failure when `due_count` is zero;
native lifecycle qualification comes from the exact loaded/enabled binding,
while job execution evidence requires a due occurrence and terminal receipt.

`maintenance wake` currently has no `--home` flag and always resolves the
current user's data root. Therefore a fixture-home enrollment is suitable for
filesystem lifecycle evidence, but not for an end-to-end fixture worker wake.
Do not present that fixture as a runtime result.

Do not treat `/maintenance wake --trigger event --event-id ID` as scheduled
job evidence in a Canary. The implementation validates the event identity, but
`internal/cli/maintenance.go:schedulerJobsForTrigger("event")` returns no
concrete scheduler job. The expected result is no event execution evidence;
record the path as `unavailable`/STOP until an event job is bound to the route.

## Interpreting the v0.1.20 product Canary

The acceptance run must distinguish a reproducible defect from an intentionally
unavailable capability:

- agent identity personalization must accept the canonical managed IDs emitted
  by the Maestro agent identity service (`maestro`, `walter`, `darwin`) as well as the
  documented omitted-ID form;
- top-level `status`, `doctor` and `maestro status` accept either a workspace
  path or an ID bound by the initial workspace setup; ID resolution is private, fail-closed and
  never searches user files;
- Docling/MarkItDown ingestion remains a release gap, not a successful base
  capability, until Q-037 is closed and a verified platform pack is installed;
- native agent orchestration remains unavailable under Spec 033 until a new
  accepted promotion decision and attended adapter evidence exist;
- a native LaunchAgent enrollment must be evaluated through its loaded/enabled
  binding; filesystem-only status must not be interpreted as proof that the
  service is unloaded; and
- lifecycle receipts prove observed events, while native qualification remains
  a separate attended runtime gate.

The direct deterministic Darwin contract is a separate surface:

```text
/darwin assess --stdin
BCGOS_MAESTRO_CAPABILITY=AUTHORIZED_VALUE \
BCGOS_DARWIN_CAPABILITY=AUTHORIZED_VALUE \
BCGOS_RECOVERY_CAPABILITY=AUTHORIZED_VALUE \
  /darwin housekeeping --stdin < health-packet.json
```

These commands prove the local contract only. They do not qualify Claude,
Codex, launchd or a native scheduler. All three capability values are required
by the CLI and must come from the authorized control plane; never place their
values in the ledger.

### Release and clean-device evidence

```text
go run ./dev/harness validate --full

go run ./dev/release candidate \
  --version VERSION \
  --channel canary \
  --output dist/release-candidate

go run ./dev/release verify \
  --directory dist/release-candidate

go run ./dev/release readiness \
  --provider-config dist/release-authority/provider.json \
  --authority-registry dist/release-authority/registry.json \
  --authority-registry-sha256 AUTHORITY_REGISTRY_SHA256 \
  --candidate dist/release-candidate

printf '%s' "$MAESTRO_ED25519_SEED_B64" | \
  go run ./dev/release sign \
    --candidate dist/release-candidate \
    --output dist/signed-release \
    --authority-registry dist/release-authority/registry.json \
    --issuer RELEASE_ISSUER \
    --key-id RELEASE_KEY_ID

go run ./dev/release verify-signed \
  --directory dist/signed-release \
  --authority-registry dist/release-authority/registry.json

# FUTURE SYNTAX ONLY: the workflows are currently disabled; record
# unavailable/STOP until the reviewed enabled-path check passes.
gh workflow run release-candidate.yml --ref main \
  -f version=VERSION -f channel=canary

gh workflow run signed-prerelease.yml --ref main \
  -f version=VERSION -f channel=canary \
  -f publish_confirmation=publish-maestro-prerelease

go run ./dev/pilot-acceptance validate-phase --receipt RECEIPT.json

go run ./dev/pilot-acceptance corporate \
  --install-receipt install.json \
  --update-receipt update.json \
  --rollback-receipt rollback.json \
  --operator OPERATOR_ID \
  --device-id-hash DEVICE_ID_HASH \
  --policy-id POLICY_ID \
  --channel canary \
  --support-owner SUPPORT_OWNER_ID \
  --output corporate-device-report.json
```

The two `gh workflow run` commands above are not currently executable. The
parent integration renamed all four repository workflows to `.disabled`,
including `release-candidate.yml` and `signed-prerelease.yml`; no enabled
dispatch path exists in this checkout. Record Release Canary and signed-release
phases as `unavailable`/STOP until a reviewed change restores the enabled files
and an administrator verifies the paths before dispatch.

## Evidence ledger

Create one ledger entry per phase. Every entry must bind:

- run ID, phase, state and recorded UTC timestamp;
- source commit, branch/ref, semantic version and channel;
- platform, architecture, runtime and adapter identity/version;
- catalog, policy, activation, authority and qualification digests where
  applicable;
- command name, sanitized arguments, exit state and receipt paths;
- occurrence digest, scheduled time, fence/lease outcome and recovery phase
  for maintenance work;
- provider release ID/tag, manifest and artifact/signature digests for release
  work;
- operator, independent reviewer, support owner and incident owner;
- stop/rollback decision, last-known-good identity and external countersignature
  decision ID.

Never place private keys, passwords, tokens, prompts, source bodies, client
content, absolute private paths, raw device identifiers or free-form provider
errors in the ledger or sanitized receipts.

## Phase checklist

### Phase 0 — scope and preflight

- [ ] Confirm the exact source commit and branch/base relationship.
- [ ] Create a unique run ID and empty evidence ledger.
- [ ] Name the release, maintenance, signing, evidence, support and incident
      owners.
- [ ] Confirm whether the action is maintenance, qualification, release or
      pilot evidence; do not reuse a receipt between tracks.
- [ ] Confirm no secrets or client data are entering the evidence path.

**Pass:** the ledger is complete enough to bind every later artifact.

**Stop:** owner, source identity, channel, authority or evidence destination is
missing or ambiguous.

### Phase 1 — contract and Maintenance Canary

- [ ] Record `maintenance catalog` and `maintenance status`.
- [ ] Confirm `catalog_only`, unavailable model-backed jobs and disabled native
      schedulers are represented honestly.
- [ ] For a local macOS rehearsal, use attended `install-macos --confirm`.
- [ ] Record filesystem-only versus native lifecycle status separately.
- [ ] Exercise only an authorized wake path and preserve its metadata result.
- [ ] Confirm no wake receipt is treated as a durable memory/wiki/model success.
- [ ] Confirm failed/unavailable work remains due and recoverable.

**Pass:** the local contract is bounded, metadata-only and fail-closed.

**Expected fail-closed result:** `state: unavailable`, exit code `3`, or an
unqualified job remaining due. This is not a successful maintenance run.

**Stop:** unexpected write scope, content-bearing receipt, overlapping lease,
success for an unavailable job or a missing occurrence fence.

### Phase 2 — native qualification

- [ ] Run in a fresh attended session for the exact runtime/platform tuple.
- [ ] Confirm adapter identity, runtime identity, OS identity and installation
      scope.
- [ ] For macOS, confirm `launchctl` loaded, enabled and identity-bound state;
      a plist on disk is insufficient.
- [ ] Observe the lifecycle event invoking the bounded worker, not merely the
      adapter command returning.
- [ ] Prove grant scope, non-blocking lease, timeout, crash recovery, retry
      fencing and terminal receipt publication.
- [ ] Prove native negative cases: missing authority, stale digest, wrong
      scope, replay, malformed receipt, provider/runtime unavailable.
- [ ] Capture metadata-only evidence and have an independent reviewer compare
      it with the conformance fixture.
- [ ] Qualify each Claude/Codex/runtime/OS/model-class tuple independently.

**Pass:** fresh observed native evidence satisfies the exact tuple and all
negative gates. The qualification digest is recorded and has an invalidation
rule.

**Stop:** launchctl/task scheduler mismatch, missing native event, authority
leakage, stale or replayable receipt, live fence violation or any data-boundary
breach.

**Rollback:** pause or uninstall the attended lifecycle; recover quarantine only
after the original process is confirmed gone and the exact recovery receipt is
published.

### Phase 3 — technical release rehearsal

- [ ] Full development harness passes on the exact source commit.
- [ ] Candidate uses an unused semantic version and `canary` channel.
- [ ] All target binaries report the requested version.
- [ ] Candidate manifest, asset closure and unsigned digests verify.
- [ ] Evidence is labeled `technical_rehearsal` or `engineering_only`.
- [ ] No unsigned artifact enters the production installation path.

**Pass:** deterministic candidate closure is proven. This phase cannot produce
`signed_release` or `pilot_ready`.

**Stop:** changed source commit, missing/extra asset, digest mismatch or any
attempt to use unsigned output as trusted distribution.

### Phase 4 — signed Release Canary

- [ ] Protected `main`/approved base, required checks and no bypass actor.
- [ ] Actions runner reaches `in_progress` before release dispatch.
- [ ] Protected `maestro-prerelease` environment requires independent review.
- [ ] Provider config, authority registry and release issuer bind the exact
      repository and version.
- [ ] Windows Authenticode and macOS Developer ID/notarization evidence pass.
- [ ] Immutable tag/release policy is verified.
- [ ] Published assets, manifest, source commit, signatures and provider
      attestation are captured in the ledger.

**Pass:** one immutable authenticated release set exists for the exact run.
It remains a signed prerelease, not a pilot decision.

**Stop:** signing/key mismatch, provider mismatch, missing environment approval,
mutable asset replacement, credential leakage or failed attestation.

**Rollback:** activate the previously approved release through the authenticated
bootstrapper, bound to its provider release ID, manifest digest and activation
receipt. There is no generic rollback slash command and no generic release
rollback workflow; release-level rollback is therefore `unavailable`/STOP
until the external provider/bootstrapper invocation and receipt contract are
attached. Never retag, replace assets or use an unsigned override.

### Phase 5 — clean-device acceptance

- [ ] One clean managed Windows device and one clean managed macOS device.
- [ ] Approved installation channel delivers the signed bootstrapper and exact
      authority registry.
- [ ] Each device completes `none -> A -> B -> A` under one run ID.
- [ ] Install, update and rollback receipts validate independently.
- [ ] Corporate reports bind release, manifest, bootstrapper, registry, signer,
      activation receipt, operator and support owner.
- [ ] External evidence owner countersigns both reports.

The clean-device scripts are the currently executable rollback surface. They
invoke the approved bootstrapper's `rollback --data-root DATA_ROOT` only after
binding the active release to the prior update activation receipt. Use the full
platform command below; do not substitute the update activation slash command
(`/update --confirm`), which activates an update, not a rollback.

```text
bash acceptance/clean-device/macos.sh \
  --phase rollback --run-id RUN_ID --device-id-hash DEVICE_ID_HASH \
  --version BASELINE_VERSION --provider-release-id BASELINE_PROVIDER_RELEASE_ID \
  --release-tag maestro-vBASELINE_VERSION --manifest-sha256 BASELINE_MANIFEST_SHA256 \
  --expected-signer-id EXPECTED_SIGNER_ID --managed-root /managed-root \
  --data-root /data-root --workspace /workspace --sentinel /path/sentinel \
  --sentinel-sha256 SENTINEL_SHA256 \
  --activation-receipt /evidence/update-activation-receipt.json \
  --output /evidence/rollback.json

powershell -File acceptance/clean-device/windows.ps1 \
  -Phase rollback -RunID RUN_ID -DeviceIDHash DEVICE_ID_HASH \
  -Version BASELINE_VERSION -ProviderReleaseID BASELINE_PROVIDER_RELEASE_ID \
  -ReleaseTag maestro-vBASELINE_VERSION -ManifestSHA256 BASELINE_MANIFEST_SHA256 \
  -ExpectedSignerID EXPECTED_SIGNER_ID -ManagedRoot C:\\managed-root \
  -DataRoot C:\\data-root -Workspace C:\\workspace -Sentinel C:\\evidence\\sentinel \
  -SentinelSHA256 SENTINEL_SHA256 \
  -ActivationReceipt C:\\evidence\\update-activation-receipt.json \
  -Output C:\\evidence\\rollback.json
```

Validate the resulting receipt with
`go run ./dev/pilot-acceptance validate-phase --receipt rollback.json`.

**Pass:** both corporate reports pass and are externally countersigned.

**Stop:** signature warning, sentinel mutation, failed restoration, unexpected
workspace/owner-data mutation, receipt continuity break or missing counter-sign.

### Phase 6 — pilot decision and closeout

- [ ] Q-011/use-case, cohort, metric and explicit stop criteria are approved.
- [ ] Support and incident channels are live.
- [ ] One Windows and one macOS user complete the guided flow without developer
      dependencies.
- [ ] Observation window closes without S1/S2 incident or data-boundary breach.
- [ ] Release owner records the human pilot decision.

**Pass:** the release is `pilot_ready` and eligible for the human decision under
Spec 022. No automation promotes it.

**Stop:** any safety incident, privacy breach, unresolved support path or failed
rollback. Preserve evidence and keep the last-known-good release.

## Event gap and documented limitation

Specs 009 and 037 require an event identifier for continuous/event signals. The
current CLI accepts `--trigger event` but does not expose `--event-id`, and its
current scheduler mapping does not provide a concrete event job. Until a
separate code change is authorized, event wake is removed from the executable
Canary flow and must be recorded as `unavailable`/STOP rather than treated as
execution evidence.

Likewise, the current `maintenance wake` command has no fixture-home selector.
Do not claim an isolated `--home` installation has exercised the worker; it has
only exercised filesystem lifecycle state.

## Completion packet

A run is closed only when the evidence owner has assembled:

1. the phase ledger and exact source/version/channel identity;
2. maintenance catalog/status and all native qualification receipts;
3. candidate or signed-release manifest and artifact/signature evidence;
4. clean-device phase receipts and corporate reports;
5. stop/rollback decisions, incidents and last-known-good identity;
6. independent reviews and external countersignatures;
7. the final state decision: `contract_validated`, `native_qualified`,
   `technical_rehearsal`, `signed_release`, `pilot_ready`, `stopped` or
   `unavailable`.

No receipt or report may be reused to close a different run, source commit,
version, platform, runtime or authority tuple.
