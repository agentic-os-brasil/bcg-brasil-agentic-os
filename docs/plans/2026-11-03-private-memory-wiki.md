# Deliver source-owned private workspace-memory navigation

Issue #506 completes private navigation on the maintenance lifecycle landed in
#505 / PR #508. [Spec 008](../../specs/008-wiki-update-okf.md), decision PWKC and
the existing memory/privacy contracts define authority; the
[operator guide](../product-mcp-wiki.md) defines the supported journey.

## Starting point

At main `d1e8f18dd13d680f81a552567c869bcc4f57ac73`, reviewed capture and daily
consolidation work, but `maintenance.qualify` for `wiki-incremental-sync` returns
`maintenance_unavailable`. The existing public source-to-readback SDK test
reproduces that missing owning route after its real admitted memory prelude.
This is a genuine feature baseline, not a compiler/setup failure. Imported
historical tests remain regression evidence rather than a claim that their
original implementation was developed test-first here.

## Complete delivery

1. Keep `internal/privatewiki` pure: deterministic concepts, indexes, backlinks
   and profile validation from bounded eligible committed rollups. Memory owns
   scope, source/event evidence, owner authority, confined IO and retirement.
2. Publish a source intent before memory commit visibility and cumulative
   denial evidence in its checkpoint. Protect the new cohort from old writers;
   recover missing historical events from actual source truth, with explicit
   recovery evidence. Preserve parent/source/cancellation checks and last-good
   source authority when graph staging or cleanup fails.
3. Bind three real jobs to the existing exact review/qualification/enrollment
   lifecycle: event-only incremental synchronization, reconciliation and
   integrity diagnosis. Durable graph effects and terminal event closure are
   separate checks; a graph alone cannot close a failed receipt publication.
4. Expose only `wiki.status`, `wiki.read` and `wiki.source` through the existing
   read-only query tool. Require exact generation and returned references for
   bounded pages. Queries/startup inspect without compilation, bootstrap or
   repair. Method selection reports registered operations while host review,
   wake and provider admission remain independently unavailable.
5. Project the actual recipes into plugin/connector packets and regenerate
   canonical metadata/managed documentation from current sources. Preserve
   the landed document formats, owner, memory and maintenance contracts.

The detailed source/output limits, authority profile, visibility and retirement
rules live in Spec 008; implementations must preserve every applicable rule.
Native Windows private wiki remains unavailable until its separate owning
held-handle qualification exists. Background service setup and installed-client
qualification remain separate deliveries.

## Acceptance and review

Run focused public SDK and actual restarted-stdio source-to-readback tests,
closed/read-only/cross-workspace queries and correction/deletion refusal. Verify
owning parent/source/live-grant fences, interrupted event recovery, current
memory consumers and complete shipping method packets. Missing source or
native authority stays unavailable; tests must not fabricate successful effects
or treat refusal as positive native execution evidence.

Record the exact source hash, authentic baseline, focused results and remaining
limits. Four independent source reviews and hosted CI precede merge. Source,
local tests, hosted/native execution and real client qualification remain
separate evidence; no local full suite, model/provider call or service activation
is required to establish this bounded source capability.
