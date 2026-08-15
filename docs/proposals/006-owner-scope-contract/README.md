# Proposal 006 — Owner atlas scope contract

**Status:** accepted architecture; implementation pending. Defines the
owner-atlas boundary and activates no runtime capability by itself.

**Original contribution:** Marcelo Petrof Sanches. Refined against the current
advisory-runtime and owner-sovereignty contracts.

**Depends on:** Spec 013 (Owner Context) and Spec 014 (Human atlas bootstrap).

## Executive summary

The owner needs one private, cross-project place for professional trajectory,
reflections, methods, development and selected learnings. Spec 014 created the
physical `atlas/owner/` root but deliberately did not authorize agent or ritual
writers. This proposal defines that missing scope.

The owner atlas is not another SELF database and is not a client workspace. The
owner controls its content, may edit the Markdown directly and may authorize
Maestro to help maintain it. Templates and named operations improve reliability;
they do not limit what the owner may privately record.

The generative runtime decides what is relevant, proposes synthesis and may
consult registered agents. The mechanical core protects only the boundaries that
must remain deterministic: path isolation, atomic writes, provenance,
revocation, non-overwrite and prevention of unintended cross-scope leakage.

## Relationship to the existing authorities

The three local layers remain distinct:

| Layer | Authority | Typical content |
| --- | --- | --- |
| `owner/self/` | Spec 013 Owner Context | identity, preferences, voice, motivations, decision rules and boundaries |
| `atlas/owner/` | this proposal over Spec 014 | professional trajectory, retrospectives, methods, development, concepts and owner-authored cross-project learnings |
| `<workspace>/brain/` | the active workspace | client, account, case, project, stakeholder and deliverable context |

When content overlaps, `owner/self/` remains the only authority for the owner's
current identity and working profile. An atlas page may link to a SELF facet or
record the history behind it, but may not silently override or promote a facet.
Likewise, an owner reflection about an engagement does not become an authority
for the client or case.

## Owner sovereignty

The owner owns the second brain and may read, create, edit, organize, export,
archive, redact or delete content in `atlas/owner/` directly. In particular:

- free-form Markdown is allowed;
- segment templates are optional orientation and interoperability aids, not an
  admission gate;
- a missing adapter never blocks direct human use, conversation or retrieval of
  content the owner explicitly asks to use;
- the system may advise on sensitivity, minimization, retention and audience,
  but does not replace the owner's judgment about lawful private notes;
- permanent destructive or external effects still require a clear owner choice.

Managed operations must never overwrite a hand-edited page silently. They use
revision checks and return a conflict or a reviewable proposal when the source
changed underneath them.

## Content boundary

Owner scope may contain owner-authored professional material, including:

- objectives, retrospectives, development plans and self-assessment;
- feedback the owner received, with source and context when the owner chooses;
- methods, concepts, reusable heuristics and working practices;
- a daily or periodic work log;
- the owner's professional history and relationships;
- sanitized cross-project patterns and learnings authored or approved by the
  owner.

Names of colleagues, feedback sources, clients and projects remain personal or
confidential metadata even when they are not the primary subject. The system
must label and minimize them appropriately; it must not claim that attribution
eliminates third-party considerations.

Raw engagement findings, figures, deliverable bodies, credentials, stakeholder
dynamics and client-confidential source material remain in the workspace that
owns them. The owner atlas may hold a pointer, identifier or owner-authored
sanitized synthesis. It does not enumerate workspaces or copy their bodies by
default.

Cross-project continuity is an intended use of the owner atlas. Signals may
enter it through direct owner authorship, an individually approved promotion or
a previously authorized recurring ritual. Automatic workspace crawling or
silent promotion remains out of scope.

## Readers and projections

The private root is never broadcast as ambient context. Access is purpose-bound
and minimized:

1. The owner may inspect the full local source directly.
2. The owner-facing session may request a bounded projection relevant to the
   current task.
3. Maestro may receive that bounded projection to route and synthesize work.
4. Yoda may receive a stale-checked, relevance-selected projection when he is
   acting as the owner's self proxy.
5. Case, Client Account and PA Expert agents receive only an explicitly
   authorized, attenuated excerpt or pointer when it is necessary for the task;
   they never receive the owner root.

These projections add no tools, data scope or effect authority. They follow the
current `native_advisory` consultation model. The strict signed-packet backend
remains available for assurance or unattended execution, but its depth-one
shape is not a global limit on attended native reasoning.

## Writes and recurring routines

A write requires either:

- an attended owner request for that operation; or
- a standing grant created by the owner for a named ritual, segment, operation,
  cadence and retention policy.

Standing grants are inspectable, pausable, revocable and optionally expiring.
Each occurrence is idempotent and records provenance. Revoking a grant prevents
future occurrences without erasing owner-authored content.

If the reliable adapter or scheduler is unavailable, only the automation
degrades. The runtime may still discuss the work, prepare a reviewable draft and
help the owner edit local content through an available bounded native path. It
must not claim that a write or scheduled occurrence happened without evidence.

## Execution model — no new role is created

This proposal adds no agent role, edge or persistent tool grant. It uses the
current separation of responsibilities:

```text
owner request or standing grant
  -> native-advisory reasoning selects or proposes content
  -> bounded owner-atlas operation performs the local effect
  -> provenance and result are returned to the owner session
```

The command layer is the preferred transactional path for repeatable writes.
It is not an authority over what the owner may think, write or retain in the
owner's own private Markdown.

## Mechanical hard boundaries

The implementation must enforce only boundaries whose failure would create a
real mechanical or security defect:

- canonical owner root and descriptor-anchored/no-follow path resolution;
- no implicit write into workspace, account, managed or credential roots;
- atomic application, revision conflict detection and crash recovery;
- non-overwrite of hand-authored content;
- explicit confirmation for irreversible deletion or external publication;
- provenance and revocation for managed or scheduled writes.

Missing telemetry, stale optional context, absent native qualification or an
unavailable scheduler are diagnostics, not gates on ordinary owner use.

## Consequences

- Proposal 007 may define the preferred transactional operation set.
- Future segments and rituals may build on this scope without creating an
  owner-scoped agent role.
- Workspace-scoped writers require their own boundary and do not inherit owner
  grants.
- Spec 013 remains the sole Owner Context/SELF authority.
- Spec 014's non-overwriting bootstrap remains authoritative.

## Explicit non-decisions

- no role is added to, or removed from, the agent catalogue;
- no global persistence or external-system grant is created;
- no client or workspace body is automatically promoted;
- no ritual, segment, provider or schedule is activated by this document;
- no runtime capability is reported available by merging this proposal.
