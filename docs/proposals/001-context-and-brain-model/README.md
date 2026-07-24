# Proposal 001 — Context and Brain Model

**Status:** the central idea is embedded in the Agentic OS architecture; implementation is incremental.

**Original contribution:** Marcelo Petrof Sanches.

**Architecture reconciliation:** BCG Brasil Agentic OS maintainers.

## The central thesis

The Agentic OS needs two complementary information surfaces, with different jobs and different cost profiles:

```text
Compact continuity for the agent
small, traceable summaries of recent and durable work
selected at session start, within a limited context budget

Human-readable work corpus (for people)
Markdown pages · timelines · project maps · decisions · links
rich · navigable · potentially extensive · never injected in full
```

This distinction embeds the core contribution from the proposal: a useful Second Brain cannot be only an opaque runtime memory, and a rich human knowledge base should not be treated as an unlimited prompt. The corpus is a source surface. The scoped Atlas/Wiki is a separate, derived navigation view over eligible sources; it is not another editable authority.

## What each layer does

### Compact continuity for the agent

The agent needs a small amount of reliable continuity when a new session begins. It therefore keeps compact, traceable summaries of what happened recently and of what remains important over time. A periodic consolidation routine reduces many selected signals into successively more durable summaries, so the agent does not need to read every file or every conversation again.

The first of these summaries is **not** a copy of a day or a transcript. It is made from selected, sanitized signals from both:

- the human daily log; and
- Claude/Codex conversation records.

Each retained signal records where it came from. Raw sensitive content stays at its original source unless the owner deliberately promotes it.

### Human-readable work corpus

The human-readable work corpus is the readable work surface. It is Markdown-first and can be opened directly in an editor, a file explorer, or a Markdown reader without an agent. It holds richer project context, timelines, decisions, source links and working notes.

A corpus page may inform memory, but only through explicit selection, sanitation, provenance and rollup. Conversely, a durable memory item should point back to its human-readable source when one exists.

## Scope and privacy model

The corpus is not one undifferentiated `brain/` folder. It has three scopes:

| Scope | Purpose | Typical contents |
| --- | --- | --- |
| Managed | Organization-maintained, broadly reusable material | BCG methods, approved playbooks, shared skills |
| Owner-private | The individual owner's durable context | `owner/self`, preferences, personal professional learnings |
| Workspace-private | Context for a specific local workspace or engagement | clients, projects, people, daily notes, development artifacts |

`owner/self` is deliberately distinct from a generic `profile/` folder: it contains the owner context used to shape collaboration, while still being private and reviewable.

## Initial human navigation model

Within the appropriate scope, the initial corpus can navigate these areas:

```text
owner/
└── self/

brain/
├── clients/
├── projects/
├── people/
├── learnings/
├── concepts/
├── daily/
└── development/
```

Each section has a human index (a mother file / map of content) and simple templates. These indexes optimize orientation for people. The scoped Atlas/Wiki and its derived index remain navigation layers; they are not competing sources of truth.

Tasks are intentionally absent from this taxonomy. The task system remains authoritative, while the Atlas may contain explicit pointers from a client or project page to its relevant task view.

Neither the corpus nor the derived Atlas/Wiki replaces an authoritative task system, a project or decision record, or the canonical-memory contract.

Managed navigation can be implemented under the current navigation specifications. Owner-private and workspace-private corpus and Atlas/Wiki surfaces remain an architectural direction until enrollment, approved local storage, authorization and deletion-propagation contracts are implemented and tested.

## Working principles

1. **Markdown-first and locally readable.** The user owns and can navigate their sources.
2. **Top-down retrieval.** The agent starts with an oriented index, then opens the relevant page, then reads only the detail needed.
3. **No blind ingestion.** Rich pages and raw logs do not enter injected memory wholesale.
4. **Sensitivity by default.** People, client and daily content are scoped and retained deliberately.
5. **Low-friction consistency.** Templates guide users; they do not impose paperwork for its own sake.
6. **Progressive implementation.** The proposal gives direction; each runtime behavior is delivered through a spec, decision log, tests and validation harness.

## What this proposal does not decide

- the task-management product or its data model;
- a search engine or an Obsidian dependency;
- automatic promotion of raw content to memory;
- cross-workspace sharing of sensitive client or people information;
- the exact contents of future client, project, person and daily templates.

Those decisions are made through the normal Agentic OS development harness as the bootstrap, ingestion, memory and navigation capabilities are implemented.

## Giveback to the original proposal

The proposal's most important idea is now embedded in the architecture: people need a human-readable work corpus they can read and trust, in parallel with the compact continuity that makes an agent useful. The reconciliation adds the boundaries necessary for a BCG-wide product: scope, privacy, traceability of source, owner self-context, and a clear separation between source corpus and derived Atlas/Wiki navigation.
