# Spec 010 - Local ingestion runtime

Status: authenticated DOCX runtime source implemented; native execution, installed
client and distribution qualification remain separate gates.

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

- a pinned Python runtime and Docling dependency set; the current SimplePipeline closure is model-free;
- CPU-first PyTorch and approved local models only for a separately approved future model-backed profile;
- a verified manifest, checksums, provenance and compatibility range;
- preflight checks for operating-system support, free disk, writable local
  application storage and required corporate-network access;
- model prefetch only in that separately approved future profile, with a visible progress/retry path; the current profile fetches no models;
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

The source supports target-specific construction for Darwin arm64 and Windows
amd64. Darwin preserves its exact 51-wheel closure and bridge. Windows has a
separate 52-wheel/CPython closure, held owner/DACL/no-reparse inventory verification
and native ordinary-user zero-capability LPAC execution. The launcher opts out
of ALL_APPLICATION_PACKAGES and requires access mask 6 on the actual suspended
child before resume. Its capability array stays empty and TokenCapabilities must
report count zero. The redirected TEMP tree is created under execution-local
LOCALAPPDATA before inventory custody and SID grants. Child creation
is prohibited; a suspended child joins a timeout-controlled kill-on-close Job
before execution. Only staged authenticated bytes and private scratch receive
the per-conversion SID; original documents and installed-pack ACLs stay intact.

Windows readiness requires actual explicit authenticated placement qualification:
real synthetic SimplePipeline conversion and native network/filesystem/child
negative controls precede a private current-user/machine DPAPI proof bound to
signed pack bytes, production implementation/dependency identity and hashed local
kernel/owner/install identity. Status only verifies that proof and current bytes;
missing/stale proof reports requalification_required without side effects.
Profile create/remove and smoke execution belong only to explicit setup/apply or
conversion, never queries. No admin/service/firewall/global ACL change is required.

This source composition is unqualified. Schema 3 DPAPI qualification identity
invalidates ordinary-AppContainer and historical registryRead trial receipts,
while retaining exact pack, manifest, implementation, host and source bindings.
The pinned Windows Documents installation defers pydantic-settings' genuine
asyncio import to its existing coroutine CLI branch; the upstream wheel stays
immutable and exact source/RECORD hashes plus installed provenance are required
at pack admission. This change does not establish zero-capability compatibility.
The independent Go child's eight network probes still require WSAEACCES; a
WSAStartup 10107 observation is a failed candidate, never an accepted denial.
The existing manual workflow is proof-only for this candidate. Export, assembly,
signing and distribution remain unavailable pending separately authorized exact
native proof and review. Historical R17 receipts cannot qualify it.

Source tests, a cross-build or an assembled pack do not qualify native Windows.
The closed ordinary-account driver and manual-only native workflow must actually
pass real conversion and denial controls before native readiness claims. Corporate
signing, publisher authority, installed client loading and device acceptance are
separate gates.
Read the [authenticated runtime guide](../docs/product-offline-runtime.md) when
assembling a pack, diagnosing authority/availability or qualifying conversion.
The owning code is `internal/ingest/docling`; `cmd/maestro-docling-pack` constructs
an unsigned artifact and the native MCP constructor supplies trust. The later
reviewed-document route and modern signed installer remain separate capabilities.
Real converter fixtures, native installation/client acceptance and corporate
release authority must be observed before claiming the broader platform promise.

## References

- [Docling installation](https://docling-project.github.io/docling/getting_started/installation/)
- [Docling offline models and artifact paths](https://docling-project.github.io/docling/usage/advanced_options/)
- [Docling supported formats and local execution](https://github.com/docling-project/docling)

## NTXT model-free document extension

Spec061 and decision NTXT admit encoded-text PDF and modern PPTX through the
separate authenticated `model-free-documents-v1` closure. NativePdfPipeline uses
PyPdfium explicitly, with rendering/model/OCR/remote paths disabled; the result
is textual fidelity with page/bounding-box provenance. This does not authorize
full visual PDF parsing, model downloads, OCR or implicit MarkItDown fallback.
The legacy eleven-format profile and its archive bytes remain supported exactly.
Per-format status follows the verified installed profile, not the global policy
allowlist. Read Spec061 for the closed refusal and acceptance boundaries.
