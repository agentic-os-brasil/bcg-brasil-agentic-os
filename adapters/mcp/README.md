# Maestro product MCP — execution, owner onboarding and methods

This module exposes one admitted workspace ledger through the official Go MCP SDK.
`maestro_context` returns binding metadata, `maestro_query` pages execution metadata
reads a 2048-byte canonical continuation, or discovers bounded artifact criterion IDs and relative paths with execution.contract, and `maestro_apply` accepts seven
closed execution variants. Owner status/interview/review metadata and reviewed
track/proposal/apply/onboarding operations use the same admitted binding. See
the [owner journey and local fallback guide](../../docs/product-mcp-owner.md)
for exact operations. Roots and workspace identity are selected locally.

Reviewed memory adds capture, paged discovery/search/readback, bounded context,
daily/weekly consolidation and revision-aware correction/deletion/retention.
Read the [memory journey and local fallback guide](../../docs/product-mcp-memory.md).
Explicit `init` prepares its fixed authority; queries/startup never bootstrap it.

Local document operations add verified per-format coverage, explicit conversion and bounded artifact
readback. The model-free Apple Silicon profile admits eleven existing policy
extensions only after its exact 51-input pack verifies; old/unseeded/unsupported
profiles remain unavailable. See the [format matrix](../../docs/product-mcp-content.md). Knowledge creation and append require exact trusted review against that
artifact; named collection remains bounded. See the
[document knowledge guide](../../docs/product-document-knowledge.md) for the closed
operations, local operator fallback and qualification boundaries.

Ordinary authored workspace pages use the nine closed `brain.*` operations without
a converted-document artifact. Fixed-template preparation and create/append/update/
field/link changes require exact trusted review and canonical page revisions.
Read the [authored workspace guide](../../docs/product-authored-brain.md); method
selection does not qualify native Windows, host, provider or index compilation.

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
ownership/storage remain unavailable. The native private-state implementation and its
four mandatory ordinary-user proof families are described in the
[Windows engine contract](../../docs/product-windows-private-state.md). Use the
[migration runbook](../../docs/external-state-migration.md) for explicit migration.

Use `plugin` or `mcpb` instead of `serve` with an admitted build-time binding,
plus `--server /absolute/path/maestro-mcp --destination /new/local/packet --version 0.1.0`.
For MCPB the destination ends in `.mcpb`. Both builders emit portable, unbound
managed assets: the builder's owner, workspace, locator, memory and credentials
are neither included nor captured in server arguments. Client configuration asks
for Owner directory and Workspace; each participant chooses their own external
paths. See [portable packet preparation](../../docs/product-runnable-packets.md)
for exact emitted commands, integrity checks and the client qualification boundary.
Windows packet proof extends the existing ordinary-user compiled public workflow;
its four required format/version outcomes remain separate from real client loading.
Build N+1 to a new destination; existing/newer packets and external state survive.
Builders do not install or change host profiles, authentication or model settings.

The Code plugin includes all 92 canonical methods and two operator/resume aliases.
MCP exposes 92 prompts and 248 immutable method/policy/provenance/license resources.
Method availability is distinct from functional/native parity; check the adapter
policy and actual operations before use. Both formats carry the native executable, six canonical runtime/memory files and
complete method resources. The Desktop MCPB does not install native Skills. The recipes preserve host authority and owner
scope. Execution evidence uses artifact snapshots. Private profile text reaches
only trusted host forms or an explicit local operator. No model-supplied approval
flag, producer credential, root selection or process execution is accepted.

Deterministic tests use `ClientSession.CallTool` over memory and the compiled CLI's
stdio transport, then build N/N+1 packets against the same external ledger. Set
TMPDIR and GOTMPDIR under Developer before testing. Native host/model acceptance,
Windows qualification, organizational signing, release and rollout remain separate.

## Reviewed maintenance lifecycle

`maintenance.status` and `maintenance.report` are read-only. `maintenance.qualify`, `maintenance.enroll`, `maintenance.pause`, `maintenance.resume` and `maintenance.revoke` require trusted exact review and the current revision. `maintenance.run` dispatches enrolled due work; it neither installs a service nor attests host wake support. See [the owning guide](../../docs/product-mcp-maintenance.md) for the finite eight-job catalog and local stdin fallback. Private wiki queries use the separate source-owned navigation journey below.

## Private workspace-memory navigation

`wiki.status`, `wiki.read` and `wiki.source` are bounded read-only variants of `maestro_query`. Use the exact returned generation and references; queries never compile or repair. Reviewed maintenance qualifies and enrolls `wiki-incremental-sync`, `wiki-reconcile` and `wiki-integrity-check` separately. See [the owning guide](../../docs/product-mcp-wiki.md) for source eligibility, revocation and recovery. Native Windows wiki requires actual owner/pinned-namespace admission; exact-source ordinary-account execution and real client loading remain separate evidence.
