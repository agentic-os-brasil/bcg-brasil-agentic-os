# Attended history through product MCP

The product exposes explicit selection/read controls and retained user-prompt controls through `maestro_query` and `maestro_apply`. Source reading, retained prompts and confirmed owner-profile changes have separate authority. These operations do not install collection hooks or enable background capture.

## Read selected conversations

1. Ask the owner to select the exact current workspace, source directories and session files for this invocation. `history.selection` accepts `history_manifest`: a `workspaces` array and a `sources` array. Each source has `kind` (`claude_code` JSONL or `claude_export` JSON), `workspace`, `root`, and `sessions` containing exact relative `path` and `id`. Every selected workspace must equal this client's admitted workspace. Another worktree needs its own bound invocation.
2. The query reads metadata only and returns `selection_digest`, file count and byte count. It creates no registry and returns no source body or source path.
3. Call `history.read` with the unchanged `history_manifest` and `history_selection_digest`. Optional `history_cursor` starts at zero; `history_batch_size` is 1–5 and defaults to 5. The native review shows every selected source/path, metadata identity, destination binding and exact batch. Only the host's acceptance releases source bodies.
4. Continue using the returned `next` cursor and the same digest. Each batch gets fresh review and revalidates every selected file, including files outside the current batch. A changed source requires a new selection and review. The response's `sessions` are ephemeral context; `audit` and cursor contain metadata only. Reading does not retain or promote the transcript.

Bounds: at most 4 selected sources/workspaces, 20 files, 2 MiB per file, 8 MiB combined and 64 KiB serialized manifest. JSON exports contain a top-level array of at most 20 conversations. Symlinks/reparse points, hardlink aliases, wildcards, unsafe relative paths, nonregular files and mismatched JSONL `cwd` are refused. The source files remain unchanged. The compiled reader uses the existing no-follow, nonblocking filesystem owner and requires no Python installation.

## Retain a user prompt

`history.record` requires `owner_id`, `history_prompt`, `history_language`, `history_session_id`, `history_occurrence_id` and `history_recorded_at` (RFC3339 with offset). Supply the same timestamp and occurrence when retrying. The owner label is reviewed data and is checked against an existing store and confirmed identity profile; it is not authentication. The admitted OS-user-local home provides the local authority.

The server derives scope from the registered current workspace and validated case setup. Active client work retains case scope keyed by its validated registered case-agent ID, disambiguating equal legacy slugs across accounts; ordinary/internal workspace work retains workspace scope. Pending or conflicting selection is unavailable, without falling back to owner scope. Source is `owner`, content kind is `user_prompt`, and the prompt is limited to 32 KiB of UTF-8. Exact host review is required for each record. No consent to future collection is implied.

Same occurrence, owner, session, text, language and scope is an exact retry; different content conflicts. Retained data remains in canonical `HOME/brain/owner/prompt-history/`. Raw history stays outside execution receipts, diagnostics, managed bundles and generated memory. Profile changes still use the existing reviewed owner proposals.

## Inspect and control retained data

- Query `history.status` for metadata, retention, revision and `capture_mode: attended_only`. Optional `offset` and `limit` page at most 20 current-scope receipts. A fresh read creates nothing. `automatic_capture` is false.
- Apply `history.export` or `history.delete` with `owner_id` and 1–8 exact `history_ids` from the admitted scope. Export is separately reviewed and limited to 32 KiB serialized output; it returns only the explicitly approved selected entries. Delete affects the retained copy only.
- Apply `history.configure` with `owner_id` and `history_config`: schema version 1, optional consistent owner label, `max_entries` 1–10,000, `max_bytes` 1–8 MiB and `max_age_seconds` 1 hour–10 years. The review contains current/proposed retention and every removed entry's metadata. Global retention policy covers this owner's retained store, including other retained scopes.
- Apply `history.reset` with `owner_id` for an explicitly reviewed clearing of the whole owned retained store. The full removal metadata is shown; original source logs remain intact.

Mutation review binds the complete current pair and exact proposed revision. The owner serializes cooperating writers, rejects stale review and journals configuration/entries before publishing either file. Each individual publication is atomic; the two-file transition becomes readable only after its journal is cleared. An interruption retains both preimages in the private journal, blocks normal reads/mutations and preserves source logs. It does not promise atomic control over arbitrary programs that ignore the private store's protocol.

If status reports `recovery_required`, use its exact `review_digest` as `history_recovery_digest` in `history.recover`, together with `owner_id`. A fresh native review shows the original intent and exact pending operation. Recovery accepts only the original before/config-first/after states; unrelated changed state stays unavailable and preserved. No query repairs state or deletes locks.

Full removal metadata at the legal store ceiling needs a finite history-only 16 MiB review envelope. Other operation review limits remain unchanged. Actual host acceptance of a very large native form is a separate client qualification concern.

## Evidence boundary

Synthetic public SDK tests exercise accepted and declined review, missing host review, changed source/store/scope, restart replay, conflicting occurrence, explicit export/delete/configure/reset and a 32 KiB escaped prompt above the old request-size gate. Domain tests exercise interrupted publication and exact reviewed recovery. These are local adapter/domain results; they do not attest a native Desktop/Code session or Windows execution.
