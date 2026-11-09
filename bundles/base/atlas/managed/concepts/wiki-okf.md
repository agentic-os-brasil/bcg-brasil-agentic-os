---
type: Architecture Contract
title: Wiki update lifecycle and OKF profile
description: The lifecycle, validation and publication contract for OKF bundles.
resource: repo://specs/008-wiki-update-okf.md
tags:
    - okf
    - lifecycle
    - validation
sources:
    - id: wiki-okf
      resource: repo://specs/008-wiki-update-okf.md
      title: Wiki update lifecycle and OKF profile
status: stable
x-maestro-profile-version: "1"
x-maestro-stable-id: managed/wiki-okf
x-maestro-scope: managed
x-maestro-source-fingerprint: 62ce62c72db4a8c76fc634fd980e62e881ad1fd5926351c8642455b7630b497d
x-maestro-freshness: fresh
x-maestro-status: active
x-maestro-generator-version: maestro-managed-wiki/0.2
x-maestro-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `specs/008-wiki-update-okf.md`. The source remains authoritative.

## Related

- [Content navigation through a compiled LLM wiki](/concepts/content-navigation.md)
- [Human atlas bootstrap](/concepts/human-atlas-bootstrap.md)
- [Maestro release and distribution](/concepts/release-distribution.md)
- [Wiki and atlas entrypoint](/concepts/wiki-entrypoint.md)

## Source content

# Spec 008 - Wiki update lifecycle and Maestro OKF profile

Status: architecture accepted; initial managed compiler and partial
OKF/profile validator implemented; complete profile/security validation,
schemas and enterprise bundle composition remain incomplete. The bounded local workspace-memory profile below is source-composed under #506 with separate runtime/client qualification.

## Objective

Update each compiled wiki safely and incrementally while keeping its output portable across humans, agents and tools.

Open Knowledge Format v0.2 defines the exchange envelope. The Maestro Atlas Profile v1 defines the additional governance and lifecycle required for professional, owner-private and workspace-private knowledge. OKF remains a format, not an authorization or storage system.

The normative OKF base is the GoogleCloudPlatform `knowledge-catalog/okf/SPEC.md`
v0.2. Maestro does not require Google Cloud, Knowledge Catalog or a proprietary
consumer to read or publish a conformant bundle.

## Standards boundary

### OKF v0.2 core

Every atlas is an OKF Knowledge Bundle:

- a hierarchical tree of UTF-8 Markdown;
- one concept document per non-reserved `.md` file;
- parseable YAML frontmatter with a non-empty `type` on every concept;
- optional `title`, `description`, `resource`, `tags` and `timestamp`;
- standard Markdown links and citations in the body;
- reserved `index.md` for progressive disclosure and `log.md` for human-readable update history;
- concept ID derived from the bundle-relative path without `.md`;
- best-effort consumption of unknown types, optional fields and broken links.

The root `index.md` declares `okf_version: "0.2"`. Maestro does not introduce a competing concept-ID field or a proprietary link syntax.

### Maestro Atlas Profile v1

The profile extends concept frontmatter with namespaced governance metadata:

```yaml
type: Memory Rollup
title: Example thematic route
description: Compact authorized navigation summary.
resource: maestro://memory/opaque-resource-reference
tags: [memory, thematic]
timestamp: 2026-07-20T00:00:00Z
x-maestro-profile-version: "1"
x-maestro-stable-id: opaque-stable-id
x-maestro-scope: workspace
x-maestro-tenant-ref: opaque-tenant-ref
x-maestro-owner-ref: opaque-owner-ref
x-maestro-workspace-ref: opaque-workspace-ref
x-maestro-memory-layer: L3
x-maestro-memory-commit: opaque-commit-version
x-maestro-rollup-version: opaque-rollup-version
x-maestro-source-fingerprint: opaque-keyed-fingerprint
x-maestro-sensitivity: client_restricted
x-maestro-freshness: fresh
x-maestro-status: active
x-maestro-generator-version: "1"
x-maestro-policy-version: "1"
```

Managed concepts omit owner and workspace references. Source relations and citations remain standard Markdown links in the body. The `resource` field or citations may use opaque Maestro URIs that require authorization to resolve.

Maestro consumers preserve unknown OKF fields. Unknown `type` values remain consumable as generic concepts. Unknown or missing values for security-bearing profile fields such as scope, sensitivity, status or policy version fail closed within Maestro runtimes.

## Physical bundle separation

V1 has three independent roots:

```text
atlases/
  managed/                    # sanitized product knowledge
  owners/<owner-ref>/         # owner-global private knowledge
  workspaces/<workspace-ref>/ # one private workspace per root
  organization/
    sharepoint-work/          # explicit prior-work retrieval metadata
```

These are logical paths; approved operating-system directories remain pending initialization decisions.

- Managed, owner and workspace bundles never share a manifest, staging area or active pointer.
- V1 concept documents contain no cross-bundle links.
- Session composition may select concepts from multiple authorized bundles only after evaluating actor, tenant, owner, workspace, purpose and policy.
- Client or workspace content cannot be promoted into managed or organizational bundles through the compiler.
- The SharePoint work-retrieval bundle is governed separately by Spec 037. It
  accepts only normalized metadata/facets from explicitly enrolled roots and is
  never composed into general navigation without explicit retrieval intent.

## Update event contract

Wiki updates are event-driven with periodic reconciliation. Canonical event reasons are:

```text
managed_source_changed
memory_commit_activated
source_corrected
source_deleted
access_revoked
policy_changed
bundle_updated
reconcile_requested
```

Each event has an opaque event ID, target atlas scope, source watermark, reason, policy version and creation timestamp. It carries no raw prompt, client content, human-readable private path or reusable public hash of sensitive content.

The source publisher writes a transactional outbox record before or at the same durable visibility boundary as the source update. A memory commit includes enough information for the updater to rediscover a missing event during reconciliation. Consumers process events idempotently and may coalesce enrichment events for one scope, but never coalesce away deletion, correction or access-revocation barriers.

## Update pipeline

```text
source event
  -> synchronous revocation barrier when required
  -> durable outbox
  -> scope and policy resolution
  -> pin source watermark
  -> compute dirty concepts and backlinks
  -> compile OKF candidates in staging
  -> OKF core validation
  -> Maestro profile and security validation
  -> atomic atlas manifest publication
  -> metadata-safe log and receipt
  -> runtime pointer refresh
```

### 1. Resolve and pin

The updater resolves the exact managed, owner or workspace root and pins the authoritative source versions it will consume. For memory, the watermark includes the newest fully valid memory commit and the referenced rollup versions.

### 2. Compute the dirty set

The updater identifies affected concepts, indexes and backlinks from source fingerprints and reverse dependencies. It does not rebuild unrelated scopes. A periodic full reconciliation detects missed events, stale dependencies and generator drift.

### 3. Compile in staging

The compiler creates or refreshes concept documents, directory indexes and the scoped log in a staging transaction. Dreaming remains the only producer of rollups; the wiki compiler produces navigation views over them.

### 4. Validate in two layers

Hard OKF checks:

- parseable frontmatter for every concept;
- non-empty `type`;
- valid reserved-file structure.

Hard Maestro checks:

- recognized profile and policy versions;
- valid scope, sensitivity, status and authorization metadata;
- no forbidden cross-scope or cross-bundle reference;
- source watermark and memory commit still active;
- provenance resolvable within the same authorization scope;
- revoked or deleted sources absent;
- output and context budgets respected.

Soft diagnostics:

- broken links and orphan concepts;
- weak or missing optional descriptions;
- duplicated concepts or near-duplicate routes;
- stale summaries and missing citations;
- unknown non-security extension fields or concept types.

OKF's permissive consumption remains intact; the stricter hard checks are requirements of the Maestro profile and do not redefine OKF conformance.

### 5. Recheck and publish atomically

Before publication, the updater rechecks the source watermark. If it changed, the staged view is discarded and a new idempotent event is scheduled. A validated atlas is published as immutable versioned files plus one atomic manifest pointer. Readers see the previous complete view or the new complete view, never a mixed graph.

### 6. Record and refresh

The updater appends a metadata-safe `log.md` entry and an opaque machine receipt. It then refreshes runtime pointers. Private logs and receipts never contain source bodies, personal names, client names, sensitive paths or fingerprints that can be used as offline guessing or correlation oracles.

## Revocation and deletion barrier

Deletion, correction that removes information, and access revocation are security events rather than ordinary compile requests.

1. Write a synchronous denial barrier or tombstone to the authoritative access layer.
2. Make adapters and Session Start consult that barrier before any atlas page, cache or last-known-good view.
3. Persist the corresponding high-priority outbox event.
4. Rebuild the affected bundle without the denied concepts, links, indexes and backlinks.
5. Retire or crypto-erase inaccessible private versions according to retention and user-rights policy.

Last-known-good preservation never bypasses a denial barrier. If rebuilding fails, unaffected concepts may remain available, but denied concepts and their resolvable metadata remain inaccessible.

## Cadence and triggers

- **Managed atlas:** compile in CI after allowlisted product sources change; publish with the compatible managed bundle.
- **Local Darwin reconcile:** a bounded maintenance job may run the development-only managed compiler and validator against a configured Maestro checkout; it never reads private bundles, calls an LLM, commits, pushes or publishes a release.
- **L1 activity:** update bounded day/session pointers after a valid daily memory commit; do not synthesize semantic pages from raw captures.
- **L2/L3/lifetime:** compile temporal and semantic routes after a successful weekly deep-memory commit.
- **Correction/deletion/revocation:** apply the denial barrier synchronously, then compile immediately from the high-priority event.
- **Policy or generator change:** rebuild every affected scope under the new version.
- **Weekly reconciliation:** compare active source watermarks, outbox receipts and atlas manifests; regenerate missing or stale views and run full lint.
- **Session Start:** read-only. It may report staleness and enqueue presence-based catch-up after startup, but never blocks startup on compilation or invokes a model to update the wiki.

Spec 009 owns cadence recovery. A successful weekly memory commit emits the durable source event consumed here; a scheduler receipt neither activates memory nor publishes an atlas. `wiki-reconcile` may be woken by a native schedule or later presence and always compares authoritative watermarks before declaring success.

`sharepoint-work-sync` is a separate provider-backed job. It may collect only
through an authorized Claude SharePoint adapter. Codex leaves collection due
and reports `unavailable`; it may still validate or query the last active local
catalog. A scheduler receipt never substitutes for the active catalog manifest.

## Incremental update semantics

For an unchanged ordered source set, policy and generator version, compilation is a no-op. Event IDs and source watermarks make retries idempotent. Multiple non-security events for one scope may be coalesced to the newest watermark.

Backlinks and indexes belong to the same atomic atlas transaction as their changed concepts. A concept is never published without the matching index and backlink view. Full reconciliation remains necessary because incremental dependency maps can drift or miss events after interruption.

## Managed versus private history

- Managed OKF bundles live in the source and release pipeline; Git provides review, diff and history.
- Private OKF bundles live under user-local versioned storage and are not committed to Git by default.
- Private `log.md`, manifests and receipts contain only metadata-safe opaque references.
- Deletion rights apply to generated concepts, indexes, backlinks, caches, logs and inaccessible versions; immutable history is not a reason to retain prohibited private content.

## V1 implementation boundary

V1 implements the managed bundle first. The initial deterministic compiler and
partial validator are now available through the development-only harness. It
writes a reviewable local candidate with a best-effort directory swap; it is
not yet the complete Maestro profile/security validator or the durable
versioned-manifest/last-known-good publication mechanism. The remaining items
are the following:

1. Complete Maestro Atlas Profile v1 schema, authorization and security checks.
2. Durable staging, source watermark, atomic manifest and last-known-good behavior.
3. Full link/provenance/revocation diagnostics and runtime pointer refresh.
4. CI tests for complete OKF/profile conformance and product/private boundaries.

Private bundle implementation follows only after Owner Context, enrollment, user-local storage, denial barriers, deletion propagation and approved compilation-provider contracts exist.

## Test expectations

- OKF core conformance and permissive handling of unknown types and optional fields;
- fail-closed handling of unknown security-bearing Maestro profile values;
- physical isolation and no cross-bundle links;
- durable outbox recovery after interruption;
- idempotent retry and safe coalescing;
- source-watermark discard and retry;
- atomic concept, index and backlink publication;
- immediate revocation despite compiler or queue failure;
- no private content or identifying metadata in managed bundles, logs or receipts;
- deletion propagation and crypto-erasure where policy requires it;
- equivalent Claude and Codex routing over the same active manifest.

## Deferred decisions

- exact managed content allowlist and concept taxonomy;
- user-facing command vocabulary and update diagnostics;
- approved private compilation provider and offline behavior;
- private retention, deletion and crypto-erasure policy;
- freshness targets and retry/backoff limits per event class;
- organizational bundle approval and cross-bundle composition after V1.

## Private workspace-memory compiler v1

Decision PWKC authorizes a deterministic, model-free private compiler over one
admitted workspace's committed memory rollups. This is a separately versioned
local compilation contract, not approval for an external provider or an
enterprise tenant. Dreaming remains the rollup producer. Authored atlas pages,
raw captures, owner-global content, other workspaces and SharePoint sources are
outside this profile.

### Local authority and portable profile

The new recognized profile is `private-memory-local-v1`, with explicit
`x-maestro-authority-kind: local-home-workspace`. Bind opaque domain-separated
home/workspace references to the actual admitted private home and registered
workspace. Do not manufacture tenant/person identity or promote caller-declared
history OwnerID. Enterprise and cross-owner composition are unavailable. The
existing generic/managed profile remains separate; unknown security-bearing
profile values refuse authorization.

Use OKF v0.2 concepts, standard relative links, reserved indexes/log and
bundle-relative concept IDs. Pin the normative reference to upstream commit
`782c937447bc1a822cb5330142c37fb629a59adb`, blob
`c06e3eede0c910d0ecf12524c34204156f8795ac`. The local profile additionally binds
source commit, rollup versions, current denial generation, keyed source
fingerprint, policy/compiler/configuration provenance and the admitted scope.
Public diagnostics/logs/receipts carry opaque metadata, never private bodies,
names, source paths or reusable plain hashes of private sources. Read the
owner-reviewed key at `.attestation/owner-reviewed.key`, already prepared by
`PrepareOwnedCaptureAuthority`, through memory's `ownedKey` path with
`create=false` for a narrow domain-separated wiki identity operation. Do not
require `capture-v2.key`, fall back between keys, export a key or create one on
reads. This identifies the actual local-owner authority initialized by the
product, without claiming a skill-route attestation or enterprise principal.

### Source ownership and budgets

`internal/privatewiki` owns pure deterministic compilation and profile/graph
validation. Memory owns source selection, snapshots, outbox, confined IO,
publication, readback and retirement. The pure compiler cannot acquire a
filesystem root, source authority, provider or publication permission.

The fixed private derivative root is:

```text
<resolved-home>/brain/memory/workspaces/<registered-workspace-id>/.private-wiki/
```

It has separate staging, immutable versions and an exact-parent current
pointer. It shares neither a pointer nor a manifest with source memory, authored
atlas, managed content or another scope. Only explicit owning compilation or
source mutation may prepare fixed private metadata; status, query and startup
never initialize, repair, lock or create keys/directories.

Select eligible L1/L2/L3/lifetime artifacts from one newest valid canonical
commit, using the owning artifact/eligibility reader in canonical order. Cap
the complete source set at 128 artifacts, 1 MiB each and 8 MiB aggregate. Cap a
complete generation at 1,024 files and 16 MiB, including frontmatter, indexes,
backlinks, manifest and log. Overflow refuses without silently dropping sources.
Bound public page reads and cursors to one exact authorized generation.

### Source events and cohort compatibility

Persist and fsync a narrowly typed commit-intent event before the immutable
commit rename makes that source visible, under the same owning guard. The event
references the prospective source commit, scope, policy, actual commit time and
opaque event ID. An orphan intent grants nothing. Reconciliation always checks
current source truth before acknowledging an event.

Publish compact cumulative denial-event metadata in the same atomic visibility
checkpoint bytes as its monotonic denial Generation and permanent denied map.
Capture-only revision updates preserve that event. A later queue failure cannot
undo the barrier or reopen an old generation; acknowledgment never deletes a
denial fence.

The private checkpoint extension has an explicit `private-memory-wiki-v1`
writer/reader cohort. New readers strictly accept validated historical records
without the extension and recognized current records; partial/unknown cohort
metadata refuses. Old strict decoders deliberately refuse eventful checkpoints
and cannot silently rewrite them without the new fence. The first explicit
owning mutation/compilation upgrades the checkpoint before derived publication;
queries never upgrade it. Historical commits/checkpoints without events remain
source truth. Missed-event recovery is labelled reconciliation with its actual
recovery time, never invented original durability or historical timestamps.

### Publication, current views and retirement

A watermark pins exact valid commit/artifact versions, denial Generation,
source policy and admitted scope. Capture Revision alone does not invalidate
committed rollups. Stage and validate the entire graph, then recheck watermark
and expected atlas parent under the established order: memory activation/
visibility lock, live product control, current scheduler lease, derived pointer
publication. Never request the memory lock while holding a wiki lock in reverse
order. Actual compiler/configuration provenance participates in generation
identity; clock/nonce-only retries cannot manufacture execution evidence.

The first profile conservatively hides the entire view whenever the selected
committed source or denial generation changes. No last-known-good fallback may
serve a replaced/removed rollup. Readback uses bounded before/after authority
checks and creates no activation lock. Immutable concepts, indexes, backlinks,
log and pointer are one graph transaction, with exact-parent CAS and the owning
held-directory/no-follow/private-owner/single-link/fsync primitives.

After denial, retire the fixed derived staging, all versions, pointer, indexes,
backlinks and cached bodies. Use bounded held-directory deletion; an alias must
refuse cleanup without touching outside data. The already committed barrier
continues to hide every old view if compile, queue or cleanup fails. Report
incomplete physical retirement honestly, and retry it before any new view can
be admitted or published. Retain cumulative denial metadata outside retired
derived bodies. Windows remains unavailable until its held-handle contract is
qualified.

Incremental sync, full reconciliation and integrity diagnosis must share this
owning transaction and actual durable evidence. Scheduler receipts cannot
substitute for a readable current graph. Startup stays read-only. The cohesive
delivery includes all three real job bindings and bounded product readback;
source tests do not establish native wake, deployment or pilot acceptance.

### Compatible readers and current-build qualification

Compatible readers may consume a recognized, completely validated and sealed
current-source graph produced by another executable/configuration. Preserve
and validate its actual stored provenance; do not synthesize current provenance
or require byte-for-byte re-rendering by a changed current implementation.
Report provenance mismatch separately and require genuine current-build/config
compilation for current job qualification. This does not relax local authority,
exact source/denial, profile, policy, checkpoint cohort or graph integrity checks;
unsupported security contracts still refuse.

### Product inspection, event execution and queries

Memory may expose one bounded body-free maintenance inspection projection over
its existing source/event/cohort/graph readers. It reports recognized source
readiness, actual eligible durable event identity/time, denial/retirement state,
admissible parent, keyed graph evidence/counts and actual producer/current-build
flags. Valid historical source without a graph is distinct from partial protected
authority. Inspection creates no key, directory, lock, event, repair or grant,
and exports no private body, raw manifest, source filename or plain content hash.
Pending retirement stays unavailable for reads and qualification. The existing
owning compiler may retry physical retirement under its normal live grant;
inspection cannot authorize publication or success before cleanup completes.

The incremental job is event-only. Product maintenance derives its occurrence
from that actual event and reuses the existing Worker, grants, lease and terminal
fences; it cannot fall back to calendar/presence or invent an event on a read.
An explicit protected event-only schedule field must make old strict readers
refuse that record. Read back a typed durable event execution receipt through
the existing maintenance evidence owner; never fabricate a calendar receipt or
add another queue. Reconciliation recovers valid historical missing events.

Current sync/reconcile qualification requires genuine current-build/configuration
compilation and complete owning graph validation. Verified no-change is valid
normal execution but cannot mint qualification. Integrity performs an actual
complete check over a genuinely current graph and persists only body-free
diagnostics; it never compiles or repairs. Source drift, pause/revoke and lease
loss must block pointer, diagnostic and terminal success publication.
Qualification retains body-free evidence of that genuine historical successful
run under the same build/configuration and live control epoch. Later ordinary
source events do not themselves erase qualification or require the historical
graph to remain readable. Every new execution reviews its current source/event
and independently validates its actual result before success; old qualification
cannot substitute for that proof or reopen a stale/denied view.

The selected-workspace product query has only wiki.status, wiki.read and
wiki.source variants. Read/source require exact generation, registered reference,
finite offset and 4–4,096-byte UTF-8 pages. Derive scope from admitted binding;
accept no workspace/root/provider/identity/approval argument. Status/startup
remain read-only. Fixed metadata failure codes disclose no private error body.

### Native availability is independent of ordinary memory

The private-wiki native gate admits the actual registered owner/workspace only
when its supported retained-namespace operation succeeds. On Windows, complete
source, staging/version and pointer custody supplements ordinary private-memory
admission; the operation retains those handles through its effects. Unavailable
custody returns the distinct native-unavailable error before source access and
cannot downgrade a protected cohort. Current-platform execution, native host
loading and distribution remain independently evidenced; no imported flag or
prior startup observation can authorize publication or retirement.
