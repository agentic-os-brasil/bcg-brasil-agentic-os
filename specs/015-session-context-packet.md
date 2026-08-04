# Spec 015 - Session Context Packet

Status: local packet and runtime-neutral Session Start bridge implemented;
Claude and Codex native lifecycle adapters remain unavailable.

## Objective

Give Claude and Codex Session Start adapters one small, identical and safe
description of the current user and workspace. The serialized packet orients
an adapter and remains pointer-only; the local hook boundary may separately
attach an ephemeral, bounded generated memory context.

## Packet contents

`bcgos session packet [workspace-path]` returns JSON with:

- the interaction-profile ID and configuration source;
- workspace identity, readiness and brain readability;
- pointers to owner facets that both explicitly allow the `session` reader and
  appear in the reviewed session-safe allowlist;
- a pointer to current operating state, deterministic onboarding progress and
  an explicit unchecked-task count when the owner-local state is readable;
- an opaque active-execution pointer only when exactly one running or paused
  execution item can be resolved;
- managed, owner and workspace atlas availability and pointers;
- the managed skills-catalog pointer, not its full contents;
- at most two relative managed-skill pointers whose IDs, reasons and runtime
  paths are validated as one unit; while onboarding is pending this is exactly
  the integrity-checked `maestro-onboarding` guide;
- the managed agents-catalog pointer, Maestro hub ID and explicit definition
  versus runtime-activation states;
- local memory state plus portable pointers for valid generated layers, without
  artifact bodies or storage paths; and
- omission diagnostics for sources that are not ready.

The packet must never contain an owner-facet body, a client/project/daily page,
a conversation transcript, a memory artifact body, a credential, or a
Walter-only facet such as the psychological profile. It also must never contain
an execution item ID, attempt ID, objective, done contract or checkpoint body.
Open task titles and bodies stay behind the owner-local operating-state pointer;
Session Start may report only the count and must not invent a backlog.
When onboarding is `review_required`, the packet includes only the SHA-256
digest of the reviewable non-sensitive facets so an explicit confirmation can
be bound to the exact version shown. It never includes the facet bodies.
Before that confirmation, prompt hooks suppress unrelated Case methods instead
of letting a lexical prompt silently bypass the deterministic first-use flow.

The active execution capability has three states:

- `available` exposes only `bcgos://execution/active`;
- `unavailable` exposes no path when there is no running or paused item; and
- `ambiguous` exposes no path and marks the packet `partial` when more than one
  item is active.

An authorized next session resolves an available pointer explicitly with
`bcgos work next --active`. The packet itself never performs that read.

Atlas pointers are portable logical references (`bcgos://atlas/<scope>`), not
local filesystem paths. An adapter resolves a reference only after it has
resolved its own workspace, purpose and authorization.

`reader=session` is necessary but not sufficient for an owner facet. The
packet fails closed: a sensitive facet is always omitted, and a new non-
sensitive facet remains omitted until it is added to the reviewed session-safe
allowlist with contract tests.

## Runtime boundary

The packet is a local, runtime-neutral contract. It does not install a Claude
hook, configure Codex or claim that native Session Start is qualified. The
shared local adapter may resolve only the newest valid memory commit for the
exact workspace and render its already-generated layers inside the managed
per-layer and 8 KiB total budgets. It reports active-but-empty memory separately
from invalid/unavailable state and never reads raw captures as fallback.

`bcgos session bridge --runtime claude|codex [workspace-path]` emits the same
bounded Session Start envelope for either runtime. It is an adapter input, not
native lifecycle evidence: local memory assembly and emitted content cannot
change the capability state reported by `bcgos doctor`.
The envelope reports adapter delivery separately from native qualification: a
direct bridge is `contract_only`; an adapter serializer may report
`adapter_payload_emitted`; `injection_state` remains `unavailable` until the
qualifying native-session protocol succeeds.

## Validation

- reviewed session-safe owner pointers are included; sensitive, Walter-only
  and unreviewed owner pointers are omitted;
- missing owner, workspace or atlas sources produce a valid `partial` packet;
- the skills catalog remains a pointer only;
- selected skill pointers are relative, match their declared IDs and use only
  governed selection reasons; absolute, traversal or caller-invented pointers
  fail closed;
- available memory exposes portable layer pointers only in serialized JSON;
  bodies and storage paths remain absent.
- Session Start may carry bounded generated memory, while `UserPromptSubmit`
  never repeats it and corrupt state fails closed without raw fallback.
- packet references never reveal absolute local filesystem paths.
- agent definitions remain pointer-only and runtime activation remains
  explicitly unavailable until orchestration enforcement exists.
- Claude and Codex receive equivalent bridge envelopes while native injection
  remains explicitly unavailable.
- a two-session handoff proves that Session A can checkpoint and pause, Session
  B can resolve and resume with a new attempt, and the stale Session A attempt
  can no longer mutate the item.
