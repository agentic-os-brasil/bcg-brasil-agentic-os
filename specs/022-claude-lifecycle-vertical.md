# Spec 022 - Claude lifecycle vertical

Status: implemented behind runtime-neutral contracts; pilot capability promotion pending a qualifying native-session receipt.

## Scope

This is Maestro's first product runtime vertical. It maps the canonical lifecycle
to Claude Code without changing distribution, workspace layout, federation,
memory ingestion or worker execution.

| Canonical event | Claude native event | Inline behavior |
|---|---|---|
| `session_start` | `SessionStart` | bounded pointer-only Session Context Packet |
| `context_inject` | `UserPromptSubmit` | same bounded packet, no prompt body persisted |
| `pre_action_guard` | `PreToolUse` | deny only unambiguous `rm -rf /`, never grant permission |
| `post_action_observe` | `PostToolUse` | async, metadata-only idempotent receipt signal |
| `stop_finalize` | `Stop` | async, metadata-only idempotent receipt signal |

The adapter installer owns exactly these local Claude entries. It preserves all
unrelated hook groups and removes only Maestro-owned commands.

## Non-blocking boundary

`SessionStart` and `UserPromptSubmit` read only the last committed bounded
snapshot. `PreToolUse` evaluates an in-process rule; unknown actions are left
to Claude's own permission system. `PostToolUse` and `Stop` are configured as
asynchronous command hooks and write one small receipt with no locking,
retries, network, model call, source-body read or worker wait.

Receipts live in private local data under `runtime/receipts/<workspace-id>/`.
They retain runtime, canonical event, timestamp, optional tool name, opaque
idempotency key and safe diagnostic only. They never retain session IDs,
prompts, transcripts, commands, tool output, workspace paths, owner context or
client content. `bcgos doctor` reports receipt evidence separately from the
capability manifest.

## Capability rule

The capability manifest stays `unavailable` in this implementation. Unit and
conformance fixtures prove the contract and local adapter configuration; they
do not prove that a qualifying Claude Code version loaded the local settings
and invoked each hook in a real native session. Only the pilot conformance
protocol may promote capability state after such evidence exists.

## Codex parity

Codex retains the same canonical event vocabulary and conformance fixtures.
It has no product lifecycle bindings in this vertical, so its product
capabilities remain unavailable rather than being represented as native.
