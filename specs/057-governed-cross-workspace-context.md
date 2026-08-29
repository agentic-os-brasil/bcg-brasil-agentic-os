# Spec 057 - Governed cross-workspace context

Status: accepted for implementation and local contract validation. Native Claude
and Codex qualification remains separate evidence.

## Objective

An owner working directly inside one enrolled repository or worktree may
explicitly consult a bounded Maestro-owned projection from another enrolled
workspace without opening the target checkout to the current runtime. This is
a narrow context-transfer capability, not filesystem delegation, workspace
switching, import, synchronization or mutation authority.

The normal boundary remains exact-workspace isolation. Absence of an active,
valid grant always means cross-workspace access is denied.

## Public control-plane contract

The installed CLI exposes the capability under the existing workspace control
plane:

```text
bcgos workspace access grant --runtime claude|codex \
  --source <enrolled-repository-or-worktree> \
  --target <enrolled-repository-or-worktree> \
  --purpose reference_context|compare_implementation|reuse_learning|dependency_coordination \
  --include context,memory,continuity --ttl 30m --confirm

bcgos workspace access read --runtime claude|codex \
  --source <enrolled-repository-or-worktree> --grant-id <opaque-id>

bcgos workspace access status --runtime claude|codex \
  --source <enrolled-repository-or-worktree> --grant-id <opaque-id>

bcgos workspace access revoke --runtime claude|codex \
  --source <enrolled-repository-or-worktree> --grant-id <opaque-id>
```

The canonical `bcgos-operator` method lets Maestro translate a natural-language
owner request into this bounded flow. The command remains an internal control
plane; the normal product experience is conversational. `grant` is the only
creation authority and requires `--confirm`. `read` and `status` are read-only.
`revoke` records an irreversible denial for the grant but changes no repository
or projected context source.

## Grant authority

Every grant is schema-versioned and binds:

- the exact source and target `workspace_id` and `repository_id` values;
- the selected runtime, authenticated local-principal reference and local-device
  reference;
- one purpose from the closed registry;
- one or more unique sources from `context`, `memory` and `continuity`;
- issue and expiry instants; and
- a private integrity authenticator over the complete authority-bearing record.

The target must differ from the source. Both paths must resolve through Git and
report `enrolled` for the selected runtime at grant time. TTL defaults to 30
minutes, has a minimum of 5 minutes and a maximum of 2 hours. The private store
keeps a bounded number of grants per source workspace and rejects symlinks,
unsafe modes, malformed records and index overflow. Durable records contain no
owner name, OS username, repository path, remote URL, branch, client name,
prompt, context body or arbitrary purpose text.

The grant identifier is opaque and unguessable. A grant cannot be replayed from
another source workspace, runtime, principal or device. Revocation is durable;
the record is retained as revoked rather than deleted or silently recreated.

## Explicit read and source resolution

Before every read, the control plane:

1. resolves and validates the exact source enrollment supplied by the caller;
2. authenticates the grant and checks source, runtime, principal, device,
   purpose, source allowlist, expiry and revocation;
3. resolves the target only through the bounded owner-private enrollment index;
4. reruns normal target enrollment status and requires the original target
   workspace and repository identities; and
5. reads only the granted Maestro-owned source adapters.

The target checkout is never used as a content root. `context` reads the
workspace-private, bounded session-context projection; `memory` calls the
canonical generated-memory assembler with managed per-layer budgets; and
`continuity` reads the explicit bounded continuity surface. Readers reject
symlinks, non-regular files, oversize input, invalid generated-memory state and
any source that resolves outside its canonical private root. Missing authorized
content returns an explicit empty/unavailable state and never falls back to raw
captures, transcripts, repository files or another workspace.

The complete JSON response is bounded to 8 KiB and contains only schema version,
opaque source/target identities, purpose, expiry and ordered source sections.
No path is returned. Bodies are emitted only to the invoking process and are
never written to the source checkout, receipts, lifecycle logs, memory or grant
record. This first slice does not automatically inject cross-workspace bodies at
SessionStart or UserPromptSubmit.

## Runtime and agent behavior

Claude and Codex projections advertise the same capability ID, purposes,
sources and safety policy through the canonical contract. A direct native Hub
may offer governed access only after an explicit request that identifies a
target and use. It must explain when no grant exists or a grant has expired and
may not silently broaden or renew one.

The capability belongs to the main Maestro session. Client Account, Case, Yoda,
Darwin and PA Expert specialists do not receive the grant, target bodies or a
new delegation edge. Existing pre-action guards continue resolving every tool
against the source worktree. Possessing a grant does not make target paths valid
for read, write, shell, Git or file tools.

## Failure and recovery

Malformed input, missing confirmation, unknown purpose/source, identical
source and target, enrollment drift, integrity failure, expiry, revocation,
identity mismatch, target ambiguity, target removal and budget overflow fail
closed before context output. Errors state that no repository or work content
was changed and give one safe next command. Grant creation and revocation use
atomic owner-private writes and a cross-process lock; partial writes never
become readable authority.

Removing or repairing a workspace projection never silently migrates a grant.
An identity-preserving repair may restore readability after normal revalidation;
removal or a different worktree identity invalidates access. Cleanup of expired
grant records is a separate bounded maintenance operation and is not required
for denial.

## Required evidence

Implementation starts with failing observable tests for:

1. confirmation, closed purpose/source registries, distinct enrolled identities
   and TTL bounds;
2. integrity, principal/device/runtime/source binding, expiry and durable
   revocation;
3. bounded reads of each allowed Maestro-owned source with no checkout or
   cross-workspace fallback;
4. target removal, enrollment drift, symlink, malformed, oversize and index
   overflow denial;
5. grant records, errors and responses excluding raw paths, identities and
   unauthorized bodies;
6. existing file/Git guards still denying target checkout access while a grant
   is active;
7. Claude/Codex capability, policy and conversational-method parity; and
8. unchanged Maestro-folder Hub behavior and direct-worktree lifecycle tests.

The development gate is `go run ./dev/harness validate --full`, with race tests
for every touched package. This proves local contracts only. Native runtime and
release qualification remain part of the final matrix, not this specification.

## Out of scope

- arbitrary target-repository file reads or search;
- writes, commits, branches, worktrees or commands in the target;
- automatic or standing context injection;
- grants to specialists or recursive delegation;
- cross-device, remote or organizational federation;
- owner-atlas promotion or Hub-memory migration; and
- signed-release or pilot-readiness claims from repository tests alone.
