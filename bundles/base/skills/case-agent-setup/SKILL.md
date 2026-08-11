---
name: case-agent-setup
description: Create or resume one Case Agent through a reviewed interview, approved public research, sourced evidence and an optional public economic snapshot. Use whenever a new client project case is initialized or its briefing needs refresh.
---

# Case Agent Setup

Build useful project context while preserving the case workspace as a
confidentiality boundary. The Case Agent owns this workflow and must never
borrow context from another case or from the Client Account Agent.

## Runtime-first surface

`$case-agent-setup` is the user-facing workflow. The agent owns the interview,
normalization and persistence of the reviewed case result. Do not ask the
owner to run `bcgos`, create JSON, provide a run ID or translate interview
fields into a command envelope. The installed `bcgos workspace-agent` surface
is a compatibility implementation for older workspaces and is not the normal
execution path for a new case.

Use the workspace recipes and canonical locations already present in the
workspace:

- `brain/projects/` for case context and working plans;
- `brain/decisions/` for decision records and rationale;
- `brain/tasks/` for explicitly accepted open work;
- `brain/deliverables/` for reviewed outputs;
- `brain/sources/` for authorized source pointers, never copied client bodies.

Create only the smallest directory or Markdown artifact needed by the case.
Keep owner context outside the workspace and never invent a second memory or
task store.

## Interaction profile

Resolve the canonical `interaction-profile` before starting. It controls how
much technical detail is shown during setup, but never changes approval,
classification, provenance or case isolation requirements.

## First useful result

1. Confirm the active workspace from the runtime orientation. Do not infer a
   workspace from an arbitrary path or conversation fragment.
2. Use the six prompts as a flexible starting recipe: decision and horizon;
   audience and constraints; useful result; authorized material; balanced
   hypotheses; and next step. Maestro decides which questions are needed from
   the conversation and existing workspace context.
3. Keep answers as a temporary conversational draft. Resolve ambiguity in the
   flow; do not reject the conversation because a field name or optional detail
   is missing.
4. Show the consolidated brief and one-to-three-action plan in plain language.
   If the owner has asked to do the work and the next action is ordinary,
   proceed without an additional command-level confirmation. Ask only for
   destructive, secret-bearing, cross-workspace or externally publishing work.
5. Write the decision brief, plan and handoff as readable
   Markdown in the canonical workspace locations. Include date, owner, scope,
   evidence pointers, assumptions, open questions and next step. Do not write
   prompts, transcripts, credentials or client bodies into a control file.
6. On a correction, edit the reviewed Markdown artifact when the owner asks;
   preserve prior decision or revision history when it matters. On a later
   session, inspect the existing artifact and continue from its next step.

The first-value flow does not browse, ingest documents, query a wiki, dream
memory, refresh economics or create an agent. A SharePoint root named in the
brief is not a connection or collection grant. External research and source
processing remain separate approved flows below.

## Optional public research

1. Propose a minimized public research plan. Use only hostname allowlist entries
   and never include confidential project names, stakeholder names, unpublished
   strategy or client-provided facts in query themes.
2. Display the exact purpose, query themes and source allowlist. Continue only
   after the user explicitly approves it.
3. Execute external research only when the runtime exposes both an approved
   web-search/browser tool and an enforceable workspace/pre-action guard.
   Otherwise report the research capability as unavailable; never substitute
   an unapproved API, credential or provider, and never claim hard isolation.
4. Keep approved queries and retained public claims in the reviewed case
   Markdown artifact with source URL, retrieval date, evidence strength,
   validity date and classification. Stop when the approved budget is
   exhausted.
5. A public macroeconomic snapshot may be written as a reviewed Markdown
   artifact beside the case decision record. Every claim must be public and
   reference a declared source.
   The human attestation is a governance boundary, not automated content
   detection; never use workspace queries, metadata or client-derived synthesis.
6. Return the workspace, briefing version, approved plan, sourced findings,
    economic snapshot version, freshness gaps and unavailable capabilities.

## Refresh

Refresh only when requested, when a decision date approaches, when a source is
stale or when a material event creates an evidence gap. A new query outside the
approved themes or domains requires a new plan and approval.

## Invariants

- The compact state contains pointers and operating signals, not research
  bodies or transcripts.
- Briefs, plans, approvals and evidence are immutable, case-scoped
  artifacts with provenance.
- No cross-case lookup or automatic promotion to Client Account context occurs.
- A project SharePoint pointer stays case-scoped. Cross-project prior-work
  retrieval uses the separate explicitly enrolled index and is never inferred
  from this brief.
- Attested public economic snapshots are stored outside every workspace and can
  only be attached by immutable ID. Each claim points to declared public
  sources and records who attested that no workspace material was used.
- Runtime filesystem isolation remains fail-closed: if the runtime cannot
  enforce the declared workspace root, state that limitation explicitly and do
  not claim hard isolation.
