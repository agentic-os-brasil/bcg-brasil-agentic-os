# Spec 029 - Execution Ledger V1

Status: accepted direction; resumable checkpoint loop in implementation.

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
logical artifact references and its source attempt. An authorized
`bcgos work next --active` resolves a projection of at most 2 KB only when
exactly one running or paused item exists. Ambiguity fails closed and requires
an explicit item ID. Session Context exposure of `bcgos://execution/active`
remains a separate adapter-facing slice.

## Evidence and completion

V1 supports two core-witnessed evidence types:

- `artifact_snapshot`: the core computes and later revalidates the artifact
  digest;
- `command_check`: the core executes a validated argument vector directly and
  records its exit status against declared snapshots or Git state.

Agent-authored assertions are not completion evidence. Completion reads the
contract and receipts from disk and succeeds only when all required criteria
remain valid.

The executable slice fixes command checks to a small no-shell registry:
`go version`, `go test ./...` and `go vet ./...`. The Go executable resolves
from the CLI runtime's `GOROOT`, its binary digest enters the receipt, and the
child receives a closed, offline environment that ignores caller `PATH`,
`GOFLAGS`, toolchain download and proxy settings. Evidence execution rejects a
caller-supplied `GOROOT` before resolving the trusted binary. The
argument vector lives only in the immutable contract. A receipt exposes its
digest, tool digest, exit code and outcome, never arguments, stdout or stderr.
`go test` and `go vet` execute code from the selected workspace: they are valid
only for a user-authorized trusted workspace and are not a process sandbox.
Artifact
snapshots accept one allowlisted `bcgos://workspace/...` file reference, reject
workspace escapes and symlink escapes, and record a core-computed digest.
Completion re-runs every command and re-hashes every artifact before committing
`completed`; a stale or failed criterion leaves the item running and unchanged.

## Privacy

Transition history is allowlist-only: opaque IDs, enum state, timestamp and
revision. It never stores prompts, responses, arguments, raw errors, URLs,
queries, absolute paths or professional content. Contract and checkpoint bodies
remain private local data and are never injected automatically. Mutation
commands return metadata-only receipts; `next` is the only handoff-body output,
while `inspect` and `export` are explicit full-body operations.

## Implemented slices

The foundation implements:

- item creation, start, inspection, export and confirmed deletion;
- immutable contract digest;
- atomic local state replacement;
- revision-checked mutation;
- workspace and path isolation;
- allowlisted transition history.

The resumable handoff slice implements:

- bounded private checkpoints with allowlisted logical artifact references;
- explicit pause after a checkpoint from the current attempt;
- explicit resume through a new attempt identity;
- `attempt_id + state_revision` fencing against stale writers;
- bounded `next` projection without the objective or completion contract;
- fail-closed active-item resolution when more than one item is active;
- crash recovery from the immutable checkpoint revision.

The evidence-backed completion slice implements:

- immutable, metadata-only tool receipts for artifact and command witnesses;
- complete evidence history in explicit export;
- no-shell command execution with discarded stdout and stderr;
- revalidation of every completion criterion against the immutable contract;
- completion only after all latest receipts still pass.

Session Context pointer remains a separate adapter-facing slice.

## V1 non-goals

- business task synchronization;
- generic agent or tool-call tracing;
- event bus or cloud telemetry;
- scheduler integration or unattended execution;
- automatic retries, budgets or cost accounting;
- evaluator plugin framework;
- Claude or Codex lifecycle wiring;
- dashboard or cross-workspace delegation.
