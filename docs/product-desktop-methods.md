# Desktop method instruction frontend

The offline factory emits a public instruction/resource plugin with the fixed
identity `maestro-desktop-methods`. It can coexist structurally with the
`maestro-product` Code plugin and a participant-local Desktop extension. Actual
account synchronization, discovery and selected-host coexistence remain separate
acceptance checks. See decision DMFP and Specs 063/065.

From the `adapters/mcp` module, run the factory with a new absolute output path
whose existing parent uses its canonical filesystem path:

```sh
go run ./cmd/maestro-method-frontend --destination /absolute/new/frontend-output --version 0.1.48
```

This is a factory command, separate from the installed backend. It needs neither
a server binary nor product, owner or workspace selections. It creates a fresh
directory and preserves an existing target. The directory contains:

- `maestro-desktop-methods/`: 92 canonical methods, the `operator`/`resume` entry
  skills, all nested method resources, canonical agent reference documents,
  shared policy/contracts/index, licenses and upstream provenance.
- `maestro-desktop-methods-VERSION.zip`: the same sorted public files with fixed
  archive metadata and regular non-executable file modes.
- `inventory.json`, `provenance.json` and `SHA256SUMS`: exact file hashes, byte
  counts, instruction-only scope and archive integrity. The inventory excludes
  itself and these factory sidecars; it covers every file inside the ZIP.

The renderer qualifies method invocations and explicit Agent instruction
references with the fixed frontend namespace using the shared parser. Canonical
IDs, frontmatter, policy, URI contracts, licenses and non-Markdown resources stay
unchanged. `methods-index.json` records canonical source hashes;
`methods-projection.json` records distributed resource hashes after projection.
The Code/default renderers and native identity admission remain unchanged.

The frontend includes no MCP configuration, hooks, native agents, executables,
managed runtime/memory, private state/bindings or participant proof. Agent
reference documents are resources, not native runners. Namespaced instructions
do not grant native-role recognition or execution authority. Complete delivery
of 92 recipes does not prove 92 functional host journeys; methods that require
subagents or other unavailable host dependencies remain unavailable.

Before any upload, inspect the exact artifact and its hashes. Account upload,
installation and activation are separate actions. Reuse the separately admitted
local extension only when the chosen host exposes it; confirm its workspace ID
and read the complete shared method authority and agent-skill policy. Onboarding
and protected changes require successful actual Desktop review through the
existing admitted channel. Command-line tests cannot establish graphical owner
review. Stop when the connector, scope or trusted review is unavailable.

Chat and Cowork have different supported components. This instruction carrier
does not choose a surface or qualify hooks, delegation, scheduled execution or
full mission parity. Keep the experimental backend and its activation evidence
separate; generating these instructions requires no backend release update.
