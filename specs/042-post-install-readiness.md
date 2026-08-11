# Spec 042 - Deterministic post-install readiness

Status: Claude-first/Codex-compatible configuration verifier implemented; native runtime qualification remains separate.

## Objective

Give the signed installer a deterministic, read-only check to run after
`bcgos init` and `bcgos adapter install --runtime <claude|codex>`. The check proves that
the installed CLI, initialized workspace, managed runtime projection and all
runtime-specific lifecycle bindings still agree. It never starts a runtime, invokes
a hook, calls a model or changes global runtime settings.

## Canonical identities

The owner-local install state is the authority for the managed root and active
CLI version. The verifier accepts only the regular, non-symlink
`managed-root/bin/bcgos[.exe]` executable and requires the current CLI path and
version to match it exactly. A caller-selected alternate executable is not a
valid readiness identity.

The workspace must be an existing canonical directory with no symlinked path
component. Its strict `.bcgos/workspace.json` identity must match the physical
path produced by `bcgos init`. The selected runtime projection must contain a regular
`CLAUDE.md` (Claude) or `AGENTS.md` (Codex), strict `.bcgos/runtime-projection.json`, selection-scoped policy
and every manifest-owned skill. Their paths, hashes and managed orientation
block must match the active embedded bundle and the owner-confirmed capability
tracks; self-consistent but rewritten local manifests are not accepted.

Readiness also requires the installation dependencies that make the first
session deterministic: a protected, strict-JSON
`.bcgos/maestro-orchestration-state.json` snapshot, an initialized owner
context/onboarding registry, the Case Agent dossier and the signed Case Agent
scaffold. `bcgos init` materializes these surfaces
idempotently; an interrupted or legacy installation fails with a precise
check rather than being presented as ready. The verifier does not install
external Claude Code, Codex or provider packs: those remain explicit runtime
availability and authority gates outside the local Maestro transaction.

The regular workspace-local `.claude/settings.local.json` (Claude) or
`.codex/hooks.json` (Codex) must contain exactly one
Maestro-owned command for each canonical binding:

| Semantic event | Native event | Claude command suffix | Codex command suffix |
| --- | --- | --- | --- |
| `session_start` | `SessionStart` | `hook claude session-start` | `hook session-start --runtime codex` |
| `context_inject` | `UserPromptSubmit` | `hook claude context-injection` | `hook codex context-injection` |
| `pre_action_guard` | `PreToolUse` | `hook claude pre-action-guard` | `hook codex pre-action-guard` |
| `post_action_observe` | `PostToolUse` | `hook claude post-action-receipt` | `hook codex post-action-receipt` |
| `stop_finalize` | `Stop` | `hook claude stop-finalization` | `hook codex stop-finalization` |
| `subagent_start` | `SubagentStart` | `hook claude subagent-start` | n/a |
| `subagent_stop` | `SubagentStop` | `hook claude subagent-stop` | n/a |

Every command uses the exact installed CLI, the `--adapter-source maestro`
marker and `.bcgos/maestro-orchestration-state.json`. Timeout is five seconds;
Codex entries are synchronous. Claude `PostToolUse` is asynchronous and Claude
`Stop` is synchronous for route completion enforcement. Duplicate, legacy, mismatched or
Maestro-marked commands on another event fail closed. Unrelated user hooks are
preserved and ignored by the read-only check.

The orchestration pointer is operational rather than a configuration marker.
At hook time it resolves only to the exact relative path inside the canonical
initialized workspace; absolute paths, traversal, symlinked workspace/`.bcgos`
components, symlinked state files, oversized state and non-strict JSON fail
closed. The shared durable store is opened and validated for every installed
hook that reaches workspace-bound processing. Session/context observations and
post-action/stop receipts bind to a digest of the validated metadata-only
snapshot. A pre-action request that clearly crosses a protected mutation
boundary also validates and binds the snapshot before authorization; if that
protected request is unsafe or unevaluable, it is denied before any workspace
this workspace-bound path and remain with the host runtime's normal permission
flow.
None of these bindings authenticates an agent, changes an orchestration branch
or promotes native capability.

## Evidence boundary

The structured report uses `evidence_class=configured` and
`native_observation=not_observed`. Released Claude capabilities may be
`operational_beta`, configured, not adapter-observed and not native-qualified
in the embedded capability manifest. Codex remains unavailable. An incoherent
evidence claim makes the post-install check fail; this command cannot create
qualification evidence.

The CLI surface is:

```text
bcgos adapter verify --runtime <claude|codex> [workspace-path]
```

It emits one schema-versioned JSON report and exits non-zero on any failed
check. Unsupported runtimes, custom executable paths, missing install state,
missing or tampered files, missing installation dependencies, symlinks,
path/version drift and lifecycle mismatches are rejected without writes.
