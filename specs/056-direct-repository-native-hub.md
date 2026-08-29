# Spec 056 - Direct repository native Hub

Status: accepted for implementation and local contract validation. Native Claude
and Codex qualification remains separate evidence.

## Objective

An enrolled repository or worktree must expose Maestro through a host-native,
trusted main-session surface instead of asking a SessionStart hook to establish
an operating identity. This is a direct-entry adapter change only. Opening the
Maestro Hub folder continues to use the existing product path unchanged.

Maestro remains the runtime-neutral control plane: canonical skills, policies,
state, memory, lifecycle and specialist topology remain product authorities.
The projected native Hub is their conversational frontend, not a new source of
truth and not a sixth specialist.

## Claude direct projection

Scoped Claude installation projects `.claude/agents/maestro-hub.md` and selects
`maestro-hub` as the repository's native main agent. The managed definition:

- states transparently that Claude Code is the host and Maestro is the governed
  operating layer;
- preloads the canonical `maestro-operator` skill;
- inherits the host's ordinary tool surface so direct repository work does not
  lose Git, terminal, file or future host capabilities;
- may route to the existing Client Account, Case, Yoda, Darwin and PA Expert
  agents through the main thread; and
- does not grant specialists recursive delegation or widen any hook policy.

The definition is generated from a canonical bundled contract. Adapter-local
prompt text is not an independent policy authority.

Only scoped repository/worktree installation selects this main agent. Ordinary
Claude adapter installation used by the Maestro Hub must neither create the
file nor set the native `agent` option.

## Codex direct projection and parity

Codex consumes the same canonical operating contract through its native thin
adapter. Its host mechanism need not have the same filename or configuration
shape as Claude. Observable parity is semantic:

- the same Maestro capability and policy identifiers;
- the same owner, workspace, privacy and external-effect boundaries;
- the same five specialist roles and routing invariants;
- the same factual lifecycle state; and
- the same truthful disclosure of Maestro and the named host runtime.

Contract tests derive expectations from the canonical manifest and fail if a
capability, role, policy or lifecycle invariant is projected for only one
runtime. Host-specific features may differ only when explicitly recorded by a
runtime capability state, never through silent omission.

## Hook boundary

Direct SessionStart and prompt hooks carry bounded facts and pointers: opaque
workspace identity, enrolled root, lifecycle state, authorized owner context,
memory and selected skill references. They do not tell the model who to pretend
to be, ask it to conceal its provider or architecture, require silent command
execution, or embed the complete stable operating manual.

PreToolUse, PostToolUse, Stop, SubagentStart and SubagentStop retain their
existing deterministic enforcement and lifecycle responsibilities. Moving the
conversation contract out of SessionStart does not weaken Git guards,
protected-root checks, external-action confirmation, workspace isolation,
agent-flow enforcement or metadata-only receipts.

For Claude direct mode, tracked `CLAUDE.md` preservation no longer causes the
full canonical orientation to be copied into SessionStart; the native frontend
and preloaded method carry stable policy. Codex may retain a host-appropriate
compact orientation projection until it has an equivalent native main-session
channel, provided the complete hook context remains within 8 KiB and parity and
transparency remain proven. A tracked `AGENTS.md` never causes the complete
orientation document to be repeated through SessionStart or prompt hooks.

## Coexistence and ownership

If Claude's local settings already select a different main agent, enroll and
repair fail before their first write, explain that no work was lost and provide
one safe next command. Maestro never overwrites that scalar setting.

Installation is idempotent when the selected agent is `maestro-hub` and every
managed digest agrees. Removal deletes the managed definition and native
selection only while both remain Maestro-owned. Unrelated settings, hooks,
agents and user bytes survive exactly. If the user changes the selection after
enrollment, removal preserves the new value and removes only still-owned
surfaces. Transaction snapshots and rollback include the native Hub definition
and selection.

Claude and Codex may coexist in one checkout. Removing either runtime preserves
shared state and every projection still referenced by the other.

## Required evidence

Implementation starts with failing observable tests for:

1. scoped Claude install creates and selects the native Hub while ordinary Hub
   install does not;
2. an existing different main agent blocks before mutation;
3. repair, rollback and removal preserve user-owned configuration;
4. direct Claude SessionStart is factual and excludes imperative identity,
   concealment, silent-command and tracked-orientation injection;
5. the existing Maestro-folder SessionStart contract remains unchanged;
6. Claude and Codex projections fail validation on one-sided capability,
   specialist, policy or lifecycle drift; and
7. worktree identity and dual-runtime reference-aware removal remain intact.

The development gate is `go run ./dev/harness validate --full`. That proves
repository contracts only. Release readiness additionally requires fresh
native sessions covering tools, governed skills, routing, hooks, resume,
compaction and distinct Git worktrees for both supported hosts.

## Out of scope

- replacing the Maestro control plane with an autonomous agent;
- changing the Maestro-folder Hub experience;
- allowing specialists to spawn subagents;
- global Claude or Codex configuration;
- importing or relocating repositories; and
- claiming native or release qualification from local tests alone.
