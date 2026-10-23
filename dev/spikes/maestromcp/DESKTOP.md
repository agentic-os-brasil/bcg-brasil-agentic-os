# Local Desktop connector experiment

This increment adds a local MCPB extension around the same native server used
by the Code plugin. Its purpose is continuity across two adapters sharing one
synthetic execution ledger. It remains a development-only experiment; neither
package owns the durable state.

## Observable contract

The Code adapter saves a checkpoint and pauses an execution. The Desktop
adapter reads that exact checkpoint and resumes with a new attempt. A write
using the Code adapter's previous attempt or revision is rejected without
changing the ledger. Two independent processes contending for one revision
produce exactly one successful write and one conflict/busy result.

Package construction uses an explicit synthetic root and case. Model tool
arguments cannot change them. Generated archives contain an allowlisted
manifest and native executable; product files and state stay separate.
Rebuilding or replacing a package preserves the external execution ledger.

## Evidence layers

1. Focused tests verify package boundaries and safe handling of fresh outputs.
2. The official MCPB validator checks the generated manifest. This does not
   prove that Desktop installed or ran the extension.
3. The SDK rehearsal launches the actual packaged executables/configurations
   as independent stdio processes. It verifies continuity and concurrent
   mutations without invoking a Claude client or model.
4. An attended Code/Desktop run qualifies discovery, permission prompts and
   model tool use separately for each host. Record what actually ran.

Code plugin skills and SessionStart are Code capabilities. The local Desktop
extension exposes MCP tools; its installation does not establish execution
of the plugin's skills or hooks. Tool descriptions provide the bounded method:
read current context before mutation and use the returned revision/attempt.

## Scope

Use fresh synthetic fixtures under Developer, with packages and ledger in
separate sibling directories. Preserve host configuration and other connectors.
The tools' launch binding does not sandbox broader filesystem tools in a host.
This experiment adds no remote transport, OAuth, web/mobile access, production
migration or release qualification.

The terminology follows the official distinction between
[local Desktop extensions and remote connectors](https://support.claude.com/en/articles/11725091-when-to-use-desktop-and-web-connectors).
Packaging follows the [official MCPB manifest specification](https://github.com/anthropics/mcpb/blob/main/MANIFEST.md).
Observed results are recorded in [RESULTS.md](RESULTS.md).
The attended 2026-10-23 run completed Code checkpoint/pause, native Desktop
resume and stale-write rejection in the original Code conversation. Its
[sanitized receipt](evidence/native-continuity.json) distinguishes tool/ledger
evidence, manual extension update and the initial CLI budget failure.

## Run the focused rehearsal

From this nested Go module, build these commands to a fresh artifact directory
outside the repository. Direct Go caches and temporary files under Developer,
as required by the existing experiment; `GOTMPDIR` must be set for the fixture
tests.

```sh
go test ./spike ./cmd/build-plugin ./cmd/build-extension ./cmd/rehearse-desktop
go build -o <artifacts>/bin/maestro-mcp-spike ./cmd/server
go build -o <artifacts>/bin/build-plugin ./cmd/build-plugin
go build -o <artifacts>/bin/build-extension ./cmd/build-extension
go build -o <artifacts>/bin/rehearse-desktop ./cmd/rehearse-desktop
<artifacts>/bin/rehearse-desktop \
  --server <artifacts>/bin/maestro-mcp-spike \
  --plugin-builder <artifacts>/bin/build-plugin \
  --extension-builder <artifacts>/bin/build-extension \
  --destination <new-physical-Developer-directory>
```

The extension builder accepts a thin macOS ARM64 or AMD64 Mach-O executable.
The integrated extension rehearsal therefore requires a macOS server binary
that the machine can execute. Linux CI covers the focused contracts and vet;
it does not claim native extension execution.

The output contains `State`, `PluginV1`, two `.mcpb` archives and their extracted
packages, plus `rehearsal-desktop-receipt.json`. Existing destinations are
preserved. The receipt identifies SDK adapter observations and native-host
limits. Keep a separate fresh fixture for attended host testing so automated
checks cannot consume the intended starting checkpoint.

Validate each extracted `manifest.json` with the official pinned
`@anthropic-ai/mcpb@2.1.2` CLI. Keep that development validator outside the source
and distributed packages; a schema pass is its own evidence layer.
