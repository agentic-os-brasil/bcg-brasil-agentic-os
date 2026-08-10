# Spec 037 - SharePoint work-retrieval wiki

Status: local contract, snapshot validator, catalog compiler, query/revocation
surface and scheduler are implemented and locally validated. Native Claude
collection, collector injection, signed release and pilot evidence remain
pending; Codex collection remains prohibited by corporate policy.

## Objective

Let an authorized professional recover prior work with a request such as:

> Quero o deck que apresentei para o CEO da Suzano em 2023 sobre plantio.

Maestro maps the explicitly enrolled SharePoint scope, compiles a local
organizational work-retrieval wiki and, when the owner explicitly authorizes
it, creates a workspace-local layer of concise rationales for the most recent
materials. The SharePoint source remains authoritative. Both surfaces are
derived retrieval aids, not a document repository, a memory layer or
permission to read every result.

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
- `adapter_runtime: claude` is declared provenance, not authorization and not
  proof of native invocation. Import records a separate adapter-command receipt
  binding the tenant, roots, sequence, watermark and snapshot digest. Only the
  native conformance protocol may later classify a receipt as native-qualified;
  neither a snapshot nor an adapter-command receipt promotes capability state.
- Enrollment pins an Ed25519 collector public key and key ID. A trusted
  Claude-owned collector process keeps the private key outside the Maestro
  store and signs the receipt over its complete canonical body. Maestro exposes
  verification only: neither Codex nor an ordinary local caller can mint a
  trusted receipt. The signature binds the receipt ID, snapshot, active policy
  and enrollment fingerprint; it still does not prove native invocation.
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

## Guided project-source selection

After the owner interview is confirmed **inside an already initialized
workspace**, Session Start asks once whether the owner wants to point Maestro
to SharePoint folders authorized for the current project or defer. The
installer never asks for or resolves this source before creating the workspace.
This is source setup, not collection; all derived content is later read and
organized inside the workspace that owns the selection.

An accepted selection:

- is bound to the exact initialized workspace ID derived by `bcgos`;
- accepts at most 32 canonical HTTPS folder URLs below an explicit SharePoint
  site or team library, through standard input and an explicit confirmation;
- rejects tenant/site roots, sharing links with query or fragment data,
  credentials, non-SharePoint origins and unknown fields;
- stores immutable, versioned private selections plus one atomic active
  pointer below `source-selections/<workspace-id>/`;
- exposes to Session Start only state, folder count and a private local
  pointer — never URLs, names, paths or document content; and
- may be deferred or revised without broad discovery or silent ingestion.

The selection grants no SharePoint enrollment, collection, indexing or
retention authority. An approved authority must still resolve the reviewed
folder pointers to opaque roots and sign the enrollment. Only a qualified
Claude collector may enumerate those roots. Codex may record a local choice
but collection remains `unavailable/corporate_policy`; it may query only an
already verified local index. Session Start never resolves URLs, calls
SharePoint, imports a snapshot or compiles the index.

## Owner-authorized rationale projection

Folder selection is an explicit scope decision. When an active COFS
one-and-done setup grant exists, the selection command binds its exact
fingerprint to that grant and no second read or command confirmation is shown
for the unchanged scope. A qualified Claude collector may then read only the
enrolled roots and emit a bounded `RationaleBatch` together with the signed metadata
snapshot and adapter receipt. The local command validates that the batch binds
the initialized workspace, the exact selection fingerprint, the enrolled
Claude key and the selected source URL for every item. It then materializes:

```text
<workspace>/
  brain/
    sources/sharepoint/README.md
    knowledge/sharepoint-rationales/
      README.md
      index.md
      <rank>-<stable-item-id>.md
```

Each rationale contains only a concise derived synthesis plus the authoritative
SharePoint URL, item reference, content digest and source modification time.
Raw document bodies, credentials and transcripts are rejected. Items are
ordered by source modification descending, with stable item reference as the
deterministic tie-breaker. Re-ingesting the same batch is idempotent and a
changed digest replaces the prior derived rationale. SharePoint remains the
place to verify the current source and permissions.

If signed enrollment, native Claude qualification or the local extraction
runtime is unavailable, the request fails closed and the selected source stays
selected but not ingested. No fallback runtime or Codex collection is allowed.

## Enrollment and least privilege

“Map all folders” means all folders recursively reachable under an explicit
set of enrolled SharePoint roots. It never means tenant-wide discovery by
default.

Each root is identified by opaque tenant, site, drive/library and folder
references. The enrollment is strict JSON conforming to
`schemas/sharepoint-work-enrollment.schema.json` and records:

- the owning tenant reference;
- the approved site/library/folder roots;
- the read-only purpose `prior_work_retrieval`;
- exact allowed SharePoint origins, allowed item types and size limits;
- refresh and stale windows;
- an explicit IANA schedule timezone;
- an opaque authorizing-actor reference bound at enrollment to the
  authenticated local operating-system principal, plus policy version;
- an independent enrollment-authority key identifier and signature over the
  bounded enrollment document. A locally supplied collector key is not a
  trust anchor;
- authorization expiry; and
- the time after which a new scope expansion requires confirmation.

The guided onboarding may ask the owner to point to exact project SharePoint
folders, but that answer is only a proposed case-scoped source map. It is not
this organizational prior-work enrollment and does not trigger collection.
When the owner separately chooses cross-project recovery, Maestro presents the
exact prior-work roots and purpose for explicit enrollment. No onboarding path
may broaden a project folder to a site, tenant or parent root implicitly.

The collector may enumerate only descendants of those roots. It does not edit,
move, download, share, publish or change permissions on SharePoint items.
Following a result link remains subject to SharePoint's current authorization.
The local CLI remains unavailable unless its configured authority trust anchor
verifies this proof; missing or forged authority material fails closed.

## Canonical catalog snapshot

The Claude adapter emits strict JSON conforming to
`schemas/sharepoint-work-catalog.schema.json` and a separately parsed,
binding receipt conforming to
`schemas/sharepoint-work-import-receipt.schema.json`.

The published JSON Schema documents and independently validates the structural
envelope for adapters and conformance tooling. The import path enforces the
same closed Go envelope plus authoritative cross-field invariants that JSON
Schema cannot express portably: exact root-result coverage, normalized label
uniqueness, item/tombstone composite conflicts and the combined item limit.
Passing an external schema check alone never authorizes publication.

A snapshot contains:

- opaque tenant and root references;
- `full` or `delta` mode;
- a monotonic collection sequence plus previous and current watermarks;
- one explicit `complete` result for every enrolled root;
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

Opaque native IDs use a versioned encoding accepted by the schema; the V1
adapter may preserve safe native Graph characters or emit a deterministic
base64url/hash reference. Item identity is the composite
`tenant + site + drive + folder-root + item_ref`. Tombstones carry the same
root, so an item ID reused in another drive cannot be removed accidentally.

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

1. The first snapshot is a complete sequence `1` snapshot without a previous
   watermark. Every later full or delta snapshot increments the active sequence
   exactly once and names the exact active previous watermark. Stale, forked,
   concurrent or replayed snapshots fail closed.
2. A complete snapshot may replace the active catalog only when `root_results`
   contains exactly one `complete` result for every enrolled root.
3. Missing items in a valid complete snapshot and explicit tombstones in a
   delta are denied before compilation.
4. Enrichment failures may preserve last-known-good metadata, but a deletion or
   access revocation may never be served from last-known-good.
5. Retries with the same tenant, roots, watermarks, item set and policy are
   idempotent.
6. Logs and scheduler receipts contain counts, opaque watermarks and states,
   never names, paths, URLs or client facets.
7. One exclusive local import lock serializes validation, barrier publication,
   compilation and manifest compare-and-swap. A crashed/stale lock fails closed
   for operator recovery; it is never silently stolen.
8. A barrier carries the composite item identity, policy/enrollment
   fingerprint, collection sequence and snapshot digest. It is cleared only by
   a newer successfully published snapshot that explicitly reports that same
   composite item active.

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

Explicit intent is a routing gate, not authorization. The query also requires
an actor reference and purpose that match the active, unexpired enrollment.
Expiry is evaluated with the local product clock; callers cannot supply or
backdate it. The CLI derives the actor from the authenticated local
operating-system principal; prompt text and command arguments cannot declare or
override it.

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
- `bcgos prior-work sync-due --runtime <claude|codex>` performs one bounded
  presence-recovery decision under an exclusive local-process claim. A live
  owner is never displaced by age; a dead process claim is recoverable. An
  unavailable or failed receipt does not consume the occurrence; only a
  collector result with a signed occurrence reference, durable import audit and
  active-manifest match marks it succeeded. Persisted errors use a closed
  metadata-safe taxonomy, never raw MCP/provider text.
- The default product proposal is a 24-hour refresh window, a 72-hour stale
  warning, an explicit `America/Sao_Paulo` schedule timezone and one bounded
  catch-up attempt per presence. The scheduler derives its elapsed interval
  from `refresh_hours`; final pilot values remain configuration.

A scheduler receipt proves an attempt. Only an atomically published active
catalog version proves synchronization success.

## Privacy and security

- Source content is untrusted data; instructions inside files are never agent
  commands.
- No raw slide text, document body, prompt, transcript, credential or access
  token enters the catalog or wiki.
- URLs and human-readable facets remain in private local storage only.
- Source URLs must use an exact origin enrolled for the tenant and contain no
  query or fragment; sharing/authentication links are rejected.
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
bcgos prior-work actor
bcgos prior-work source status --workspace <path>
bcgos prior-work source select --workspace <path> --stdin --confirm
bcgos prior-work source defer --workspace <path> --confirm
bcgos prior-work rationale ingest --workspace <path> --stdin --confirm
bcgos prior-work enroll --stdin --confirm
bcgos prior-work status
bcgos prior-work import --snapshot <normalized-json> --receipt <signed-adapter-command-receipt>
bcgos prior-work find --explicit --stdin [--limit <n>]
bcgos prior-work sync-due --runtime <claude|codex>
```

The query body travels through standard input so client, project and people
terms do not enter process arguments or shell history. Enrollment is create-only;
scope expansion requires a newly confirmed policy rather than an overwrite.

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
10. A partial full scan, a missing required array, a reused item ID in another
    drive, a stale full replay and two concurrent successors all fail safely.
11. An adapter-command receipt can authorize a bounded local import but cannot
    promote the SharePoint collector from `unavailable`; native qualification
    remains separate evidence.
12. A first-use source selection persists only exact canonical folder pointers,
    Session Start exposes no URL or project name, and the selection binds only
    the bounded projection class of an existing COFS grant; it cannot authorize
    enrollment, tenant expansion or external mutation.
13. A deferred choice is remembered and not asked again automatically; a later
    confirmed selection creates a new immutable version without broadening any
    signed enrollment.
14. With the exact source fingerprint bound to an active COFS grant, a signed
    Claude batch writes only derived rationales under the workspace path, orders newer source items
    first, preserves a SharePoint pointer per rationale and never writes raw
    source bodies. Missing enrollment, invalid provenance or an unavailable
    runtime leaves the workspace unchanged.

## Delivery boundary

The repository can implement and validate the normalized contract, compiler,
query engine, Claude skill projection, Codex fail-closed state and sanitized
fixtures. A real SharePoint scan remains unavailable in Codex and requires a
separately authenticated Claude session. Until that native trial exists, the
runtime capability remains `unavailable` even when all local tests pass.
