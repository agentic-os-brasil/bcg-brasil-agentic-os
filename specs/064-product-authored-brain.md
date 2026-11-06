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
Relative Markdown page names are canonical, bounded and reject traversal,
backslashes, absolute/hidden paths and the machine journal. Existing case
selection policy applies to both selected and linked pages. A read may name one
page for a declared purpose; it returns revision/pointer, a UTF-8-safe page of at
most 4096 bytes and the actual authored-page contract diagnostics. It does not
enumerate or repair the corpus. `status` and `templates` write nothing.

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
it reports the incomplete outcome without deleting or repairing anything.

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
