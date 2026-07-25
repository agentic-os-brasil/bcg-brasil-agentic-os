# Spec 018 - Execution Ledger V1

Status: accepted direction; initial contract and store in implementation.

## Objective

Prove one recoverable loop:

```text
create -> start -> checkpoint -> pause -> resume in another session or agent
       -> attach core-witnessed evidence -> complete
```

The execution contract, history and evidence stay in workspace-scoped local
storage. A model receives only a bounded projection and logical pointers.

## Authority boundary

An execution item is not a business task. V1 fixes `authority_kind` to
`local_execution` and does not store business priority, due date, owner or
external status. Future task-provider binding requires a separate accepted
decision, optimistic synchronization and an auditable receipt.

## Local layout

```text
<data-root>/workspaces/<workspace-id>/execution/
  items/<item-id>/
    contract.v1.json
    revisions/<revision>.json
    state.json
```

Each immutable revision atomically contains state, attempt and transition.
`state.json` is a regenerable projection and is never the completion authority.
Directories and files use user-private permissions. IDs are opaque and
validated before path construction. The CLI resolves an initialized workspace
manifest before the store receives its opaque workspace ID.

## Contract

The immutable contract contains:

- item and workspace IDs;
- `authority_kind=local_execution`;
- objective and initial next step;
- typed completion criteria;
- allowed logical references;
- schema and contract version;
- creation timestamp.

After `start`, a contract digest stored in state detects any replacement or
mutation. Changing a running contract requires cancellation and a new item.

## State and concurrency

V1 states are `ready`, `running`, `paused`, `evaluating`, `completed` and
`cancelled`. Every mutation requires the current `state_revision`. A mutation
publishes one complete immutable revision before refreshing projections.
Recovery ignores incomplete temporary files and selects the newest valid
revision. Immutable revision publication is no-clobber even if exclusion fails.
The short-lived mutation lock uses an atomic directory plus an owner token;
fresh incomplete locks remain busy, stale takeover cannot be removed by the
previous owner, and normal unlock removes only its own token.
Confirmed delete first publishes the next immutable cancelled revision as a
tombstone. It purges the item only after winning that no-clobber revision CAS,
so a stale writer and delete cannot both commit.

`start` creates an attempt. A future `resume` creates a new attempt and
invalidates the previous writer. V1 uses recoverable short-lived item-local
exclusion plus `attempt_id + state_revision` fencing. Delete is also
revision-checked and cannot remove a running or evaluating item. Distributed
leases, parallel execution of one item and unattended retry remain out of
scope.

## Checkpoint and context

A checkpoint may contain only a bounded summary, next step, optional blocker,
logical artifact references and its source attempt. Session Context exposes
only `bcgos://execution/active`; an authorized `bcgos work next --active`
resolves a projection of at most 2 KB.

## Evidence and completion

V1 supports two core-witnessed evidence types:

- `artifact_snapshot`: the core computes and later revalidates the artifact
  digest;
- `command_check`: the core executes a validated argument vector directly and
  records its exit status against declared snapshots or Git state.

Agent-authored assertions are not completion evidence. Completion reads the
contract and receipts from disk and succeeds only when all required criteria
remain valid.

## Privacy

Transition history is allowlist-only: opaque IDs, enum state, timestamp and
revision. It never stores prompts, responses, arguments, raw errors, URLs,
queries, absolute paths or professional content. Contract and checkpoint bodies
remain private local data and are never injected automatically.

## Initial implementation slice

The first slice implements:

- item creation, start, inspection, export and confirmed deletion;
- immutable contract digest;
- atomic local state replacement;
- revision-checked mutation;
- workspace and path isolation;
- allowlisted transition history.

Checkpoint, resume, Session Context pointer and completion evidence follow in
separate contract-tested slices.

## V1 non-goals

- business task synchronization;
- generic agent or tool-call tracing;
- event bus or cloud telemetry;
- scheduler integration or unattended execution;
- automatic retries, budgets or cost accounting;
- evaluator plugin framework;
- Claude or Codex lifecycle wiring;
- dashboard or cross-workspace delegation.
