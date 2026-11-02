# Preserve runnable managed artifacts through bundle delivery

Authority: Spec020 release distribution, FD-004/006/009 and the canonical
release verifier/install transaction. This is the first complete construction
slice on `codex/integration/maestro-distribution`; no PR opens until the wider
distribution front delivers its participant journey.

## Global Constraints

Use the isolated `maestro-distribution-payload` worktree. Root owns Git, the
plan and integration merges. One implementation writer; maintenance runs in a
different worktree. Reuse explicit factory allowlisting, exact archive digest
verification and canonical extraction. Preserve regular-file, exclusive-create,
path/type/size bounds and all signing requirements. No installation on the host,
credential access, provider/model call or production authority. Focused tests
only; source, engineering fixtures, native execution and release readiness are
separate evidence.

## Task 1 — Preserve the authenticated executable distinction

Own `internal/dev/releasepack/releasepack.go` and directly affected tests,
`internal/installtx/archive.go` and directly affected tests. Read AGENTS and
develop-change, Spec020, and the existing distribution binding/preflight maps.
Inspect actual factory and extraction callers before changing code.

Both factory tar headers and canonical extraction currently force regular files
to 0644. A native runtime helper consequently loses its executable permission.
Observe a focused behavioral RED that demonstrates this public factory-to-
delivery defect. Preserve the factory's explicitly allowlisted executable
distinction in authenticated archive metadata, normalized to ordinary safe
regular-file permissions. Extraction must preserve that distinction while
discarding setuid/setgid/sticky and group/world write bits. Non-executable
content stays non-executable. Do not infer executable permission from extensions
or turn every managed file into a program. Do not add another manifest, updater,
archive reader or trust bypass.

Prove the real archive factory output and canonical extraction preserve a
nonempty helper's bytes and runnable mode, ordinary content stays data, and
unsafe mode bits cannot survive. Reuse a canonical authenticated install fixture
if an existing public activation test can exercise this without inventing a
seeded factory or native signing authority; otherwise report exactly which
boundary is proven and leave that later activation gate explicit. Retain
traversal, symlink/special-entry, no-overwrite and extraction-limit behavior.
Use a harmless local fixture for any execution proof; do not run untrusted or
real owner files. Do not broaden to installer restoration, agents/hooks, formats
or Windows private-handle work.

Run the smallest directly affected releasepack/installtx selections, gofmt and
diff check. Freeze exact source hashes, save a report in this plan's SDD
workspace, and obtain independent Spec+Quality review before root commits and
merges the exact source into the distribution integration branch. Do not claim
all full-package, signing, native client, synced-folder or pilot gates closed.

## Preflight interface scan

| Tasks | Producer/consumer or internal check | Resolution |
| --- | --- | --- |
| 1 | Factory archive headers feed canonical extraction | Normalize the same executable distinction at both boundaries; archive digest authenticates metadata |
| 1 | Existing deterministic archive tests expect data modes | Keep ordinary fixtures non-executable and add an independently runnable fixture |
| 1 | Existing activation needs a verified release | Reuse its engineering fixture, preserve verifier/signatures, report unavailable native authority separately |
| 1 | Safe extraction versus filesystem privilege bits | Retain exclusive regular-file creation and bounds; copy only ordinary executable distinction |
