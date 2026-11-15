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

## Qualify the installed profile

Read the emitted `maestro-packet.json` first. `mcp_lifecycle` is the explicitly
configured three-event passive profile; successful context or Stop calls do not
establish full enforcement. The full `maestro_planner` profile has seven events
and its own exact-host review and qualification. `native_qualification` inside
adapter-command context is not a substitute for that manifest and proof.

Use the existing local `native-review` and explicitly confirmed
`native-qualify-and-build` workflow with the actual selected Code executable,
owner, workspace, new destination, private scratch and synthetic challenge
paths. Review reports the real host version/configuration directory and the
supported local source closure. It does not create a model session or modify
stored settings. A source, executable, settings, selection or profile change
requires a new review. No copied receipt, test flag or declared version can
qualify a host.

The finite installed-plugin profile supports one exact user-scoped
`maestro-product@<origin>` installation from the actual configuration's registry.
Its complete canonical packet, server digest, physical root and any bound
owner/workspace selection must match. Unknown IDs, additional physical
candidates, project/local installation scopes, discovered/synced plugins,
unsafe aliases and unsupported source controls remain explicit refusals.
Nonempty user/project/local hook groups outside the plugin are also unsupported;
the producer cannot close arbitrary external scripts from a settings hash.
Empty maps/event lists remain harmless. This is not general third-party plugin compatibility. The producer never changes
an owner's profile to make it pass.

If that verified installation would load alongside the new candidate, review
shows `session_settings_override` disabling only the installed ID for the three
qualification sessions. The candidate loads through its separate inline plugin
path. This override is local to those calls; no settings file is patched. The
review/receipt bind its digest and the exact source closure. Changed inputs
refuse before another session or final native packet assembly.

The `native-planner-v2` proof requires both an allowed Read and an allowed Write
with actual host tool results, canonical hook observations and exact physical
write readback. The new file control has one reviewed path and literal content;
the separate Edit control must remain unchanged when its missing, crashing or
timed-out guard blocks. Normal tool permissions still apply. A denied positive
control is an incomplete qualification, not permission to bypass the host.
The experiment's finite Read/Write/Edit/Agent tool set does not expand the
shipping specialist profiles. Its Case/Gamma dispatches must use the exact
`maestro-product:` IDs; a same-named project or user agent cannot qualify the
plugin. Shipping event normalization remains unchanged.

New native capability metadata records the qualification protocol and source/
session-settings digests. Passive, engineering-fixture and actual observed
native evidence remain distinct. These fields are construction/diagnostic
evidence, never an importable admission or permission grant. Provider estimates
and per-call caps remain estimates; qualification does not install or register
the resulting packet.

## Windows ownership boundary

The source includes a Windows launcher that associates each command with its
owned Job Object inside process creation, before child code executes. It limits
inherited handles to stdio, refuses unsupported association, bounds output and
waits for all owned members to exit after cancellation, output refusal or parent
exit. It does not kill a global process name or grant breakaway. Independently
created processes outside that job remain unattested.

New scratch/challenge directories reuse current-user private ACL and no-reparse
owners and retain their namespace for the qualification lifetime. Existing paths
are preserved. Managed-file inspection uses the platform's ClaudeCode system
location instead of assuming a Unix directory on Windows.

The reviewed [ordinary-account Windows proof](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/actions/runs/43695879338)
has passed the private-scratch and owned-process prerequisite, including
transient and late descendants. Its
source, sealed image, exact generic ZIP and actual non-admin token were verified;
a supplied flag, elevated skip or cross-build is insufficient. The platform-only
block is removed so a matching Windows host/server target can attempt the
existing review and qualification path. This does not qualify a Windows client:
actual authentication, finite source/profile closure, exact reviewed inputs and
all real native controls remain required before a native packet can be produced.
CLI and the Desktop Code tab require separate observations; a Desktop MCP
extension carries neither Code hooks nor native specialist activation.
