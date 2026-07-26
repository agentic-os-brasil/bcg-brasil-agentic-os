# Proposal 003 — Internal people are cross-project

Status: draft (not yet submitted for review)
Author: Marcelo Petrof Sanches, with Maestro (Claude)
Date: 2026-07-26
Related: decision `ATLS`; specs `014` (human-atlas bootstrap), `016` (workspace-agent
boundaries); Proposal 004 (depends on this)

## The change in one line

Move the **internal-colleague** context (`people/`) out of the per-workspace atlas and up
into the **owner / account (cross-project)** layer.

## Why

The accepted human-atlas model (Spec 014 / `ATLS`) places `people/` inside
`<workspace>/brain/`. That's the right home for **client-side stakeholders** — the Acme
CFO belongs to the Acme engagement. But it's the wrong home for **internal colleagues**
(project leads, principals, partners, peers): they are inherently **cross-project**. The
same partner staffs three of your cases over a year.

Kept inside a workspace, an internal colleague's file gets:
- **re-created in every workspace** you share with them — duplication;
- **fragmented relationship history** — how they like to receive updates, feedback
  patterns, what you learned working together, all split across engagements and lost when
  a workspace is archived.

An internal colleague is durable, owner-level context — exactly what the account layer is
for (curated, cross-project, not tied to one client's boundary).

## Proposed placement

| Kind of person | Layer | Home | Owner |
|---|---|---|---|
| Client-side stakeholder | workspace | inside the client file | `workspace_agent` |
| Internal colleague (PL, partner, peer) | account / owner | `people/` under the owner atlas | `account_agent` |

This keeps the workspace boundary intact (client stakeholders stay workspace-scoped) while
letting the relationship with an internal colleague accumulate in one durable place across
every engagement.

## Consequences

- Spec 014's bootstrap creates `people/` under the **owner** root, not the workspace root.
- The `people-keeper` capability specialist (Proposal 004) is **account-scoped**, dispatched
  by the `account_agent`, not the `workspace_agent`.
- No new sharing tier: this is still owner-private context (one person's view of their
  colleagues), governed like the rest of the account layer — not an org-wide people
  directory.

## Not in scope

- An organization-wide shared people directory (that would be a separate governance
  question, like the shared-`concepts` one).
- Any change to how client-side stakeholders are recorded (they stay in the client file).
