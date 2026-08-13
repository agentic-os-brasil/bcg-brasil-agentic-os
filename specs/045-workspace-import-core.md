# Spec 045 - Workspace Import Core

Status: accepted; local contract implementation in progress.

## Objective

Provide a bounded, fail-closed path for bringing an external workspace into an
existing Maestro workspace without treating that source as a document-ingestion
feed or as authority to migrate a native workspace.

## Source classification

Every read-only inspection reports exactly one of:

- `maestro_native` — current `.bcgos` metadata is present. Native workspace
  migration is unsupported and the plan cannot be approved.
- `maestro_legacy` — a recognized pre-current Maestro marker is present.
- `kowalski` — a recognized Kowalski marker is present.
- `foreign` — a readable directory without a recognized product marker.
- `unsupported` — the source is not a safe readable directory (including a
  source symlink) or cannot satisfy the bounded inspection contract.

Classification uses only bounded filesystem metadata and marker names. It never
opens source file bodies during inspection.

## Inspection and plan

Inspection is read-only and bounded by entry count, depth, per-file bytes and
total bytes. It records relative paths, kind, size, mode and modification
metadata. Symlinks and special files are recorded as unsafe and never followed.
The source and destination are separate non-symlink directories. Each planned
regular file also receives a bounded SHA-256 content digest; the plan still
contains no source body.

`plan` derives a sorted, immutable origin-to-destination plan with a SHA-256
digest over the complete canonical plan body. Existing destination paths are
conflicts and cannot be approved. Managed runtime directories, symlinks and
special files are exclusions. Plans do not contain source content.

The import surface is not document ingestion. Plain, allowlisted workspace
text can be copied as an explicit migration action. Office/PDF/archive formats
that require Docling or an optional fallback adapter are marked `unavailable` and quarantined;
other unallowlisted formats are marked `unsupported` and quarantined. No
runtime pack is inferred or activated by this feature.

## Approval and execution

Approval binds the plan ID and digest and requires the exact explicit
confirmation token `IMPORT`. Execution revalidates the plan, source metadata
and content digests, and opens source components through no-follow directory
descriptors so parent symlink swaps fail closed. It stages regular-file copies
under the private Maestro data root, then atomically renames them into the
destination. The source is never modified.

Before any destination commit, execution writes a durable metadata-only journal
of every intended stage-to-destination move. A per-plan lock prevents concurrent
executions. After a crash, the journal reconciles staged files and destination
digests before producing the terminal receipt. Execution writes a receipt
containing IDs, digests, relative paths and state. A repeated execution of the
same plan returns the existing receipt only after strict identity, path and
digest validation.
Quarantined files land under a run-specific `.bcgos/import-quarantine` path and
remain clearly unavailable/unsupported; they are not converted or indexed.
Failure removes files committed by the incomplete transaction. Explicit
rollback requires the original plan, receipt and the exact `ROLLBACK` token,
and removes only paths created by that receipt after verifying their recorded
content digest. Changed destination files are preserved and rollback fails
closed. Staging and commit failures are persisted as `failed` receipts.

## Boundaries and evidence

This contract does not migrate native workspaces, collect remote sources,
invoke Claude/Codex, install Docling or any optional ingestion adapter, or claim native runtime
qualification. Tests use synthetic temporary fixtures only. Full harness,
hosted CI, human review, signing and production data-policy approval remain
separate evidence gates.
