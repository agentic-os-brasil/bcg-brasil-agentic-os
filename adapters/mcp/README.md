# Maestro product MCP — execution, owner onboarding and methods

This module exposes one locally admitted workspace and canonical owner state
through the official Go MCP SDK. `maestro_context` returns binding metadata;
`maestro_query` provides bounded domain reads and continuation; `maestro_apply`
accepts closed canonical effects with the required trusted review. Execution,
owner onboarding, memory and workspace knowledge use the same admitted binding. See
the [owner journey and local fallback guide](../../docs/product-mcp-owner.md)
for exact operations. Roots and workspace identity are selected locally.

The modern signed factory and explicit reviewed first install are described in
the [installation guide](../../docs/product-install.md). They materialize complete
participant-local packets after genuine admission. Their engineering source
evidence does not establish native signing, client registration/loading or the
still-pending modern update lifecycle.

Reviewed memory adds capture, paged discovery/search/readback, bounded context,
daily/weekly consolidation and revision-aware correction/deletion/retention.
Read the [memory journey and local fallback guide](../../docs/product-mcp-memory.md).
Explicit `init` prepares its fixed authority; queries/startup never bootstrap it.

Qualified maintenance adds exact-job qualification, separate recurring enrollment,
foreground due execution, bounded reports and revision-aware pause/resume/revoke.
Read the [maintenance workflow and operator fallback](../../docs/product-mcp-maintenance.md)
when inspecting availability, authorizing a job or recovering a refused transition.
Eight deterministic bindings execute through one canonical service; the remaining
catalog jobs and native background scheduling retain their separate gates.

Local document conversion adds immutable scoped artifacts, bounded UTF-8 readback
and reviewed workspace knowledge creation/append through the canonical atlas
engine. Read the [content journey and local fallback guide](../../docs/product-mcp-content.md).
The initial authenticated runtime capability is Darwin-arm64 DOCX; unseeded or
unsupported runtime inputs remain unavailable without blocking unrelated work.

Read the [native plugin operator guide](../../docs/product-native-plugin.md) for
explicit native-review and native-qualify-and-build commands, actual target
binding, data/cost review and complete packet inventories. Plain packaging
remains no-probe and MCP-only without actual host admission; MCPB carries the
shared server/resources without Code agents/hooks or Desktop skill installation.

Build from this directory with `go build -o /absolute/local/path/maestro-mcp ./cmd/maestro-mcp`.
The installed product directory and owner directory must already exist. Choose a
new workspace path outside the product and private state. Run the binary explicitly:

```sh
maestro-mcp init --product-root /installed/product --user-home /owner --workspace /work/project
maestro-mcp inspect --product-root /installed/product --user-home /owner --workspace /work/project
maestro-mcp serve --product-root /installed/product --user-home /owner --workspace /work/project
```

`init` validates all selected paths and their separation before writing, then confirms only a fresh empty home and initializes the selected workspace
through canonical engines, prepares canonical owner metadata and verifies the
admitted binding before reporting ready. Already committed external migrations
remain admitted without replacing their origin. `inspect` reads the home diagnostic without bootstrap.
A partial bootstrap preserves its files: retain the home/locator and inspect before
support-assisted recovery. Startup and tool calls never initialize, migrate or repair.
Pending, incomplete or tampered migrations and unverified Windows
ownership/storage remain unavailable. Use the
[migration runbook](../../docs/external-state-migration.md) for explicit migration.

Use `plugin` or `mcpb` instead of `serve` with the same three binding flags, plus
`--server /absolute/path/maestro-mcp --destination /new/local/packet --version 0.1.0`.
For MCPB, the destination ends in `.mcpb`. Both packets bind the same owner locator
and workspace. Build version N+1 to a new destination; external state is neither
copied nor overwritten. Local packets contain participant paths and stay local.
The shared source has no participant state. The builders do not install or change
host profiles, authentication, model settings or credentials.

The Code plugin includes all 92 canonical methods and two operator/resume aliases.
MCP exposes 92 prompts and 261 immutable resources: 247 method/policy/provenance/
license resources and fourteen agent assets.
Method availability is distinct from functional/native parity; check the adapter
policy and actual operations before use. The Desktop MCPB contains a native executable and
manifest; it does not install Skills. The recipes preserve host authority and owner
scope. Execution evidence uses artifact snapshots. Private profile text reaches
only trusted host forms or an explicit local operator. No model-supplied approval
flag, producer credential, root selection or process execution is accepted.

Deterministic tests use `ClientSession.CallTool` over memory and the compiled CLI's
stdio transport, then build N/N+1 packets against the same external ledger. Set
TMPDIR and GOTMPDIR under Developer before testing. Native host/model acceptance,
Windows qualification, organizational signing, release and rollout remain separate.
