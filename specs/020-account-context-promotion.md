# Spec 020 - Governed account context promotion

Status: accepted architecture; runtime-neutral promotion, revocation and audit
core implemented. Native account/workspace authorization adapters remain
unavailable.

## Objective

Let a workspace deliberately contribute one durable fact to its client/account
context without giving the account agent permission to browse project
workspaces or aggregate their memory.

## Promotion contract

A promotion requires:

- a unique promotion, account and source-workspace ID;
- one curated statement of at most 1,000 bytes;
- one canonical artifact URI inside the source workspace;
- the artifact SHA-256, verified against the bytes of the resolved regular
  workspace file, plus author and workspace-owned source receipt;
- `account_safe` classification and `approved` review status;
- approver, approval time and validity window of at most 366 days; and
- a capability-bound authority with explicit `promote` grants for both the
  source workspace and destination account.

The source is opened through an OS-enforced scoped root: path traversal and
symlinks that resolve outside the workspace are rejected without a
check/use race. The workspace keeps the raw source URI in its own signed source
receipt.
The account record receives the curated statement, source hash and opaque
source-receipt ID, never the raw workspace pointer. Account reads require a
separate `read_account` capability and a known promotion ID; the core exposes no
workspace enumeration surface.

## Two-phase evidence

Promotion writes HMAC-authenticated, create-only evidence in this order:

1. `promotion_prepared` audit receipt;
2. workspace-owned source receipt;
3. account-safe promotion record; and
4. `promoted` completion receipt.

Before writing evidence, the service creates a `preparing` entry in a trusted
monotonic anchor store outside the account/workspace evidence tree. An account
record becomes active only after every durable write and the anchor transition
to `active`. The completion receipt, account-record hash, workspace
source-receipt hash and keyed authenticators must all match that anchor. Partial
writes, coordinated edits and receipt substitution fail closed.
All evidence reads, directory creation and atomic publication also run through
an OS-enforced root opened on the BCGOS state directory, so a symlinked account,
workspace or audit directory cannot redirect data outside that boundary.

## Revocation

Revocation requires capability-bound `revoke` grants for both the
caller-declared source workspace and destination account, plus an actor,
timestamp and reason.
Authorization occurs before any promotion lookup, and trusted anchored metadata
is verified before use. It writes:

1. `revocation_prepared`;
2. an immutable account revocation marker; and
3. `revoked`.

The monotonic anchor transition to `revoked` is the synchronous barrier and
happens before evidence is written. Reads and revocations are linearized: a
successful read is ordered before revocation, and every read after the
transition returns revoked even if the marker is deleted or final audit writing
was interrupted. Revocation never deletes the account record, workspace source
receipt or prior audit evidence. Expiration is evaluated only against the
service's trusted clock; expired context is unavailable and requires a new
promotion ID after review.

## Runtime boundary

`internal/contextpromotion` does not authenticate a human name on its own. It
accepts only capability-bound authorities, a private HMAC key and an
`AnchorStore` provisioned by a future private runtime adapter.
`MemoryAnchorStore` exists only for tests and conformance. Claude and Codex
remain unavailable until their adapters resolve authorities and the integrity
key from the approved credential store, provide a durable atomic anchor store
and enforce the same account/workspace filesystem boundaries.

## Acceptance criteria

1. The account-visible record contains no raw workspace URI.
2. Promotion requires explicit workspace and account authority grants.
3. Account reads require account-scoped authority and a known promotion ID.
4. Cross-workspace sources, forged capabilities, expired context and duplicate
   IDs are rejected.
5. Revocation immediately blocks reads without deleting evidence.
6. Prepared/final receipts preserve interrupted-operation diagnosis.
7. Source bytes, account records, workspace receipts and audit sequences are
   authenticated and verified before use.
8. A durable monotonic anchor prevents deletion rollback; native activation is
   denied while only the in-memory conformance store exists.
