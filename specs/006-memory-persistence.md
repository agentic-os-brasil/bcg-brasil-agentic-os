# Spec 006 - Memory persistence and dreaming

Status: architecture, runtime-neutral core engine and initial CLI bridge implemented; synthesis adapters, scheduling and executable dreaming pending.

## Objective

Preserve useful professional context across sessions without treating the full work history as prompt context, mixing client work with the managed core or allowing an unattended model pass to overwrite authoritative sources.

## Memory pyramid

The canonical memory pipeline has three generated operating layers plus a curated lifetime layer:

| Layer | Purpose | Input | Default shape |
|---|---|---|---|
| L1 | Recent daily continuity | Sanitized Claude/Codex conversation signals plus selected sanitized human daily-log signals | Append-only daily journal plus dense daily digest |
| L2 | Cross-day continuity | Recent L1 digests | Weekly rollup that collapses repeated threads |
| L3 | Medium-term continuity | Recent L2 rollups | Rolling thematic synthesis of open, changing or structurally important threads |
| Lifetime | Stable recall and routing | Repeated, durable evidence promoted from L3 | Compact index with drill-down pointers to owned memory files |

Lifetime is not a blind fourth rollup. Its curation owner is the weekly deep dreaming cycle: eligible updates may be activated automatically only when an explicit eligibility policy is configured and its promotion rules, provenance validation and versioned writes all pass. Without an eligibility policy, activation fails closed. Existing lifetime state is never overwritten in place. Decisions, source documents and project state keep their own authoritative stores; memory points to them rather than replacing them.

## Dreaming cadence

Dreaming has two depths over the same deterministic engine:

- **Daily light dreaming:** captures and compacts recent allowed signals into L1. It cannot write L2, L3 or lifetime memory.
- **Weekly deep dreaming:** consumes the week's L1 evidence, refreshes L2 and L3, and consolidates eligible durable evidence into lifetime memory in one staged transaction.

### L1 source composition

The target L1 model combines two complementary local evidence streams for the
same authorized workspace: (1) session signals captured by the Claude or Codex
adapter, and (2) selected signals from the human daily log. Neither raw
conversation transcript nor an entire daily Markdown page is a valid capture
by itself. Before the deterministic memory engine may accept a daily-log
signal, the capture contract must be extended to preserve a source kind,
provenance and a verifiable sanitization attestation from the source adapter.

That extension is not implemented yet. The current capture core has only its
existing `Kind` and `Sanitized` fields, and it must not treat a self-declared
CLI flag as the required evidence. Until the extended contract and its adapter
tests exist, daily logs remain human-readable sources only and cannot enter
L1. L1 remains a bounded continuity product, never a copy of either source.

Manual invocation, a lifecycle hook, a scheduler and presence-based catch-up may all trigger these cycles. The trigger does not own memory semantics. A missed weekly run remains pending until the same idempotent deep cycle succeeds.

## Dreaming contract

Dreaming is the promotion pipeline from L1 to L2 to L3 and, during the governed weekly deep cycle, into lifetime memory. It is deterministic-first:

1. Select source files through a deterministic recency window and stable ordering.
2. Compute a source fingerprint and skip an already-produced equivalent rollup.
3. Stage synthesis output outside the active memory paths.
4. Validate size, layer and the provenance envelope; reject capture input not already marked sanitized by its adapter.
5. Publish immutable artifact versions, then atomically expose the complete set through one validated commit manifest.
6. Record completion or failure without changing source layers.

An LLM may compress, group and propose durable evidence, but it does not choose storage boundaries, mutate source memory, bypass lifetime eligibility or decide whether validation passed. A failed or empty synthesis is a no-op and remains diagnosable.

## Storage and privacy

- Releases contain schemas, policy, templates and code only.
- User memory, rollups, logs and run state live under user-level local data selected by the CLI for each operating system.
- Workspace memory is isolated by workspace identity and must never be copied into the managed bundle.
- Raw prompts, credentials, client files, client-identifying examples and unsanitized artifacts are not valid shared-memory inputs.
- Updates may migrate memory through versioned, reversible migrations but never replace it with bundle defaults.

Owner self learning is a separate local surface, not an additional memory
layer: canonical Owner Context facets remain authoritative, while a
stale-checked `UserSelfSnapshot` is only a packet projection. Maestro evaluates
every interaction and persists only material, authenticated owner signals as
minimal metadata. Walter hypotheses, prompts, client documents and generated
output cannot become canonical self evidence; explicit owner controls govern
promotion, correction, inspection, export and deletion.

The repository does not define the final Windows or macOS data path yet; that remains governed by `Q-007`.

## Runtime portability

The policy, layer identifiers, provenance envelope, rollup state and injection order are runtime-neutral. Claude and Codex adapters may use different native lifecycle events or schedulers, but they must preserve the same observable invariants and capability reporting from Spec 004.

Scheduling is not part of the memory truth model. Manual invocation, session-stop observation, periodic execution and presence-based catch-up are interchangeable triggers for the same idempotent dreaming operation. Spec 009 owns the runtime-neutral scheduler contract: native schedulers accelerate execution, while durable occurrence state and presence recovery detect missed work. A scheduler receipt reports execution metadata but never substitutes for the atomic memory commit that proves a dream succeeded.

## Context injection

Startup consumes the compact layers from broadest to most recent:

```text
lifetime -> L3 -> L2 -> L1
```

Each injected layer has an independent budget and an explicit pointer to deeper evidence. A missing or stale generated layer is skipped with a diagnostic; it never causes raw unbounded history to be injected as a silent fallback. Authoritative project state and decisions retain precedence over generated memory.

The base policy requires a budget for every layer but deliberately leaves each value as required runtime configuration. Exact pilot values remain pending evidence from real sessions and must never be hard-coded in an adapter.

## Wiki navigation

The private content atlas defined by Spec 007 is the navigation layer over memory rollups. Dreaming remains the only producer of L2, L3 and governed lifetime artifacts; the wiki consumes their active metadata and summaries to organize topic, entity and time routes, then preserves drill-down pointers back to the rollup and its provenance. It never edits a rollup or becomes a new memory layer.

Only artifacts reachable from the newest fully valid memory commit may appear as active navigation targets. L1 remains bounded and may be exposed only as local day or session pointers when policy permits; the wiki must not copy raw or unbounded captures into generated pages. The intended retrieval path is `wiki route -> rollup -> source evidence`, with every deeper read preserving the same owner, workspace and purpose authorization.

Memory correction, deletion, eligibility reversal or commit invalidation must propagate to derived wiki pages, indexes, backlinks and caches. Managed or shared atlases may never include owner or workspace memory. Private memory navigation remains unavailable until storage, enrollment, privacy and deletion propagation are implemented and tested.

Every activated memory commit must be discoverable by the wiki updater through a durable, idempotent outbox contract defined by Spec 008. Correction, deletion or authorization revocation writes a synchronous denial barrier before asynchronous wiki recompilation; an old atlas manifest or last-known-good view can never bypass that barrier.

## Initial executable contract

`bundles/base/memory/policy.json` is the sanitized default policy shipped with the managed bundle. `internal/memory` validates its structural invariants. The current contract requires:

- exactly L1, L2 and L3 in promotion order;
- light daily dreaming restricted to L1 and deep weekly dreaming owning L2, L3 and governed lifetime consolidation;
- lifetime updates with provenance, version history and no in-place overwrite;
- immutable sources, staged validation, atomic activation and last-known-good preservation;
- user-local, workspace-isolated data;
- broad-to-recent injection order with per-layer budgets and drill-down pointers.

## Executable engine

`internal/memory` now implements the runtime-neutral mechanics used by future CLI and runtime adapters:

- sanitized, append-only L1 capture isolated under `workspaces/<workspace-id>/`;
- deterministic source selection and SHA-256 source fingerprints;
- idempotent daily light and weekly deep cycles through injected synthesis and eligibility interfaces;
- one workspace-wide filesystem activation lock, so daily cycles and weekly cycles from different periods cannot race over shared state;
- full staging and validation before activation, followed by immutable version publication and a single durable manifest commit;
- crash-safe visibility: readers observe either the previous complete commit or the next complete commit, never a partial L2/L3/lifetime combination;
- version history through the parent-linked commit chain and mandatory eligibility records for lifetime;
- bounded context assembly in canonical broad-to-recent order with configurable freshness and missing/stale-layer diagnostics.

Generated state uses this logical layout under each workspace:

```text
captures (append-only inputs)
  -> .transactions (uncommitted staging)
  -> versions/<transaction-id>/ (immutable artifacts)
  -> commits/<timestamp>-<transaction-id>.json (atomic visibility boundary)
```

Readers resolve memory only from the newest fully valid commit manifest. An interrupted run before commit may leave an orphaned immutable version or pending manifest, but neither becomes visible. A corrupt or incomplete newest manifest is ignored in favor of the previous fully valid commit. If commit files exist but none is valid, the workspace is reported as corrupt rather than empty or missing. The activation lock itself remains fail-closed after a process crash until a future diagnostic and human-confirmed recovery flow exists.

The engine deliberately does not select a model, sanitize raw input, decide lifetime eligibility or schedule itself. Those capabilities belong to versioned adapters and policies. A leftover lock after a process or machine crash fails closed and requires a future `bcgos doctor` recovery path; it is never deleted heuristically by the engine.

The canonical product skill is `bundles/base/skills/dream-memory/SKILL.md`. It routes daily, weekly and status intents to the installed adapter and must report the capability as unavailable until that adapter exists.

`cmd/bcgos` now connects the safe adapter-independent operations to the engine: sanitized capture, commit/layer status and bounded context assembly. `bcgos memory dream daily|weekly` is deliberately present as an explicit unavailable capability until synthesis and lifetime-eligibility adapters are configured. It never substitutes a hard-coded model, eligibility rule, data directory or context budget.

## Test expectations for implementation

- deterministic source selection and fingerprints;
- idempotent reruns;
- no mutation on empty, invalid or interrupted synthesis;
- old-or-complete visibility at every injected activation interruption point;
- daily cycles cannot write lifetime and weekly cycles preserve version history;
- provenance and drill-down validation;
- workspace isolation and update preservation;
- equivalent policy behavior on Windows and macOS;
- Claude and Codex conformance fixtures for context injection and failure reporting.
- concurrent-cycle exclusion and fail-closed leftover-lock behavior.

## Deferred decisions

- synthesis provider and whether local-only operation is required;
- exact retention and rollup windows;
- default context budgets;
- default scheduling windows, unattended model permission and catch-up limits;
- lifetime promotion eligibility and correction flow;
- shared organizational knowledge governance.
- private wiki indexing policy, retention and context budgets per memory layer.
