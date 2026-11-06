# Authored workspace pages

Use this recipe for ordinary professional pages in the registered workspace.
The active MCP schemas define the accepted arguments. The page source is the
workspace brain; reviewed owner facets, eligible memory and generated indexes
have their own operations.

1. **Bind.** Use the Maestro connector selected for this conversation. Read
   `maestro_context` and compare its workspace ID with the requested workspace.
   An ambiguous connector or different ID ends this route before any write.
2. **Inspect.** Query `brain.status`. Continue a mutation only when
   `mutation_available` is true. `prepared` reports fixed-template readiness;
   it does not attest client loading or imply all second-brain capabilities.
   A missing operation or unavailable backend ends this route with that result.
3. **Prepare when requested.** Query `brain.templates` for the four actual
   authoring templates. Use `brain.prepare` through `maestro_apply` for the
   fixed workspace scaffold. Finish this step only on its actual ready result.
   Existing pages remain under the owner's control.
4. **Author or read.** For a new page, use an actual template, change its ID to
   the new relative path without `.md`, and supply the title/H1, summary, scope,
   status, sensitivity and content date for that page. Use `brain.create` with
   the complete text. For an existing page, query `brain.read` with its relative
   name and the purpose of opening it. Continue with `next_offset` until it is
   `-1` when the task needs the full page. Each text page is at most 4096 UTF-8
   bytes; every continuation must retain the same content revision.
5. **Edit the reviewed base.** Read the current page first and retain its
   string `revision`. Pass that value as `expected_page_revision`: append to
   one existing heading with `brain.append`; replace the complete source with
   `brain.update`; edit a declared `- **Field:** value` with `brain.set_field`;
   or add a reference to a named existing page with `brain.link`. Read the link
   target through the same connector and admitted scope. Frontmatter changes
   use a complete page update. A truncated read is not a replacement body.
6. **Review and verify.** Every mutation requires the host's approval of the
   exact displayed record. The server binds the full current/proposed page,
   workspace and target to that response. Let the actual host complete its
   form or round trip; MCP arguments do not carry consent. Confirm the result
   state and read the named page back with its returned revision. A written
   result is a persisted page; unchanged may mean a preserved file or an exact
   journal replay. Neither result proves an index rebuild or private-memory
   promotion.

If reading a multi-page body finds a changed revision, reread the complete page
before proposing a replacement. For `revision_conflict`, retain newer work and
prepare a fresh reviewed edit. For `confirmation_unavailable` or
`confirmation_declined`, report the unchanged domain result and stop; another
host or an independently supported operator route requires its own explicit
admission. Use the MCP route above for product work.

For `brain_prepare_incomplete`, report `completed`, `preserved_pages` and
`pending` separately. A pending page is unconfirmed and may already exist after
an interrupted publication. Inspect `brain.status`, then offer a newly reviewed
explicit `brain.prepare`; preserve all existing pages and machine state. The
incomplete result does not authorize automatic repair, deletion or rollback.

Read diagnostics preserve legacy pages as they are. An append or declared-field
edit can retain legacy prose; a new page or complete replacement needs valid
authored metadata. Roots, absolute paths, private owner/memory locations,
arbitrary files and other case scopes are outside this recipe. Page names use
forward slashes relative to this workspace's brain on Mac and Windows.
