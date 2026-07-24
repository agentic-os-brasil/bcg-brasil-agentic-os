# Proposal 001 — Context and Brain Model

Status: draft (not yet submitted for review)
Author: Marcelo Petrof Sanches, with Maestro (Claude)
Date: 2026-07-21
Related: `specs/006-memory-persistence.md`; decisions `MEMO`, `DREM`, `ATOM`, `MCLI`; open questions `Q-012`, `Q-013`, `Q-021`-`Q-027`

## 1. Problem

The repository already has a working, well-tested **canonical memory engine** (L1 daily -> L2
weekly -> L3 thematic -> lifetime, via deterministic "dreaming"). It has no opinion yet on
**where a user's actual professional knowledge lives** — client facts, project state, decisions,
reusable frameworks.

Separately, the Maestro side has a domain-organized Markdown knowledge
base (folders per client/project/decision, not per time period) that has been extremely useful specifically *because* it is navigable by a human, not just injectable into
a prompt. Comparing the two in a recent working session, and independently in a Slack conversation
between Daniel and Marcelo on 2026-07-21, both arrived at the same underlying distinction. This
proposal formalizes it, proposes an initial folder model, and states the design principles the
model must satisfy.

## 2. Core distinction: Human Atlas vs. Canonical Memory

Two structures, two jobs, both real, neither optional:

```
Canonical memory                          Human atlas
L1 / L2 / L3 / lifetime                   Markdown pages, one file per entity
compact, versioned, provenance-tracked    rich, navigable, potentially long
used only for budgeted context injection  never injected whole; opened/read directly
```

- The **atlas** is the source of truth a person (or an agent acting on their behalf) reads,
  writes, and links by hand — one file per client, per project, per group of decisions or learnings. It is what makes the system worth using even with no agent running at all. Think of it as a real BCGer's memory, with knowledge that is built day by day, gaining more "consulting experience" as time goes by
- **Canonical memory** (already implemented in `internal/memory`) is a compaction cache: it
  exists so that a session can start with a bounded amount of continuity without reading every
  atlas file or the full session history. It has no independent value as a place to look
  something up — its role is injection under budget, with a `drill_down` pointer back to the
  real source. It makes the system more responsive and faster, but does not replace the full atlas memory.

Neither replaces the other. A design that only has canonical memory produces exactly the
"unstructured aggregation" feeling flagged in review: everything blurs together by time period
instead of staying organized by the thing it's about. A design that only has the atlas has no
answer for session-start continuity at scale.

## 3. Design principles the atlas must satisfy

These are constraints on *any* folder we add later, not just the ones proposed in section 4.

### 3.1 Legible to humans and addressable by agents, equally

The atlas is read by two very different consumers, and neither can be favored at the other's
expense:

- **Plain Markdown only.** No proprietary or binary formats, no content that only renders inside
  a specific tool — a human opens the file directly; an agent reads/greps it with no exporter.
- **Predictable shape per segment.** Each folder has a template, so an agent never has to infer
  structure from prose, and a human always finds the same section in the same place across
  clients or across projects.
- **Every cross-reference is a real, path-preserving Markdown link** (`[\`path\`](path)`), never a
  bare mention or a wikilink. This is not only for a human clicking through — it is what lets an
  agent grep the exact path from a cold start with no alias-resolution step.
- **Folder "mother" files** (`_<folder>.md`) are simultaneously a human table of contents and a
  cheap machine-readable index — an agent reads one file to learn what exists in a folder instead
  of listing the filesystem and guessing.
- **Unique naming** (no two files share a name) removes ambiguity for a human searching and for an
  agent resolving a reference the same way.

### 3.2 Living documents, updated through use, owned per segment

Atlas files are never a one-time export. They accumulate continuously as work happens — a fact
lands in the right file the same day it's learned or during retro sessions at the end of the week.

Concretely:

- Each segment has an **owning agent or ritual** responsible for writing to it. Not every agent
  writes everywhere. In Maestro's current implementation (offered here as a reference point, not
  as the final roster — the exact agent lineup is a phase-2 decision once this model is agreed):
  a `client-keeper`-type agent owns `clients/`, a `work-logger`-type agent owns `daily/` and
  `tasks/`, each project's own decisions are written through a dedicated decision-recording flow
  rather than any one agent, `learnings/` is written through both immediate capture and a
  reflective/retro-type ritual, and `development/` is written through feedback/retro rituals.
- Other agents may **read** any segment relevant to their task, but should not write outside the
  segment they own — this is what keeps two agents from producing conflicting updates to the same
  file.
- This section fixes the *principle* (explicit per-segment ownership, continuous update). Which
  concrete agents own which segment in the merged system is deliberately left open here — it
  depends on the agent/skill merge that follows this proposal.

## 4. Proposed atlas structure

Adapted from Maestro's `brain/` (in production use) and Daniel's Kowalski structure (`clients`,
`tier1`, `concepts`), reconciled into one taxonomy. For each segment: the question it answers,
what connects to it, and who maintains it (per 3.2, Maestro's current implementation as reference).

A worked reference implementation of every segment below (mother files + templates, adapted
from Maestro's real, in-use files) lives alongside this document in
[`atlas-example/`](atlas-example/) — read the two together.

### `atlas/profile/`
- **Answers:** "Who is this person, and how do they like to work?" — role, seniority, working
  style, communication preferences, background. Composed initially of 3 founding documents: (1) `bio.md`; (2) `case-history.md`; (3) `working-preferences.md` — all of which should be created when the user onboards the system (over the development of this system, we should build a robust onboarding routine to guarantee a solid construction of these founding documents).
- **Connects to:** read at the start of nearly every substantive task, since it calibrates tone,
  depth and delegation; informs what "growth" means in `development/`.
- **Maintained by:** low-frequency, direct edits — not really "produced" by a specialist agent;
  updated when the user states something durable about themselves.

### `atlas/clients/`
- **Answers:** "What do we know about client X — people, org, history, sensitivities,
  commercials?"
- **Connects to:** bidirectional link with `projects/` (a client lists its workstreams; a project
  names its client); referenced by `daily/` entries that touched that client; named by any decision
  recorded in that client's project(s).
- **Maintained by:** one agent owns writes here; other agents read from it (e.g. to prep an
  interaction) but don't edit it directly.

### `atlas/people/`
- **Answers:** "Who do I work *with* internally (project leads, managers, partners, peers),
  and how do I work well with them?" — the mirror image of `clients/`, but for the firm's own
  people rather than the client side. Not in the original draft of this proposal; added after
  finding it already real and in active use in Maestro's `brain/people/`.
- **Connects to:** referenced from `projects/`/`clients/` team listings; feedback given to a
  person may link back from `development/project-feedback/`.
- **Maintained by:** direct, low-frequency edits — updated when a working relationship
  produces something durable worth remembering.

### `atlas/projects/`
- **Answers:** "What is the project's scope and core objectives? How is the workplan set up? What's
  the definition of success? What are the potential risks? Where does workstream X stand today —
  current state, key artifacts, the load-bearing numbers?" Each project file also carries its own
  **"Decisions" subsection** — durable, case-specific choices (methodology, scope, commercial terms)
  belonging to this project alone, recorded in `daily/` the moment they're made and then promoted
  into this subsection; append-only, never rewritten, same discipline as any other durable record.
- **Connects to:** links to its client; cites the `concepts/` playbooks it draws on; `daily/`
  entries reference which project(s) a day touched and record decisions the day they happen.
- **Maintained by:** more than one agent writes to different subsections of the same file (status
  and narrative from one, quantitative "current truth" figures from another/an analyst-type role,
  decisions logged through a dedicated decision-recording flow) — worth naming explicitly since not
  every segment has a single writer.

### `atlas/learnings/`
- **Answers:** "What have I learned about doing this work well, leading, and navigating a career —
  the kind of wisdom that outlives any single project and compounds over years?" Not case-specific
  and not a technical playbook — closer to the reflective, career-spanning lists senior people
  sometimes write looking back on a long tenure. The bet here is building that same depth
  continuously, day by day, through ordinary interaction with the agent, rather than reconstructing
  it once, years later, from memory.
- **Connects to:** fed from `daily/` the moment a standout insight surfaces in conversation, and
  systematically through the weekly retro, which distills durable learnings out of the week's
  `daily/` evidence. A learning may eventually crystallize into a concrete, operational entry in
  `profile/working-preferences.md` once it becomes a standing practice — but most entries stay as
  narrative wisdom, never operationalized, and that's fine. Loosely relates to `development/` (a
  retro may surface or reference the same evidence) but is deliberately less formal/institutional
  in register.
- **Maintained by:** the retro ritual (primary distillation point) plus immediate capture whenever
  a standout insight surfaces mid-conversation.

### `atlas/concepts/`
- **Answers:** "What's our established way of doing X?" — the hard, technical, reusable
  methodology (e.g. "a spend-cube analysis is always run in this order, with this framework") built
  up over a career; deliberately the *technical* counterpart to `learnings/`'s *soft* register — one
  is the craft an associate builds toward partner, the other is the wisdom about people, leadership
  and self that comes with the same years.
- **Connects to:** `projects/` link out to the playbooks they use.
- **Maintained by:** curated directly, typically hand-filed once a reusable method has proven
  itself across more than one project — the one segment flagged (open question 7.2) as a
  candidate for org-wide sharing rather than per-user.

### `atlas/tasks/`
- **Answers:** "What do I need to do, and in what order?"
- **Connects to:** `daily/` logs open and close tasks; backlog items link to their `project`; may
  sync with an external task tool.
- **Maintained by:** one agent maintains it; a separate prioritization-focused agent reads it to
  recommend sequencing.

### `atlas/daily/`
- **Answers:** "What happened on/around day Z" — the raw evidence trail.
- **Connects to:** this is the **source layer**: durable facts get promoted out of `daily/` into
  their proper home (a client/project file, a project's Decisions subsection, or `learnings/`)
  rather than staying only here. A short
  `Related:` line names the client(s)/project(s) touched, so a day is never an orphan even before
  promotion happens. This is also the literal raw material canonical memory compacts into L1.
- **Maintained by:** written incrementally through the day by whichever agent did the work;
  finalized at closing time by one agent.

### `atlas/development/`
- **Answers:** "How is this person growing, and what is the evidence?" — objectives, periodic
  retrospectives, project-level feedback, career-conversation synthesis.
- **Connects to:** densely cross-linked internally — objectives, retros, project feedback and
  career-conversation synthesis all reference each other; retros pull their evidence directly from
  `daily/` logs; feedback folds into objectives; a career-conversation resets them.
- **Maintained by:** a reflection-focused ritual (retros) plus a dedicated feedback-capture flow.


## 5. Conventions carried over as-is from `brain-conventions.md`

These are mature, load-bearing, already satisfy section 3's principles and were established to guarantee a healthy development and maintenance of the whole second brain:

- **Retrieval-first**: file a fact where it will be searched for, not where it arrived.
- **Folder mothers (MOCs)**: every folder has a `_<folder>.md` describing it and listing members.
- **No orphans**: every file links to >=1 neighbour; every cross-reference is a real Markdown
  link with the path preserved, never a bare path or wikilink.
- **Single source of truth**: load-bearing numbers live in exactly one place (a project's
  `Current truth` block); a project's decisions live only in that project's Decisions subsection;
  career-spanning wisdom lives only in `learnings/`; everything else links to it.
- **Naming**: no two files share a name; the folder gives the type.

## 6. Scope of canonical memory under this model

Canonical memory keeps its existing contract (`specs/006`) unchanged — **this proposal requires no
engine code change**. A single workspace, covering all of a user's professional work (matching the
engine's implicit default today), is sufficient. Per-entity separation is not the memory engine's
job to solve by splitting into many workspaces — it is already solved by the atlas (one file per
client/project, no workspace concept involved there at all).

The constraint this proposal adds is a **usage boundary, not a structural one**:

> Canonical memory answers "what has generally been going on lately," for session-start
> continuity. It is never authoritative about any single client or project — the atlas always is.
> An agent must never cite a canonical-memory digest as the record of a specific entity; it opens
> the atlas file instead.

Consequence: no per-entity workspace bookkeeping, and no routine to enumerate "active entities" and
loop `AssembleContext` across them — one call at session start is enough. The earlier concern (two
clients' notes blurring together in one weekly digest) stops being a problem once nobody treats
that digest as entity-specific truth in the first place — that authority always belonged to the
atlas.

This also relaxes the requirement on the `Synthesizer` adapter (still unbuilt): it can be a
general-purpose "what's happened recently" summarizer rather than one required to preserve
per-entity boundaries.

## 7. Open questions this proposal raises (for Daniel)

1. **Decision format, now scoped per project.** Project decisions moved from a single global log
   into each project's own "Decisions" subsection (section 4), but the format question doesn't
   disappear: the four-letter permanent code (this repo) solves cross-branch collision safety via
   `decision available <CODE>` tooling; Maestro's `D-NNN` solves stale-decision visibility via a
   `Review by` field, an explicit anti-anchor rule, and flipping the old entry's `Status` on
   supersession (this repo's format never edits a superseded entry — the pointer lives only in the
   new entry's `Supersedes` field). Proposal: keep the four-letter permanent code **and** add a
   `Review by` field plus the superseded-entry status flip, applied within each project's Decisions
   subsection rather than a single cross-project log.
2. **`concepts/` (a.k.a. `reference/`) — shared org-wide or per-user?** Both of us keep this folder
   today as personal. `Q-012` (knowledge governance) asks this at the org level; this proposal
   doesn't resolve it, just flags that a shared "concepts" tier is probably the first candidate if
   BCG Brasil ever wants shared knowledge across the pilot cohort.
3. **Does this fully answer `Q-013`?** This proposal answers "what is a workspace" narrowly — a
   single scope covering all of a user's professional work, used only for general continuity, never
   as an authority on any one entity (that's the atlas's job). It doesn't address whether
   non-professional domains (Kowalski's personal/financial/investments) exist in the merged system
   at all — per decision `WORK`, this OS is professional-only, so they may simply be out of scope
   here by design, not an open gap.

## 8. Explicitly not decided by this proposal

Bundle packaging mechanics, installer/update mechanics, the synthesis provider and its prompt
contract, exact retention windows and per-layer context budgets, and the concrete agent-to-segment
ownership roster (section 3.2 fixes the principle, not the roster). These stay governed by
`specs/006`, the pending adapter work, and the agent/skill merge that follows this proposal.
