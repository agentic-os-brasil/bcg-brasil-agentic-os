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
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/wiki-okf
x-bcgos-scope: managed
x-bcgos-source-fingerprint: 9aff0e5974db0dde5642e35230dec2ac150ae00a0eb8ac0c5d1d6be8711f494d
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: bcgos-managed-wiki/0.2
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `specs/008-wiki-update-okf.md`. The source remains authoritative.

## Related

- [Content navigation through a compiled LLM wiki](/concepts/content-navigation.md)
- [Human atlas bootstrap](/concepts/human-atlas-bootstrap.md)
- [Maestro release and distribution](/concepts/release-distribution.md)
- [Wiki and atlas entrypoint](/concepts/wiki-entrypoint.md)

## Source content

# Spec 008 - Wiki update lifecycle and BCGOS OKF profile

Status: architecture accepted; initial managed compiler/validator implemented; schemas, event outbox, durable manifests and runtime integration pending.

## Objective

Update each compiled wiki safely and incrementally while keeping its output portable across humans, agents and tools.

Open Knowledge Format v0.2 defines the exchange envelope. The BCGOS Atlas Profile v1 defines the additional governance and lifecycle required for professional, owner-private and workspace-private knowledge. OKF remains a format, not an authorization or storage system.

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

The root `index.md` declares `okf_version: "0.2"`. BCGOS does not introduce a competing concept-ID field or a proprietary link syntax.

### BCGOS Atlas Profile v1

The profile extends concept frontmatter with namespaced governance metadata:

```yaml
type: Memory Rollup
title: Example thematic route
description: Compact authorized navigation summary.
resource: bcgos://memory/opaque-resource-reference
tags: [memory, thematic]
timestamp: 2026-07-20T00:00:00Z
x-bcgos-profile-version: "1"
x-bcgos-stable-id: opaque-stable-id
x-bcgos-scope: workspace
x-bcgos-tenant-ref: opaque-tenant-ref
x-bcgos-owner-ref: opaque-owner-ref
x-bcgos-workspace-ref: opaque-workspace-ref
x-bcgos-memory-layer: L3
x-bcgos-memory-commit: opaque-commit-version
x-bcgos-rollup-version: opaque-rollup-version
x-bcgos-source-fingerprint: opaque-keyed-fingerprint
x-bcgos-sensitivity: client_restricted
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: "1"
x-bcgos-policy-version: "1"
```

Managed concepts omit owner and workspace references. Source relations and citations remain standard Markdown links in the body. The `resource` field or citations may use opaque BCGOS URIs that require authorization to resolve.

BCGOS consumers preserve unknown OKF fields. Unknown `type` values remain consumable as generic concepts. Unknown or missing values for security-bearing profile fields such as scope, sensitivity, status or policy version fail closed within BCGOS runtimes.

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

These are logical paths; approved operating-system directories remain pending `bcgos init` decisions.

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
  -> BCGOS profile and security validation
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

Hard BCGOS checks:

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

OKF's permissive consumption remains intact; the stricter hard checks are requirements of the BCGOS profile and do not redefine OKF conformance.

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
validator are now available through the development-only harness. It writes a
reviewable local candidate with a best-effort directory swap; it is not yet the
durable versioned-manifest or last-known-good publication mechanism. The
remaining items are the durable lifecycle pieces:

1. BCGOS Atlas Profile v1 schema and OKF validator.
2. Deterministic compiler for allowlisted managed sources.
3. Staging, source watermark, atomic manifest and last-known-good behavior.
4. Generated `index.md`, `log.md`, backlinks and lint report.
5. CI tests for OKF conformance, profile enforcement, determinism and product/private boundaries.

Private bundle implementation follows only after Owner Context, enrollment, user-local storage, denial barriers, deletion propagation and approved compilation-provider contracts exist.

## Test expectations

- OKF core conformance and permissive handling of unknown types and optional fields;
- fail-closed handling of unknown security-bearing BCGOS profile values;
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
