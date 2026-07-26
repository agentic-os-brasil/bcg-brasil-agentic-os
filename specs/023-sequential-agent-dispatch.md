# Spec 023 - Sequential agent dispatch and work packets

Status: accepted architecture; runtime-neutral dispatcher and packet contract
implemented. Native Claude and Codex wiring remains unavailable.

## Objective

Let Maestro and an owning workspace, account or practice agent delegate one
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
specific `bcgos://public/...` artifact. A whole workspace, account, practice or
public root is too broad and is rejected. Pointers do not contain file bodies,
prompt history, credentials or raw workspace context. Practice packets cannot
point to workspace resources. A child packet inherits the root packet scope
and names the parent packet.

Byte and count limits are not semantic sanitization. The issuing Maestro,
workspace, account or practice agent remains responsible for minimizing the
free-text objective and constraints; native adapters must not auto-copy source
bodies or conversation history into those fields.

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

## Runtime boundary

`internal/agentdispatch` is runtime-neutral and calls the shared
`internal/agentorchestration` guard. It does not spawn agents, read pointers or
persist native runtime state by itself. Claude and Codex remain unavailable
until their installed adapters:

1. load credentials from the approved private store;
2. persist and atomically restore the shared orchestration snapshot;
3. turn native agent lifecycle events into dispatcher operations;
4. deliver only the verified packet to the target; and
5. pass the same conformance and replay tests.

## Acceptance criteria

1. Root and child packets are signed, bounded, expiring and pointer-only.
2. Child scope and scope kind exactly match the parent.
3. A second active branch or child is rejected.
4. Tampering, expiry and cross-scope pointers are rejected.
5. Root and child packet IDs prevent replay against later dispatches.
6. Workspace, account and practice resource domains remain isolated.
7. Runtime activation stays unavailable until native delivery and durable state
   persistence exist.
