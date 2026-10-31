# Spec 057 — Product MCP execution workflow

Status: accepted for implementation under the full-construction instruction.
Scope: E1 of the [product construction plan](../docs/plans/2026-10-31-maestro-product-mcp.md).
The complete parity inventory remains open until its own acceptance gates pass.

## Observable result

A participant explicitly initializes a new private home and workspace, then uses
Code and Desktop against the same canonical execution ledger without a synthetic
marker. The participant creates a work contract, starts, checkpoints, pauses,
resumes in another process, gathers artifact evidence and completes. A stale
writer and changed artifact cannot commit. Package updates preserve external state.

## State and authority

Product, owner HOME and workspace must be distinct, selected locally and validated.
Roots and workspace identity never come from a tool call. Startup cannot initialize
or migrate implicitly. An unconfirmed or conflicting origin, unsafe filesystem
path or unverified Windows ownership must not grant mutation authority. Fresh
bootstrap cannot overwrite an existing origin or locator; migrated origin remains
unavailable until its migration journal is verified. Existing spike behavior stays intact.

The ledger must bound reads and use descriptor-relative/no-follow file operations
for persistence and artifact evidence on admitted platforms. The product adapter
uses `execution.OpenConfinedStore` as an independent admission gate: Unix supports
the confined implementation; Windows remains unavailable until its implementation
and qualification are supplied. Existing Windows Store callers retain compatibility
and must not be represented as confined product admission. Keep revision/attempt validation, immutable
publication, locks and recovery. A check-then-open pathname is not the proof of
confinement needed by this adapter. Reuse the scheduler's existing secure primitives
where applicable; do not build a second ledger or copy source data into another store.

## MCP interface

The product module is `adapters/mcp`, separate from the development spike. It uses
the existing pinned official SDK and imports canonical runtime packages only.

- `maestro_context`: read-only binding/capability metadata, no blanket content export.
- `maestro_query`: closed read operations `execution.list`, `execution.next` and
  `execution.contract`. Metadata lists are paged/bounded; continuation uses the
  canonical bounded projection. Contract lookup exposes only the bounded artifact
  criterion IDs and relative paths needed by a fresh client to collect evidence;
  it never exports raw contracts, command criteria or file content.
- `maestro_apply`: closed variants `execution.create`, `execution.start`,
  `execution.checkpoint`, `execution.pause`, `execution.resume`,
  `execution.evidence`, `execution.complete`. Each variant has its own required and
  allowed fields; unknown fields and cross-variant combinations are rejected.

The adapter supplies WorkspaceID/WorkspaceRoot, producer identity and policy.
Mutation outputs are metadata receipts; errors are bounded codes without private
paths or user content. Evidence is initially artifact_snapshot only, never arbitrary
commands, self-asserted approval or raw export/delete authorization. Read and mutation
annotations must be accurate; no read operation performs bootstrap or mutation.

## Public test interfaces and acceptance

Use existing public ledger calls, the product CLI and SDK ClientSession.CallTool.
Fixtures live in private temporary directories under Developer; tests invoke no model.

1. New explicit bootstrap and registered workspace bind without a synthetic marker.
2. Two independent MCP sessions/processes create/start/checkpoint/pause/resume the
   same item; old revision/attempt and foreign-workspace item IDs fail without mutation.
3. Evidence and completion go through the canonical engine; changing the artifact
   after evidence prevents completion. No command execution is exposed.
4. Unknown fields/operations, oversized payloads and unsafe/private paths are rejected.
   Symlink/reparse, oversize metadata and concurrent-writer regressions use public calls.
5. Product tree remains read-only; external state survives package version changes.
   Init rejects overlapping or symlinked bindings before creating owner/workspace
   state. A fresh process learns artifact criterion IDs and relative paths solely
   from MCP responses before collecting evidence.
6. Plugin and MCPB point to the same participant binding and contain no factory
   fixture, credentials, real client content or new authority.
7. Focused deterministic tests and independent reviews pass before source integration.
   Model invocation, platform qualification, signed release and rollout are separate.

This spec does not silently close #433/#434, full memory/provider/agent parity or
native client qualification. E2–E7 remain implementation work, not optional follow-up
for missing parts of the E1 workflow.
