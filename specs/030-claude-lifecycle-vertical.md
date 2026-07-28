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
any workspace or owner inspection. A malformed or oversized payload, missing
guard field or command-evaluation failure returns a native Claude
`PreToolUse` denial with an actionable reason and a confirmation that nothing
was changed. Workspace state can neither bypass nor delay this safety decision.

The implemented policy denies only a recursive forced `rm` whose simple command
unambiguously targets `/` or the current home root. It canonicalizes the
explicit executable and target forms required by the policy, including
`/bin/rm`, balanced quoted roots, `/.`, `~`, `$HOME` and `${HOME}` variants.
The evaluator recognizes a deliberately small simple-command grammar; it
understands only the explicit HOME expansions above and rejects other parameter
expansions, globbing, shell operators, substitutions, escapes and unbalanced
quotes instead of claiming to be a general shell parser. All other
successfully evaluated actions remain subject to Claude's own permission flow.

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

Codex retains the same canonical event vocabulary and Session Start serializer.
Its adapter registry marks Session Start as configured and contract-tested but
not natively observed; context injection, pre-action, post-action and Stop are
explicitly blocked because the current runtime exposes no verified product
surface for them. Its capabilities remain unavailable rather than being
represented as emulated or degraded.

Failure receipts are not claimed by this vertical: the installed native hooks
observe successful PostToolUse and Stop callbacks only. A later failure-hook
contract must add a producer and tests before `failed` diagnostics become part
of the lifecycle receipt vocabulary.
