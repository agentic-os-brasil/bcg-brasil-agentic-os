# Spec 055 - Direct repository and worktree entry

Status: accepted for implementation as a locally contract-tested vertical;
native runtime qualification and signed portable release evidence remain
separate.

## Objective

Maestro supports two entry modes without a desktop application:

1. **Hub** - open the extracted Maestro folder and use the existing
   conversational product surface; and
2. **Repo/Worktree** - open an ordinary Git repository or a worktree created
   with `git worktree add` and receive the governed Maestro projection there.

The ZIP remains transport. After activation, every projected hook invokes the
exact verified installed CLI by absolute path. No hook depends on `PATH`, a
persistent `cd`, an ancestor Maestro folder or global Claude/Codex settings.

## Authorities and identities

The implementation keeps these values distinct:

- `managed_root`: the activated, versioned Maestro core;
- `data_root`: owner-private profile, memory, continuity and enrollment state;
- `repository_root`: the canonical primary working-tree root derived from a
  conventional Git common `.git` directory; for non-conventional separate Git
  directories it falls back to the exact checkout root;
- `git_common_dir`: the canonical directory returned by
  `git rev-parse --git-common-dir`;
- `worktree_root`: the exact canonical checkout returned by
  `git rev-parse --show-toplevel` and opened in the runtime;
- `repository_id`: an opaque local SHA-256-derived identity over the canonical
  Git common directory;
- `workspace_id`: an opaque local SHA-256-derived identity over the exact
  worktree root; and
- `case_id`: an optional professional scope association held only in private
  state when an existing governed case authority supplies it.

Remote URLs, branch names, client names and repository folder names never
participate in public identity, receipts or telemetry. Two worktrees sharing a
Git common directory share `repository_id` and have different `workspace_id`
values.

## Public lifecycle

The installed control plane exposes:

```text
bcgos workspace enroll --runtime claude|codex <repository-or-worktree>
bcgos workspace status --runtime claude|codex <repository-or-worktree>
bcgos workspace repair --runtime claude|codex <repository-or-worktree>
bcgos workspace remove --runtime claude|codex <repository-or-worktree>
```

The namespace follows the historical `bcgos workspace` grouping. Import and
versioned managed-workspace migration remain separate contracts; direct
enrollment does not import files, create a worktree or move Git state.

`enroll`, `repair` and `remove` are explicit mutation authorities. `status` is
read-only. Repeating any operation is idempotent. A failed safety check reports
that no source code or work was lost and returns exactly one safe next action.

## Thin runtime projection

Spec 056 defines the direct-entry native Hub layered on this projection. It
changes only enrolled repositories and worktrees; the Maestro-folder Hub keeps
its existing adapter path.

Enrollment writes only bounded, regenerable local runtime material:

- Claude: a path-free managed block in an untracked `CLAUDE.md`, a direct-only
  native `maestro-hub` main frontend selected in
  `.claude/settings.local.json`, governed skill projections and the supported
  SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop,
  SubagentStart and SubagentStop bindings;
- Codex: the equivalent path-free managed block in an untracked `AGENTS.md`,
  `.codex/hooks.json`, governed skill projections and the supported five-event
  lifecycle; and
- runtime-scoped local projection manifests under `.bcgos/` containing opaque
  identities, ownership hashes and the absolute roots required for inspection
  and repair.

Claude and Codex projections may coexist in the same exact checkout. Their
orientation, native settings, runtime-projection manifest, selection policy
and private enrollment binding are independently owned. The workspace identity,
orchestration state and mutation lock are shared. Removing one runtime preserves
every shared surface and exact Git exclude still referenced by the other.

When `CLAUDE.md` or `AGENTS.md` is already tracked, enrollment
preserves it byte-for-byte, records `orientation_mode: preserved_tracked` and
delivers Maestro's compact canonical runtime directive through the bounded
Session Start hook instead of dirtying a tracked file. It does not append the
complete orientation document to SessionStart or UserPromptSubmit. An untracked orientation receives the removable
path-free managed block. Every byte outside that block remains user-owned and
survives enroll, repair and remove exactly, including CRLF, leading whitespace
and repeated trailing line endings. The runtime manifest records whether the
orientation file was created by Maestro or existed before enrollment so a
whitespace-only user file is never mistaken for an empty managed file during
removal. Every machine-specific managed file is added as an exact
entry to the validated Git common `info/exclude`; broad `.claude`, `.codex`,
`.bcgos` or `data` ignores are forbidden. A runtime configuration already
tracked by Git blocks the complete transaction before the first write.

The projection never copies the complete core, owner profile, memory,
credentials, logs or client data. Skills are bounded managed projections and
remain replaceable only while their recorded hashes match.

## Hook contract

Every managed hook command passes `managed_root`, `data_root` and
`workspace_root` explicitly and invokes the exact installed CLI by absolute
path. The CLI re-resolves Git identity and the private enrollment before
reading scoped context or accepting a lifecycle event.

Session Start may compose only bounded, authorized workspace-scoped context:
Maestro orientation, integrity-checked skill pointers, owner-approved context,
workspace memory and explicit continuity. Reviewed SELF and generated memory
are rendered before the pointer packet so they survive the native delivery
boundary; a packet that does not fit is omitted whole rather than truncated.
Prompt, file and Git-command bodies
are never written to receipts. A Codex guard receipt may retain only the
SHA-256 of its normalized command so native qualification can bind a denial to
the exact synthetic action without retaining the command body. The complete direct-worktree Session Start
output is capped at 8 KiB; each optional workspace context source retains its
independent read ceiling but cannot widen that final output budget. The compact
canonical runtime directive supplies essential orientation when a tracked
runtime instruction file forced `preserved_tracked` mode. UserPromptSubmit
never repeats the complete orientation, reviewed SELF or generated memory.
Pre-action enforcement resolves paths against
the exact `worktree_root`; traversal, symlink escape and unresolved shell or
environment path expansion fail closed. The direct route invokes the existing
runtime global protected-root guard and canonical user-bound external-action
challenge, plus the Claude managed-agent guard where applicable, before local
command classification. Git
mutation enforcement parses the Git executable, global options and subcommand
rather than relying on one literal command substring. Inline Git aliases are
resolved recursively before classification; indirect literal-Git wrappers,
shell aliases, `eval`, dynamic executables and environment-backed alias
expansion fail closed because their executable effect cannot be bounded
statically. Unknown Git subcommands also fail closed because Git may resolve
them through external configuration or a `git-<name>` executable. Ambiguous
`git checkout` positionals fail closed because the same token may
name either a branch or a pathspec that discards work; safe agent-driven branch
changes use `git switch`, whose force, discard, orphan and force-create forms
remain denied, including combined short options. Opaque code
and script execution wrappers, including interpreter eval forms and shell
`source`, fail closed when their filesystem or Git effects cannot be bounded
from the hook payload. Claude
managed specialists also pass through the
canonical tool-free/Case workspace guard.
Post-action, Stop and Claude subagent hooks advance the existing metadata-only
lifecycle receipt and native-agent flow instead of acknowledging those events
without processing them. Claude Stop returns a native block on malformed or
identity-less payloads and on receipt, strategic-flow or persistence failure;
those errors never degrade to `continue: true`. Claude and Codex may use different native event
shapes, but preserve the same isolation, identity, privacy and local-effect
invariants.

The direct hook must enter through the same runtime-neutral Session Context
Packet as the Hub path. Runtime-specific serialization follows Spec 056:
direct Claude hooks carry factual state while the native main-session frontend
and preloaded canonical method carry stable operating policy; Codex uses its
equivalent thin adapter. Neither route may reduce the Maestro operating layer,
misrepresent the named host runtime or require concealment of provider, hooks,
limitations or architecture.
After confirmed onboarding, it may attach the bounded ephemeral professional
SELF projection defined by Spec 015. The packet remains pointer-only;
`personal-context`, other sensitive facets, unconfirmed templates and any
other workspace remain excluded, and prompt hooks do not repeat owner bodies.
Claude and Codex tests must reject a one-sided identity or owner-context path.
The current ZIP's schema-v1 owner registry is accepted only through the bounded
compatibility rule in Spec 015; this bridge neither treats templates as answers
nor grants cross-workspace access.

Native qualification must observe an affirmative response containing both the
Maestro layer and the correct host runtime and must reject refusal or
prompt-injection language. Merely finding `Maestro` in a response is not proof
that the runtime accepted or used the configured layer.

## Transaction and conflicts

Before mutation, the manager:

1. rejects lexical traversal, symlinked roots or managed path components;
2. resolves the repository top level and Git common directory with Git;
3. verifies the target runtime configuration is not tracked;
4. checks every existing managed file against its ownership marker and digest;
5. snapshots all prospective managed files, private binding state and exact
   Git exclude contents under bounded limits; and
6. performs no branch, index, worktree, remote, Git configuration or Git-hook
   mutation.

The manager publishes the private binding only after projection, hook and Git
exclude verification succeeds. Any partial failure restores the complete
snapshot. Rollback failure is surfaced as a conflict and never hidden.

User-authored prefixes in untracked `CLAUDE.md` and `AGENTS.md` are preserved;
tracked orientation files are never edited by the projection. A changed
managed block, skill, agent, runtime configuration, manifest or ownership hash
is a conflict: reinstall, repair and remove do not overwrite or delete it.

## Update, move, repair and removal

A newer intact managed core may regenerate projections through `repair`. The
private workspace binding pins the prior runtime-projection manifest digest;
an intact prior projection whose canonical embedded content changed is reported
as `repair_required` with reason `managed_core_updated`. Co-tampering a managed
file and its checkout-local manifest cannot authorize update because it no
longer matches that private digest. Reinstallation replaces only content still
matching the trusted prior managed hash.
When the activated core moves, `status` returns `repair_required` with reason
`managed_root_moved`; explicit `repair` rewrites the absolute CLI/root bindings
from the currently executing verified installation after rechecking every
other invariant.

Portable activation follows the same update boundary. A new version may
atomically replace `data/install.json` only after its transported CLI digest is
verified; the prior activation state is preserved as
`data/install.previous.json`. Different CLI bytes under the same semantic
version fail closed. Owner profile, memory, cases and workspace bindings are
not part of this replacement.

`remove` deletes only intact manifest-owned files and the managed orientation
block, removes only exact Git exclude entries owned by that runtime enrollment,
and deletes the corresponding private binding. User files, branches, index,
working tree and unrelated ignore rules remain unchanged.

## Evidence boundary

Unit and contract tests use synthetic repositories and real local Git
worktrees. They may establish implemented and locally validated behavior only.
They do not prove Claude/Codex native invocation, CI, signed release integrity,
clean-device acceptance, release readiness or pilot readiness.

The development-only native matrix is reproducible for the supported
macOS-arm64 Claude and Codex paths:

```text
installers/zip/build-release.sh <version> macos-arm64
go run ./dev/native-qualification \
  --artifact dist/Maestro-Portable-<version>-macos-arm64-local-beta-unsigned.zip

go run ./dev/native-qualification \
  --runtime codex \
  --codex-model gpt-5.6-sol \
  --artifact dist/Maestro-Portable-<version>-macos-arm64-local-beta-unsigned.zip
```

It uses a fresh extracted ZIP and a synthetic Git repository, invokes Claude
Code in the Hub and directly in the enrolled repository, and removes the
projection at the end. The bounded JSON result covers first-run activation,
Hub and direct Doctor execution, all seven Claude lifecycle events, discovery
of all five managed native agents, managed subagent flow, dangerous-Git denial,
metadata-only lifecycle receipts, a second-session continuity sentinel, Git
cleanliness and reversible removal. Raw native streams, prompts, tool payloads,
workspace paths and session identifiers remain only in the temporary fixture
and are deleted by default. A temporary Codex authentication copy is scrubbed
even when the sanitized fixture is explicitly retained for diagnosis. A
passing record qualifies only the exact
runtime/version/platform/artifact tuple it names. The Codex path keeps
`--approve-for-me` and the workspace-write sandbox active and bypasses only the
interactive hook-trust review for the inspected synthetic projection. Its
bounded report fixes the model and post-initialization isolated Codex
configuration digest; the qualification home inherits authentication only,
never user instructions, skills, plugins, MCP servers or configuration. Its
checks distinguish configured native event bindings from observed context
injection, require stage-local schema-valid receipts for context and
dangerous-Git denial, require the exact guarded Git-command digest, reject continuity
sessions containing command, MCP or unknown tool items, and validate
metadata-only PostToolUse/Stop receipts by
content rather than filename. The Codex Doctor gate requires a successful exact
read of the canonical projected `SKILL.md`, the exact installed-CLI version
output and a version-bounded verdict. Any qualification or fixture-cleanup
failure changes the report to `failed` before it can be serialized, so a
cleanup error can never publish a passing record. Neither path establishes Windows, organization
signing, release readiness or pilot readiness.
