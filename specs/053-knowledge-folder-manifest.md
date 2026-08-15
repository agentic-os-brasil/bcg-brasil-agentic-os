# Spec 053 - Knowledge folder manifest for delta analysis

Status: proposal. Contract shape and storage layout described; deterministic
engine, adapter wiring and CLI surface are deferred to later slices.

## Objective

Give every folder that participates in a recurring knowledge scan — an atlas
projection, an ingestion pass, a wiki compile, a Darwin observability sweep —
a small, machine-owned manifest that records what was already analyzed and
under which analyzer state. On the next scan of that folder, the engine
consults the manifest, compares it against current file content and current
analyzer identity, and processes only the delta. Content that has not moved
under an analyzer that has not moved is skipped without ceremony.

The manifest is agent-writable and human-readable; it is never edited by the
owner and never accepted as authoritative when its structure fails validation.
It is a residue of prior work, not a new memory layer and not a substitute for
`specs/006-memory-persistence.md` or `specs/052-agent-context-snapshot.md`.

## Distinction from existing layers

- Spec 006 owns workspace-scoped continuity across L1, L2, L3 and lifetime.
  The knowledge folder manifest is folder-scoped, path-keyed and never becomes
  a memory layer.
- Spec 007 (content navigation) and spec 008 (wiki update OKF) own the
  compiled projection of the corpus. The manifest is the scan-side ledger
  that feeds those pipelines; it does not replace them.
- Spec 047 (agent breadcrumbs) tracks a metadata-only, hash-linked tail per
  agent invocation. The manifest tracks per-path fingerprints per analyzer,
  which is a different key space.
- `internal/ingest.Fingerprint` already computes the per-file sha256. The
  manifest reuses that primitive; it does not reinvent hashing.

## Storage

Each tracked folder carries one manifest file per analyzer at its own root:

```text
<folder>/
  .knowledge-manifest.<analyzer_id>.json
```

The file is intentionally hidden, intentionally at the folder root, and never
placed at a repository root that would aggregate unrelated analyzers. Keying
the file by `analyzer_id` from day one lets legitimate concurrent consumers of
the same folder — `wiki-compile`, `atlas-collect`, `darwin-signal` — coexist
without a future migration and without a merge protocol. A folder that is not
tracked has no manifest and no implicit one; absence is a legal state and is
reported as `unmanifested`, not synthesized.

The file shape:

```json
{
  "schema_version": 1,
  "folder_id": "docs/onboarding",
  "written_at": "2026-08-14T18:22:00Z",
  "writer": {
    "analyzer_id": "wiki-compile",
    "analyzer_fingerprint": "sha256:…",
    "runtime": "go"
  },
  "entries": [
    {
      "path": "windows-contributor-prompt.md",
      "content_sha256": "…",
      "analyzed_at": "2026-08-14T18:20:00Z",
      "analysis_ref": "wiki/index.json#docs/onboarding/windows-contributor-prompt.md",
      "analysis_status": "usable"
    }
  ],
  "dropped": [
    { "path": "old.md", "reason": "path missing at reconcile" }
  ]
}
```

Paths are folder-relative and forward-slash normalized. `content_sha256`
reuses the streaming hash from `internal/ingest.Fingerprint`. `analyzer_id`
is the durable name of the analyzer that produced the entry (for example
`wiki-compile`, `atlas-collect`, `darwin-signal`). The manifest deliberately
omits a `session_id` field on the writer: session correlation is the key
space of spec 047 (agent breadcrumbs), and joining a folder ledger to a
session identity here would begin turning the manifest into a fourth memory
layer against the non-goals below.

`analyzer_fingerprint` is not a self-attested opaque string. It is produced
by a shared helper in `internal/ingest` (co-located with `Fingerprint`) that
accepts an ordered, canonicalized tuple of the analyzer's declared inputs —
prompt digest, code module digest, model identity, resolved dependency
digest — hashes the canonical form with sha256 and returns the `sha256:…`
value. The engine validates that a stored `analyzer_fingerprint` is
structurally the output of that helper before trusting it. A malformed or
non-canonical `analyzer_fingerprint` is not repaired: the entry is treated as
if it were missing and the file is re-analyzed. This makes analyzer honesty
a structural property of the write path rather than a trust assumption about
the analyzer.

## Delta rule

On a scan of a tracked folder, the engine walks the folder's files, computes
each `content_sha256` and joins with the manifest's `entries` by path. A file
is re-analyzed when any of these hold:

- the path is not present in `entries`;
- the stored `content_sha256` differs from the current one;
- the stored `analyzer_fingerprint` differs from the current analyzer's
  fingerprint;
- the entry lacks an `analysis_ref` that the current analyzer can dereference.

A file that passes all four checks is skipped and its entry is preserved
unchanged. A file whose `analysis_ref` no longer resolves is treated as if
the entry were missing, not as an error, because the analyzer's output store
is authoritative for its own state.

## Reconcile-drop

At the start of each scan, the engine reconciles the manifest against the
folder listing. Entries whose path no longer exists are moved to the
`dropped` list with a `reason` string and are not re-analyzed. `dropped` is
bounded to the most recent 128 removals per folder; older removals fall off
without ceremony. Reconcile-drop is a folder-local operation and never
mutates other folders, memory layers or breadcrumbs.

## Atomic write

The manifest is rewritten only when the scan produced at least one change to
`entries` or `dropped`. The write path is the durable rename pattern already
used by `internal/memory` (see `durable_rename_unix.go` and its Windows
counterpart): the engine writes a sibling `.knowledge-manifest.json.tmp`,
fsyncs it and renames it into place. A crash between write and rename leaves
the previous manifest untouched. A crash after rename leaves a fully valid
new manifest. There is no intermediate visible state where two analyzers can
race on a half-written file.

Because the file name embeds `analyzer_id`, two analyzers scanning the same
folder write to two distinct files and never race on the same bytes. The
engine still refuses a write whose `writer.analyzer_id` does not match the
`analyzer_id` embedded in the file name — that mismatch is a bug, not a
merge, and is reported as `manifest_analyzer_id_mismatch`. There is no
in-file merge protocol between analyzers and none is planned.

## Non-goals

- No new memory layer. The manifest is not injected into any prompt, not
  merged into any wiki page and not surfaced to the owner as narrative.
- No cross-folder aggregation. There is no central index of all manifests in
  this slice; a scanner that needs one composes it at read time.
- No content storage. The manifest never carries a file's body, a snippet, a
  summary or a title. It carries hashes and references.
- No delegation. A delegated agent may read the manifest of a folder it is
  authorized to read; it never writes one.

## Runtime portability

The file shape, path convention, delta rule, reconcile-drop policy and atomic
write contract are runtime-neutral. A Go analyzer under `internal/` and a
skill-driven analyzer projected under `.claude/skills/` must observe the same
invariants. Codex adapters must consume the same manifest through a thin
adapter without introducing a private manifest shape.

## Test expectations for this slice

- structural validation of the manifest shape, including `schema_version`,
  `folder_id`, `writer` and per-entry required fields;
- delta rule: content change, fingerprint change, missing entry and
  unresolvable `analysis_ref` each independently force re-analysis;
- reconcile-drop moves vanished paths to `dropped` with a `reason` and never
  re-analyzes them within the same scan;
- atomic write leaves the previous manifest intact on simulated write
  failure and leaves only the new manifest after a successful rename;
- rejection of a write whose `writer.analyzer_id` differs from the
  `analyzer_id` embedded in the file name, reported as
  `manifest_analyzer_id_mismatch`;
- rejection or forced re-analysis of an entry whose `analyzer_fingerprint`
  is malformed, non-canonical or was not produced by the shared helper in
  `internal/ingest`;
- isolation between folders: a scan of folder A never mutates folder B's
  manifest, even when both analyzers share an `analyzer_id`;
- isolation between analyzers in the same folder: `wiki-compile` writing its
  manifest never mutates `atlas-collect`'s manifest in the same folder.

## Open questions deferred to a later slice

- Cross-folder and cross-analyzer rollup, once at least two analyzers
  actually consume the shape in production.
- Migration from `schema_version: 1` to future shapes through the reversible
  migration pattern already used by `internal/memory`.
- A read-only projection for Darwin observability that reports staleness
  without triggering re-analysis.
