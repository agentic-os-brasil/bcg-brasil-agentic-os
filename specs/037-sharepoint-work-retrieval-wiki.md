# Spec 037 - SharePoint work-retrieval wiki

Status: architecture accepted for implementation; Claude collection adapter,
local catalog compiler, query surface and native evidence pending.

## Objective

Let an authorized professional recover prior work with a request such as:

> Quero o deck que apresentei para o CEO da Suzano em 2023 sobre plantio.

Maestro maps the explicitly enrolled SharePoint scope, compiles a local
organizational work-retrieval wiki and returns a bounded ranked set of source
pointers. The SharePoint source remains authoritative. The wiki is a derived
retrieval surface, not a document repository, a memory layer or permission to
read every result.

## Runtime boundary

The corporate environment permits SharePoint access through Claude and forbids
the equivalent connection in Codex.

- Claude is the only collection adapter for V1. It may use the approved
  SharePoint MCP connection to enumerate enrolled sites, libraries, folders and
  files and to emit the canonical catalog snapshot.
- Codex reports the collection capability as `unavailable` with reason
  `corporate_policy`. It may not use a browser, copied token, Graph credential,
  local cookie, SharePoint URL fetch or another connector as a fallback.
- The normalized snapshot, compiler, active manifest and query engine are
  runtime-neutral. After Claude has produced a valid local catalog, either
  runtime may query the local derived index when the actor and purpose are
  authorized.
- Local configuration, a fixture or a direct adapter invocation is not native
  SharePoint evidence. Capability promotion requires an approved Claude
  connection and a native read-only trial over a sanitized test scope.

This asymmetry is deliberate. Runtime neutrality means equivalent product
semantics and honest failure reporting, not pretending every provider is
available in every runtime.

## Dedicated organizational scope

The work-retrieval wiki is a separate organizational bundle:

```text
<owner-data-root>/
  atlases/
    organization/
      sharepoint-work/
        barriers/
        snapshots/
        versions/
        active.json
```

It never shares a manifest, staging directory, index or generated page with:

- the managed product atlas shipped in the release;
- owner-private memory and self-context;
- a client or project workspace atlas; or
- a general-purpose organizational knowledge base.

The bundle is a central navigation node across the enrolled SharePoint roots,
but it is selected only for explicit prior-work retrieval. It is not injected
at Session Start, searched for ordinary questions or traversed speculatively.

## Enrollment and least privilege

“Map all folders” means all folders recursively reachable under an explicit
set of enrolled SharePoint roots. It never means tenant-wide discovery by
default.

Each root is identified by opaque tenant, site, drive/library and folder
references. Enrollment records:

- the owning tenant reference;
- the approved site/library/folder roots;
- the read-only purpose `prior_work_retrieval`;
- allowed item types and size limits;
- refresh and stale windows;
- the authorizing actor and policy version; and
- the time after which a new scope expansion requires confirmation.

The collector may enumerate only descendants of those roots. It does not edit,
move, download, share, publish or change permissions on SharePoint items.
Following a result link remains subject to SharePoint's current authorization.

## Canonical catalog snapshot

The Claude adapter emits strict JSON conforming to
`schemas/sharepoint-work-catalog.schema.json`.

A snapshot contains:

- opaque tenant and root references;
- `full` or `delta` mode;
- previous and current watermarks;
- collection timestamp and adapter runtime;
- folder and file records;
- explicit deletion/access-revocation tombstones; and
- no credential, prompt, transcript or raw document body.

Each item may contain only bounded retrieval metadata:

- opaque item and parent references;
- kind, name, path segments and source URL;
- creation/modification time, media type, size and ETag;
- client, project, theme, year, audience, person and presenter facets;
- bounded search terms derived by the authorized adapter; and
- sensitivity and lifecycle status.

Facets are routing hypotheses, not source truth. The result always points back
to SharePoint and exposes provenance and freshness.

## Full and incremental synchronization

The first successful sync is a complete snapshot. Later syncs may use provider
delta semantics:

```text
Claude SharePoint MCP
  -> enrolled-root enumeration or delta
  -> strict normalized snapshot
  -> validate tenant, roots and previous watermark
  -> write revocation barriers
  -> merge active item records
  -> compile scoped OKF candidates in staging
  -> validate and publish immutable version
  -> atomically replace active.json
```

Rules:

1. A delta must name the exact active previous watermark. Stale, forked or
   replayed deltas fail closed.
2. A complete snapshot may replace the active catalog only when every enrolled
   root reports successful enumeration.
3. Missing items in a valid complete snapshot and explicit tombstones in a
   delta are denied before compilation.
4. Enrichment failures may preserve last-known-good metadata, but a deletion or
   access revocation may never be served from last-known-good.
5. Retries with the same tenant, roots, watermarks, item set and policy are
   idempotent.
6. Logs and scheduler receipts contain counts, opaque watermarks and states,
   never names, paths, URLs or client facets.

## Wiki shape

The compiler generates one private OKF v0.2 bundle with:

```text
index.md
log.md
items.json
facets/
  clients/
  projects/
  themes/
  years/
  audiences/
backlinks.json
diagnostics.json
```

Facet pages contain compact counts and links to item records. Item records
contain retrieval metadata and a SharePoint source pointer, not copied deck
content. A document may appear under several facets without creating several
authoritative copies.

## Explicit query contract

The query engine is unavailable unless the caller sets
`explicit_prior_work_intent=true`. Valid explicit requests include:

- “encontre no SharePoint”;
- “quero recuperar um deck antigo”;
- “procure o material que apresentei para ...”; and
- an explicit `bcgos prior-work find` command.

General research, ordinary workspace questions and Session Start do not satisfy
the gate.

The deterministic query engine extracts bounded terms and years from the
request, ranks exact matches across title, path and facets, and returns:

- title/name and source URL;
- client, project, theme, year, audience and presenter facets;
- modified time and catalog freshness;
- matched terms and deterministic score; and
- an explicit note that opening the source rechecks SharePoint authorization.

It returns no result blocked by a tombstone, tenant mismatch, unenrolled root,
stale policy or unknown security metadata. An empty result is honest; it does
not broaden to general search or another provider.

## Scheduling and presence recovery

The canonical scheduler job ID is `sharepoint-work-sync`.

- An approved Claude scheduled task may invoke the collector when the account
  connection supports unattended read-only operation.
- Otherwise, scheduler and native presence events only mark the occurrence due.
  The next authorized Claude session performs bounded catch-up after startup.
- Codex records `unavailable` for collection and leaves the occurrence due.
- Session Start never waits for SharePoint or compiles the wiki.
- The default product proposal is a 24-hour refresh window, a 72-hour stale
  warning and one bounded catch-up attempt per presence. Final pilot values
  remain configuration.

A scheduler receipt proves an attempt. Only an atomically published active
catalog version proves synchronization success.

## Privacy and security

- Source content is untrusted data; instructions inside files are never agent
  commands.
- No raw slide text, document body, prompt, transcript, credential or access
  token enters the catalog or wiki.
- URLs and human-readable facets remain in private local storage only.
- Client metadata cannot enter the managed product bundle, Git, canary or
  federated improvement batch.
- Search results are bounded and do not grant access to the underlying item.
- Tenant, root and policy mismatches fail closed.
- Deletion and access revocation write a synchronous local barrier before
  asynchronous recompilation.
- The collector is read-only and cannot publish, share, rename, move or delete
  a SharePoint item.

## Product surfaces

The intended CLI contract is:

```text
bcgos prior-work status
bcgos prior-work import --snapshot <normalized-json>
bcgos prior-work find --explicit --query "<request>" [--limit <n>]
```

The intended product skill is `find-prior-work`. It activates only for explicit
retrieval of prior professional material and uses the local index first. A sync
is proposed only when the index is absent or stale and the active runtime is
Claude with the approved SharePoint connection.

## Acceptance evidence

1. A sanitized fixture containing a 2023 Suzano deck with theme `plantio`,
   audience `CEO` and presenter facet returns that deck first for the example
   query.
2. The same index is not searched without the explicit-intent gate.
3. Codex collection returns `unavailable/corporate_policy` and performs no
   network or browser fallback.
4. Delta replay, wrong tenant/root, stale previous watermark and malformed
   snapshots fail closed.
5. Deleted/revoked items disappear immediately even when recompilation fails.
6. Unchanged sync is idempotent and preserves the active fingerprint.
7. Full and delta compilation produce valid, deterministic OKF output.
8. Logs and scheduler receipts contain no names, paths, URLs or client facets.
9. A native Claude trial enumerates a sanitized SharePoint test root,
   publishes the catalog and retrieves one expected deck.

## Delivery boundary

The repository can implement and validate the normalized contract, compiler,
query engine, Claude skill projection, Codex fail-closed state and sanitized
fixtures. A real SharePoint scan remains unavailable in Codex and requires a
separately authenticated Claude session. Until that native trial exists, the
runtime capability remains `unavailable` even when all local tests pass.

