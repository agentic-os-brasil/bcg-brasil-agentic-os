# Spec 030 - Claude lifecycle vertical

Status: implemented behind runtime-neutral contracts; pilot capability
promotion pending qualifying native-session evidence.

## Scope

This is Maestro's first complete product runtime lifecycle vertical. It maps
the canonical lifecycle to Claude Code without changing distribution,
workspace layout, federation, memory ingestion or worker execution.

| Canonical event | Claude native event | Inline behavior |
|---|---|---|
| `session_start` | `SessionStart` | bounded pointer-only Session Context Packet |
| `context_inject` | `UserPromptSubmit` | same bounded packet; prompt body is not persisted |
| `pre_action_guard` | `PreToolUse` | deterministic local denial; never grants permission |
| `post_action_observe` | `PostToolUse` | async metadata-only idempotent receipt |
| `stop_finalize` | `Stop` | async metadata-only idempotent receipt |

The workspace-local installer owns exactly these Claude entries. It preserves
all unrelated hook groups, corrects only Maestro-owned entries and removes only
those owned entries.

## Fail-closed guard

The pre-action guard reads at most 64 KiB and parses the native payload before
any workspace or owner inspection. A malformed or oversized payload still
returns a native Claude `PreToolUse` denial with an actionable reason and a
confirmation that nothing was changed. A valid payload with incomplete tool
metadata is handed to Claude's own permission flow; Maestro does not turn
missing metadata into a user-facing block.

The implemented policy denies only a recursive forced `rm` whose simple command
unambiguously targets `/` or the current home root. It canonicalizes the
explicit executable and target forms required by the policy, including
`/bin/rm`, balanced quoted roots, `/.`, `~`, `$HOME` and `${HOME}` variants.
The evaluator recognizes a deliberately small command grammar. In addition to
a single command, it accepts a quote-aware sequence of at most four commands
joined only by `&&`, evaluates every segment independently, and denies the
entire sequence if any segment removes a protected root. This keeps ordinary
forms such as `rm archived.md && echo ok` usable without weakening the protected
root boundary. It understands only the explicit HOME expansions above and
rejects other parameter expansions, globbing, every other shell operator,
substitutions, escapes and unbalanced quotes instead of claiming to be a
general shell parser when the command could be a removal. All other
successfully evaluated actions remain subject to Claude's own permission flow.

An installed guard may short-circuit workspace-state inspection only for a
closed, simple-command allowlist of read-only local BCGOS diagnostics: help,
version, doctor, product status, owner status/interview and owner onboarding
status. The executable may be the exact quoted installed path. Shell operators,
expansions, additional flags, mutation verbs and unknown forms do not enter the
exception. The short-circuit emits no allow decision and does not bypass the
runtime's own permission model; its only purpose is to keep diagnosis usable
when mutable orchestration state is absent or under repair.

"Installed path" is an identity check, not a basename check: the command must
name the same absolute path as the currently running BCGOS executable and the
resolved filesystem objects must match. A homonymous `bcgos` on `PATH`, a copy
under another directory, an arbitrary `.exe` path or a symlink spelling does
not qualify for the short-circuit and falls back to normal fail-closed state
validation.

## Non-blocking receipts

`PostToolUse` and `Stop` are configured with `async: true`. They emit one small
receipt without a worker lock, retry, network request, model call, source-body
read or prompt/tool payload persistence.

Receipts live under private local data at
`runtime/receipts/<opaque-workspace-id>/`. They retain only schema version,
runtime, canonical event, `adapter_command` provenance, timestamp, optional
validated tool name, closed diagnostic and a one-way idempotency digest. The
provenance proves only that the bounded adapter command ran, not that Claude
invoked it natively. Native session/tool-use IDs,
commands, tool output, prompts, workspace paths, owner context and client
content are not stored. Workspace IDs and filename components are validated
before path construction.

## Capability and parity rule

The capability manifest stays `unavailable`. Unit and configuration fixtures
prove the local contract, not that a qualifying Claude version trusted and
invoked each hook in a fresh native session. Only the pilot conformance protocol
in Spec 021 can supply that evidence and authorize a later capability
promotion. The executable cross-runtime matrix in Spec 035 keeps this
distinction testable.

Codex retains the same canonical event vocabulary and bounded serializers.
Its current runtime exposes all five command-hook surfaces, so the adapter
configures Session Start, prompt submission, pre-action, post-action and Stop.
All remain contract-tested but not natively observed; capabilities remain
unavailable rather than being represented as emulated or degraded.

Failure receipts are not claimed by this vertical: the installed native hooks
observe successful PostToolUse and Stop callbacks only. A later failure-hook
contract must add a producer and tests before `failed` diagnostics become part
of the lifecycle receipt vocabulary.
