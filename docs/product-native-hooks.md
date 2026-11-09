# Finite product hook adapter

The admitted `maestro-mcp` executable handles seven Claude Code events through
current runtime-neutral owners. This source delivery does not install hooks or
qualify a host. Complete carrier assembly and attended host qualification belong
to #495; installed-client acceptance remains with #469/#470.

## Invocation contract

Use the existing executable and explicit external selections:

```text
maestro-mcp hook --event EVENT --packet --product-root PRODUCT --user-home OWNER --workspace WORKSPACE
```

`--packet` validates the current complete canonical managed inventory and actual
executable before opening the owner/workspace or handling the event. Ordinary
non-packet invocations retain current admission. No hook initializes missing
state. Setup uses the existing explicit `init` operation separately.

A carrier must preserve argv boundaries and host substitutions; participant
owner/workspace selections stay external. No private path, state, prompt,
transcript, credential or native qualification receipt belongs in a managed
packet. #495 owns any selection fingerprint and host capability validation.

| Event | Matcher / supplied identity | Success | Failure exit |
| --- | --- | --- | --- |
| SessionStart | All session starts; session ID and absolute cwd | Pointer-only product context; read-only | 1 |
| UserPromptSubmit | Every prompt; optional actual prompt ID | Context and bounded occurrence state | 2 |
| PreToolUse | All tools, including Agent and Task; tool-use ID and object input | `{}` keeps host permissions, or explicit deny JSON | 2 for unavailable input/admission |
| SubagentStart | All subagents; actual agent ID and exact managed type | Observation context after sole pending target correlation | 1 |
| PostToolUse | All tools; tool-use ID and object input | `{}` after admitted observation | 1 |
| SubagentStop | All subagents; agent ID and explicit stop-hook boolean | `{}` after observation; internal empty type grants no authority | 2 |
| Stop | Every main stop; explicit stop-hook boolean | `{}` or bounded unresolved-route block JSON | 2 |

The stdin JSON must carry `hook_event_name` equal to EVENT, a bounded `session_id`
and absolute `cwd` inside the admitted workspace. The input limit is 64 KiB.
Optional `prompt_id` is the host's actual occurrence ID: absence reports
`turn_reset_unavailable` and preserves a conservative idempotent occurrence.
Prompt text never supplies identity or resets an unresolved route.

## Authority and effects

Each invocation revalidates product, owner and registered workspace. Agent/Task
predispatch reserves one canonical specialist; only the sole matching native
start can gain child authority. Six exact identities reuse the canonical
projector, optionally under `maestro-product:`. Case Agent has finite local file
tools; the other specialists are tool-free. Child delegation is refused.

One shared policy checks filesystem fields, prospective paths, patch targets,
Glob patterns and search trees for both the product adapter and specialist guard.
The ZIP guard delegates its existing case policy to the same owner. Grep content
regex remains text. Native Windows local paths and mixed separators are handled
as native paths; foreign drives, device/UNC/drive-relative/alternate-stream
spellings, outside targets, cross-case targets and ambiguous globs refuse.
Hook-time path checks do not provide filesystem race confinement for host tools.

Schema-2 route state is bounded by 8 KiB, 96 replay identities and 32 transitions;
current private file owners hold locks through validated atomic publication.
Existing schema-1 passive flow calls remain separate and cannot authorize a
schema-2 route. Receipts contain validated metadata, canonical types and hashed
identities; prompt, transcript, tool-result and assistant bodies are excluded.
Failed receipt recording grants no later child authority. Repeated admitted
observations are idempotent; changed/unsafe private state refuses without repair.

A stop observation proves no artifact completion, owning return verdict or
review approval. Stop recursion has no finalization effect. SessionStart,
SubagentStart and PostToolUse failure diagnostics do not claim host prevention.
Flags, booleans and serialized receipts do not self-qualify a host; plain MCP,
passive lifecycle observation and host-qualified native enforcement retain their
separate contracts.

## Evidence

The existing twelve compiled hook families exercise the actual public command,
including positive permission controls, read-only startup, malformed payloads,
paths, namespaces, replay, concurrency, privacy, changed bindings and stop
recursion. Their current-owner composition controls include complete packet
admission and all six canonical specialists. Native Windows execution and real
client enforcement require their own observed results; cross-compilation is
only build evidence.
