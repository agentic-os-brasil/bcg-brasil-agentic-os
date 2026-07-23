---
name: ingest-content
description: Ingest one professional document or supported source through the BCG Brasil Agentic OS local extraction contract. Use for PDFs, Office files, webpages, images, emails and other work material that should become governed local knowledge.
---

# Ingest Content

Use the installed runtime adapter. Do not manually parse a document, create a
Python environment, request an API key or send source content to a remote
provider as a fallback.

## Workflow

1. Resolve the active owner, workspace and local ingestion capability through
   the installed runtime adapter.
2. Check source scope, size, type and retention policy before reading content.
3. Use the managed local Docling runtime as the primary extractor.
4. Validate the structured result, Markdown rendering, provenance and output
   budget before routing any derivative downstream.
5. If Docling is unavailable or the result is invalid, use only an approved
   deterministic format-specific fallback. Report why the fallback occurred.
6. Return the extraction route, fidelity classification, provenance pointer,
   retained-artifact policy and any capability limitation.

## User profiles

- `standard`: preserve the local, no-key default and show only the concise
  result.
- `advanced`: offer approved diagnostics, OCR choice, batches, templates and
  intermediate exports when useful.
- `power`: propose explicit provider or model alternatives only after policy,
  consent and credential preflight.

Profiles control suggestions, not permissions. They never authorize a remote
provider, weaken data boundaries or bypass release verification.

## Invariants

- The original source remains in its user-chosen location.
- Raw source content, credentials and client material never enter the managed
  bundle, Git history or a shared atlas.
- A failed extraction changes no memory or wiki state.
- A remote provider is never an implicit fallback.
- If the local runtime pack is unavailable, report `unavailable` and explain
  the next safe installation action.

## Current delivery boundary

This skill defines the product route. The managed Docling runtime pack and the
`bcgos ingest` command are not installed yet, so the runtime must report the
capability as unavailable rather than emulate ingestion.
