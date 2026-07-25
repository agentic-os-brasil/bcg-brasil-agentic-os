# Spec 016 - Workspace agent initialization and context

Status: accepted architecture; guided interview contract, compact local state
and research-plan approval validation implemented. External research execution,
economic rollups and runtime adapters remain unavailable.

## Objective

Create a useful workspace agent without turning its state into an untraceable
context blob. Creation combines a user interview, authorized external research
and a small public economic snapshot; all substantive context remains
workspace-scoped, versioned and attributable.

## Creation flow

`bcgos init` registers the workspace boundary first. It then offers a guided
workspace-agent setup that collects only the information needed to establish
the mandate:

1. client/account and project label;
2. objective, deliverables, time horizon and decision to support;
3. user roles, stakeholders and information classification;
4. known facts, working hypotheses, constraints and available local material;
5. desired research questions and permitted external-disclosure level;
6. success signals, risks and preferred refresh cadence.

The user reviews the resulting mandate and explicitly confirms any external
research plan before the first query is sent. Setup remains resumable; a
workspace may exist without completed research.

## External research guardrail

External research is a disclosure boundary, not a background convenience. The
agent must:

- show the proposed query themes, allowed sources and disclosure risk before
  searching;
- use the least revealing terms that answer the approved research question;
- avoid unpublished project names, confidential facts, stakeholder names and
  client strategy unless the user explicitly authorizes those exact terms;
- prefer an approved source allowlist appropriate to the question, including
  primary sources for company, regulation and macroeconomic facts;
- record each external query, its approval, source URL, retrieval time,
  extracted claim, evidence strength and classification;
- label uncertainty, disagreement and freshness rather than presenting a
  research summary as established fact.

The initial dossier may include market/client context, facts, key trends,
upside and downside hypotheses, open questions and a public macroeconomic
snapshot. Bullish and bearish theses are structured hypotheses: each declares
its evidence, assumptions, counter-evidence and signals that would change it.
They are never represented as investment advice or as facts about a client.

## Three context layers

### 1. Agent state: compact operational control plane

The agent reads a small state at the start of work. It contains only:

- workspace identity, mandate version and active lifecycle state;
- current objective, active workstream and next decision or deliverable;
- material constraints, approvals and open risks;
- pointers to the current dossier, handoff and approved research plan;
- freshness/refresh signals and the last successful work summary.

It must not contain raw documents, transcripts, broad research summaries,
embeddings, unreviewed facts or accumulated discussion history.

### 2. Workspace dossier: versioned evidence and reasoning

The workspace dossier is the source for interview notes, research results,
claims, sources, bullish/bearish hypotheses, stakeholder context and decision
history. Every entry has provenance, retrieval/creation date, classification,
authoring agent or user and review/freshness state. It remains inside the
workspace authorization boundary.

### 3. Public economic rollup: reusable but isolated

The OS may maintain a separate, public-only economic rollup sourced from
approved public material. It contains versioned macroeconomic snapshots and
their provenance, never client data, workspace-derived queries, metadata,
synthesis or automatic write-back.

A workspace agent can request a filtered snapshot and records the snapshot
version it used. The snapshot does not grant access to any other workspace.

## Refresh and promotion

Research is refreshed from explicit triggers: approaching a decision date,
user request, source expiry, material news or an identified evidence gap. The
agent proposes a new external research plan when the prior approval does not
cover the query.

Only reviewed, non-confidential facts can be promoted to the account-agent
layer under Spec 006. Neither interview data nor research findings flow upward
automatically.

## Acceptance criteria for implementation

1. Workspace setup can be completed, paused and resumed without losing the
   authorization boundary.
2. No external query executes without recorded user approval and an approved
   scope.
3. The startup state stays within the compact control-plane schema and points
   to, rather than duplicates, the dossier.
4. Every dossier claim can show source, date, classification and confidence.
5. Bullish/bearish hypotheses expose assumptions, counter-evidence and
   invalidation signals.
6. Public economic rollups can be versioned and injected into a workspace,
   but cannot read from or write to any workspace.
7. Claude and Codex adapters provide equivalent approval, provenance and
   workspace-isolation behavior or declare setup unsupported.
