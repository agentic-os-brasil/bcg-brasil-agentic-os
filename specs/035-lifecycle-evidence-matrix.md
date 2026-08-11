# Spec 035 - Lifecycle evidence matrix

Status: Claude lifecycle and managed subagents are operational for the
controlled beta; native qualification remains telemetry. Codex remains
configured but unavailable pending its own activation path.

## Objective

Make runtime lifecycle status auditable without treating a configuration file,
unit test, direct hook command or local receipt as proof that Claude or Codex
invoked Maestro inside a native session.

`adapters/conformance/lifecycle.json` is the executable matrix. Its tests bind
each canonical event to the capability manifest and reject a false
`native_qualified` claim while fresh native evidence is absent.

## Evidence classes

The executable matrix uses four promotion classes, in ascending order:
`configured`, `contract-tested`, `adapter-observed` and `native-qualified`.
`configured` proves only topology, `contract-tested` adds direct deterministic
tests, and `adapter-observed` proves that the bounded Maestro command emitted
an `adapter_command` receipt. Only `native-qualified` may support a native
qualification claim, and it must carry a reproducible fresh-session
observation from the runtime itself. Operational availability is a separate
product state. `blocked` is an implementation state for a missing native
surface, not an evidence class.

Lifecycle envelopes make the same boundary machine-readable. The additive
`adapter_delivery_state` says whether the value is only a contract or whether
the adapter serializer emitted its bounded payload. The existing
`injection_state` continues to represent native qualification and therefore
remains `unavailable` until the pilot protocol is complete.

| Class | What it proves | What it does not prove |
| --- | --- | --- |
| Local configuration | The workspace contains Maestro-owned bindings with expected timeout/async settings. | Runtime trust or invocation. |
| Direct contract / harness | Serializers, guard, bounded outputs and metadata-only receipts satisfy their local contract. | A native hook executed. |
| Adapter-command receipt | The Maestro adapter command emitted a receipt with `provenance=adapter_command`. | Native runtime origin; the command can be invoked directly. |
| Native-session observation | A fresh runtime session invoked the exact installed command and surfaced its bounded result. | Cross-platform qualification by itself. |
| Qualified capability evidence | The complete runtime/platform pilot record required by Spec 021. | Future version compatibility. |

Only the last class can set `native_qualified=true`. The Claude manifest uses
`operational_beta` for released behavior while keeping that evidence bit
false; Codex remains `unavailable` in this version.

## Current matrix

| Semantic event | Claude binding | Claude evidence | Codex binding | Codex evidence |
| --- | --- | --- | --- | --- |
| `session_start` | `SessionStart`, pointer-only packet | operational beta; qualification telemetry | `SessionStart`, pointer-only packet | contract-tested; native pending |
| `context_inject` | `UserPromptSubmit`, pointer-only packet | operational beta; qualification telemetry | `UserPromptSubmit`, pointer-only packet | contract-tested; native pending |
| `pre_action_guard` | `PreToolUse`, bounded deterministic deny | operational beta; qualification telemetry | `PreToolUse`, bounded deterministic deny | contract-tested; native pending |
| `post_action_observe` | async `PostToolUse`, metadata-only receipt | operational beta; qualification telemetry | `PostToolUse`, metadata-only receipt | contract-tested; native pending |
| `stop_finalize` | synchronous `Stop`, metadata-only receipt plus native-agent completion gate | operational beta; native qualification is telemetry | `Stop`, metadata-only receipt | contract-tested; native pending |
| `subagent_start` | `SubagentStart`, admission and bounded context | operational beta; qualification telemetry | none | not implemented |
| `subagent_stop` | `SubagentStop`, deterministic transition | operational beta; qualification telemetry | none | not implemented |

## Probe

Run the development-only environment probe before a native-session trial:

```text
go run ./dev/lifecycle-probe --runtime claude
go run ./dev/lifecycle-probe --runtime codex
```

The probe parses only a semantic version from stdout; stderr warnings are not
part of `runtime_version`. A detected executable alone is not an aggregate
readiness claim. Exact Claude projection reports `operational_beta`;
qualification remains visible and independent. Codex remains
`capabilities_unavailable` until activated.

It reads only the local executable path and `--version` under a two-second
budget. It starts no model session, changes no runtime configuration, writes no
receipt and cannot modify the capability manifest. It also reports each
canonical event's binding, evidence class and blocker. A result of `blocked` or
`not_observed` is evidence of a limitation, not a product failure to hide.

For Claude, the current lifecycle contract requires at least `2.1.177` before
a native-session trial may begin. Codex `0.144.1` exposes the five command-hook
events and the adapter configures all five. The probe keeps every Codex event
unqualified until a fresh native-session observation is captured.

## Native trial protocol

When a qualifying runtime is available, follow Spec 021 for each runtime and
platform. Preserve only version/OS identity, local configuration identity,
direct command result, bounded native-session observation and removal result.
Do not record prompts, source bodies, client material, workspace paths, native
session IDs, tool arguments or outputs. An adapter-command receipt may be
attached as supporting diagnostics but is never the native observation.
