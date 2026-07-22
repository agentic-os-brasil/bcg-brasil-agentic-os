# Spec 010 - Local ingestion runtime

Status: architecture accepted; distribution pack, adapters and executable command pending.

## Objective

Make professional-document ingestion work for a non-technical pilot user without
requiring an API key, a Python installation, a package manager or a manual model
setup.

## Default route

Docling is the default local extraction substrate for every supported ingestion
intent. It converts a source into a structured representation plus derived
Markdown and records provenance needed to inspect the result later.

```text
user source
  -> policy and size check
  -> managed local Docling runtime
  -> structured document + Markdown + metadata-safe receipt
  -> validation and quality classification
  -> downstream skill, memory or wiki route
```

The original source remains where the user placed it. BCGOS does not upload it,
commit it, or copy it into a managed bundle. Derived private artifacts, when
retention is permitted, remain in user-local BCGOS storage under the enrolled
owner and workspace scope.

## Fallbacks

Docling is the first attempt, not an excuse for silent degradation:

1. Use Docling locally for an eligible source.
2. Validate the declared input type, generated structure and output budget.
3. When Docling is unavailable, does not support the source, or produces an
   invalid result, try a format-specific deterministic extractor only when one
   is approved for that format.
4. Report the route, fallback reason, fidelity classification and retained
   artifacts. Never invoke a remote model or service as an implicit fallback.

An extraction failure changes no memory, wiki or shared knowledge state.

## Managed ingestion runtime pack

The product bundle remains a thin Go CLI. Docling belongs to a separately
versioned, per-platform ingestion runtime pack managed by `bcgos`:

- a pinned Python runtime and Docling dependency set;
- a compatible CPU-first PyTorch distribution and approved local models;
- a verified manifest, checksums, provenance and compatibility range;
- preflight checks for operating-system support, free disk, writable local
  application storage and required corporate-network access;
- model prefetch at installation or first explicit ingestion, with a visible
  progress and retry path;
- local artifact cache outside workspaces and outside synchronized roots.

`bcgos` asks for confirmation before downloading or activating the pack. The
normal user never runs `pip`, chooses a model cache path or supplies a key.
The pack may be installed on demand rather than included in the minimal CLI
installer; that keeps initial installation small while preserving a one-command
ingestion experience.

The pilot must validate the pack on Windows and macOS before claiming parity.
Intel macOS, CPU-only performance, disk size, first-use downloads and corporate
proxy behavior are explicit acceptance cases.

## Progressive user profiles

Profiles control progressive disclosure, not authorization. They never bypass
BCG policy, data boundaries, release verification or credential requirements.

| Profile | Default experience | Additional suggestions |
|---|---|---|
| `standard` | Local Docling, automatic approved fallback, concise quality result and no key. | None by default. |
| `advanced` | Same safe default. | OCR engine choice, batch processing, extraction templates, intermediate exports and quality diagnostics. |
| `power` | Same safe default. | Explicit local-model, VLM, remote-provider and custom-pipeline proposals, subject to policy and preflight. |

The user may self-declare a profile through a local preference. An advanced or
power profile does not automatically enable a remote service. Any remote route
requires an explicit user action, an approved provider policy and credentials
stored in the operating-system credential store.

## Runtime-neutral contract

Claude and Codex adapters route an ingestion intent to the same installed
runtime contract and report one of `supported`, `unavailable`, `blocked` or
`degraded`. They do not shell out to ad-hoc Python environments or accept a
credential in chat. The Agentic OS owns policy, provenance, storage boundaries
and capability reporting; Docling owns document conversion.

## Delivery boundary

This specification does not install Docling, download models, package Python,
or send a document to any service. The next engineering step is a controlled
distribution spike that produces and validates a signed Windows/macOS runtime
pack and measures installation size, first-use time, offline behavior and
extraction quality on sanitized fixtures.

## References

- [Docling installation](https://docling-project.github.io/docling/getting_started/installation/)
- [Docling offline models and artifact paths](https://docling-project.github.io/docling/usage/advanced_options/)
- [Docling supported formats and local execution](https://github.com/docling-project/docling)
