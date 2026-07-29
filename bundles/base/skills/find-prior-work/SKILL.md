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
2. Run `bcgos prior-work status`.
3. If a local catalog exists, send the user's exact retrieval request through
   standard input to:
   `bcgos prior-work find --explicit --stdin`.
4. Return a short ranked list with title, client/project/theme/year facets,
   freshness and the SharePoint source pointer. Explain that opening the source
   rechecks current SharePoint authorization.
5. If the catalog is absent or stale, inspect
   `sharepoint_work_collection` for the active runtime:
   - in Claude, propose or run only the approved read-only collector when the
     capability is qualified and the enrolled scope is unchanged;
   - in Codex, report `unavailable/corporate_policy` and stop. Do not open a
     browser, reuse cookies, request Graph credentials or try another connector.
   Use `bcgos prior-work sync-due --runtime <claude|codex>` only as a bounded,
   non-blocking presence-recovery check. Failed or unavailable attempts remain
   due and never count as a successful synchronization.

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

Local fixtures, direct adapter commands and configuration do not qualify native
collection. Until a sanitized native Claude trial passes, report collection as
unavailable even though local query and import tests pass.
