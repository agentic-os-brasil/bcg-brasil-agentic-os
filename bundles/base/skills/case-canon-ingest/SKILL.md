---
name: case-canon-ingest
description: Compile a reviewed document, interview synthesis, hypothesis, framework or benchmark into the admitted case canon through reviewed workspace operations. Use when distilled insights need to survive across sessions; save only when requested.
---

# Case Canon Ingest

Resolve [interaction-profile](../interaction-profile/SKILL.md) for presentation.
Use the intended Maestro connector and its current schemas. Read the packaged
[authored-page recipe](../maestro-operator/resources/authored-brain.md), available
as `maestro://methods/maestro-operator/resources/authored-brain.md` through
`methods.read`, before persistence. It owns review, bounded reads and recovery.

## Bind the case and source

1. Query `case.setup.status` and verify the intended workspace and current
   client binding. Use the returned `setup` and `setup.input` account/case IDs;
   continue only when that exact case is admitted for reads and writes. An
   absent, incomplete, conflicting or reconciliation-required setup goes to
   [case-agent-setup](../case-agent-setup/SKILL.md). Preserve the draft meanwhile.
   Never read selection files, infer a case from session history or hand-write
   a marker. Internal project notes use their separate workspace scope, not an
   invented client canon.
2. Name only the authorized source and destination. Canon pages use the bound
   case prefix `accounts/<account-id>/cases/<case-id>/canon/` relative to its
   workspace brain. Agree `<type>-<kebab-title>.md`; the bound IDs, not a title,
   supply the prefix. Query `brain.status` before mutations and use `brain.read`
   to inspect that named page. A typed missing page is absence; another refusal
   is not permission to create or overwrite it.
3. For existing named workspace sources use `knowledge.collect` with `pages`
   and a declared `purpose`, or complete revision-consistent `brain.read`.
   Collection is bounded to 24 pages/128 KiB; a missing/omitted or truncated
   source is a coverage limitation. For a current converted artifact, read its
   returned `artifact_id` through `content.read` and retain the actual source
   revision/hash and conversion provenance. Conversion itself belongs to
   [ingest-content](../ingest-content/SKILL.md); unavailable conversion leaves
   that source pending. A supplied message/interview synthesis is an authored
   source, never a fabricated conversion artifact.

Keep raw documents at their authorized source. Extract distilled evidence,
separate inference and uncertainty, and keep every source inside this case.
Structural choices belong to
[case-decision-log-entry](../case-decision-log-entry/SKILL.md).

## Distill one artifact

Agree type, title and source reference. Keep the relevant content:

| Type | Required substance |
| --- | --- |
| `hypothesis` | Statement, sourced support, counter-evidence, assumptions and exploratory/supported/rejected assessment |
| `interview` | Confirmed participant identity/role, date if known, themes, up to three literal source quotes, constraints and open questions |
| `data` | Dataset/version, metrics, source locations, validity window and gaps |
| `framework` | Origin, core logic, case application and supported recommendation |
| `benchmark` | Source, comparable numbers, applicability, URL/retrieval date when known |

Preserve unknown dates, identities and versions as unknown. Quotes must be
present in the authorized source. The evidence assessment belongs in the body;
frontmatter `status` uses the authored-page vocabulary.

For a new page, show the complete body with these eight fields and real source
metadata before Save:

```markdown
---
id: accounts/<account-id>/cases/<case-id>/canon/<type>-<kebab-title>
title: "<Title>"
summary: "<What this evidence establishes and its limits>"
type: <hypothesis|interview|data|framework|benchmark>
scope: account/<account-id>/case/<case-id>
status: active
sensitivity: client-confidential
updated: <content date>
source: "<authorized source reference and known version>"
---

# <Title>

<Distilled content>

## Evidence

<Source locations, actual revisions/hashes when returned, coverage and uncertainty>

## Updates
```

## Save through the owning operation

- **Authored synthesis:** use `brain.create(page,text)` for a new page. For an
  existing page preserve history with `brain.append` under an existing heading,
  carrying `expected_page_revision` from the complete current read. A requested
  full replacement uses `brain.update` with that current revision and complete
  body; preserve unrelated prose and optional metadata.
- **Converted-source synthesis:** when an actual current conversion artifact
  supplies the evidence, use `knowledge.create(artifact_id,page,text)` or
  `knowledge.append` with that ID, an existing `section` and current
  `expected_page_revision`. The owner rechecks the source and binds its
  conversion provenance. A source pointer alone does not create an artifact ID
  or authorize conversion.
- **Review and verify:** save only on the owner's persistence request and the
  operation's exact native review. The server supplies provenance; arguments
  never carry consent, roots, actor IDs or grants. Confirm the actual result and
  read the complete named page back with `brain.read`, retaining one revision
  across all pages. `proposed` is pending, not saved.
- **Retry:** reuse the exact original operation, text, source ID and expected
  revision after an interrupted response. Inspect returned state/readback before
  numbering or appending again. A changed source/page/review requires a fresh
  read and reviewed proposal; preserve newer work. Declined/unavailable review
  leaves the draft pending, with no filesystem or CLI fallback.

Return the saved page, revision, source coverage and next useful action. Linking
it to the brief, creating tasks or recompiling navigation requires separate
requested operations; a canon save does not perform them.
