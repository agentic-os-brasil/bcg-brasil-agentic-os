# Spec 023 — Maestro-mediated sequential dispatch

Maestro owns every dispatch. It may open one direct spoke at a time at depth
one. Case, Client Account, PA Expert, Walter, Darwin and a bounded errand
helper are leaves; none can issue a packet to another agent.

Packets are signed, bounded, expiring and pointer-only. They bind the exact
target, immutable scope, authorization digest, capability digest, state
snapshot and plan digest. A second active branch, nested start, forged actor,
replay or cross-scope pointer fails closed.

The Maestro planner can choose a direct answer, account-assisted Case,
direct Case, standalone account/advisory/review/health/errand route or no
branch. Account framing and post-Case validation are paired: validation exists
if and only if framing was used. Walter is a separate materiality gate and is
invoked if and only if the resolved plan requires it; a low-materiality skip
has a typed reason and evidence.

All runtime adapters use one durable installation state store. Atomic state
updates, restart recovery, replacement fencing and replay checks are tested in
the Claude and Codex conformance surfaces. Native runtime qualification is
separate from configured bindings and adapter-observed receipts.

Each schema-v2 packet also carries a signed, scope-bound `DoneContract`.
Authenticated producer returns must satisfy its required evidence pointers and
minimum evidence count; Walter can close only through the typed verdict path.
The shared adapter persists a bounded, hash-linked metadata-only breadcrumb
tail for every branch and tool event, so recovery does not depend on model
context. Native adapters remain unavailable until attended event conformance is
observed.
