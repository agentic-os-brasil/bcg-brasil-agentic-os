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
evidence includes target release/bundle, managed-root identity, an authenticated
core-state digest and the `stable-bootstrapper` authority label. A caller cannot
turn a boolean or an environment variable into this evidence.

The current `bcgos update` service can carry the migration contract in its
pending update plan, but cannot provide a post-bootstrap workspace target and
authenticated activation evidence. It therefore reports
`pending_core_activation` with execution `unavailable` and does not mutate a
workspace.

## Transaction

After confirmation, the manager:

1. rechecks workspace identity, source states and governed preflight rules;
2. snapshots only the bounded hook configuration, managed orientation block,
   projection manifest/policy and manifest-owned skill files;
3. applies `adaptercfg` and `runtimeprojection` through their conflict-safe
   ownership rules;
4. validates workspace readiness, installed hooks, projection integrity and
   routing inputs;
5. writes a metadata-only receipt after success.

Any error restores the snapshot. An `applying` execution marker is durable so a
subsequent recovery can restore an interrupted operation before another apply.
Snapshot limits are 128 files, 512 KiB per file and 4 MiB total. User-authored
files are never selected as managed targets; when a managed file contains an
authored prefix (for example `CLAUDE.md`/`AGENTS.md`), the complete bounded
file is snapshotted only to preserve it during rollback.

## Non-goals and readiness

This contract does not import external workspaces, enumerate arbitrary user
folders, choose an onboarding track, activate a runtime, or claim native hook
qualification. The manager is `transactional` when called with authenticated
bootstrapper evidence. Product update integration remains `unavailable` until
the bootstrapper supplies that evidence and an explicit workspace-selection
authority; a pending plan is not a completed migration.
