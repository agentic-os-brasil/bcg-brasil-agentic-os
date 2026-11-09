# Reviewed local document knowledge

Use this guide when converting a selected local document, saving reviewed workspace
knowledge, or diagnosing a refusal. The MCP/plugin keeps the existing three tools;
inspect their current schemas before choosing an operation. A method describes the
workflow and grants no additional authority.

## Convert, review and verify

1. Read `maestro_context` and confirm the intended workspace. Query `content.status`.
   Conversion requires an already admitted authenticated runtime; an unavailable
   result is a stop for conversion, with unrelated Maestro operations preserved.
2. Apply `content.convert` with a workspace-relative `source_path`. The current
   runtime admits DOCX on Apple Silicon macOS. It converts locally under network
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

## Local operator fallback

When the client cannot present trusted review, a local operator supplies the same
closed mutation JSON to `knowledge-review` and `knowledge-apply`. Requests contain
only operation, artifact ID, page, text and the append fields where required.
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
`state/content/<workspace-id>/artifacts/`; authored pages stay in the exact
registered workspace's `brain`. The immutable artifact binds source digest,
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
