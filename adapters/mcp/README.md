# Maestro product MCP — E1 local execution

This module exposes one admitted workspace ledger through the official Go MCP SDK.
`maestro_context` returns binding metadata, `maestro_query` pages execution metadata
reads a 2048-byte canonical continuation, or discovers bounded artifact criterion IDs and relative paths with execution.contract, and `maestro_apply` accepts seven
closed execution variants. Roots and workspace identity are selected locally.

Build from this directory with `go build -o /absolute/local/path/maestro-mcp ./cmd/maestro-mcp`.
The installed product directory and owner directory must already exist. Choose a
new workspace path outside the product and private state. Run the binary explicitly:

```sh
maestro-mcp init --product-root /installed/product --user-home /owner --workspace /work/project
maestro-mcp inspect --product-root /installed/product --user-home /owner --workspace /work/project
maestro-mcp serve --product-root /installed/product --user-home /owner --workspace /work/project
```

`init` validates all selected paths and their separation before writing, then confirms only a fresh empty home and initializes the selected workspace
through canonical engines and verifies the admitted binding before reporting ready. `inspect` reads the home diagnostic without bootstrap.
A partial bootstrap preserves its files: retain the home/locator and inspect before
support-assisted recovery. Startup and tool calls never initialize, migrate or repair.
Migrated origins and unverified Windows ownership/storage remain unavailable.

Use `plugin` or `mcpb` instead of `serve` with the same three binding flags, plus
`--server /absolute/path/maestro-mcp --destination /new/local/packet --version 0.1.0`.
For MCPB, the destination ends in `.mcpb`. Both packets bind the same owner locator
and workspace. Build version N+1 to a new destination; external state is neither
copied nor overwritten. Local packets contain participant paths and stay local.
The shared source has no participant state. The builders do not install or change
host profiles, authentication, model settings or credentials.

The Code plugin includes the existing canonical Pocock methods, interaction profile,
and operator/resume recipes. The Desktop MCPB contains a native executable and
manifest; it does not install Skills. The recipes preserve host authority and owner
scope. They expose artifact snapshots only; no process execution, migration,
approval self-assertion, export or deletion tool exists.

Deterministic tests use `ClientSession.CallTool` over memory and the compiled CLI's
stdio transport, then build N/N+1 packets against the same external ledger. Set
TMPDIR and GOTMPDIR under Developer before testing. Native host/model acceptance,
Windows qualification, organizational signing, release and rollout remain separate.
