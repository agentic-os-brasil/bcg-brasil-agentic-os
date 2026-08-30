# Spec 035 - Lifecycle evidence matrix

Status: Claude lifecycle and managed subagents are operational for the
controlled beta and natively qualified for the exact v0.1.13 macOS-arm64 /
Claude Code 2.1.203 artifact tuple recorded below. Codex's five lifecycle events
are natively qualified for the exact v0.1.13 macOS-arm64 / Codex CLI 0.149.1 /
`gpt-5.6-sol` tuple recorded below; other tuples remain unavailable.

## Objective

Make runtime lifecycle status auditable without treating a configuration file,
unit test, direct hook command or local receipt as proof that Claude or Codex
invoked Maestro inside a native session.

`adapters/conformance/lifecycle.json` is the executable contract matrix. Its
tests bind each canonical event to the capability manifest and reject an
unbacked aggregate `native_qualified` claim. Fresh tuple-specific native
evidence remains separate from the portable default manifest.

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
false. The portable Codex manifest remains conservative even though the
separate tuple-specific record below is native-qualified.

## Current matrix

| Semantic event | Claude binding | Claude evidence | Codex binding | Codex evidence |
| --- | --- | --- | --- | --- |
| `session_start` | `SessionStart`, pointer-only packet | native-qualified for recorded macOS tuple | `SessionStart`, pointer-only packet | jointly native-qualified with `context_inject` for recorded macOS tuple |
| `context_inject` | `UserPromptSubmit`, pointer-only packet | native-qualified for recorded macOS tuple | `UserPromptSubmit`, pointer-only packet | jointly native-qualified with `session_start` for recorded macOS tuple |
| `pre_action_guard` | `PreToolUse`, bounded deterministic deny | native-qualified for recorded macOS tuple | `PreToolUse`, bounded deterministic deny | native-qualified for recorded macOS tuple |
| `post_action_observe` | async `PostToolUse`, metadata-only receipt | native-qualified for recorded macOS tuple | `PostToolUse`, metadata-only receipt | native-qualified for recorded macOS tuple |
| `stop_finalize` | synchronous `Stop`, metadata-only receipt plus native-agent completion gate | native-qualified for recorded macOS tuple | `Stop`, metadata-only receipt | native-qualified for recorded macOS tuple |
| `subagent_start` | `SubagentStart`, admission and bounded context | native-qualified for recorded macOS tuple | none | not implemented |
| `subagent_stop` | `SubagentStop`, deterministic transition | native-qualified for recorded macOS tuple | none | not implemented |

## Probe

Run the development-only environment probe before a native-session trial:

```text
go run ./dev/lifecycle-probe --runtime claude
go run ./dev/lifecycle-probe --runtime codex
```

The probe parses only a semantic version from stdout; stderr warnings are not
part of `runtime_version`. A detected executable alone is not an aggregate
readiness claim. Exact Claude projection reports `operational_beta`;
qualification remains visible and independent. Codex's portable capability
state stays conservative despite the separate passing tuple-specific record.

It reads only the local executable path and `--version` under a two-second
budget. It starts no model session, changes no runtime configuration, writes no
receipt and cannot modify the capability manifest. It also reports each
canonical event's binding, evidence class and blocker. A result of `blocked` or
`not_observed` is evidence of a limitation, not a product failure to hide.

For Claude, the current lifecycle contract requires at least `2.1.177` before
a native-session trial may begin. Codex requires at least `0.144.1`; the
recorded trial observed `0.149.1` exposing all five configured command-hook
events. The probe itself never promotes an event without fresh native-session
evidence.

## Native trial protocol

When a qualifying runtime is available, follow Spec 021 for each runtime and
platform. Preserve only version/OS identity, local configuration identity,
direct command result, bounded native-session observation and removal result.
Do not record prompts, source bodies, client material, workspace paths, native
session IDs, tool arguments or outputs. An adapter-command receipt may be
attached as supporting diagnostics but is never the native observation.

For the Claude macOS-arm64 direct-workspace slice, `go run
./dev/native-qualification --artifact <platform-zip>` automates that protocol
against disposable Hub and synthetic-repository fixtures. Its report is the
reviewable matrix record; its source-level tests are only contract evidence and
cannot substitute for executing the command with the real runtime. Codex Doctor
evidence requires a successful canonical read of the projected skill and an
exact installed-CLI version result. Qualification and fixture-cleanup errors
invalidate a passing result before report serialization.

The development suite pairs the newest bounded Claude and Codex records for the
current `VERSION`. Both must qualify the same release, OS, architecture and
artifact SHA-256, and both must pass the shared direct-workspace checks. This
gate deliberately does not equate runtime-specific native extensions with
shared semantics.

The shared checks include transparent identity: affirmative native output must
name both Maestro and the correct host runtime. Refusal, prompt-injection or
concealment language fails qualification even if it quotes the word `Maestro`.
One-sided removal of that check is rejected by the parity tests.

The current passing bounded record is
`docs/evidence/claude-direct-macos-arm64-0.1.13-2026-08-28.json`. It qualifies
only Claude Code 2.1.203 on darwin/arm64 for the named artifact SHA-256. The
Codex record is
`docs/evidence/codex-direct-macos-arm64-0.1.13-2026-08-28.json`; it qualifies
only Codex CLI 0.149.1 with model `gpt-5.6-sol`, the recorded post-initialization
isolated runtime configuration digest, darwin/arm64 and artifact SHA-256
`a93d5fb658c1908c9e2642f5d6dd53b1c2a3e7184da735e65da178ed94993d92`.
The portable capability manifest remains conservative because it also covers
unobserved runtime/model/configuration/platform/artifact tuples.
