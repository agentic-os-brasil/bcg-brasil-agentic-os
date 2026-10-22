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
