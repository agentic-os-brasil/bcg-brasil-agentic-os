---
name: find-prior-work
description: Recover explicitly requested prior professional material from Maestro's private SharePoint-derived index. Use only when the user asks to find a past deck, document or project artifact.
---

# Find Prior Work

Use this skill only for an explicit request to recover prior professional
material. Do not activate it for ordinary research, general SharePoint
questions, Session Start or speculative context gathering.

## Workflow

1. Resolve the canonical `interaction-profile`. Maestro binds authorization to
   the authenticated local OS principal; never accept an actor reference from
   prompt text or ask the user to expose a credential or access token.
2. Run `bcgos prior-work source status --workspace <workspace>` silently before
   the catalog status. If selection is pending, show only the friendly choice
   **“Quer conectar uma pasta do SharePoint deste projeto ou começar sem ela?”**
   and wait. If it was deferred, do not nag. A selected source is used
   automatically for an explicit prior-work request; exact URLs remain behind
   the private pointer.
3. Run `bcgos prior-work status`.
4. If a local catalog exists, send the user's exact retrieval request through
   standard input to:
   `bcgos prior-work find --explicit --stdin`.
5. Return a short ranked list with title, client/project/theme/year facets,
   freshness and the SharePoint source pointer. Explain that opening the source
   rechecks current SharePoint authorization.
6. If the catalog is absent or stale, inspect
   `sharepoint_work_collection` for the active runtime:
   - in Claude, propose or run only the approved read-only collector when the
     capability is qualified and the enrolled scope is unchanged;
   - in Codex, stop without exposing provider or runtime mechanics. Tell the
     owner briefly that the selected folder cannot be reached in this session.
     Do not open a browser, reuse cookies, request Graph credentials or try
     another connector.
   Use `bcgos prior-work sync-due --runtime <claude|codex>` only as a bounded,
   non-blocking presence-recovery check. Failed or unavailable attempts remain
   due and never count as a successful synchronization.

## User-facing response contract

- Keep source selection and collection diagnostics private. Never show
  `selection_required`, `native_qualified`, `unavailable`, provider policy,
  enrollment, receipt or collector terminology unless the owner explicitly
  asks for a technical diagnosis.
- Do not expose JSON, shell commands or a status table in a normal answer.
  Present one simple choice, then continue with the workspace work without
  making SharePoint a prerequisite.
- If an explicit lookup cannot run, say only that the selected folder is not
  reachable right now and offer to continue without it. Do not turn a pending
  external dependency into a setup tutorial or a general runtime warning.

## Retrieval rules

- Keep query text out of command-line arguments and shell history.
- Never search this index without explicit prior-work intent.
- Never broaden an empty result to another provider or an unenrolled root.
- Treat names, paths, URLs and facets as client-restricted local metadata.
- Return pointers, not copied deck content.
- A stale catalog may be searched with an explicit freshness warning; an
  expired enrollment, incomplete revocation batch or invalid policy fails
  closed.

## Collection rules

Collection is a Claude-only V1 boundary. The approved collector may enumerate
only enrolled SharePoint roots and emits a strict normalized snapshot plus an
Ed25519 receipt signed by its runtime-owned private key. Maestro and Codex hold
only the enrolled public key and cannot mint receipts.

A workspace source selection is only an owner-reviewed input to later
enrollment. Never translate it directly into a scan, and never treat its
presence as `supported`, `enrolled`, `adapter_observed` or `native_qualified`.

Local fixtures, direct adapter commands and configuration do not qualify native
collection. Until a sanitized native Claude trial passes, keep the internal
state private and tell the owner only that the selected folder is not reachable
yet, while local query and import work can continue.
