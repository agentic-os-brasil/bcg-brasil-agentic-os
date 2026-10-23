# Observed architecture verdict — 2026-10-22

The local composition works at the protocol, execution-ledger and plugin-loading
layers. An enabled Claude plugin can supply its own native MCP server while
ordinary files preserve Maestro execution state outside the plugin. Model-driven
use of the operator skills remains unqualified because CLI authentication was
absent. This verdict is limited to a synthetic macOS arm64 experiment.

| Observation | Result | Evidence boundary |
|---|---|---|
| Real MCP discovery and three tools | PASS | Official Go SDK v1.8.0, stdio subprocess, protocol 2026-07-28 |
| Checkpoint, pause, process restart and resume | PASS | Existing execution.Store; restored checkpoint and a new attempt |
| Stale attempt/revision and override rejection | PASS | Nine root/workspace/item overrides, oversized input and cross-case attempt rejected without mutation |
| Plugin V1/V2 independence | PASS | Each packaged executable/config read the same checkpoint; state and product snapshots unchanged |
| Native plugin validation | PASS | CLI 2.1.233; optional author attribution warning |
| Native plugin-provided MCP | CONNECTED | Actual CLI health check with final fixture, isolated profile |
| Native SessionStart and skills | PASS | Same-code V1 startup: orientation exit 0, two skills registered, MCP connected |
| Model orchestration | BLOCKED | Missing login; zero model cost and API duration |
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
