# Spec 035 - Lifecycle evidence matrix

Status: Claude configuration and direct-contract harness implemented; Codex
Session Start configuration implemented; native runtime evidence unavailable.

## Objective

Make runtime lifecycle status auditable without treating a configuration file,
unit test, direct hook command or local receipt as proof that Claude or Codex
invoked Maestro inside a native session.

`adapters/conformance/lifecycle.json` is the executable matrix. Its tests bind
each canonical event to the capability manifest and reject a promotion while
native evidence is absent.

## Evidence classes

| Class | What it proves | What it does not prove |
| --- | --- | --- |
| Local configuration | The workspace contains Maestro-owned bindings with expected timeout/async settings. | Runtime trust or invocation. |
| Direct contract / harness | Serializers, guard, bounded outputs and metadata-only receipts satisfy their local contract. | A native hook executed. |
| Adapter-command receipt | The Maestro adapter command emitted a receipt with `provenance=adapter_command`. | Native runtime origin; the command can be invoked directly. |
| Native-session observation | A fresh runtime session invoked the exact installed command and surfaced its bounded result. | Cross-platform qualification by itself. |
| Qualified capability evidence | The complete runtime/platform pilot record required by Spec 021. | Future version compatibility. |

Only the last class can support a later capability-manifest promotion. The
manifest remains `unavailable` for all lifecycle events in this version.

## Current matrix

| Semantic event | Claude binding | Claude evidence | Codex binding | Codex evidence |
| --- | --- | --- | --- | --- |
| `session_start` | `SessionStart`, pointer-only packet | configuration + direct contract; native pending | `SessionStart`, pointer-only packet | configuration + direct contract; blocked until a complete Codex lifecycle contract exists |
| `context_inject` | `UserPromptSubmit`, pointer-only packet | configuration + direct contract; native pending | none | blocked: no product binding |
| `pre_action_guard` | `PreToolUse`, bounded deterministic deny | configuration + direct contract; native pending | none | blocked: no product binding |
| `post_action_observe` | async `PostToolUse`, metadata-only receipt | configuration + direct contract; native pending | none | blocked: no product binding |
| `stop_finalize` | async `Stop`, metadata-only receipt | configuration + direct contract; native pending | none | blocked: no product binding |

## Probe

Run the development-only environment probe before a native-session trial:

```text
go run ./dev/lifecycle-probe --runtime claude
go run ./dev/lifecycle-probe --runtime codex
```

It reads only the local executable path and `--version` under a two-second
budget. It starts no model session, changes no runtime configuration, writes no
receipt and cannot modify the capability manifest. A result of `blocked` or
`not_observed` is evidence of a limitation, not a product failure to hide.

For Claude, the current lifecycle contract requires at least `2.1.177` before
a native-session trial may begin. Codex has a locally configured Session Start
command, but this is not a supported native-trial path on its own: the probe
remains blocked until the missing product lifecycle bindings are implemented.

## Native trial protocol

When a qualifying runtime is available, follow Spec 021 for each runtime and
platform. Preserve only version/OS identity, local configuration identity,
direct command result, bounded native-session observation and removal result.
Do not record prompts, source bodies, client material, workspace paths, native
session IDs, tool arguments or outputs. An adapter-command receipt may be
attached as supporting diagnostics but is never the native observation.
