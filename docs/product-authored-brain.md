# Authored workspace pages through Maestro

An attended conversation can create a professional Markdown brief, continue it
in another client, append decisions, replace reviewed prose, edit a declared
field and link another page. These are bounded operations on the registered
workspace brain. They use the existing Maestro connector and its three MCP
tools; a document converter is unnecessary for ordinary authored text.

The operator's executable recipe is
[authored workspace pages](../bundles/base/skills/maestro-operator/resources/authored-brain.md).
The owning source contract is [Spec 064](../specs/064-product-authored-brain.md).
This functionality is tracked in [#461](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/461)
under [epic #460](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/460).

## Identify the installed capability

Start with `maestro_context` from the intended pilot connector and verify its
workspace ID. Then query `brain.status`. Its `prepared` field describes the
fixed template set; `mutation_available` reports the owning platform gate.
Queries create no pages. An enabled extension, a context result of ready or a
successful build does not attest writable authored pages.

Mac/Unix uses the existing held-descriptor publisher and corpus journal. Windows
uses staged ordinary-account qualification: the held publication kernel is
proved with the public mutation gate closed, then the actual shipping-server
journey and interrupted journal recovery must pass before that gate opens.
Use the installed server's status and retained native evidence for availability;
cross-compilation and source changes alone do not establish Windows support.
Real Code, Code-tab and Desktop loading remain separate client acceptance.

This current-main composition consumes the canonical Windows private-state
foundation tracked in #504 before Windows qualification. Historical native
observations do not attest this new composition; fresh owning and shipping-server
proof is required. Native Code/Desktop loading remains a separate gate.

## Available operations

The current MCP schemas are the authority for accepted fields and limits.
Queries go through `maestro_query`; mutations go through `maestro_apply`.

| Operation | Result |
| --- | --- |
| `brain.status` | Read-only workspace ID, readiness, platform mutation availability, missing fixed pages and legacy metadata diagnostics. |
| `brain.templates` | Read-only client, project, person and daily authoring templates. |
| `brain.read` | One named page for a declared purpose, content revision, pointer and UTF-8-safe continuation. |
| `brain.prepare` | Reviewed creation of the fixed workspace scaffold; existing pages are preserved. |
| `brain.create` | Reviewed creation of one ordinary authored page; an existing file is preserved. |
| `brain.append` | Reviewed insertion under one unambiguous existing heading, with an exact expected revision. |
| `brain.update` | Reviewed complete replacement, with an exact expected revision and valid authored metadata. |
| `brain.set_field` | Reviewed edit of one existing Markdown list field, with an exact expected revision. |
| `brain.link` | Reviewed relative reference to a named existing page in the same admitted corpus, with an exact expected revision. |

The MCP creates no arbitrary file API. Paths are relative Markdown page names
inside `<registered-workspace>/brain`; owner-global/private memory and another
case's state remain separate. New source uses the eight fields in the
[brain page contract](../bundles/base/brain-contract.md). General pages have
`workspace/<workspace-id>` scope; account/case pages must match their admitted
path and remain client-confidential. A new authored page cannot impersonate an
owner facet or a generated memory layer.

## Team rehearsal

Use synthetic content and the one connector associated with the prepared pilot
workspace. In an ordinary conversation, ask:

> Read Maestro context and brain.status from this pilot's connector without
> changing anything. Show the workspace ID, prepared and mutation_available.
> Stop if the workspace differs from the ID supplied by the pilot preparer.

When mutation is available, ask:

> Prepare the authored workspace templates through Maestro and show the actual
> review. Then draft a synthetic project brief at projects/pilot-brief.md from
> the project template. Let me review the full proposed page before saving it.

Accept the actual host review, then request a readback of that named brief.
Open the other client connected to the same workspace and ask it to read the
same page through Maestro. The returned text and content revision must agree.
Next ask for a decision appended under an existing heading, a declared field
updated, and a link to another synthetic page created in this same workspace.
Each edit must use its current content revision and a new exact host review.

A read returns at most 4096 bytes of text. Follow `next_offset` until `-1` for a
complete replacement, retaining one revision throughout. A revision conflict
requires a fresh read and review. Never manufacture an approval argument or
copy a proposal into another connector to get around a refused review.

## Outcomes and recovery

`written` confirms a persisted page effect. `unchanged` confirms preservation or
an exact journal replay. An expected revision is a content digest string; it is
different from the integer revisions used by execution-continuity operations.
The same typed operation and original request can converge through the
canonical receipt without repeating its page effect. A different request,
changed scope, changed review or newer page fails closed.

`brain_prepare_incomplete` is an error with a bounded progress record:
`completed` lists confirmed canonical receipts, `preserved_pages` names confirmed
unchanged pages, and `pending` names unconfirmed remaining effects. Pending does
not mean absent: a file may have been published before an interrupted receipt.
No rollback or permission repair runs. Query status, then explicitly review a
new preparation to finish; existing and already completed pages are preserved.

LF and CRLF source remain usable through append, declared-field edits and links;
only inserted text follows its target heading while surrounding bytes remain
unchanged. Legacy pages remain readable with diagnostics. Preparation preserves hand edits
and does not retrofit metadata. Appending, declared-field edits and links retain
the surrounding legacy source; a full replacement requires valid metadata.
This authored-page route does not recompile the authored navigation index,
create private memory, activate daily/weekly rituals or attest an external
SharePoint publication. Those remain independently owned product capabilities.


## Shared knowledge and effect-time scope

Named `knowledge.collect` reads and converted-knowledge destination reviews use
the same workspace path admission owner as authored pages. Legacy converted
Markdown stays usable without the authored eight-field contract. Converted
sources are admitted before conversion/artifact disclosure and revalidated at
the serialized knowledge effect; an artifact is not destination authority.

The private review binds a link target's revision into its canonical request.
Source and target admission, existence and reviewed target content are checked
after acquiring the corpus lock, before receipt replay/finalization, and again
before publication. A waited-for lock or old receipt cannot renew lost target
authority. These checks do not serialize arbitrary external case-marker writers.
