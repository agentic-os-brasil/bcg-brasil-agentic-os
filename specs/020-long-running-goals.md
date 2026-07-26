# Spec 020 - Maestro long-running goals

Status: accepted architecture; runtime-neutral core and receipt-backed recovery
implemented. macOS Keychain and Windows Credential Manager anchors are
implemented; native Claude/Codex trigger wiring remains pending.

## Objective

Let Maestro continue a bounded professional objective across turns and context
compaction without treating conversation history as durable state. A goal is
complete only when its explicit Done Contract has evidence and independent
Walter approval.

## Roles and loop ownership

Maestro owns the goal, phase selection, evidence ledger and user-facing
status. It is the only role that may change a goal state.

The workspace agent owns raw workspace context. It turns the active goal into
a scoped work state and prepares a minimal packet for a specialist. It never
hands general workspace access to a specialist or Walter.

A specialist receives a minimum packet for one bounded capability question and
returns findings, evidence references, risks and a recommended next action. A
specialist cannot close a goal, alter the Done Contract or write workspace
state directly.

Walter receives the Maestro-composed orchestration record, not raw workspace
content. It returns one of `approved`, `refine`, or `needs_human_decision`
**only to Maestro**. Walter cannot change code, workspace state, a goal or a
release; it never speaks directly to the user or another loop.

## Encadeamento

1. Maestro opens or resumes a goal with a Done Contract.
2. The workspace agent validates scope and publishes a compact work state.
3. Maestro delegates a minimum work packet to one specialist.
4. The specialist returns evidence-backed findings to the workspace agent.
5. The workspace agent attaches the permitted findings and returns the bounded
   result to Maestro.
6. Maestro composes an advancement against the Done Contract and records that
   composition as a breadcrumb.
7. Walter reviews the sanitized Maestro record and returns only to Maestro.
8. On `refine`, Maestro sends the work back down to the workspace/specialist
   loop. `needs_human_decision` pauses at a named decision; once resolved,
   Maestro may resume the same downward loop. `approved` allows the next phase
   or completion audit.

No role may skip directly from specialist output to completion.

## Done Contract

A Done Contract contains an opaque bounded-objective reference, required
deliverables, required evidence classes, explicit non-goal references and the
authority boundary for external actions. It is immutable after activation; a
material scope change creates a new revision with a breadcrumb explaining why.

Completion requires all of the following:

- every required evidence class has a verified item;
- no unresolved blocker remains;
- the active phase is complete;
- Walter returns `approved` for the current revision; and
- Maestro performs the completion audit.

## Breadcrumbs and evidence

Breadcrumbs preserve a compact recovery point: opaque decision reference,
evidence reference, state and next safe action. Evidence items retain only an
opaque reference, class and verification state; source bodies remain under the
workspace agent's authorization boundary. File URIs, absolute paths and
arbitrary prose are not valid core references.

The loop may compact conversation context freely because it reconstructs work
from the goal, Done Contract, breadcrumbs and evidence ledger. A missing
evidence reference is not treated as remembered proof.

## States

`draft -> active -> awaiting_walter -> active|awaiting_human -> completed`

`blocked` is reserved for a repeated external condition that prevents useful
progress. Completion is irreversible in the current revision. Runtime hooks,
schedulers and manual commands are interchangeable triggers; none can alter
the state transition rules.

## Data boundary

The core contract contains no workspace document body, transcript, prompt,
client identifier, absolute path or Walter-only owner facet. Workspace and
specialist adapters resolve references only after their existing authorization
checks. The core is runtime-neutral for Claude and Codex.

## Acceptance criteria

1. A specialist result cannot complete a goal directly.
2. A goal cannot complete without all required evidence and current Walter
   approval.
3. Walter cannot receive a workspace payload or alter a goal state.
4. A `refine` review returns to the active loop and records a breadcrumb.
5. A human-decision review keeps the goal resumable without losing the
   evidence ledger.
6. The same loop inputs yield equivalent state transitions for Claude and
   Codex adapters, or an adapter reports the capability unavailable.
7. A Walter approval is bound to the exact contract and ledger revisions it
   reviewed; every material ledger change invalidates it.
8. A completion audit records phase completion, every deliverable and absence
   of blockers before a goal can enter `completed`.

## Implemented local lifecycle

Workspace and Walter mutations are deliberately not generic CLI JSON commands: only a
capability-bound runtime adapter may invoke them through the shared core. The
store signs each typed transition using a user-local integrity key, commits a
separate monotonic event head and rebuilds the Goal by replaying its signed
receipts. The persisted snapshot is rejected if it disagrees with that replay
or if a valid history prefix tries to roll back the committed head. This
requires a secure host monotonic anchor outside the user-local data root. The
macOS adapter uses Keychain and Windows uses Credential Manager; the core
fails closed when a host anchor is unavailable. This remains an adapter seam
and recovery surface, not a mechanism to run a model, read a workspace or
install an unattended background hook.
