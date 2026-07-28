# Spec 031 - MarkItDown local ingestion adapter

Status: accepted decision; contract and adapter implementation are being
delivered incrementally behind Spec 010.

## Objective

Extend the local-first ingestion route with a bounded deterministic converter
for common office and text formats that are not the primary Docling path.

## Role of MarkItDown

Microsoft MarkItDown is an adapter, not the ingestion authority. Docling stays
the default extraction substrate for supported document intents. MarkItDown is
selected only when the product policy explicitly permits the format and the
primary route is unavailable or does not cover that source.

The adapter uses only MarkItDown's built-in offline conversion path. Plugins,
URLs, YouTube, Azure Document Intelligence, Azure Content Understanding and
other remote routes are unavailable in the initial contract.

## Request contract

An ingestion request must provide:

- an explicit local regular-file source;
- an initialized workspace scope;
- a format allowlist and input/output byte limits;
- a provider route selected by the core, not by arbitrary source content.

Symlinks, directories, network locations, plugins and implicit remote fetches
are rejected. The source remains where the user selected it; derived Markdown
and metadata-safe receipts are written to user-local application storage under
the workspace scope.

## Result contract

The adapter emits Markdown plus a JSON result conforming to
`schemas/ingestion-result.schema.json`. The result contains the source
basename, SHA-256 fingerprint, format, route, workspace identity, output size,
status, fidelity and warnings. It never includes source bodies, prompts,
provider responses or absolute paths.

The core reports `unavailable`, `blocked` or `degraded` explicitly. A provider
failure must not update memory, the atlas, the wiki or shared knowledge. The
core owns a Docling-first route selector; MarkItDown is selected only after
that selector observes `unavailable`, `unsupported` or `degraded` primary
state.

## Runtime-pack boundary

MarkItDown and its required format extras belong to the separately versioned
ingestion runtime pack. The pack must pin versions, verify hashes, carry
managed-installer provenance and expose a single local executable contract to
the Go CLI. The CLI must not execute a pack without an approved verifier
supplied by the managed installer; the current source-only slice therefore
remains `unavailable` until that signed installation path exists. It must not
shell out to a contributor's ad-hoc Python environment, accept credentials in
chat or enable network access as a fallback.

## Initial acceptance evidence

The first implementation must prove:

1. allowlisted local DOCX/XLSX/HTML/text fixtures convert offline;
2. ZIP, URLs, plugins, symlinks and oversized files fail closed;
3. malformed or low-signal output is classified as `partial` or `degraded`;
4. the same request/result contract is observable from Claude and Codex
   adapters once those product adapters are wired;
5. sanitized Windows and macOS fixtures produce bounded artifacts without
   copying source content into the managed bundle.
