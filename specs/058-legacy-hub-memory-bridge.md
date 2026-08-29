# Spec 058 - Legacy Hub memory bridge

Status: accepted for implementation and local contract validation. Native Claude
and Codex qualification remains separate evidence.

## Objective

An owner may explicitly review and import selected Markdown memory from the
portable Maestro Hub's legacy global tree into the canonical generated-memory
authority of one exact enrolled repository or worktree. This restores useful
continuity for direct-repository entry without silently sharing global memory,
mutating the legacy tree or bypassing canonical promotion policy.

The bridge is a one-way, review-gated migration operation. It is not automatic
projection, synchronization, cross-workspace access or a new memory layer.

## Public control-plane contract

The installed CLI exposes the bridge below the existing workspace control
plane:

```text
bcgos workspace memory bridge inspect --runtime claude|codex \
  --workspace <enrolled-repository-or-worktree>

bcgos workspace memory bridge preview --runtime claude|codex \
  --workspace <enrolled-repository-or-worktree> --candidate <opaque-id>

bcgos workspace memory bridge apply --runtime claude|codex \
  --workspace <enrolled-repository-or-worktree> \
  --candidates <opaque-id,...> --attest-target-scope --confirm
```

The canonical operator skill turns a conversational request into inspect,
bounded preview and explicit apply. `inspect` returns metadata only. `preview`
returns at most 4 KiB from one candidate. `apply` is the sole mutation and
requires both flags: the owner confirms the selected content belongs in the
exact target workspace and attests that it contains no credentials, secrets or
unauthorized material. Runtime choice changes only the adapter projection; it
does not change memory semantics.

## Legacy source boundary

Discovery is restricted to direct regular `.md` children of these fixed roots:

- `data/memory/recent`;
- `data/memory/weekly`;
- `data/memory/medium-term`; and
- `data/memory/lifetime`.

Directories, files and roots reached through symlinks are denied. Nested files,
other extensions and invalid UTF-8 are not candidates. Discovery fails closed
above 256 directory entries, 128 valid candidates, 32 KiB per file or 512 KiB
of discovered candidate content. It never follows a path supplied by the
runtime.

Each candidate ID is a lowercase 32-character opaque digest bound to the
legacy layer, safe relative filename and complete content digest. Inspect emits
only schema version, candidate ID, legacy layer, byte count and digest; it
never emits a local path or content. Preview and apply recompute the candidate
set immediately, so renamed or changed content invalidates an earlier ID.

Apply accepts between one and sixteen unique candidate IDs with no more than
64 KiB selected content. Selection order does not affect identity or output.

## Canonical import and provenance

The canonical memory engine computes a deterministic import ID from the target
workspace identity and sorted selected candidate identities and digests. Under
the existing workspace-wide activation lock it:

1. stages exact immutable selected-source snapshots below the target workspace
   private import authority using only the import and candidate opaque IDs;
2. validates source digests, UTF-8, bounds and the target workspace identity;
3. builds a same-day L1 artifact that preserves the complete active L1 content
   and provenance and appends the reviewed imported blocks;
4. validates the complete artifact against the managed L1 budget; and
5. publishes and activates it through the existing atomic commit manifest.

No source filename or source path enters the artifact, snapshot name, command
response or error. The L1 provenance identifies the legacy layer, candidate
digest and deterministic import synthesizer. Legacy `weekly`, `medium-term`
and `lifetime` labels remain source metadata only: every selected item enters
L1 and can reach L2, L3 or lifetime solely through normal dreaming and lifetime
eligibility.

The bridge never edits, deletes, renames or normalizes a legacy source file.
Exact source snapshots preserve its bytes. If preserving the active L1 plus
the complete selected material exceeds the L1 budget, apply fails before
activation; truncation and partial selection are forbidden.

## Idempotency, interruption and recovery

A valid historical commit carrying the exact import synthesizer/import ID is
authoritative evidence that the import already committed, even when later
memory commits changed active L1. Retry returns `already_applied` and creates
no duplicate content. History inspection is bounded and accepts only validated
immutable manifests and artifacts; overflow fails closed.

Staged snapshots and artifacts are inert until a complete commit is atomically
activated. Failure before that point preserves the previous active commit.
Failure after activation but before response is recovered by the same
historical idempotency check. Orphaned identical snapshots may be reused only
after their bytes and digests validate; conflicting immutable content is
denied. Normal memory status and repair remain authoritative.

## Runtime, workspace and product boundaries

Claude and Codex advertise and invoke the same capability, candidate grammar,
attestations, budgets, provenance and failure contract. The installed hook
guard permits only the exact installed CLI, current enrolled worktree and
closed command grammar; PATH-spoofed, compound, cross-worktree and hidden-root
variants are denied.

An import targets one exact workspace identity. It never appears in another
workspace, modifies a checkout, opens repository filesystem access or grants a
specialist new authority. Normal Maestro-folder Hub startup and its legacy
memory readers remain unchanged. Direct Session Start sees imported material
only through the existing bounded canonical memory assembler after successful
activation.

## Required evidence

Implementation starts with failing observable tests for:

1. fixed-root discovery, symlink/nesting/extension/UTF-8 and all entry/byte
   bounds;
2. opaque metadata, bounded preview and absence of paths or filenames in every
   result and error;
3. immediate candidate revalidation, selection limits and both owner
   attestations;
4. byte-identical preservation of legacy sources and selected-only immutable
   snapshots;
5. active-L1 preservation, provenance and forced L1-only import from every
   legacy layer;
6. idempotent retry after later valid commits and fail-closed bounded history;
7. old-or-complete visibility at every injected staging and activation failure;
8. exact target isolation across repositories and distinct worktrees;
9. Claude/Codex CLI, capability, hook grammar and session-context parity; and
10. unchanged Maestro-folder Hub, dreaming and direct-worktree lifecycle
    behavior.

The development gate is `go run ./dev/harness validate --full`, with race tests
for every touched package. Two independent Claude Opus reviews must pass before
the next implementation point begins. Native runtime and release qualification
remain part of the final matrix, not this specification.

## Out of scope

- automatic import, synchronization or import into every enrollment;
- arbitrary legacy paths, repository files, transcripts or raw prompts;
- direct legacy-to-L2, L3 or lifetime mapping;
- deduplicating semantically similar but byte-different memories;
- editing or deleting the legacy Hub tree;
- cross-device, remote or organizational memory federation; and
- signed-release or pilot-readiness claims from repository tests alone.
