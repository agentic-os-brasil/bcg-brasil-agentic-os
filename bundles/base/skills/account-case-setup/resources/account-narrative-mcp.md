# Save the shared account narrative

Use this workflow only when the owner explicitly asks to save account-level
relationship context. Finish the existing first-case setup first; it must be
active in client mode, with no pending or reconciliation state.

1. Call `account.narrative.status`. Reuse its exact account ID, current case agent
   ID, page, revision and current-workspace brief reference. The server derives
   HOME and account scope; these operations accept no selected account or root.
2. If a page exists, call `account.narrative.read` with a concrete purpose. Keep
   its complete text and exact revision while drafting. Missing state is not a
   reason to overwrite another page or fall back to workspace brain operations.
3. Draft one full account page, at most 64 KiB UTF-8, with the required authored
   frontmatter. Its `id` is the returned page without `.md`, `type` is `account`,
   `scope` is `account/<returned-account-id>`, and `sensitivity` is always
   `client-confidential`. Keep the title and H1 identical. Capture only supplied
   relationship context; account save imports no case prose automatically.
4. Preserve existing case references unless the owner explicitly reviews their
   removal. The proposed `case_agent_ids` is a unique list of at most 32 exact
   authenticated case-agent IDs, including this workspace's current case. Sort
   these IDs lexically. Include exactly one `## Cases` section with a blank line,
   followed by one line per selected ID in that order:

   ```text
   - `<agent-id>` — account `<account-id>`; case `<case-scope-id>`
   ```

   Reuse immutable case/account tuples returned by status or the existing page.
   Do not guess foreign case IDs. The server independently checks every named
   registration and refuses unrelated or tampered references. Unknown legacy
   lines remain visible in the full current page and are explicitly unverified.
5. Call `account.narrative.save` only after an explicit Save request, passing
   `text`, `case_agent_ids` and `expected_page_revision` (`absent` for a new page,
   otherwise the exact returned SHA-256). The trusted client review displays the
   complete current/proposed page, exact account registration, proposed signed
   case records and existing verified references. A chat confirmation is not
   this native approval. Decline, unavailable review or changed state preserves
   the narrative; inspect and review afresh rather than retrying a stale edit.
6. Read back with `account.narrative.read`; compare complete text and revision
   with the saved result. On an interrupted request, retry only the exact original
   logical request; canonical atlas receipts identify an already applied effect.
   Report a saved page separately from first-case setup or runtime activation.

This changes the sole authored account page in admitted HOME/brain. It never
rewrites case briefs, grants foreign-workspace read access, promotes client
knowledge, changes selection, creates identities, starts agents or enrolls jobs.
The current case's workspace brief remains separate. Case code references name
registered identities, not foreign brief links. Broader account/relationship
recall, automatic indexing, foreign brief links, cross-case promotion and agent
hiring remain unfulfilled parts of `domain.account_case_scaffold`.
