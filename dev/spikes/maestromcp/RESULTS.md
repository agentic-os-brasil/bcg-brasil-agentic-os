# Observed architecture verdict — 2026-10-23

The local composition now works through authenticated native Code and Desktop
clients: Code saved and paused a checkpoint, Desktop resumed it, and the original
Code conversation received `revision_conflict` when it tried to write with its
old revision and attempt. Files preserved the same checkpoint outside both
packages. This verdict is limited to an attended synthetic macOS arm64 experiment.
The dated observations below preserve the earlier protocol and SDK evidence.

| Observation | Result | Evidence boundary |
|---|---|---|
| Real MCP discovery and three tools | PASS | Official Go SDK v1.8.0, stdio subprocess, protocol 2026-07-28 |
| Checkpoint, pause, process restart and resume | PASS | Existing execution.Store; restored checkpoint and a new attempt |
| Stale attempt/revision and override rejection | PASS | Nine root/workspace/item overrides, oversized input and cross-case attempt rejected without mutation |
| Plugin V1/V2 independence | PASS | Each packaged executable/config read the same checkpoint; state and product snapshots unchanged |
| Native plugin validation | PASS | CLI 2.1.233; optional author attribution warning |
| Native plugin-provided MCP | CONNECTED | Actual CLI health check with final fixture, isolated profile |
| Native SessionStart and skills | PASS | Same-code V1 startup: orientation exit 0, two skills registered, MCP connected |
| Model orchestration on 2026-10-22 | BLOCKED then | Superseded by the native qualification below |
| Code tab, Windows and host-managed update | NOT_RUN | Separate qualification remains required |

The final integrated rehearsal passed all 13 observations after the review
correction that derives Developer from the current user rather than a
machine-specific path. The fixture was new; earlier receipts were preserved.
See [SDK receipt](evidence/sdk-rehearsal.json) and
[native host receipt](evidence/claude-host.json). Local paths are sanitized in
these committed copies; the original receipts remain with the local demo.

The context tool exposes the canonical bounded execution-ledger projection,
not L1/L2/L3 memory. Checkpoint/pause are two existing ledger commits; partial
failure is surfaced. The experiment does not add a memory engine, claim
host-wide filesystem confinement, or prove automatic marketplace updates.
Its root checks protect synthetic fixtures, not hostile concurrent filesystem
replacement. Plugin instructions are guidance rather than a security sandbox.

Focused checks covered the spike/server packages, plugin builder and relevant
vet targets, followed by the integrated subprocess rehearsal and native host
observations. No full repository suite or authenticated model workflow ran.

## Local Desktop connector increment — 2026-10-22

The extension builder now emits a two-file `.mcpb` archive containing a manifest
and the same native executable bundled by the Code plugin. It validates the
synthetic fixture and a macOS Mach-O input before creating a new artifact.
The builder writes no host configuration or execution state.

The new integrated rehearsal passed all eight observations using independent
SDK subprocesses launched from the actual plugin and extension manifests:

| Observation | Result |
|---|---|
| Source, plugin and extension binaries have identical SHA; initial packaging preserves state | PASS |
| Plugin adapter discovers the three MCP tools | PASS |
| Desktop adapter reads the exact submitted checkpoint at paused revision 4 | PASS |
| Resume creates a new attempt at revision 5 and preserves the handoff | PASS |
| Previous Code revision and attempt are rejected without mutation | PASS |
| Two live processes contend revision 5; one commit advances to revision 6 | PASS; loser returns `item_busy` |
| Extension V2 reconnect preserves ledger, earlier commits and package snapshots | PASS |
| Generated artifacts and bounded evidence retained outside packages/state | PASS |

The race requires an MCP tool rejection with a bounded `revision_conflict` or
`item_busy` code. A transport failure, disconnected process or unrelated tool
error fails the rehearsal. Both sessions remain readable, exactly one immutable
revision is added and earlier commits remain unchanged. Checkpoint/pause still
uses two existing Store commits and retains its explicit partial-failure limit.

Both V1/V2 manifests passed the official `@anthropic-ai/mcpb@2.1.2` validator.
The official CLI also unpacked V1 with identical binary bytes and executable
permission. A fresh isolated Claude Code 2.1.233 profile validated the plugin
and reported its MCP connected. This health observation made no model call.

Focused tests passed for `spike`, `build-plugin`, `build-extension` and
`rehearse-desktop`; vet passed for those packages and both server/rehearsal
commands. Test-first evidence includes a failing behavioral conflict/busy
classification test and a failing non-macOS-binary rejection test before the
respective implementations. Initial package/helper tests first failed at
compilation because the new entry points did not yet exist.

The Desktop application was present at version 3.1623.0; installation of this
extension and native Desktop tool/model use were not run. The CLI authentication
check remained logged out. Model-driven two-host continuity, host prompts,
installation/removal, automatic updates and platform signing remain unqualified.
The SDK proof does not establish that Desktop runs the Code plugin's hooks or
skills. No production migration, owner data or global profile was changed.

See [Desktop SDK receipt](evidence/desktop-sdk-rehearsal.json),
[official validation summary](evidence/desktop-validation.json),
[fresh Code health receipt](evidence/desktop-code-health.json) and
[reproduction instructions](DESKTOP.md#run-the-focused-rehearsal).

## Authenticated native continuity — 2026-10-23

Claude Code 2.1.233 loaded the plugin in an authenticated isolated profile.
Claude Desktop 3.1623.0 installed and enabled the local extension, then manually
updated it from 0.0.1 to 0.0.2. All 63 external synthetic files remained
byte-identical across that update. Both packages used the same server binary.

| Native observation | Result | Evidence |
|---|---|---|
| Code operator reads context, saves exact checkpoint and pauses | PASS action | Matched tool-use/result; revision 2 to paused revision 4 |
| Desktop reads paused revision 4 and resumes exactly once | PASS | Attended user tool transcript, extension call log and canonical ledger; running revision 5, new attempt, same checkpoint |
| Original Code conversation submits its old revision and attempt | PASS | Resumed same CLI session; single rejected call, exact `revision_conflict` tool error |
| Code reads context after rejection | PASS | Running revision 5 and preserved handoff; all state file hashes unchanged |
| Manual native extension update | PASS | 0.0.1 to 0.0.2; external state unchanged |

The exact checkpoint summary was `MAESTRO_NATIVE_CODE_TO_DESKTOP_2026_10_23`.
Its next step was `Retomar o mesmo caso no Claude Desktop pela extensão local.`
The Desktop log contains request IDs and call/result timestamps rather than
argument/result bodies. Its content attribution relies on the attended user
transcript plus the ledger. A log offset captured after the calls was not used
as proof of absence. Raw traces, account/session identity, machine paths and
screenshots remain local; the [public receipt](evidence/native-continuity.json)
contains an allowlisted summary. Assistant prose is not the state authority.

The first Code action succeeded, but that CLI invocation exited with
`error_max_budget_usd` after reporting an estimate of USD 0.964737 against an
initial USD 0.25 per-run limit. Calls were suspended until the user explicitly
removed the budget restriction. The final stale-writer call exited successfully
and reported USD 2.01502215. These are separate CLI estimates, not verified
billing; Desktop cost is unattested. An `unrecognized_model` warning also limits
pricing confidence. Preserve the incident rather than marking the first CLI
invocation successful. The documented budget check happens after responses and
can overshoot: [budget headroom](https://code.claude.com/docs/en/agent-sdk/agent-loop#budget-headroom).

The experiment now proves native local continuity across the two clients.
It leaves the Desktop Code tab, Windows, remote transport/OAuth, full memory and
skill catalog, marketplace automatic updates, removal, platform signing,
corporate/shared-folder admission, migration, pilot and rollout unqualified.
No additional product code changed for this attended qualification.

## Experimental team distribution — 2026-10-23

The synthetic experiment now has a native publisher and participant installer.
A typed Ed25519-signed feed binds target, version, monotonic sequence and the
sizes/hashes of exactly two artifacts. The participant installer pins its
public key independently, verifies a payload snapshot and materializes local
plugin/MCPB generations bound to that participant's synthetic state. The local
marketplace is promoted last; old generations and state remain available.

Two separate proofs passed on macOS arm64:

| Proof | Observations | Boundary |
|---|---|---|
| [Real-binary SDK rehearsal](evidence/team-distribution-rehearsal.json) | Eight PASS: install, checkpoint/pause, update, no-op, resume, stale writer, damaged payload and replay | Uses the Apply library and an ephemeral key; does not execute the installer or install in either Claude host |
| [Native installer and Code CLI](evidence/team-distribution-cli.json) | Actual pinned installer ran install/update, official CLI commands succeeded, installed plugin read back as 0.0.4, all state hashes unchanged | Isolated Code profile; no model call, Desktop installation or real SharePoint synchronization |

Focused tests cover signed-feed/path validation, mutation, incomplete sync,
replay, state preservation, package identity, native binary parsing and launcher
arguments. A publisher symlink-escape regression failed before the correction
and passed after ordinary-directory confinement was applied. The publisher
stages artifacts under Developer; upload/copy to the selected library remains
an external delivery operation. No library has been selected by this code.

The generated entry points are Instalar Maestro and Atualizar Maestro. Code
activation uses supported CLI commands and reports command success separately
from host execution. Desktop opening remains `DESKTOP_CONFIRMATION_REQUIRED`.
The first installer needs independently checked provenance; feed signatures
cannot authenticate the executable that first performs verification.

Builds exist for macOS ARM64/AMD64 and Windows AMD64. Only macOS ARM64 has the
native installer/CLI proof above. The other builds and header tests do not
qualify native host behavior. Platform signing/notarization/Authenticode are
unattested; the experimental key grants no production authority. Actual
SharePoint upload/hydration, clean team devices, Desktop confirmation and
full Maestro product admission remain pending. See [TEAM-PILOT](TEAM-PILOT.md).
