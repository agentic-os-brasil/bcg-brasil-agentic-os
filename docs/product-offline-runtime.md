# Authenticated offline document runtime

**Current source capability:** assemble and execute the pinned, authenticated DOCX
pack on Apple Silicon macOS. Runtime construction, genuine converter execution,
CI, installed clients and distribution each require separate evidence. Windows,
Intel macOS and Linux conversion remain unavailable in this profile; a successful
cross-build does not change that state.

Read this guide when assembling a runtime, diagnosing its availability, or
qualifying conversion. Source policy and fallback rules remain in
[Spec010](../specs/010-local-ingestion-runtime.md).

## Prepare exact inputs and construct the pack

The maintainer supplies an already acquired directory containing the Python
archive and all 45 wheels named by `internal/ingest/docling/runtime/inputs.json`.
`assemble.go` binds the interpreter and lock digests. Assembly checks every raw
input before creating its destination, then constructs a deterministic archive,
complete file inventory and read-only installed tree. It downloads nothing and
never runs an ambient Python or package manager.

```sh
go run ./cmd/maestro-docling-pack --inputs "$MAESTRO_AUTH_INPUTS" --destination "$MAESTRO_AUTH_OUTPUT"
```

Both variables must name explicitly selected local directories, with a new output
destination. The result is unsigned. The existing release authority separately
signs the archive and release manifest; assembly does not grant trust or install
a production key.

## Admit and execute

`docling.New` receives an owner-bound DataRoot, CLI compatibility version and a
trusted authority-registry path/digest from the native constructor. Those values
are absent from model tool arguments. Existing scripts/server packet startup
resolves its fixed registry sidecar and verifies the linker-bound registry digest;
a binary without that seed leaves conversion unavailable and preserves unrelated
MCP behavior. This slice does not introduce modern installation or enrollment.

`Status` is read-only. It verifies active registry authority, signed manifest,
compatibility, archive signature/hash, the canonical interpreter/lock/input
closure and every installed regular single-link file. Missing, extra, linked,
altered or foreign inputs refuse; checking status does not install or repair them.

`Convert` requires a canonical workspace-relative DOCX source bound to the exact
registered workspace. It bounds compressed input and expanded ZIP contents,
revalidates the source and executes a fresh private snapshot. The pinned Python
runs isolated, without site packages or bytecode writes, under native network
denial. The bridge independently checks denied IPv4/IPv6 access and empty model
and cache directories. Remote services and external plugins are disabled.

Returned Markdown and structured document bytes stay private; metadata-only
provenance records their hashes and the authenticated pack. The caller decides
whether to retain reviewed knowledge later. Conversion itself changes no memory,
wiki or authored page. Its private execution directory is removed on return.
Output/diagnostic buffers, a 90-second process deadline, cancellation and the
bridge's output budget bound execution. Unsupported formats do not trigger a
silent network or MarkItDown fallback.

## Verify the capability

- Run focused assembly, authority/refusal, policy, release-manifest and scheduler
  no-follow contracts without converter assets.
- Supply an isolated official input directory through `MAESTRO_DOCLING_TEST_INPUTS`
  to run `TestAuthenticatedRealDocling`. It builds a real pack and ephemeral
  engineering authority, converts the retained DOCX marker and exact 2x2 table,
  exercises tampering/scope/budget/refusal cases, cancelled/expired requests and
  unchanged source/archive bytes. The authority is a test fixture, not production.
- Retained conversion and alias tests require their own explicit fixture and
  independent registry seed. Their default skip is missing evidence, not PASS.
- The focused `authenticated-offline-runtime.yml` CI job acquires exact official
  inputs, requires native Apple Silicon and runs the real converter acceptance
  with no skipped cases. It retains only synthetic test logs and metadata.
- Record exact source, native OS/architecture, official-input hashes, authority
  provenance, test names/outcomes and output evidence. A cancelled request before
  execution does not prove a mid-process kill or power-loss cleanup.

The full source cannot be called delivered merely because this guide, a catalog
entry or a pack manifest exists. Hosted native conversion and final signed
installation/client acceptance remain explicit gates under issue #485 and epic #460.
