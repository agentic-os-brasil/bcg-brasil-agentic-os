# Spec 064 — Attended authored brain through MCP

Status: accepted for construction in epic #460, functional child #461.

## Observable result

An attended conversation creates a professional Markdown page, reads it from
another client, appends a decision, replaces reviewed prose, edits one declared
field and links another page through Maestro. No converter, shell, arbitrary
filesystem RPC or model-supplied approval is required. The authored corpus is
`<registered workspace>/brain`; private owner/memory and managed product remain
separate. Code, Code tab and Desktop native acceptance are distinct from SDK,
compiled stdio and native filesystem evidence.

## Owning interface

Keep the existing three MCP tools. Add closed variants:

- Queries: `brain.status`, `brain.templates`, `brain.read`.
- Reviewed mutations: `brain.prepare`, `brain.create`, `brain.append`,
  `brain.update`, `brain.set_field`, `brain.link`.

Every root and workspace identity comes from the admitted product binding.
Relative Markdown page names use canonical reserved namespace spellings (`accounts`,
`cases`, `owner`, `memory`, `atlas/owner`) on both case-sensitive and
case-insensitive filesystems. Names are bounded and reject traversal,
backslashes, absolute/hidden paths and the machine journal. Existing case
selection policy applies to both selected and linked pages. A read may name one
page for a declared purpose; it returns revision/pointer, a UTF-8-safe page of at
most 4096 bytes and the actual authored-page contract diagnostics. It does not
enumerate or repair the corpus. `status` and `templates` write nothing. Status reports the owning platform
`mutation_available` gate independently from template readiness.

Reuse `atlasops.OpenWorkspace`, CreatePage, AppendEntry, SetField, Link, named
collection/review, grant checks, the corpus lock and prepared operation journal.
Add whole-page replacement to this same engine and journal, not a second store.
Changing an existing page requires its exact current content revision. An
interrupted explicit retry finalizes only the matching before/after state and
preserves later work as conflict. Returning `proposed` is not a completed write.

## Source pages and templates

Explicit `brain.prepare` creates only a fixed workspace template/orientation
set. It never initializes owner-global atlas, reviewed-memory authority or a
case selection. Existing files, templates and hand edits remain unchanged.
Review presents the exact plan and missing-page effects. Repetition is
idempotent. Partial preparation preserves existing and already created state;
it reports `brain_prepare_incomplete`, with confirmed `completed` receipts,
`preserved_pages`, and unconfirmed `pending` pages. Pending is not a claim of
absence: an interrupted publication may already exist. The operation preserves
all state for a new explicit reviewed preparation and performs no rollback,
deletion or permission repair.

New pages and replacement bodies require the canonical eight-field authored
frontmatter: `id`, `title`, `summary`, `type`, `scope`, `status`, `sensitivity`,
`updated`. For pages in this registered workspace the scope is
`workspace/<workspace-id>`, or the path-matching admitted `account/<id>` or
`account/<id>/case/<id>`. This extends the legacy brain page scope vocabulary
without interpreting workspace content as owner-global state. Account paths
retain client-confidential sensitivity. New content cannot impersonate private
owner facets or generated memory layers. Stable ID is the page path without
`.md`; later move semantics are owned by FS2 and are not implied here.

Preserve unknown optional frontmatter fields and accept LF/CRLF. Missing or
legacy metadata is an explicit diagnostic on reads; never rewrite an existing
page just to make it indexable. Appending or changing a declared field preserves
legacy prose. New templates include valid frontmatter and declared sections;
orientation README/index files retain their separate role.

## Attended review and provenance

Ordinary authored text is its own truthful source kind, not a fabricated Docling
artifact or eligible memory capture. A deterministic intent binds operation,
page/target, exact text/field/section and expected revision. Trusted review binds
that intent, admitted workspace and the current complete page. Current scope,
source and reviewed base are rechecked after confirmation and at the effect.
Missing, declined, cancelled or retargeted confirmation writes no domain state.
MCP arguments cannot provide roots, actor IDs, grants, sanitized flags or consent.

Use the existing confirmation protocol and its MRTR binding. The host sees all
relevant content, while internal proofs remain server-owned. Page and replacement
bodies are at most 256KiB; entries are at most 8KiB. Bound serialized review size
before invoking the host. Public failure results contain typed reason codes,
not private bodies or filesystem errors. Exact local operator fallback, if
needed by a host without forms, is a separate supported invocation route rather
than a model-supplied approval bypass.

## Native filesystem requirements

Unix keeps the current held-descriptor publication and compatible owner-corpus
behavior. Windows must use native09 held-directory/current-owner/ACL/no-reparse
primitives for actual read/create/replace and corpus-lock custody. Replacing
`workspaceMutationAvailable` with success without that implementation is not
support. Snapshot directory identities eagerly from held handles, not lazy
path-based metadata, and revalidate the registered binding before every read and
effect. An outside path, hardlink, reparse/ancestor replacement, unsafe DACL or
occupied exclusive target must preserve outside/newer bytes. No permission
repair, broad access-denied retry or alternate storage root is allowed.

## Public acceptance

1. SDK and actual compiled server reject the absent attended-page operation
   before implementation; afterward reviewed create and restarted second-client
   read return the literal expected body without a conversion artifact.
2. Append/field/update/link complete one real brief workflow with current
   revisions. Repeated calls do not duplicate effects; competing writers and
   stale or changed reviews preserve later content.
3. Fresh queries and declined preparation are wholly read-only. Explicit
   preparation and repeated CLI setup preserve existing content, keys and modes.
4. Unsafe paths, unknown/model-authority fields, foreign case/workspace, aliases,
   hardlinks, replaced directories and oversize bodies refuse without outside
   effects. Legacy pages remain readable with truthful diagnostics.
5. Focused actual nonadmin Windows and Mac publication/recovery proofs use the
   same shipping executable/packet binding. Cross-compilation alone does not
   close Windows, and source/SDK acceptance does not close real client loading.

Authored indexing/search, move/retire, background cadence and private memory wiki
remain separate owning functions. This interface never manufactures their
activation or provenance.


## Review correction — shared workspace admission

The same workspace path admission owner governs authored pages, named converted
knowledge collection/review/destinations, and converted-document source admission.
A conversion artifact grants no destination or current case authority. This path
policy is independent of authored frontmatter; valid legacy converted pages stay
readable and editable without a metadata migration. Original owner-corpus and
unscoped kernel constructors retain their intentional compatibility.

The bound MCP constructor installs an owning atlasops workspace policy. Named
reads apply it before disclosure; mutations recheck it after the corpus lock is
acquired and again at the effect. Content-derived effects also revalidate their
current source selection under that boundary. No MCP argument supplies policy,
actor, replay authority or an admission callback.

A link binds the privately reviewed target revision into its canonical request.
Both endpoints and the target projection are checked under the corpus lock
before replay/finalization, unchanged receipts or publication. Exact replay never
renews lost endpoint authority. Arbitrary direct marker writers are not thereby
serialized; these are explicit admission checks at the actual owning boundaries,
not a claim of an atomic cross-domain marker transaction.


## Explicit authored navigation generations (#467, ANAV)

`brain.compile(pages,purpose)` is an attended foreground operation over exactly
1–24 current admitted Markdown names, within the existing 128 KiB Atlas collection
budget. The native review binds source bytes/revisions, observed named absence,
workspace ownership and the expected current generation. Duplicate, forbidden,
aliased or incomplete-budget cohorts fail before publication. Sources remain
canonical and unchanged; no root walk, private-memory compiler, repair, source
move/retirement or standing maintenance authority is introduced.

The pure compiler parses the canonical eight-field metadata, matching scope,
status, curated body and local Markdown links. Superseded or invalid-metadata
sources remain fingerprints/diagnostics, not search results. A balanced
`maestro:generated:start/end` region is excluded from search and link feedback;
unbalanced regions are diagnosed. Out-of-cohort links are unchecked and omitted
from derived references. Only an explicitly observed missing target can be a
broken link. Backlinks and orphan diagnostics describe this cohort, not a claim
about all workspace pages. External URLs, inline code and fenced examples are
not sources of local-link authority.

Deterministic fingerprints include compiler schema, workspace ID, sorted names,
exact source revisions/bytes and named absence. Complete immutable generation
records (including timestamp and attended provenance) and the current pointer
are published inside the existing owner-private `.atlasops` journal through the
owning held-parent publisher and corpus lock. The complete generation is durable
before the atomic pointer change. Interruption leaves the previous complete
pointer intact; a read neither repairs nor promotes staged data. An explicit
retry may reuse only the same validated complete generation. Identical current
input returns `unchanged` without replacing the pointer. Source revisions and
current admission are rechecked after staging and before publication.

Read-only `brain.navigation_status` reports absent/ready/stale. `brain.navigate`
reads an exact virtual reference, starting at `index.md`; hierarchical indexes,
page summaries/backlinks, `backlinks.json`, `diagnostics.json` and `log.md` are generated
references, not host paths. `brain.diagnostics` pages the diagnostic view.
`brain.search` performs deterministic Unicode case/accent-folded substring
matching over admitted titles, summaries and curated bodies and returns bounded
canonical references/revisions. It uses at most 24 matches per page and a serialized
result budget of 512–4096 bytes. Navigation/diagnostic text uses UTF-8 byte pages
of 4–4096 bytes and `next_offset=-1` when complete. Canonical full text remains
`brain.read`.

All data queries require the current exact generation and re-read the complete
explicit source cohort/current policy before disclosure, including a final
adapter recheck after formatting. Source edits, absence/move/deletion, changed
case admission or pointer replacement fence the prior generation; no last-good
text fallback is returned. Exact stored workspace scope rejects copied journals.
Uncoordinated external edits are detected by revision rechecks; this is bounded
read validation, not custody of arbitrary external writers. Foreground navigation
registration is not native-client or hosted Windows qualification.

Compiler-policy semantic changes must advance the fingerprinted compiler schema;
immutable records are never rewritten to fit a new compiler. Historical derived
records remain owner-private and inaccessible through old-generation queries;
this foreground delivery does not claim physical retirement or source lifecycle
mutation. Publication time and attended provenance live in the owning generation
envelope/receipt; the deterministic virtual `log.md` omits source names, bodies and source
fingerprints. It points to the private owning envelope as audit authority.
Markdown concepts carry parseable OKF/Profile frontmatter, the root declares
`okf_version: "0.2"`, and query records include the owning publication timestamp.
Aggregate indexes conservatively use the highest source sensitivity in the cohort.

Virtual Markdown destinations and wire `reference` selectors use the same
canonical URI path representation: percent-escape each path segment exactly
once while retaining structural slashes. Resolve an emitted relative href against
its current virtual reference, then pass that canonical encoded bundle-relative
path to `brain.navigate`. Navigation decodes exactly once and rejects fragments,
queries, traversal, encoded slashes, invalid UTF-8 and noncanonical aliases.
Canonical page selectors for `brain.read` remain the original admitted names.
Literal `%23` in a filename therefore uses `%2523` on the navigation wire and
stays distinct from `#` (wire `%23`). Markdown labels escape control syntax.
