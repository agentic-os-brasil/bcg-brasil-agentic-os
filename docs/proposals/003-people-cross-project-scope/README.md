# Proposal 003 — Organization, client account, case and PA expert boundaries

Status: draft; reopened for architectural refinement
Author: Marcelo Petrof Sanches, with Maestro (Claude)
Refinement: Daniel Scardini, with Kowalski (Codex)
Date: 2026-07-26
Related: decision `ATLS`; Specs 014, 016, 018 and 027; Proposal 004

## Decision requested

Adopt a clearer professional-agent topology that separates:

1. BCG organizational context;
2. durable client-account context;
3. project-specific case context;
4. centrally managed functional and industry expertise; and
5. Maestro's control-plane orchestration.

The current `account_agent` and `workspace_agent` model should not remain as two
overlapping product concepts for the same client. The target taxonomy is:

```text
Maestro — control plane and orchestration
│
├── Organization Context BCG
│   └── internal colleagues, advising, staffing and organizational context
│
├── Client Account Agent — one durable relationship context per client
│   ├── stakeholder intelligence and strategic account context
│   ├── Case Agent A — project execution and raw case context
│   ├── Case Agent B
│   └── Case Agent N
│
└── PA Expert Network — FPA + IPA
    └── centrally managed and versioned through Helix Brasil
```

This is a target architecture proposal, not a claim that the current catalog,
CLI, authorization graph or runtime adapters already implement these roles.

## Why the current model needs refinement

The accepted Human Atlas model correctly separates owner-private and
workspace-private data. The current agent topology also correctly makes raw
workspace access default-deny. Those invariants remain.

The product vocabulary, however, currently creates ambiguity:

- `account_agent` suggests durable client-level ownership;
- `workspace_agent` owns the raw context of a client/project interaction;
- a workspace can be perceived by users as either a client or a project;
- internal BCG colleagues are neither client stakeholders nor project-scoped
  entities; and
- `capability_specialist` and `practice_agent` do not yet express the intended
  distinction between execution skills and BCG Practice Area expertise.

Adding another account layer above the existing workspace role would deepen the
overlap. The better model is to define explicit organizational, client-account,
case and expertise boundaries.

## 1. Client Account Agent

Each client has one durable `client_account_agent`. Its system role is similar
to that of a Partner responsible for a client relationship:

- maintain the longitudinal strategic view of the client;
- preserve relevant relationship history;
- understand the principal stakeholders and their priorities;
- connect learning across cases;
- identify cross-case risks, opportunities and executive implications;
- guide and challenge Case Agents; and
- maintain continuity when project teams change.

This analogy does not replace the authority, accountability or judgment of a
human Partner. The agent is a strategic and relational memory, an advisor and a
continuity mechanism.

The Client Account Agent's context is progressively refined. It does not begin
as a complete client dossier and must not become an indiscriminate aggregation
of every project artifact.

Its technical identity is immutable. The user may choose and later edit a
display name with Maestro's guidance without changing the agent ID, scope,
authorization or provenance chain.

## 2. Stakeholder intelligence belongs at client-account level

The client account is authoritative for a stakeholder's necessary professional
identity and durable facts only after explicit promotion and review. It is not
the default home for every interaction, position, observation or interpretation
produced by a case.

The Client Account Agent may maintain explicitly promoted professional context
such as:

- role, area and formal responsibilities;
- observed influence and relevant stakeholder relationships;
- priorities and concerns;
- durable interaction outcomes relevant across cases;
- reviewed positions on relevant initiatives;
- communication preferences;
- decisions, commitments and open follow-ups; and
- freshness, source, confidence and sensitivity.

This context supports meeting preparation, audience-specific materials,
relationship approaches, message calibration, objection anticipation and
stakeholder mapping.

Observations, interpretations, hypotheses and detailed interaction history are
created and remain in the originating case by default. Only a reviewed
`CaseFactProposal` may elevate a durable, cross-case-safe record to the client
account.

The promoted record must distinguish:

| Kind | Meaning |
| --- | --- |
| Fact | Confirmed role, statement, decision or event |
| Observation | Behavior or signal recorded in an interaction |
| Interpretation | A reasoned reading of influence, interest or resistance |
| Hypothesis | An unconfirmed proposition that still needs validation |

The system must not convert a situational interpretation into a permanent fact.
Unnecessary personal or sensitive information is forbidden. One case cannot
enumerate or read another case's stakeholder context. Cross-case reuse occurs
only through an account projection created from reviewed promoted facts.

For a specific activity, the Case Agent receives a bounded
`StakeholderBrief`, not unrestricted access to the full relationship history.
The brief includes the interaction objective, relevant people, known
priorities, material history, recommended messages, cautions, evidence and
items that still require confirmation.

## 3. Case Agents

Each material project or case may have a dedicated `case_agent`.

The Case Agent:

- owns the project's detailed objective, workplan, decisions and risks;
- works with the case's raw files and authorized data;
- executes analysis, code, documents and other deliverables;
- uses the skills and tools required for the case;
- requests bounded account context or stakeholder briefs when needed;
- consults PA Experts through a sanitized advisory request; and
- proposes durable client learning for promotion.

The distinction is:

> The Client Account Agent guides, connects and preserves continuity.
>
> The Case Agent investigates, produces and executes.

A Case Agent receives its own immutable technical identity and may have a
user-defined display name. Persistent agent context is separate from ephemeral
execution attempts, which retain their own budgets, checkpoints, cancellation
and stale-recovery lifecycle.

## 4. Case-to-account promotion is periodic and governed

The Client Account Agent must not browse cases or automatically absorb their
content. Instead, each active case periodically produces promotion candidates.

The cadence is configurable, with promotion review expected:

- during an active case at an agreed interval;
- at material project checkpoints;
- after important client interactions; and
- at case closure.

Suitable promotion candidates include:

- confirmed changes in client strategy;
- relevant stakeholder updates;
- executive decisions and durable commitments;
- recurring business concepts;
- cross-case risks or opportunities; and
- learning likely to matter in future work.

Every `CaseFactProposal` must contain the source case, source date, author,
classification, confidence, review state, freshness or expiry rule, supporting
evidence and known contradictions. Promotion creates a curated account record;
it does not expose a raw case pointer.

Revocation or correction must remain possible when a source changes, expires or
is found to be wrong.

## 5. Organization Context BCG

Internal BCG colleagues are cross-project and often cross-client. They do not
belong in each client account or case.

The `Organization Context BCG` therefore sits above client accounts and holds
only legitimate internal professional context, including:

- colleagues and teams;
- professional roles and working relationships;
- capabilities and relevant experience;
- advising;
- staffing; and
- organizational ways of working.

In the initial model, colleague context remains owner-private rather than an
organization-wide shared people directory. A future shared organizational
directory requires a separate governance, consent, correction and access
decision.

An account or case receives only a bounded organizational projection for a
declared purpose. The organizational layer must never become a path for client
information to move from one account to another.

## 6. PA Experts are functional and industry agents

`PA Experts` are agents representing BCG Functional Practice Areas and Industry
Practice Areas — FPA and IPA.

They provide the best curated point of view currently available for their
domain. Examples include an Insurance IPA, a Pricing FPA or a People &
Organization FPA.

A PA Expert:

- owns a bounded professional mandate and verified knowledge canon;
- combines concepts, methods, benchmarks and managed, public or
  internally approved illustrative material;
- receives only a minimum `SanitizedAdvisoryRequest`;
- returns a structured `AdvisoryResponse` with assumptions, sources,
  limitations and expert version;
- cannot browse a client account or case; and
- does not execute project work on behalf of a Case Agent.

Programming, quantitative modeling, document production and presentation
building are execution capabilities used by Case Agents. They are not PA
Expert categories.

## 7. Helix Brasil manages the PA Expert network

The PA Expert network is part of **Helix Brasil**. Helix Brasil is responsible
for constructing, curating, installing, versioning, updating and distributing
the expert definitions and their knowledge canons.

Each PA Expert registry entry includes:

- immutable technical ID;
- display name;
- Practice Area and FPA or IPA classification;
- mandate and explicit exclusions;
- canon and source references;
- published version;
- change history;
- accountable curators; and
- lifecycle state: `draft`, `reviewed`, `published` or `deprecated`.

Published versions are immutable. An advisory receipt records the exact PA
Expert and version used. A new release does not silently change prior analyses
or active cases; adoption is explicit and auditable.

The default evolution loop is:

```text
Managed, public or internally approved knowledge signal
  -> sanitization
  -> Practice Area review
  -> versioned change
  -> Helix Brasil publication
  -> managed installation update
```

Case-derived learning does not enter this loop by default. A field signal may
become a local improvement proposal, but it remains blocked from Helix
publication and distribution until a separate derivative-publishing contract
defines client authorization, non-reconstructability, human review,
provenance, classification and an auditable receipt. Removing client names or
calling material sanitized is not sufficient authorization.

Client identities, raw case context, confidential client data and
reconstructable client-derived patterns never enter the managed expert canon.

## 8. Onboarding composition

This topology provides a repeatable knowledge foundation for an Associate
joining a project:

1. the **Client Account Agent** explains the client, relationship,
   stakeholders and strategic priorities;
2. the **Case Agent** explains the problem, current truth, decisions, progress
   and next steps; and
3. relevant **PA Experts** explain the functional and industry perspective,
   methods, benchmarks and common failure modes.

The resulting onboarding pack is a bounded composition of account, case and PA
knowledge. It is not a copy of the underlying contexts.

## 9. Maestro remains the control plane

Maestro:

- identifies the relevant client account and case;
- selects appropriate PA Experts;
- constructs the smallest permitted work packet;
- dispatches the allowed agent;
- tracks execution lifecycle and evidence; and
- synthesizes the returned results.

Maestro orchestrates but does not own the organizational, account, case or PA
Expert contexts. Ownership, dispatch and knowledge are separate planes.

## Boundary and contract summary

| Producer | Consumer | Contract | Key restriction |
| --- | --- | --- | --- |
| Organization Context BCG | Client Account or Case | `BoundedOrgProjection` | No client-to-client transit |
| Client Account Agent | Case Agent | `AccountProjection` or `StakeholderBrief` | Minimum purpose-bound context |
| Case Agent | Client Account Agent | `CaseFactProposal` | Periodic, reviewed promotion only |
| Account or Case | PA Expert | `SanitizedAdvisoryRequest` | No raw client or case access |
| PA Expert | Account or Case | `AdvisoryResponse` | Exact expert version and sources |
| Maestro | Any governed agent | sealed work packet | Dispatch does not grant ownership |

Every boundary remains default-deny. Every projection or promotion must carry a
purpose, classification, provenance, expiry or freshness rule and audit
receipt.

## Migration from the accepted topology

Decision `ATLS`, Specs 014, 016, 018 and 027 and the current implementation use
`account_agent`, `workspace_agent`, `practice_agent`,
`capability_specialist` and `subject_specialist`.

The topology in this proposal is an ownership and context-boundary map, not a
required runtime call chain. Maestro may dispatch a Client Account Agent or a
Case Agent directly. A Case Agent may invoke one bounded execution specialist
or PA Expert child when the catalog permits it. Runtime evolution must preserve
the current maximum depth of two, one active child per agent and one active
branch at a time unless a separate decision changes those invariants.

If this proposal is accepted, follow-up work must:

1. define a human-reviewed Atlas migration that classifies existing records as
   `internal colleague -> owner-private Organization Context BCG`,
   `durable client stakeholder fact -> account promotion candidate`, or
   `case-specific interaction -> originating case`; no automatic copy, move or
   deletion is allowed;
2. converge the durable client responsibilities of `account_agent` and the
   client-facing meaning of `workspace_agent` into `client_account_agent`;
3. evolve project-scoped `workspace_agent` instances into `case_agent`;
4. evolve the managed practice chain into versioned FPA and IPA PA Experts;
5. keep execution skills separate from PA Expert roles;
6. update the canonical catalog, scaffolding, schemas, authorization
   conformance fixtures and CLI vocabulary together;
7. preserve immutable IDs or provide an explicit migration map;
8. keep old role names as compatibility aliases only for a defined migration
   window; and
9. append a durable decision that supersedes the affected accepted contracts.

No runtime role should be renamed independently. Claude and Codex must migrate
through the same runtime-neutral contract and fail closed if a critical role or
scope cannot be mapped.

## What remains unchanged

- Raw client and case content stays outside the managed core.
- Client and case boundaries remain default-deny.
- Maestro remains tool-free and routes action through governed delegation.
- Client context is never promoted automatically.
- PA Experts never receive persistent access to account or case data.
- Managed bundles contain definitions and sanitized knowledge, never client or
  owner-private content.
- Runtime activation remains unavailable until adapters invoke and pass the
  shared enforcement contract.

## Out of scope for this proposal

- Implementing the new runtime roles in this PR.
- Migrating existing local agent instances.
- Creating an organization-wide shared people directory.
- Defining Helix Brasil's complete publishing and signing infrastructure.
- Storing real colleague, stakeholder, client or case data in the repository.

## Acceptance conditions for this proposal

This proposal is ready to become a durable architectural decision when review
confirms:

1. the target role names and responsibilities;
2. the owner-private versus shared governance of Organization Context BCG;
3. the default promotion cadence and review authority;
4. the Helix Brasil ownership and versioning contract;
5. the compatibility strategy for existing role IDs and local instances; and
6. the implementation sequence across specs, catalog, scaffolding, CLI and
   adapters.
