# Spec 023 - Sequential agent dispatch and work packets

Status: accepted architecture; runtime-neutral dispatcher, packet contract and
process-local pilot bridge implemented. Native Claude and Codex wiring,
delivery and recovery remain unavailable.

## Objective

Let Maestro and an owning Case or Client Account Agent delegate one
small task without copying a whole dossier, workspace or conversation into the
next agent.

## Packet contract

Every delegation uses a signed `WorkPacket` containing only:

- a 256-bit random packet ID and optional parent packet ID;
- issuer and target registered agent IDs;
- one canonical scope kind and scope ID;
- one objective of at most 1,000 bytes;
- at most 12 canonical pointers;
- at most 8 constraints of 300 bytes each;
- issue and expiry timestamps, with maximum lifetime 24 hours; and
- an HMAC-SHA256 signature from the local dispatcher capability.

Pointers may address only a specific artifact below the packet scope root or a
specific `bcgos://public/...` artifact. A whole workspace, account or
public root is too broad and is rejected. Pointers do not contain file bodies,
prompt history, credentials or raw workspace context. PA Expert packets cannot
point to workspace resources. A child packet inherits the root packet scope
and names the parent packet.

Byte and count limits are not semantic sanitization. The issuing Maestro,
Case or Client Account Agent remains responsible for minimizing the
free-text objective and constraints; native adapters must not auto-copy source
bodies or conversation history into those fields.

A `WorkPacket` is an explicit ephemeral dispatch body, not a public receipt.
The process-local pilot keeps the active packet private until completion so a
status or audit consumer cannot obtain its objective, pointers, constraints or
signature by inspecting delegation metadata.

## Sequence

1. Maestro issues a root packet and opens a branch using that packet ID as the
   unique branch instance.
2. The root agent may issue one child packet when the catalog allows its role
   edge. The child packet ID becomes the unique child dispatch instance.
3. The child returns and closes before the root may close.
4. The root returns to Maestro. A later chain receives a new packet and branch
   ID even when it uses the same workspace.

Old signed packets cannot act on or close later work: child tool and finish
events must match the active child packet ID, while root finish events must
match the active root packet ID. Tampered, expired, oversized, duplicate,
cross-scope or replayed packets fail closed.

## Authenticated executor return

Possession of a signed `WorkPacket` does not authorize completion. A target
executor returns an explicit ephemeral `ReturnBody` or `FailureBody` together
with an HMAC-SHA256 `ExecutionEnvelope` authenticated by that target's private
runtime capability. The envelope binds all of:

- packet ID and registered target agent ID;
- runtime (`claude` or `codex`);
- canonical scope kind and scope ID;
- outcome and SHA-256 digest of the normalized result or failure body;
- a 256-bit random nonce; and
- issue time within a five-minute acceptance window and the packet lifetime.

The pilot resolves the active packet from private process state, selects the
expected target credential from that record and verifies the signature,
runtime, scope, body digest, validity window and unused nonce before calling
`FinishRoot`. It never trusts envelope fields to select another target
credential. A forged target, valid credential for a different target,
cross-runtime envelope, cross-scope envelope, changed body, expired envelope or
replayed nonce fails closed and leaves the active delegation open.

Native adapters eventually own private capability loading and envelope
production. The implemented `Executor` is a runtime-neutral signing seam for
that future wiring; it is not evidence that Claude or Codex native lifecycle
events are installed.

## Bounded errand contract

The pilot does not accept a caller assertion that arbitrary work is
`reversible`. An errand carries a closed typed operation and exact resource
grant. The only accepted pilot operation is
`create_ephemeral_note` on one concrete
`bcgos://errand/<scope>/ephemeral-notes/<slug>.md` artifact. Its compensation is
derived by the core, never supplied by the caller:
`delete_ephemeral_note` on the exact same resource.

Unknown operations, workspace or account resources, collection roots and
non-canonical artifact paths are rejected before a branch opens. The exact
grant and compensation travel only in the ephemeral dispatch. This contract
does not turn the errand helper into a general mutation agent or grant Maestro
tools.

Every errand tool request also carries a fresh target-authenticated envelope
bound to the packet, runtime, exact tool, operation, resource, nonce and issue
time. The pilot verifies that envelope before invoking the shared static tool
guard; a caller who knows only the delegation ID and resource cannot borrow the
helper's capability. The private active record narrows the static resource
prefix to the exact grant.

The helper then authenticates a terminal `succeeded` or `failed` observation
bound to the accepted request nonce. Compensation is denied until the original
grant has an observed successful outcome. A successful compensation is
one-shot; it cannot be repeated. Failure close is denied while a successful
mutation remains uncompensated, and successful return is denied until the
grant itself has succeeded.

## Public receipts and process lifetime

Public pilot receipts are metadata-only. They contain delegation, owner,
target, runtime and scope identity; state and timestamps; packet and result
digests; and a bounded failure code when applicable. They never contain a work
packet, objective, pointer, constraint, return summary, evidence body,
uncertainty or failure prose.

The pilot's packet records, result bodies and nonce replay set are explicitly
process-local. They are unavailable after restart. A restarted pilot therefore
rejects completion of an old packet as unavailable and does not claim safe
recovery. The durable orchestration snapshot may still block a new branch
until the separately governed stale-recovery path is used. Native runtime
activation remains unavailable until atomic private persistence and recovery
for this dispatch state are specified and proven.

## Runtime boundary

`internal/agentdispatch` is runtime-neutral and calls the shared
`internal/agentorchestration` guard. It does not spawn agents, read pointers or
persist dispatch bodies or native runtime state by itself. Maestro still has
no filesystem, shell, web, messaging or external-system tools. Claude and
Codex remain unavailable until their installed adapters:

1. load credentials from the approved private store;
2. persist and atomically restore the shared orchestration snapshot;
3. turn native agent lifecycle events into dispatcher operations;
4. deliver only the verified packet to the target; and
5. authenticate target return and failure envelopes without exposing
   capabilities or bodies in receipts; and
6. pass the same conformance and replay tests.

## Acceptance criteria

1. Root and child packets are signed, bounded, expiring and pointer-only.
2. Child scope and scope kind exactly match the parent.
3. A second active branch or child is rejected.
4. Tampering, expiry and cross-scope pointers are rejected.
5. Root and child packet IDs prevent replay against later dispatches.
6. Workspace, account and practice resource domains remain isolated.
7. Root completion requires a fresh target-authenticated executor envelope
   bound to packet, runtime, scope and result digest.
8. Errands use the one closed ephemeral-note grant and a core-derived exact
   compensation rather than a caller reversibility assertion. Tool request and
   terminal outcome are target-authenticated, and compensation follows one
   observed successful grant.
9. Public receipts remain metadata-only and dispatch/result bodies remain
   explicit ephemeral structures.
10. Process-local state is reported unavailable after restart; runtime
    activation stays unavailable until native delivery and safe durable state
    persistence exist.
