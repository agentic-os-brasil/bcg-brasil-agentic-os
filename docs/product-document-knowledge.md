# Reviewed local document knowledge

Use this guide when converting a selected local document, saving reviewed workspace or explicitly admitted owner-private
knowledge, or diagnosing a refusal. The MCP/plugin keeps the existing three tools;
inspect their current schemas before choosing an operation. A method describes the
workflow and grants no additional authority.

## Convert, review and verify

1. Read `maestro_context` and confirm the intended workspace. Query `content.status`.
   Conversion requires an already admitted authenticated runtime; an unavailable
   result is a stop for conversion, with unrelated Maestro operations preserved.
2. Apply `content.convert` with a workspace-relative `source_path`. The current
   model-free profile admits the [eleven-format matrix](product-mcp-content.md)
   on Apple Silicon macOS only after the exact runtime verifies. It converts locally under network
   denial and returns a scoped immutable artifact ID plus source/runtime provenance.
3. Query `content.read` with that `artifact_id` and `format: markdown` or `document`.
   Use `next_offset` while `has_more` is true. Each response contains at most 16 KiB
   and preserves UTF-8 boundaries. Reading an artifact does not publish knowledge.
4. Apply `knowledge.create` with the artifact ID, relative Markdown `page` and exact
   `text`, or `knowledge.append` with the existing `section` and current
   `expected_page_revision`. The host presents the complete current page, proposed
   text, workspace and source/runtime evidence for explicit human review.
5. Query `knowledge.collect` with a declared `purpose` and named `pages`. Confirm
   the result/revision. Retrying the identical reviewed intent after reconnect uses
   the canonical journal and preserves the original operation identity.

Creation accepts a maximum 256 KiB page; append accepts an 8 KiB entry under an
existing Markdown heading. Full attended review can show one complete bounded
page while ordinary collection retains its existing aggregate budget. A stale
append preserves the current page and returns conflict metadata. Complete private
conflict proposals remain available to the private operator; the MCP response
withholds that full-page draft.

## Explicit owner-private document knowledge

`knowledge.create` and `knowledge.append` accept the optional closed
`knowledge_destination`: omitted/`workspace` retains the original request,
review and journal identity; `owner` explicitly selects owner-global private
professional knowledge. It is available only from the validated active internal
workspace setup, with no client account/case or pending/conflicting selection.
Sources in canonical `brain/accounts/` or `brain/clients/` namespaces refuse
promotion. An internal document named `case-study.md` is not excluded by its name.
These are structural checks plus the exact owner's review, not semantic
classification or a model-supplied assertion that client data is absent.

Prepare a missing corpus only through the existing separately reviewed
`owner.atlas.prepare`. The destination page is relative to the existing owner
`concepts/`, `learnings/` or `development/` subtree; SELF, generated memory and
arbitrary roots are outside this route. Review names the owner-global private
destination, original workspace, selected source/hash, artifact, full conversion
provenance, current page and exact proposed text. Source/binding/provenance are
rechecked after review and by the canonical engine's effect callback. Decline,
stale review and changed source/scope preserve existing content.

Read back through `owner.atlas.collect`, not workspace collection. Store source
workspace, source hash, artifact ID and pack evidence in the approved synthesis;
the canonical operation also retains opaque origin provenance. The artifact and
raw source stay in their original workspace. A later unavailable workspace may
make that source pointer unavailable; the owner page is not a promise of a
permanently available artifact or a cross-workspace artifact read.

Adapter fixtures labeled `SYNTHETIC_ADAPTER_NOT_REAL_CONVERSION` prove admission,
review, source revalidation and real owner page effects only. They do not prove
Docling transformation. The generic pilot build is unseeded and ships no runtime
pack; an independently admitted pack and actual format/client qualification are
still prerequisites for real document ingestion.

## Local operator fallback

When the client cannot present trusted review, a local operator supplies the same
closed mutation JSON to `knowledge-review` and `knowledge-apply`. Requests contain
only operation, artifact ID, page, text, the optional closed knowledge_destination and the append fields where required.
Model-supplied approval, provenance, roots or workspace identity are refused.

```sh
maestro-mcp knowledge-review --product-root "$PRODUCT" --user-home "$OWNER" --workspace "$WORKSPACE" < mutation.json
maestro-mcp knowledge-apply --product-root "$PRODUCT" --user-home "$OWNER" --workspace "$WORKSPACE" --review-digest "$REVIEW_DIGEST" --confirm < mutation.json
```

Read the complete private review first. `REVIEW_DIGEST` comes from that exact
review; changing the source, artifact, workspace, page, text or current review
invalidates it. Declining, omitting confirmation or providing an old digest writes
no knowledge. These commands are explicit operator actions, not an instruction
for an agent to approve its own request.

## Storage and refusal boundaries

Artifacts remain private under the admitted owner's
`state/content/<workspace-id>/artifacts/`; authored pages normally stay in the exact
registered workspace's `brain`. Explicit admitted owner promotion uses the existing
`HOME/atlas/owner` corpus as described below, without moving artifacts. The immutable artifact binds source digest,
workspace and authenticated pack evidence. Publication and reviewed application
revalidate the original source using the canonical bounded same-handle reader.
Changed, linked, foreign or tampered material refuses without replacing existing
pages or artifacts. Source permissions remain unchanged. Conversion never captures
memory, installs a runtime, writes knowledge automatically or sends content.

Code and Desktop packets carry only the pinned public authority registry alongside
the binary; runtime packs, credentials and private corpus remain external. Packet
files can remain read-only. Methods/resources retain their existing scope.

## Qualification evidence

`authenticated-offline-runtime.yml` reuses the runtime's verified official inputs
and retained synthetic DOCX. It first runs native converter contracts, then requires
actual PASS for both `TestContentRealJourneys/sdk` and `/compiled_stdio`, with no
skipped cases. The journeys cover conversion/readback, trusted and local review,
restart/replay, decline, source/artifact/workspace tampering, hardlink refusal,
large-page privacy, cancellation and read-only Code/Desktop packet parity.

Source integration, focused tests, hosted native execution and installed-client
acceptance are distinct gates. A skipped converter test supplies no runtime proof.
Windows conversion/knowledge admission, other document formats, production trust,
installation and team rollout require separate qualification. This capability does
not introduce authored-brain indexing or a new installer.
