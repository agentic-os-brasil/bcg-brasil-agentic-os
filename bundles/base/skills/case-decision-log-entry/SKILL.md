---
name: case-decision-log-entry
description: Register a confirmed structural case choice through the admitted workspace decision log. Use for "register this decision", "we decided" or a methodology, architecture, stakeholder or scope choice; preserve literal statements and save only when requested.
---

# Case Decision Log Entry

Resolve [interaction-profile](../interaction-profile/SKILL.md) for presentation.
Read the packaged [authored-page recipe](../maestro-operator/resources/authored-brain.md)
(`maestro://methods/maestro-operator/resources/authored-brain.md`) before saving.
Use the intended Maestro connector and its current closed schemas.

## Bind and inspect

1. Query `case.setup.status` for the current workspace and exact client
   binding. Use the returned `setup`/`setup.input` IDs only when that case is
   admitted. An absent, incomplete, conflicting or reconciliation-required
   binding needs [case-agent-setup](../case-agent-setup/SKILL.md); preserve the
   draft. Selection files, session history and prompt fragments supply no
   binding authority.
2. The named brain-relative log is
   `accounts/<account-id>/cases/<case-id>/decisions/decision-log.md` under that
   binding. Query `brain.status` for mutation availability and `brain.read` for
   this exact page/purpose. Read every page through `next_offset: -1`, at one
   unchanged revision. Only a typed missing-page result permits new creation.
3. From the complete log, find the highest `D-NNN` heading and propose the next
   number (D-001 when absent). Record the current revision with the proposal.
   A restart first inspects the saved log; it never allocates a second number
   merely because the prior response was interrupted.

## Draft one decision

| Field | Rule |
| --- | --- |
| ID | Next D-NNN, zero-padded to at least three digits |
| Date | Confirmed decision date, otherwise current date explicitly identified as recording date |
| Type | `methodology`, `architecture`, `stakeholder`, `scope` or `pending`; clarify an uncertain type |
| Title | Imperative phrase under 50 characters, confirmed if shortened |
| Context | Why the choice arose, with authorized evidence pointers |
| Decision | What was decided; preserve any literal owner quote verbatim |
| Rationale | Why that option was selected; separate inference from stated reasons |
| Linked | Only specs, PRs, documents or source revisions actually supplied/read |
| Review target | Recording date +7 days, or +14 for methodology/architecture, unless explicitly changed |

Show the complete entry before persistence:

```markdown
### D-NNN · YYYY-MM-DD · <type> · <title>
- **Context:** ...
- **Decision:** ...
- **Rationale:** ...
- **Linked:** ...
- **Review target:** YYYY-MM-DD
```

A missing type/title needs clarification; never invent a decision to fill the
log. A conversational draft remains nonpersisted until Save is requested.

## Persist and prove the entry

For an absent log, use `brain.create` with the complete body: eight-field
frontmatter (`id` equal to the log path without `.md`, title matching H1,
summary, `type: decision-log`, exact `account/<id>/case/<id>` scope,
`status: active`, `sensitivity: client-confidential`, content `updated` date),
then H1 and an existing `## Decisions` section containing the entry.

For an existing log, use `brain.append(page,section,text,expected_page_revision)`
under its actual decision heading at the complete read's revision. Match the
entry heading depth to that section. Preserve existing metadata, entries and
manual notes; if no suitable heading exists, propose a complete
`brain.update` with the current revision and get a fresh review.

Let the host perform the exact native review of current page, entry, scope and
revision. An earlier chat confirmation does not supply tool consent. The owning
operation supplies attended provenance; never pass roots, grants or actor IDs.
Read the complete log back and report the actual page/revision and D-NNN only
when saved. `proposed`, unavailable review and declined review stay pending.

After interruption, retry the exact original append/create and expected revision,
not a freshly numbered entry. Inspect readback before concluding it is absent.
On revision conflict preserve newer content, recompute the candidate from a
fresh complete read and obtain a new review. No direct files, shell append,
marker repair or automatic overwrite is a recovery route. A save neither
schedules the review target nor publishes navigation or another case's context.
