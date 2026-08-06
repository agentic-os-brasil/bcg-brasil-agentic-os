---
type: Implementation Boundary
title: Human atlas bootstrap
description: The local human-readable atlas scaffold and its explicit limits.
resource: repo://specs/014-human-atlas-bootstrap.md
tags:
    - atlas
    - local
    - privacy
sources:
    - id: human-atlas-bootstrap
      resource: repo://specs/014-human-atlas-bootstrap.md
      title: Human atlas bootstrap
status: stable
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/human-atlas-bootstrap
x-bcgos-scope: managed
x-bcgos-source-fingerprint: c971241aee185459ed2cba432c26ec1b7e8c56198c24e52e7750ca435ca79858
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: bcgos-managed-wiki/0.2
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `specs/014-human-atlas-bootstrap.md`. The source remains authoritative.

## Related

- [Content navigation through a compiled LLM wiki](/concepts/content-navigation.md)
- [Wiki update lifecycle and OKF profile](/concepts/wiki-okf.md)

## Source content

# Spec 014 - Human atlas bootstrap

Status: initial local bootstrap and first managed OKF compiler implemented;
private OKF bundles and runtime navigation remain unavailable.

## Objective

Create a small, inspectable Markdown foundation for professional knowledge
without treating it as memory, compiling a wiki, inventing a task system or
collapsing owner and workspace scopes.

## Local roots

`bcgos atlas init <workspace-path>` requires an initialized, readable
workspace and creates non-overwriting human orientation pages in two distinct
private roots:

```text
<local BCGOS data>/atlas/owner/
  index.md
  learnings/index.md
  development/index.md
  concepts/index.md

<workspace>/brain/
  index.md
  clients/index.md
  projects/index.md
  people/index.md
  daily/index.md
```

The owner root is private user data. The workspace root is scoped to its
workspace identity. Managed atlas content ships separately in the versioned
managed OKF bundle; the command must not create a fake managed root in user data.

`owner/self/` remains the owner identity source and is not copied into the
owner atlas. `tasks/` is deliberately absent until an authoritative task-source
and synchronization contract are accepted.

## Workspace templates

The initial bootstrap distributes four non-overwriting templates within the
workspace atlas:

- `clients/template-client.md` records only workspace-authorized client
  context, stakeholders, sensitivity and source freshness.
- `projects/template-project.md` separates objective, current truth, workplan,
  durable decisions, risks and artifacts. Critical facts require a source and
  generated memory is never an authority.
- `people/template-person.md` is restricted to necessary professional context,
  source and sensitivity; it is not a behavioural dossier.
- `daily/template-daily.md` keeps a human work log and explicitly states that
  selected daily signals may join Claude/Codex session signals in L1 only after
  approved sanitization and provenance capture.

These templates are canonical bootstrap assets, not examples copied from a
contributor's private brain. A user may edit them after creation; repeated
initialization preserves those edits.

## Boundary

This is a human navigation scaffold only. It provides no automatic writing,
cross-root links, compilation, memory ingestion, provider access, search,
task synchronization or Session Start injection. A human daily page may feed
memory only through a future approved sanitization adapter.

`bcgos atlas status <workspace-path>` reports the three root states without
reading page bodies. Repeated initialization never overwrites a user page.

## Follow-up

The private compiled atlas remains blocked on enrollment, authorization,
deletion propagation, provider and OKF lifecycle contracts from Specs 007 and
008. Templates for individual entities and agent/ritual writers require
separate decisions and tests.
