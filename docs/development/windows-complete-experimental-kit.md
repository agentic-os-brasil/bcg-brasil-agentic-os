# Complete Windows experimental kit

Use this recipe when producing the full Windows amd64 engineering kit for #599.
The default native workflow still runs proof only. Export and assembly are two
explicit runs on the **same exact clean commit and tree**.

## 1. Native unsigned export

Dispatch `Windows authenticated Docling runtime` with `mode=export`, ascending
`version` / `next_version`, one stable `installer_version`, and the existing
approved **public** registry encoded as base64 (`registry_base64`, at most 4096
characters). Keep every assembly input empty. The Windows document closure has 56 locked
packages (85,851,562 bytes) plus its 47,113,364-byte interpreter: 57 inputs /
132,964,926 bytes. The original legacy profile and default assembler stay unchanged.

The original ordinary-account driver must first pass all four native parent
tests and thirteen real conversion formats (the original eleven plus encoded-text PDF and modern PPTX), including the tamper/missing-runtime
controls. Its receipt now records the hash of the runtime actually converted.
The distinct qualification identity is `windows_docling_documents_v1`; it binds
`model-free-documents-v1`, its exact authenticated lock, six finite PDF refusals
and nine existing unsafe PPTX refusals. OCR, scanned/visual PDF reconstruction
and model downloads remain outside this profile. The export rebuild authenticates the same 57 public inputs and refuses if the
result differs from that observed hash. It then uses `BuildModernCandidate`
twice and requires byte-identical stable bootstrappers.

Download the retained `maestro-windows-unsigned-RUN-COMMIT` artifact into a new
physical Developer directory. Preserve `export.json`, its independently
reviewed SHA-256, both candidates, bootstrap, public registry and proof. The schema-v2 receipt records runtime profile and lock alongside the exact
pack hash. Historical schema-v1 legacy exports are refused by this full kit path. These
are unsigned construction artifacts; do not send them as a participant kit.

## 2. Root-only signing and public return metadata

On the root Mac, check the actual successful producer run, exact source/tree,
closed export receipt and bodies. Use the original `dev/release sign`,
`VerifiedExperimentalSeed` and `PublishGeneration` owners to sign the exact
Windows pair, issue its Windows-specific finite role and publish the signed
source. Retain the existing issuer/key/expiry; the private key stays on the Mac.
The source's current generation may select either of the two signed releases.

Then run the development-only metadata collector from the reviewed source:

```text
go run ./dev/pilot/windows-kit --mode metadata \
  --export EXPORT --export-sha256 REVIEWED_EXPORT_SHA256 \
  --source-revision EXACT_COMMIT --source-tree EXACT_TREE --producer-run-id ORIGINAL_RUN \
  --signed-n SIGNED_N --signed-n1 SIGNED_N1 \
  --role WINDOWS_ROLE --role-signature WINDOWS_ROLE_SIGNATURE \
  --source SIGNED_SOURCE --output NEW_DEVELOPER_DIRECTORY
```

The collector performs the complete cryptographic reconstruction before emitting
`public-metadata.base64`. It contains only the two signed manifests, release
notes, detached artifact signatures, finite public role and signed generation.
Artifact bodies, credentials, signing keys and private participant profiles are
not transport members. Decoded JSON is capped at 24,000 bytes and outer base64 at
32,000 characters, fitting Windows environment-variable limits while leaving the
other dispatch inputs within the 65,535-character combined allowance. An oversized result refuses; it never truncates metadata.

## 3. Native assembly

Dispatch the same workflow at the exact export commit with `mode=assemble`,
`unsigned_run_id`, `export_sha256` and `public_metadata_base64`. Keep registry
and version inputs empty.

The workflow requires that the original producer was this workflow, in this
repository, on this exact source, and completed successfully. It downloads the
original artifact by run/name, then independently checks the receipt, closed
members and hashes. The bridge reconstructs the signed releases using only the
unchanged original bodies; canonical release, role, installer and synced-source
verifiers authenticate the result before the original installed-seed factory
runs its actual native `seed-status` probe. Existing or interrupted outputs are
retained and refused.

Download the final `maestro-mcp-windows-complete-RUN-COMMIT` artifact. It contains
`Maestro-MCP-Windows.zip`, its checksum, `BUILD-IDENTITY.json` and short Markdown instructions. The ZIP
contains the existing `Maestro.cmd` preparation/test/report flow and complete
MCP/plugin/method/runtime material. Recipients extract the entire ZIP; no source
checkout, Go, Git, Python, WSL or administrator installation is required. Their
private brain remains under their selected local Developer directory.

## Acceptance boundary

A successful assembly proves native construction, public-signature closure and
the original factory probe. It does **not** prove corporate-device permission,
Claude authentication, Code/Desktop loading, Desktop agent execution, native
hooks or OneDrive update behavior. Record those actual participant checks
against the final ZIP separately. This engineering path neither supplies
company production signing nor changes the finite role's expiry.
