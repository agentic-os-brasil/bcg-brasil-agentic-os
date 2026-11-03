# Product MCP: local documents and workspace knowledge

Code and Desktop share the admitted workspace's canonical knowledge engine.
A local document becomes a private conversion artifact; an exact human review
permits a knowledge change. The original document stays in its workspace and
managed product assets remain read-only. This engineering guide covers the
retained E4 DOCX journey and the PFMT policy-format extension. The owning
eleven-format converter is reviewed and integrated. Per-format status and the
complete SDK/rebuilt-stdio knowledge/restart matrix passed in local synthetic
engineering tests: 24 positive journeys and all 31 required case IDs. Independent
Task2/front reviews, native qualification and a signed team release are separate
evidence.

## Prepare and inspect

Use explicit `init` with the locally selected product, profile and workspace,
then reuse the same binding for `serve` and operator commands. The
[module guide](../adapters/mcp/README.md) gives those three flags. Initialization
creates the registered workspace and its brain; ordinary queries and conversion
never initialize, migrate or provision a runtime.

Query `content.status` before conversion. The reviewed PFMT converter profile is
**Darwin arm64 with no model inventory**. Its eleven existing policy extensions
have actual authenticated offline content oracles and public SDK/rebuilt-stdio
reviewed-knowledge/restart evidence. Verified profile identity governs coverage; a format
allowlist, library version or old ready pack cannot establish a newly supported
route. Missing trust or an absent/changed/unrecognized pack remains unavailable.
Runtime provisioning has its own release/installation proof; querying and
conversion never download dependencies.

| Source | Intended fidelity in the authenticated PFMT profile |
| --- | --- |
| DOCX, CSV, HTML/HTM, Markdown, EPUB, XLSX | Native structured conversion with actual table/order/content oracles and rendering/resource/model features disabled. |
| BIFF XLS | Deterministic textual cell/sheet mapping, preserving declared date, cached-value and empty-cell semantics. No formula evaluation. |
| TXT, ordinary JSON, generic XML | Validated original UTF-8 represented as literal code content with textual fidelity. Markup/whitespace is not silently reinterpreted. |
| Validated Docling-export JSON | Structured import only after the runtime validates the actual export; a claimed schema name is insufficient. |

Actual conversion and artifact results report the route/parser/fidelity and
fallback reason for that source. They take precedence over intended coverage.
XLSX admission binds its finite supported content-type/relationship graph and
workbook-wide cell/range/geometry budgets; unsupported layouts refuse before
authentication. See [Spec061](../specs/061-product-local-content.md) for that
profile. PDF/OCR, images, PPTX and other platforms remain separately admitted
and qualified work; they are not part of this eleven-extension completion.

## Convert, read and write

These are `operation` values in the existing three-tool interface. Roots,
workspace identity, commands and release authority come from the local binding.

| Tool / operation | Input and result |
| --- | --- |
| `maestro_query` / `content.status` | Metadata-only verified runtime/profile availability and intended per-format descriptors. Before verification every descriptor remains unavailable with the verifier reason. |
| `maestro_apply` / `content.convert` | `source_path`, a canonical path relative to the selected workspace. Returns a scoped artifact and provenance after usable conversion. |
| `maestro_query` / `content.read` | `artifact_id`, `format` (`markdown` or `document`), and optional `offset` / `max_bytes` (4–16,384). Returns a UTF-8-safe slice of at most 16,384 bytes. |
| `maestro_apply` / `knowledge.create` | `artifact_id`, `page` and exact `text`; requests trusted review of a new page. |
| `maestro_apply` / `knowledge.append` | `artifact_id`, `page`, complete Markdown heading in `section` (for example `## Findings`), exact `text` and `expected_page_revision`; requests trusted review before append. |
| `maestro_query` / `knowledge.collect` | Declared `purpose` and explicit `pages`; returns canonical named-page readback. |

Select a source already inside the admitted workspace. Absolute paths, traversal,
aliases, special files and inputs outside the canonical budgets refuse. Conversion
reads and hashes the same confined source descriptor, verifies the authenticated
runtime and executes a private snapshot under OS network denial. Unsupported,
partial or failed conversion exposes no partial document body and changes no
knowledge. MarkItDown is a separately selected permitted route, never a silent
substitution for this operation.

Read the returned artifact in bounded slices, carrying its opaque identity and
returned offsets. Its Markdown, structured document, source digest and pack
provenance are immutable and scoped to this workspace. Another workspace cannot
read or use it. Conversion itself does not authorize a knowledge write or promote
client facts into owner-global memory.

Prepare the exact intended page text or section entry from that artifact. Human
review shows the complete domain change; private proof binds the artifact,
source/pack hashes, target, draft, scope and expected page revision. Acceptance
must come from the admitted host form or the explicit local operator. A boolean,
transport session ID or approval key supplied by the model grants no authority.
Changed source, artifact, binding, target or revision requires fresh review or a
safe canonical conflict outcome. Decline and missing host confirmation preserve
current knowledge.

Collect the named page after success, restart using the same binding, and collect
it again. Carry the returned canonical revision into later appends. Collection
is limited to 24 named pages and 128 KiB total; pages retain the canonical 256 KiB
limit and entries the 8 KiB limit. Collection reports omissions when a named page exceeds the remaining budget.
Full trusted page review does not widen this ordinary tool-read budget. Broad
whole-brain collection is not an operation.
Stable server-derived intent/provenance lets an exact interrupted write retry
recognize an already applied effect without duplicating it or replacing newer
content.

## Local operator fallback

Clients without human forms use `knowledge-review` and `knowledge-apply` with the
same closed JSON mutation through stdin. Supply the normal three binding flags
to both commands. For an existing page, the private request file contains:

```json
{"operation":"knowledge.append","artifact_id":"ARTIFACT_ID","page":"notes/example.md","section":"## Findings","text":"An exact reviewed finding from the selected artifact.","expected_page_revision":"CURRENT_REVISION"}
```

```sh
maestro-mcp knowledge-review < /private/local/request.json
maestro-mcp knowledge-apply --review-digest REVIEW_SHA256 --confirm < /private/local/request.json
```

Use the current revision from collection and the exact digest from the immediately
preceding full review. Keep full review JSON local. A modified request, stale
state, changed source or missing confirmation refuses; copying controls or
removing a journal is not recovery. These commands do not establish native host
confirmation qualification.

## Runtime engineering and qualification

The offline assembler consumes exact already-acquired inputs and a new output
directory:

```sh
go run ./cmd/maestro-docling-pack --inputs /private/raw-inputs --destination /private/new-pack
```

The assembler verifies the pinned interpreter and every locked wheel before
extraction. Its output is unsigned until the existing release authority signs
it. On every use the runtime revalidates the signed archive and complete installed
inventory, then executes a fresh authenticated snapshot with empty private caches.
The MCP executable's registry digest is build-bound; its fixed registry is
`managed/trust/release-authority-registry.json` under packet-root, the parent of
Code's `scripts/` or Desktop's `server/`. Product asset selection does not select
trust. Unseeded binaries keep unrelated product operations while conversion
reports unavailable.

Real SDK and compiled-stdio conversions took around one minute in engineering
runs on the test host. Those protocol tests received progress notifications and
cancelled calls within one second without publishing another artifact. Native
client interaction and timing still need qualification; client/global timeouts
were not widened to hide the observed latency. Hosted CI
builds an ephemeral engineering fixture from exact official inputs. It cannot
establish corporate signing, portable installation, broader format/model support,
Windows admission, actual native loading, SharePoint publication or team pilot
acceptance.

## PFMT engineering acceptance

`TestContentPolicyRealJourneys` performs actual conversion through SDK and the
rebuilt Code packet executable for all eleven extensions and validated-export
JSON. It reads document/Markdown through bounded public slices, requires exact
trusted host-form review, creates/appends distinct named pages, restarts and
checks revision/provenance and idempotent retry. It does not install precomputed
output as a new conversion artifact. The recorded local run passed 24 positive
journeys: SDK 871.78 seconds, production stdio 702.31 seconds, package 1576.371
seconds. These are measured engineering durations, not a performance promise.

The same run checks private review/scope/stale-source and concurrent-page
refusals, actual empty-content refusal and parser tampering. The actual MCPB
executable reads verified coverage, recipe/policy/license/provenance resources,
bounded artifacts and a restarted named page. It does not establish native
Code/Desktop loading or host form qualification.

The retained current fixture supplies MAESTRO_DOCLING_RETAINED_ROOT and its
exact linker-bound MAESTRO_DOCLING_RETAINED_SEED. Run the focused matrix from
the MCP module with an adequate runner window; individual product deadlines
and network denial remain unchanged. Optional MAESTRO_POLICY_PUBLIC_ROOT must
name a new private proof directory. An unseeded skip is not conversion evidence.
The product CI workflow acquires the exact 51 official inputs and requires all
31 actual PASS IDs; missing/skipped cases fail that acceptance step. Hosted CI
has not been run by this local construction record.
