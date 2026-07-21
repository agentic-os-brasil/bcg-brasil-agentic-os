# Spec 007 - Content navigation through a compiled LLM wiki

Status: architecture accepted; managed atlas implementation and private memory atlas pending.

## Objective

Make professional content progressively easier for users and agents to navigate without treating raw retrieval, complete prompt injection or repeated synthesis as the primary knowledge interface.

The Agentic OS adopts a Karpathy-inspired compiled LLM wiki: durable sources feed governed canonical artifacts, and a derived interconnected atlas exposes topics, relationships, summaries and pointers for later queries. The implementation is intentionally stricter than a wiki that an LLM may rewrite freely because professional and client contexts require explicit authority, provenance and deletion propagation.

## Roles of the knowledge surfaces

The wiki does not replace other sources of truth:

| Surface | Role | Authority |
|---|---|---|
| Original sources | Evidence such as documents, notes and system records | Authoritative within the owning system |
| Specs and decisions | Product and governance contracts | Authoritative for the managed product |
| Skills, agents and playbooks | Versioned operating procedures | Authoritative within their compatible bundle version |
| Owner and workspace memory | Governed continuity across L1, L2, L3 and lifetime | Authoritative only through the memory contract and active valid commit |
| Tasks and operational systems | Current commitments and execution state | Authoritative within the owning task or work system |
| Wiki | Compiled navigation, relationships, summaries and drill-down pointers | Derived and regenerable; never independently authoritative |

## Compiled navigation pipeline

```text
original sources
  -> governed canonical artifacts
  -> scoped compiled atlas
  -> intent-routed pages and pointers
  -> source drill-down on demand
```

Ingestion and query may propose new canonical synthesis, but generated navigation cannot silently promote a claim into an authoritative source. Reusable synthesis must first land in an owned canonical artifact or remain explicitly marked as derived.

## Atlas scopes

### Managed product atlas

The managed atlas navigates sanitized, distributable Agentic OS content such as accepted decisions, specs, product skills, agent definitions, playbooks and approved public documentation.

- It is generated from an explicit allowlist.
- It ships only managed content and contains no user, employee, client, case or workspace data.
- Its version and compatibility range follow the managed bundle.
- Git diff and CI provide review evidence for generated changes in the source repository.

### Owner and workspace private atlas

The private atlas navigates content authorized for one enrolled owner and, when applicable, one enrolled workspace. It may include pointers to owner context, local professional knowledge, tasks and memory, subject to the policy and authority of each source.

- It lives in local user data, never in the managed bundle or shared product atlas.
- Owner and workspace roots remain physically and logically separate.
- A workspace atlas cannot read another workspace or the owner-global domain without an explicit compatible policy.
- Client content never becomes organizational knowledge implicitly.
- Compilation, synchronization and provider use remain unavailable until privacy, storage and deletion contracts are approved.

## Memory navigation

The private atlas may navigate memory without becoming another memory store:

- **Lifetime:** topic and principle pages may point to eligible durable entries and their provenance.
- **L3:** thematic pages may expose current medium-term threads and deeper evidence pointers.
- **L2:** time and topic navigation may point to valid weekly rollups.
- **L1:** the atlas may expose local day or session pointers when policy permits, but must not copy raw or unbounded capture content into generated pages.

Only artifacts reachable from the newest fully valid memory commit may be indexed as active memory. Missing, stale, corrupt, conflicted or unauthorized layers are omitted with diagnostics; the wiki never falls back to raw history silently.

A wiki entry derived from memory must carry the owner ID, workspace ID when applicable, memory layer, opaque artifact version, provenance pointer, sensitivity class, freshness state and generation policy version. These fields are routing and invalidation metadata, not permission to reveal their content.

Memory correction, deletion, eligibility reversal or commit invalidation must remove or invalidate every affected wiki page, index record, backlink and cache entry. A stale derived page may not preserve information that the owning memory surface has removed.

## Minimal page and index contract

Every generated page or index record requires:

- stable scoped identifier;
- title and concise summary;
- atlas scope and content type;
- canonical source pointers;
- source version or fingerprint;
- provenance and generation timestamp;
- freshness and lifecycle status;
- sensitivity classification;
- outbound relations and backlinks;
- generator and schema version.

Human-readable Markdown is the primary navigation format. A deterministic machine-readable index may support routing, validation and adapters. The index is generated and must never become a separately maintained source of truth.

## Query and context behavior

Navigation follows least-context disclosure:

1. Resolve actor, owner, tenant, workspace, purpose and applicable policy.
2. Select only atlas scopes authorized for that request.
3. Route the intent to a bounded set of topic or entity records.
4. Inject concise summaries and pointers within an explicit budget.
5. Read canonical sources or deeper memory only on demand and with the same authorization.

Session start receives only a compact navigation projection. It must not inject the complete wiki, traverse private scopes speculatively or treat a wiki summary as more authoritative than the source it references.

Claude and Codex adapters consume the same scoped index and return equivalent provenance, omissions and failure states even when their native search or hook mechanics differ.

## Generation, invalidation and health

The atlas generator must be idempotent for the same ordered source set, schema and policy. Generation occurs in staging and publishes a complete view only after validation. The last known-good atlas remains active when generation fails.

Required health outputs are:

- index of pages and source pointers;
- backlinks;
- broken-link and orphan report;
- stale, missing and unauthorized-source diagnostics;
- generation log with source fingerprint and policy version.

Graph visualization, embeddings, vector search, Obsidian compatibility and a graphical wiki are not V1 requirements.

## V1 implementation boundary

V1 implements only the managed product atlas:

1. Define the page and machine-readable index schemas.
2. Compile allowlisted decisions, specs, product skills and product documentation.
3. Generate an index, backlinks, orphan/broken-link diagnostics and a generation log.
4. Validate determinism, provenance, exclusions and development-boundary compliance in CI.
5. Expose bounded product-content pointers for future runtime adapters.

The private owner/workspace atlas, including memory navigation, is part of the accepted architecture but remains unavailable until Owner Context, enrollment, approved local storage and correction/deletion propagation are implemented and tested.

## Test expectations

- deterministic output for an unchanged source set;
- allowlist-first inclusion and deny-by-default behavior;
- no development-harness or private content in the managed atlas;
- physical separation between managed and private roots;
- stable source provenance and drill-down pointers;
- complete invalidation after source correction or deletion;
- no indexing of memory outside the newest valid commit;
- owner and workspace isolation for every private query;
- bounded context projection and equivalent Claude/Codex failure reporting;
- last-known-good preservation after invalid generation.

## Deferred decisions

- exact managed content allowlist and page taxonomy;
- approved owner-context and workspace enrollment contracts;
- private-atlas compilation provider and offline behavior;
- per-layer memory indexing, retention and context budgets;
- organizational knowledge approval, synchronization and retirement;
- user-facing `bcgos wiki` or `bcgos knowledge` command vocabulary.
