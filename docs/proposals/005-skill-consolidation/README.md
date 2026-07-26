# Proposal 005 — Skill Consolidation

Status: draft (not yet submitted for review)
Author: Marcelo Petrof Sanches, with Maestro (Claude)
Date: 2026-07-25
Related: Proposal 004 (`../004-spoke-agent-roster/`) — the agents these skills route to;
Anthropic's [Agent Skills best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)

## What this proposal does

Proposes a set of **19 skills** in the canonical `SKILL.md` format (folder per skill,
`name` + `description` frontmatter, Markdown body), each wired to the agents from
Proposal 004.

Agent references below use Proposal 004's names, which map onto Spec 018's role graph
(most are `capability_specialist` instances under a thin `workspace_agent`/`account_agent`;
the reviewers are `walter`). Where a skill says *executed by* a keeper (`work-logger`,
`career-keeper`), the keeper prepares the update and its owning agent commits the
persistent write — the skill contract is unchanged either way.

Two principles define the set:
- **The line between an agent and a skill.** An **agent is a *who*** (an actor with a
  role, tools, and isolated context, that runs once and reports). A **skill is a *how***
  (a reusable procedure loaded into an actor's context when triggered). A capability is a
  skill only when it is (1) **shared** across the hub and/or 2+ agents, (2) a multi-agent
  **orchestration** ritual, or (3) a **heavy, independently-versioned method**. Anything
  used by exactly one agent, simple, and intrinsic to its identity lives *inside* that
  agent — not as a skill.
- **Compose, don't duplicate.** An orchestration skill *calls* atomic skills/agents; it
  never re-implements their logic. There is one source of truth per capability.

## Two kinds of skill

- **Atomic** — one reusable capability or method. (`grill-me`, `wayfinder`, `investigate`,
  `storyline`, `diagram`, `make-pdf`, `handoff`, `decision-log`, `record-learning`,
  `record-concept`, `task`, `schedule`)
- **Orchestration** — sequences atomic skills + agents in a determined flow, then
  synthesizes. (`start-day`, `eod`, `newcase`, `meeting-to-work-items`, `consolidate`,
  `retro`, `setup`)

## The 19 skills

### Atomic
- [`grill-me`](skills/grill-me/SKILL.md) — relentless interrogation to sharpen a plan/analysis/message. *Used by:* hub directly + `walter` in its pre-work review mode (which pulls it almost always).
- [`wayfinder`](skills/wayfinder/SKILL.md) — decompose a fuzzy problem into a MECE issue-tree. *Used by:* hub + `quali-analyst` + `work-planner` (the shared structuring method those three pull rather than each re-describing).
- [`investigate`](skills/investigate/SKILL.md) — systematic root-cause investigation of a broken/surprising output. *Used by:* hub + `quant-analyst`.
- [`storyline`](skills/storyline/SKILL.md) — build a cohesive narrative arc (pyramid principle, SCQA, simplicity-first). *Used by:* `deck-builder` (pulls it) + hub.
- [`diagram`](skills/diagram/SKILL.md) — turn an English description into a diagram (renders to SVG). *Used by:* hub + any agent.
- [`make-pdf`](skills/make-pdf/SKILL.md) — turn Markdown into a publication-quality document. *Used by:* hub + any agent.
- [`handoff`](skills/handoff/SKILL.md) — compact the current session into a structured resume. *Used by:* hub (operates on its own context).
- [`decision-log`](skills/decision-log/SKILL.md) — record a durable decision (four-letter code, review date, supersede rules) into the project's Decisions subsection. *Executed by:* `work-logger`. *Called by:* `eod`, `meeting-to-work-items`.
- [`record-learning`](skills/record-learning/SKILL.md) — record a durable, career-spanning learning into `learnings/`. *Executed by:* `career-keeper`. *Called by:* `retro`; also invocable the moment an insight surfaces.
- [`record-concept`](skills/record-concept/SKILL.md) — record a reusable method/framework/playbook into `concepts/`. *Executed by:* `career-keeper`. *Surfaced by:* hub or an analyst that discovers a reusable method.
- [`task`](skills/task/SKILL.md) — quick CRUD on the external task tool + backlog mirror. *Executed by:* `work-logger`. *Called by:* `meeting-to-work-items`, `eod`.
- [`schedule`](skills/schedule/SKILL.md) — calendar scheduling helper (conflicts, slots, a prefilled event). *Executed by:* `work-logger`.

### Orchestration
- [`start-day`](skills/start-day/SKILL.md) — start/resume the workday with a scoped briefing. *Calls:* `briefing-analyst` + `work-planner`; writes via `work-logger`.
- [`eod`](skills/eod/SKILL.md) — close the day: finalize the log, reconcile decisions/current-truth/tasks, preview tomorrow. *Calls:* `work-logger`, `decision-log`, `task`; on Fridays offers `retro` + `consolidate`.
- [`newcase`](skills/newcase/SKILL.md) — onboard a new case from a proposal deck. *Calls:* `case-onboarder`.
- [`meeting-to-work-items`](skills/meeting-to-work-items/SKILL.md) — turn a transcript into decisions, actions, and follow-ups, filed across the atlas. *Calls:* `briefing-analyst`, `task`, `decision-log`, `client-keeper`, `work-logger`.
- [`consolidate`](skills/consolidate/SKILL.md) — memory-hygiene pass over the atlas. *Calls:* the keeper agents (`work-logger`, `client-keeper`, `people-keeper`, `career-keeper`) to apply confirmed changes.
- [`retro`](skills/retro/SKILL.md) — weekly retro against development objectives. *Calls:* `career-keeper` (DIGEST then WRITE) and `record-learning`; the hub holds the live conversation between.
- [`setup`](skills/setup/SKILL.md) — first-run onboarding of a new user. *Calls:* `newcase` and the keeper agents to seed the atlas.

## Authoring conventions (from Anthropic's best practices)

- **Frontmatter is `name` + `description` only.** `description` is the router: third
  person, states *what it does and when to use it*, with key trigger terms. Relations
  (owning agent, what it calls) live in the body, not invented frontmatter.
- **Action-oriented names** (`wayfinder`, `grill-me`, `eod`…) — consistent with this
  repo's existing skill names (`start-contributing`, `record-decision`, `dream-memory`),
  not force-fit to gerunds.
- **Degrees of freedom matched to the task:** tool-wrappers (`diagram`, `make-pdf`) run an
  exact script (low freedom); orchestration skills give clear steps (medium); interactive
  skills (`grill-me`) give direction and trust judgment (high).
- **Concise, single `SKILL.md` each** (all well under the 500-line guidance); consistent
  terminology; no time-sensitive content.

## Explicitly not decided by this proposal

- Whether these ship in the managed bundle vs. remain development skills — that's a
  packaging decision governed elsewhere.
- Exact MCP connector names for the task-management and calendar integrations (written as
  `MCP (task-management connector)` / `MCP (calendar connector)`).
- Evaluations. Best practice is to build evals before extensive docs; for a design
  proposal we defer eval authoring to implementation, per skill.
- Any change to the Hub's own constitution (its `CLAUDE.md`) — a separate follow-up, once
  agents and skills are both settled.

## Development handoff

The promotion boundary and the first safe implementation slice are defined in
[`PROMOTION-CONTRACT.md`](PROMOTION-CONTRACT.md). The 19 draft skills remain proposal
artifacts until that contract is satisfied; `wayfinder` is the suggested first candidate
because it can be evaluated without external authority or mutation.
