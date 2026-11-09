# Private workspace memory wiki through the product MCP

Issue #506 composes the PWKC/Spec008 local source contract onto the landed
maintenance and authenticated document-format implementation. It binds private
compilation, three owning wiki jobs and selected-workspace readback to the MCP.
Focused local acceptance, independent review and hosted CI are recorded against
exact source hashes. Installed-client, native Windows and production qualification
remain separate; source composition alone does not establish them.

The private wiki turns eligible committed workspace-memory rollups into concepts,
indexes and backlinks. It uses deterministic compilation. Raw captures, authored
knowledge pages, owner-global material, SharePoint and another workspace are
outside this source profile. Reading a method or connecting a client does not
grant collection or publication authority.

## Read a selected workspace

Use the admitted product binding for the intended workspace. Both clients must
select the same canonical private home and registered workspace to share this
view. The MCP server is a local process; no hosted wiki endpoint is required.

The product query has three finite operations:

| Operation | Purpose and expected behavior |
| --- | --- |
| `wiki.status` | Inspect source/view availability, opaque generation and watermark, counts and actual producer-current flags. It never compiles, initializes or repairs. |
| `wiki.read` | Read a registered index, concept, backlink or metadata-log reference in one exact generation. |
| `wiki.source` | Drill into an eligible committed rollup using its exact opaque memory reference and generation. |

Begin with status, then read `index.md` for that exact generation. Follow only
references returned by the validated graph. Page reads require a generation
even at offset zero, use bounded UTF-8 pages of 4–4,096 bytes and retain the
same generation through pagination. The published tool schema defines the exact
argument names. Private manifests and arbitrary filesystem paths are unavailable.
Workspace, root, identity, provider and approval arguments are not accepted.

If a source or denial generation changes during paging, the old page/cursor
refuses. Refresh status and start a new current generation after the owning job
has completed. There is no historical fallback for removed or replaced content.

## Build and check the view

The existing [product maintenance workflow](product-mcp-maintenance.md) owns
review, qualification, enrollment, execution and diagnosis. Local enrollment is
explicit. A catalog default is not a live grant, and decline/pause/revoke remains
effective. Status and startup do not enroll jobs or perform maintenance.

| Job | Source and effect |
| --- | --- |
| `wiki-incremental-sync` | Processes an actual eligible durable source/denial event through the existing Worker and memory compiler. It has no calendar schedule. |
| `wiki-reconcile` | Runs under its weekly/presence contract, compares actual source and graph and recovers genuinely missing historical events through owning compilation. |
| `wiki-integrity-check` | Checks the complete current graph and persists body-free diagnosis. It does not compile or repair. |

Each execution pins the actual source/event and live control/lease. An event
execution closes its exact durable occurrence; it does not create a calendar
receipt. Native wake availability is separate from a successful local run.
Do not describe an unenrolled or unsupported wake mechanism as active automation.

A current graph does not itself close an incremental occurrence. If the graph
committed before terminal publication failed, or reconciliation ran first, the
same Worker can revalidate the current graph and durably close the event as
no-change. Missing evidence remains unclosed; corrupt, hardlinked or
recovery-required evidence refuses rather than being erased or bypassed.

Qualification requires a real current-build/configuration effect and independent
readback. A no-change reconciliation is a valid ordinary result but cannot mint
qualification. Historical qualification retains body-free evidence of its actual
successful run; it does not prove a later event succeeded. Every later run must
prove its own guarded effect and result.

The running process pins its executable identity at startup. Detected executable
replacement invalidates that process permanently, including services constructed
by later tool calls. Restart with the intended installed build before obtaining
fresh review or qualification. Returning the old file to its path does not revive
an invalidated process. This checks the managed executable's path, held file and
metadata; it does not attest kernel-loaded memory or malicious changes preserving
all checked metadata.

## Correction, deletion and failures

A selected committed-source change or denial hides the entire old graph,
including indexes, concepts, backlinks and source drilldown. Appending a raw
capture alone does not change an already committed rollup's watermark.

Denial also retires the fixed derived subtree, including historical and staged
bodies. If cleanup fails, the barrier still blocks reads. The normal owning
compiler may retry cleanup under the live grant; no new view or qualification
is admitted until retirement completes. Pause/revoke, stale review, source drift
or lease loss must block publication and terminal success.

Report unavailable, stale, pending retirement and invalid protected state
explicitly. A missing or downgraded permanent checkpoint marker on an already
protected checkpoint refuses; ordinary queries and compilation cannot recreate
that authority. Use the owning diagnosis/recovery contract rather than editing
private files or deleting markers.

A recognized sealed graph from a compatible earlier build can remain readable
while source/denial and security contracts remain current. Status preserves its
actual producer provenance and reports the mismatch. Requalifying the current
build requires genuine compilation; reading old bytes cannot relabel them.

## Scope and evidence

The explicit profile is `private-memory-local-v1` with local-home/workspace
authority. It asserts no enterprise tenant or person identity. The existing
owner-reviewed key supplies domain-separated opaque references without key
creation/fallback on reads. Metadata, receipts and diagnosis contain no private
bodies, raw manifests, source filenames or reusable plain private-content hashes.

Source limits are 128 eligible artifacts, 1 MiB each and 8 MiB aggregate. A complete
graph is at most 1,024 files/16 MiB. Overflow refuses rather than silently dropping
sources. Memory retains source selection, event/checkpoint custody, private IO,
publication and retirement; the pure compiler acquires no filesystem authority.

Focused local evidence covers all three real job effects, independent graph and
diagnosis readback, SDK and restarted production stdio, source correction/deletion
with forced failure, and live-control/lease refusals. After the event-closure
correction, SDK and restarted production-stdio subcases completed separately.
Combined invocations that reached test deadlines remain recorded as failures;
completed subcase evidence is distinct from later isolated completion. These are
engineering observations, not a whole-module performance qualification or an
exhaustive exclusion of concurrency failures. Actual Code-tab/
Desktop loading, Windows private-handle admission, native wake, hosted CI,
organization signing, distribution and participant acceptance remain separate
evidence. Independent Task2 acceptance and four complete-front reviews must close
before a publishable front is claimed.

## Actual binary compatibility qualification

A finite Unix CI job builds an actual legacy memory consumer against immutable
main `d1e8f18d` and a separate current producer with one fixture-only navigation
description change. The old consumer must first succeed at real unprotected
read/capture/delete, then refuse the protected schema2 cohort without changing
source, graph or modes. The producer must run canonical compilation and retain
its own executable provenance; the current reader preserves that evidence and
can compile a genuinely new current-build generation.

The job provisions both existing contracts, rejects missing/skipped coverage,
and retains helper source, immutable source/archive/tree and executable hashes,
exact configuration bytes/hashes and observed results. Default local runs that
omit these helper programs do not qualify binary compatibility.

The public interface is `wiki.status`, `wiki.read` and `wiki.source`; compilation
belongs to `wiki-incremental-sync`, `wiki-reconcile` and `wiki-integrity-check`
through the existing reviewed maintenance lifecycle. There is no private-wiki
search RPC in this source delivery.

Private wiki admission retains an explicit gate: the current owner must admit
the existing workspace and its supported pinned-namespace operation must
succeed. Windows retains that namespace through source snapshot, graph
publication/readback and retirement. Ordinary-memory admission alone, a
serialized flag or a prior successful check grants no wiki effect. An unavailable
operation preserves the existing unprotected-memory behavior and cannot downgrade
a protected derivative. See the [native Windows contract](product-windows-private-wiki.md)
for custody, retained construction staging and exact-source evidence boundaries.
