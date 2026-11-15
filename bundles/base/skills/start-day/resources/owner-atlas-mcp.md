# Attended owner-atlas work through MCP

Use this route for professional daily pages, development evidence, craft,
feedback, retrospectives and sanitized learnings. These pages belong to the
canonical private owner atlas, distinct from Owner Context facets, generated
memory and each client's workspace. The server selects the root; logical page
names below never become caller-selected directories.

## Open only the needed context

1. Call `maestro_query` with `operation: "owner.atlas.status"`.
2. If preparation is missing, explain the fixed professional scaffold and use
   `maestro_apply` with `operation: "owner.atlas.prepare"` for native exact
   review. Preparation preserves existing pages. A query never prepares or
   repairs the corpus. Inspect a failed/partial preparation before explicitly
   retrying; do not claim success from the presence of the directory alone.
3. Call `owner.atlas.collect` through `maestro_query`, supplying a concrete
   `purpose` and the exact `pages` required for this task, at most 24. The server
   bounds the projection. Missing pages are omissions, not invented context.
   Retain each returned `revision` for a later change.

Page names are relative to this owner atlas. Examples are
`daily/YYYY-MM-DD.md`, `development/objectives.md`, `craft/craft.md`, and
`learnings/index.md`. Legacy `brain/` notation in the method describes a
logical segment, not permission to address Owner Context or a workspace.
For an opening/closure, collect the specific daily and objective pages; an
unavailable task view stays an explicit omission. If prior daily page names
are unknown, ask for the relevant dates or report the omitted history; a few
calendar-date guesses do not establish the two latest existing pages. Never
create a substitute task system or claim an absent compiled task index was
read.

## Record one exact attended result

Preview the content and let the native host present the complete exact change.
The model supplies domain text only; approval is returned by the host.

- `owner.atlas.create`: `page`, `text`. Create a new free-form Markdown page;
  an occupied page is preserved. Include appropriate owner-private metadata
  when authoring a new page, while preserving existing free-form owner work.
- `owner.atlas.append`: `page`, `section`, `text`, `expected_page_revision`.
  Append to the actual declared Markdown heading. Use the revision just read;
  an absent or ambiguous heading is a draft to discuss, not a write to guess.
- `owner.atlas.update`: `page`, complete replacement `text`,
  `expected_page_revision`. This is a separately reviewed correction of one
  exact page. It is not the daily opening/closing operation and does not
  authorize rearranging or retiring the corpus.

`start-day` creates today's missing page with `## Prioridades` and `## Notas`,
then appends only the reviewed briefing under `## Prioridades`. `eod` collects
that same page and appends the owner's confirmed actual result under
`## Notas`. A second deliberate occurrence reads the current revision first.
An exact retry keeps the original unchanged request and canonical receipt;
it does not manufacture a new idempotency key or retry over an owner edit.

After every successful write, collect the named page again. Compare its real
text and revision with the result. A receipt alone is not current read-back.
On `revision_conflict`, preserve the newer page and request a fresh review.
On `confirmation_declined` or `confirmation_unavailable`, record no completed
write; retain the proposal in the conversation and report the actual result.

## Keep each effect in its owning scope

Owner pages contain owner-authored professional context, sanitized reflection
and pointers. Raw client findings, figures, stakeholder positions and delivery
bodies stay in their registered workspace. Use `brain.*` for an explicitly
authorized workspace artifact and the separate execution operations for its
checkpoint. Owner Context changes use `owner.propose`/`owner.apply`; eligible
memory uses `memory.*`. This route grants none of those effects implicitly.

Calendar, mail and external task enrichment used to compose a briefing are not
persisted. A scheduled marker is not a standing grant or native review. Without
an admitted unattended owner-atlas adapter, the scheduled path may produce its
marked proposal but cannot report it recorded. The MCP route never invokes
legacy commands, reads private files directly or changes owner profiles.
