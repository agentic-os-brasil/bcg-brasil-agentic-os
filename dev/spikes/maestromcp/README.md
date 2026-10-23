# Maestro architecture experiment: plugin + MCP + filesystem

Development-only experiment reopened by the architecture question in epic
[#428](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/428).
It is separate from the v0.3.0 release scope and performs no migration.
For the observed verdict and sanitized receipts, read [RESULTS](RESULTS.md).
For the second-surface experiment using a local Desktop connector, read
[DESKTOP](DESKTOP.md).

The plugin supplies startup orientation and operator/resume skills. Its
`.mcp.json` starts a bundled native stdio server. Three MCP tools adapt the
existing `internal/execution.Store`: bounded context, fenced checkpoint/pause,
and resume. Ordinary files hold the ledger outside the plugin. This is the
existing Maestro execution model, not a second memory or dreaming engine.

| Component | Authority |
|---|---|
| Plugin | Instructions, discovery and native server packaging |
| Server launch | Fixed synthetic root and case; model arguments cannot choose them |
| MCP tools | Existing Store revisions and attempt fencing |
| Filesystem | Durable execution records independent of server lifetime and plugin revision |
| Rehearsal | Real SDK subprocess client, negative controls and file snapshots |

## Run the focused proof

This directory is a separate Go module; root product dependencies are unchanged.
Use Go 1.26+ and direct all caches and temporary output to your Developer tree.
Build the commands below to a fresh artifact directory outside the source tree:

```sh
go build -o <artifacts>/bin/maestro-mcp-spike ./cmd/server
go build -o <artifacts>/bin/build-plugin ./cmd/build-plugin
go build -o <artifacts>/bin/rehearse ./cmd/rehearse
```

The integrated runner seeds two synthetic cases, exercises a real stdio
connection, saves and pauses a checkpoint, restarts the process and resumes it.
It rejects stale attempts, cross-case overrides and oversized inputs. With
the plugin builder, it also loads the actual V1/V2 bundled binaries/configs and
checks that reads and product revision changes leave persistent state intact.
See `rehearse --help` for the exact new-directory flags.

The server's isolated setup is independently runnable:

```sh
maestro-mcp-spike seed --root <new-synthetic-root>
maestro-mcp-spike serve --root <synthetic-root> --workspace case-a
```

`seed` requires a new Developer-local directory. `serve` requires its synthetic
marker and bound workspace. Stdout is MCP protocol; diagnostics use stderr.
No credentials, endpoint, model account or remote MCP is needed for this proof.

## Native Claude observation

Validate the generated plugin and use a fresh Developer-local Claude profile
to observe its bundled server. Then, in an authenticated test host, invoke
`/maestro-architecture-spike:operator` and follow the skill; it calls context
before checkpointing. The resume skill handles the paused continuation.
Preserve ordinary permission prompts and record the actual namespaced tools.

Do not use `--bare`, safe mode or strict MCP mode with an empty explicit server
config for plugin discovery: those change what the host loads. Current docs
describe MCP validation features newer than the locally observed CLI 2.1.233;
validation success alone does not prove server discovery or model tool use.

## Interpreting the result

Protocol, persistence, tool authorization, plugin loading and model-driven
orchestration are separate observations. Tool-level case isolation does not
sandbox a host with Bash or broad filesystem access. The skill's instructions
are not an independent security boundary. This experiment does not qualify
Windows, marketplace auto-update, signing, production activation or deployment.

Keep experimental state and receipts outside the plugin; preserve the branch
and link its observed verdict from the epic before adopting any production
decision. Focused checks cover only this module and the reused execution path.

Primary references: [plugin MCP support](https://code.claude.com/docs/en/mcp#plugin-provided-mcp-servers),
[manifest and variable rules](https://code.claude.com/docs/en/plugins-reference),
[pinned official SDK](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0).
