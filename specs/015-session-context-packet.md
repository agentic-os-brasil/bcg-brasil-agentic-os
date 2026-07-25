# Spec 015 - Session Context Packet

Status: local packet and runtime-neutral Session Start bridge implemented;
Claude and Codex native lifecycle adapters remain unavailable.

## Objective

Give future Claude and Codex Session Start adapters one small, identical and
safe description of the current user and workspace. The packet orients an
adapter; it does not itself read, inject or summarize private content.

## Packet contents

`bcgos session packet [workspace-path]` returns JSON with:

- the interaction-profile ID and configuration source;
- workspace identity, readiness and brain readability;
- pointers to owner facets that both explicitly allow the `session` reader and
  appear in the reviewed session-safe allowlist;
- a pointer to current operating state and the explicit unavailable task
  pointer;
- an opaque active-execution pointer only when exactly one running or paused
  execution item can be resolved;
- managed, owner and workspace atlas availability and pointers;
- the managed skills-catalog pointer, not its full contents;
- an explicit unavailable memory-injection capability; and
- omission diagnostics for sources that are not ready.

The packet must never contain an owner-facet body, a client/project/daily page,
a conversation transcript, a memory artifact body, a credential, or a
Walter-only facet such as the psychological profile. It also must never contain
an execution item ID, attempt ID, objective, done contract or checkpoint body.

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
hook, configure Codex, read a source on behalf of a model, or claim that
Session Start or context injection is available. A future adapter must resolve
purpose and authorization again before reading a pointed source, respect its
own context budget, and report unavailable or omitted sources consistently.

`bcgos session bridge --runtime claude|codex [workspace-path]` emits the same
bounded Session Start envelope for either runtime. It is an adapter input, not
native lifecycle wiring: it cannot read a pointed source, inject content into a
conversation or change the capability state reported by `bcgos doctor`.

## Validation

- reviewed session-safe owner pointers are included; sensitive, Walter-only
  and unreviewed owner pointers are omitted;
- missing owner, workspace or atlas sources produce a valid `partial` packet;
- the skills catalog remains a pointer only;
- memory injection remains explicitly unavailable until adapters exist.
- packet references never reveal absolute local filesystem paths.
- Claude and Codex receive equivalent bridge envelopes while native injection
  remains explicitly unavailable.
- a two-session handoff proves that Session A can checkpoint and pause, Session
  B can resolve and resume with a new attempt, and the stale Session A attempt
  can no longer mutate the item.
