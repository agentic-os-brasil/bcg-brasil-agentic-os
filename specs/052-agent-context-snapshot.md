# Spec 052 - Agent context snapshot

Status: contract and deterministic engine slice; runtime hook wiring, Session
Start injection, CLI surface and any model-backed semantic compaction are
deferred to later slices.

## Objective

Give each agent a short, bounded, prose-shaped operational note that persists
between its own invocations inside the same workspace, and is injected as
additional context on the next invocation of that agent. The snapshot is not a
new memory layer for the workspace and not a substitute for orchestration
breadcrumbs: it is the smallest useful residue from the last operation of one
agent, prepared by that agent for itself.

## Distinction from existing layers

- Spec 006 (memory persistence) owns workspace-scoped continuity across L1, L2,
  L3 and lifetime, produced by deterministic dreaming. The agent context
  snapshot is agent-scoped and never becomes a memory layer.
- Spec 047 (agent breadcrumbs) keeps a metadata-only, hash-linked tail for
  recovery and diagnosis; breadcrumbs are deliberately not injectable prose.
  The snapshot is prose-shaped and deliberately injectable.
- Owner Self and Owner Context remain the sole authoritative surfaces for
  owner-level facets. The snapshot is not owner state.

## Storage

Snapshots live under the same workspace boundary as memory, isolated by agent
identity:

```text
workspaces/<workspace-id>/agents/<agent-id>/
  state.md                                    (currently active snapshot pointer)
  versions/<transaction-id>/state.md          (immutable versioned body)
  commits/<timestamp>-<transaction-id>.json   (atomic activation manifest)
```

Activation follows the Spec 006 pattern: an immutable version is written first,
then a commit manifest is atomically renamed into place. Readers resolve the
snapshot only from the newest fully valid commit for the exact `(workspace,
agent)` pair. A corrupt or incomplete newest commit is ignored in favor of the
previous fully valid commit. Missing snapshots are reported as `empty` rather
than as a silent fallback to another agent, another workspace or a raw body.

The active `state.md` file is a convenience projection of the latest committed
version and must never be written directly by a runtime; the engine controls
it. Snapshot files never leave the user-local workspace boundary and are never
copied into the managed bundle. Workspace isolation and agent isolation are
independent invariants: no snapshot read for agent A in workspace W may return
data written for agent B, and no snapshot read for workspace W may return data
written for another workspace, even if the agent identity matches.

## Bounds

The snapshot body is capped by a rune budget with a bundled default of `2048`
runes. The budget is managed configuration, not an engine constant, and remains
reviewable. A `SnapshotUpdate` whose combined body would exceed the budget
triggers deterministic compaction before activation.

## Update trigger

The snapshot is refreshed by an authenticated post-invocation update from the
same agent that owns the snapshot. Updates carry:

- `AgentID` and `WorkspaceID`, both validated structurally;
- `Timestamp` with a monotonic guarantee against the last active commit;
- `SectionLabel`, a short structured tag naming the semantic slice this update
  represents (for example `last_action`, `open_question`, `handoff_note`);
- `Body`, the prose payload for that section, sanitized by the producing agent;
- `SourceDigest`, a one-way digest of the evidence that led the agent to
  attest this body (never the raw evidence).

Updates never carry raw prompts, raw tool outputs, credentials, client-
identifying content or unattested prose. Bodies that are structurally
identical to the currently active body for the same section are recognized as
idempotent no-ops rather than as new versions.

## Semantic compaction

Compaction is deterministic-first: when a new update would push the composite
snapshot past the rune budget, the engine drops whole oldest sections in order
until the body fits, preserving section boundaries and never truncating mid-
section. Compaction is a workspace-local, agent-local operation and never
mutates memory, breadcrumbs, owner state or another agent's snapshot.

A model-backed semantic compaction adapter is explicitly out of scope for this
spec. Any future adapter must be introduced through a new decision code and
must preserve the same observable snapshot contract, workspace-isolation and
agent-isolation invariants.

## Injection

At the start of the next invocation of the same agent within the same
workspace, the snapshot is appended as the most recent and narrowest layer
after the canonical broad-to-recent memory order:

```text
lifetime -> L3 -> L2 -> L1 -> agent-snapshot
```

The snapshot is injected only for the exact `(workspace, agent)` pair. Its
budget is independent from the memory layer budgets. A missing or invalid
snapshot is reported as an explicit diagnostic and never causes a fallback to
raw captures, another agent's snapshot or unbounded history.

Wiring the injection into Session Start and the per-runtime hook layer is
deferred to a later slice; this spec's engine exposes the assembly primitive
without changing existing injection behavior.

## Runtime portability

The snapshot contract, storage layout, update payload shape, compaction policy
and injection position are runtime-neutral. Claude and Codex adapters may use
different native lifecycle events to trigger updates, but they must preserve
the same observable invariants and the same authenticated update payload
structure.

## Storage and privacy invariants

- Snapshots are user-local, workspace-isolated and agent-isolated.
- Snapshots are never copied into the managed bundle or into releases.
- Raw prompts, tool outputs, credentials and client-identifying content are
  never valid snapshot bodies.
- Updates may migrate through versioned, reversible migrations but never
  replace snapshot state with bundle defaults.

## Test expectations for this slice

- structural validation of `SnapshotUpdate`, including workspace and agent
  identity;
- write/read roundtrip through the atomic commit path;
- idempotency when a repeated update matches the currently active body for the
  same section;
- deterministic compaction when a new update exceeds the rune budget;
- rejection of updates that omit `AgentID` or `WorkspaceID`;
- isolation between distinct agents inside the same workspace;
- isolation between distinct workspaces for the same agent identity.

## Deferred slices

- Claude and Codex runtime hooks that emit `SnapshotUpdate` after each
  invocation.
- Session Start integration that appends the snapshot to the injected context
  bundle.
- CLI surface for inspecting, exporting or clearing snapshots.
- Skill projection for the snapshot capability.
- Model-backed semantic compaction adapter, gated by a separate decision code.
