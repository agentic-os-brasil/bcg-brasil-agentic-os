# Spec 045 - Versioned Maestro workspace migration

Status: transactional local contract implemented; post-bootstrap execution
authority is not activated.

## Objective

Update the small managed Maestro surface inside an existing workspace when the
managed core changes, without treating the operation as external import and
without touching authored workspace material. A workspace migration is a
separate transaction from the CLI/base-bundle update.

## State detection

The manager classifies a target workspace as:

- `valid`: current `.bcgos/workspace.json` identifies the canonical path and
  the readable brain surface is present;
- `legacy`: managed Maestro markers exist but current workspace metadata is
  absent, so identity cannot be upgraded implicitly;
- `incomplete`: current metadata is valid but the required workspace surface is
  missing;
- `invalid`: metadata is malformed, mismatched or unsafe.

Only `valid` workspaces may receive a migration plan that can execute. A
runtime projection or hook conflict is an execution blocker, not permission to
overwrite the file.

## Plan and authority boundary

The manager creates an immutable, content-addressed plan containing the
workspace identity, runtime, source states, expected projection/schema
versions, target release/bundle and bounded snapshot limits. The plan is stored
under owner-local update data and is never written into the workspace before
confirmation.

Confirmation is a separate record bound to the exact plan ID and to evidence
from the stable bootstrapper that the target core is already active. The
`CoreActivation` fields are currently only a wire shape: they are not an
authentication mechanism, and a caller cannot turn a boolean, digest-shaped
string or environment variable into trusted evidence. Until the bootstrapper
verifier and safe no-follow managed-target primitives are wired, the exported
`Confirm`, `Apply` and `Recover` surfaces fail closed; only the internal engine
is exercised by synthetic tests.

The current `bcgos update` service can carry the migration contract in its
pending update plan, but cannot provide a post-bootstrap workspace target and
authenticated activation evidence. It therefore reports
`pending_core_activation` with execution `unavailable` and does not mutate a
workspace.

## Transaction

When the internal engine is called behind the future trusted bootstrapper
boundary, the manager:

1. rechecks workspace identity, source states and governed preflight rules;
2. snapshots only the bounded hook configuration, Git local exclude side
   effect, managed orientation block, projection manifest/policy and the
   bounded union of manifest-owned existing and prospective skill files;
3. applies `adaptercfg` and `runtimeprojection` through their conflict-safe
   ownership rules;
4. validates workspace readiness, installed hooks, projection integrity and
   routing inputs;
5. writes a metadata-only receipt after success.

Any error restores the snapshot. An `applying` execution marker is durable so a
subsequent recovery can restore an interrupted operation before another apply.
Recovery binds the marker and snapshot to the canonical plan root, plan ID,
runtime, workspace and source digest. A terminal `applied` receipt wins over a
leftover marker and prevents recovery from undoing a completed migration; marker
removal is verified rather than ignored.
Snapshot limits are 128 files, 512 KiB per file and 4 MiB total. User-authored
files are never selected as managed targets; when a managed file contains an
authored prefix (for example `CLAUDE.md`/`AGENTS.md`), the complete bounded
file is snapshotted only to preserve it during rollback.

## Non-goals and readiness

This contract does not import external workspaces, enumerate arbitrary user
folders, choose an onboarding track, activate a runtime, or claim native hook
qualification. The internal engine is transactional under a future trusted
bootstrapper boundary, while public execution remains unavailable until that
verifier, safe target primitives and an explicit workspace-selection authority
are wired; a pending plan is not a completed migration.
