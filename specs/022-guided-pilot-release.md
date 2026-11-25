# Spec 022 - Guided pilot release

Status: accepted pilot gate; production authorities and device evidence remain
pending until recorded.

## Objective

Make natural-language guidance the primary setup and update experience for a
non-technical pilot user, while deterministic CLI and bootstrapper contracts
retain control of authentication, verification, activation and rollback.

## Guided experience

The `maestro-setup-update` skill resolves the interaction profile, diagnoses
the current state, runs authentication and update commands for the user,
explains impact, asks one short confirmation bound to the exact update plan,
and verifies the resulting state.

The skill cannot:

- invent or store a credential;
- approve its own update;
- install an unsigned candidate;
- substitute a source clone or developer tool;
- bypass unavailable native storage, provider or signing authorities;
- describe isolated CI as corporate-device acceptance or pilot readiness.

Decisions `CARY` and `DZIP` define one narrower pre-pilot exception outside the
signed guided-pilot path: factory-built target-specific Windows amd64 and
macOS arm64 local-beta ZIPs may perform a real user-space installation for a
bounded Canary cohort while organization-owned native signing is unavailable.
Each remains bound to the exact test-only authority, registry and native
bootstrapper hashes and an authenticated `canary` manifest. The profile is
never enabled by a runtime flag, never promoted by the setup/update skill and
never counts as an authority, clean-device, two-user or pilot gate. Windows
accepts exactly `NotSigned`; macOS accepts exactly no code-signature load
command plus an agreeing native unsigned classification when built on macOS.
Ad-hoc, invalid or partially signed artifacts do not enter the unsigned
profile.

## Evidence classes

`internal/dev/pilotacceptance` recognizes two non-interchangeable report modes:

- `isolated_ci` proves engineering behavior on disposable Windows/macOS
  runners and may claim only `engineering_evidence_only`;
- `corporate_device` requires install, update and rollback passes plus an
  operator, hashed device identifier, strict policy identifier, approved
  channel and release bindings. It may claim only
  `corporate_device_operator_attestation`; corporate acceptance requires an
  approved external countersignature.

Corporate evidence uses schema-v2 reports assembled from three strict,
sanitized phase receipts. The receipts must prove the ordered transition
`none -> baseline -> update -> baseline`, bind the exact baseline and update
manifest SHA-256 values and preserve each receipt's own SHA-256 in the final
report. The report also binds both immutable provider release IDs/tags, the
native bootstrapper digest, authority-registry digest, approved native signer,
update activation receipt and accepted support owner. Unknown fields,
duplicate JSON keys, non-exact phase checks or mixed run/device/platform
identities fail closed.

A report is evidence, not automatic permission to publish or expand a pilot.

## Pilot gates

1. **Engineering gate** - full harness, release verification, transactional
   update/rollback and isolated trial installation pass on Windows and macOS.
2. **Authority gate** - production release key, Windows/macOS native signing,
   native credential stores, private provider, support owner and incident path
   are approved.
3. **Clean-device gate** - one managed Windows and one managed macOS device
   each pass first install, update and rollback with valid operator
   attestations that the approved external evidence owner countersigns.
4. **Two-user canary** - one Windows and one macOS user complete the guided
   flow without Git or developer dependencies. Observe for five business days.
5. **Ten-person pilot** - expand only after both canary users succeed, there is
   no severity-1/2 incident or data-boundary breach, rollback works, and the
   support owner accepts the observed demand.

The ten-person cohort balances Windows/macOS and classic/technical users.
Expansion, redesign or stop is a human release decision.

## Installed MCP participant self-check

The original native participant entry has one automatic MCP check action. It
uses only the shipped native programs and the existing authenticated
installation/current-client/packet owners. It never asks participants to run a
development harness, install a toolchain or assemble protocol requests.

The existing `installed-pilot-check --check-only` path checks the actual
admitted stable carrier through the public SDK/stdio transport. It verifies
workspace identity, complete method discovery and complete canonical role/method
resource identity. An in-process server is not a fallback for installed-transport
success. Constructor guards must be released before starting the child; normal
current-source/record and stable-start coordination rules remain in force.
Read-only means no domain mutations or implicit preparation, repair, migration,
qualification or grants; it does not assert absence of normal coordination locks.

The checker uses a dedicated authenticated `stable-check-launch` ->
`installed-check-serve` purpose through the same canonical admission owner. The
existing installed role/current-record handoff is revalidated before an opaque
checker capability is derived. This child serves the ordinary explicit MCP
operations but never starts the automatic maintenance worker; read-only and
functional checks must not trigger already enrolled due jobs as a startup effect.
Normal `stable-launch`, ordinary serve and other maintenance/wake processes keep
their semantics and grants. No ordinary serve flag, MCP client metadata or
environment switch selects the checker purpose. Unbound/unknown purposes and an
older unsupported bootstrap refuse; there is no normal-launch fallback. Reports
name the checker-purpose SDK scope, not byte-identical normal-client invocation.

The SDK owns the receiver directly. Each connection obtains the executable and
complete argument slice from `installer.PrepareStableCheckLaunch` and starts
that exact `installed-check-serve` process, without an intermediate installer
process or reconstructed arguments. The receiver repeats its normal independent
admission. This preserves one process handle for cancellation, escalation and
waiting on both platforms. The participant checker handles normal interrupt and
termination signals through cancellation so cleanup and the incomplete report,
including any known task identity, can finish. Forced process termination is
not a recoverable signal contract.

A separate explicit `--functional` invocation, selected by one action in both
native menus, may perform a bounded synthetic functional check. It reuses the
existing canonical DOCX fixture owner, preserving any conflicting or modified
file. It uses only normal MCP conversion/readback and execution-ledger
operations: verify the admitted runtime, convert once, reconstruct both outputs,
verify content identity/hashes, create/checkpoint/pause one synthetic task, reopen
the same stable carrier, resume once, verify artifact evidence and complete. A
stale-attempt checkpoint must refuse without changing the current projection.
There is no automatic replay of an ambiguous mutation. On failure, retain the
first diagnostic and known item identity; never hide an unfinished task.

The functional invocation installs no runtime and does not change format policy,
source, roles, private signing material, provider settings or authority. An
unavailable converter remains unavailable, with an incomplete result. It never
substitutes memory capture, knowledge/brain authorship, owner review, maintenance
qualification/enrollment or model execution. The default refresh remains
read-only; synthetic writes require the explicit functional action.

Private diagnostics and the existing shareable Markdown report expose distinct
per-stage results. Shareable output contains no document bodies, prompts,
credentials or private owner text. The observed scope is installed MCP/SDK, not
a Code/Desktop native session, hook, agent, memory, scheduler, platform or rollout
qualification. Existing detailed client acceptance remains available and
independent. A new compatible kit and actual installed acceptance are required
before delivering this new behavior; older frozen packages retain their meaning.

Refs: #655, #470, MPCK, DCHK.
