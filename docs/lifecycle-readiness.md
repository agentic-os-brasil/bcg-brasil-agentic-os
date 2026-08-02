# Lifecycle readiness audit

This is the current evidence boundary for Claude and Codex lifecycle hooks.
It deliberately separates repository configuration, direct contract tests,
adapter-command receipts and native-session qualification. No row below
promotes a capability by implication.

## Evidence snapshot

| Field | Value |
| --- | --- |
| `as_of` | `2026-08-02` |
| `base_commit` | `03fe7a0bdcb12bf6fbab693fa8e5fca418b160b3` — PR #150 head; this documentation follow-up is stacked on it |
| Repository evidence | Configured adapters, local contract fixtures and the non-invasive lifecycle probe are present; no model session was started for this documentation update. |
| Runtime evidence | Claude `2.1.119` (below the `2.1.177` floor) and Codex `0.144.1` are the recorded local versions; no fresh native-session observation is attached for either runtime. |
| Scheduler evidence | No live `launchctl` observation is attached. Filesystem/plist installation and scheduler loaded/enabled state remain separate claims. |
| Release/pilot evidence | No signed artifact, clean-device acceptance, support/incident owner or pilot-gate record is present in this snapshot. |

`configured` means that wiring is rendered or installed. `local
contract-tested` means repository behavior and fixtures cover the boundary.
`native-qualified` requires fresh supported-runtime observation. Neither
`release-ready` nor `pilot-ready` follows from either of the first two labels.

```mermaid
flowchart LR
    Configured --> ContractTested["contract-tested"]
    ContractTested --> AdapterObserved["adapter-observed"]
    AdapterObserved --> NativeQualified["native-qualified"]
    NativeQualified --> Promote["capability may be promoted"]
    Blocked["blocked / unavailable"] -.->|no silent fallback| Configured
```

## Current matrix

| Event | Claude 2.1.119 | Codex 0.144.1 | Native promotion blocker |
| --- | --- | --- | --- |
| `session_start` | configured + local contract-tested; native-qualified: no, version below floor | configured + local contract-tested; native-qualified: no, observation not captured | Fresh native-session observation from a qualifying runtime |
| `context_inject` | configured + local contract-tested; native-qualified: no, version below floor | configured + local contract-tested; native-qualified: no, observation not captured | Claude version; fresh Codex native observation |
| `pre_action_guard` | configured + local contract-tested; native-qualified: no, version below floor | configured + local contract-tested; native-qualified: no, observation not captured | Claude version; fresh Codex native observation |
| `post_action_observe` | configured async + local contract-tested; native-qualified: no, version below floor | configured + local contract-tested; native-qualified: no, observation not captured | Claude version; fresh Codex native observation |
| `stop_finalize` | configured async + local contract-tested; native-qualified: no, version below floor | configured + local contract-tested; native-qualified: no, observation not captured | Claude version; fresh Codex native observation |

The canonical manifest remains `unavailable` for every lifecycle capability.
The local probe reports the same matrix without starting a model session,
writing a receipt or changing runtime configuration.

## Readiness status

| Readiness class | Snapshot status |
| --- | --- |
| Configured | Yes — workspace-local adapter wiring is represented. |
| Local contract-tested | Repository fixtures and deterministic boundaries are present; this update did not rerun them, so no fresh test pass is claimed. |
| Native-qualified | No — no fresh qualifying native-session observation for Claude or Codex. |
| Release-ready | No — signing, publication and release-gate evidence are absent. |
| Pilot-ready | No — clean-device, support/incident ownership and pilot-gate evidence are absent. |

## Audit findings

- Claude is detected at `2.1.119`; the lifecycle contract requires `2.1.177`.
  No Claude native trial is eligible until the runtime is upgraded.
- Codex is detected at `0.144.1`; its stable hooks feature exposes the five
  command-hook events. The adapter configures all five, but none is promoted
  without fresh native-session observation.
- `adapter_command` receipts remain diagnostic only. They do not become native
  evidence and cannot change the manifest.
- `.codex/RUNTIME-CONTRACT.md` and `.codex/CODEX-RUNTIME.md` are not present in
  the snapshot commit; their absence is recorded as a documentation gap rather
  than replaced by an invented runtime contract.
- PA Expert stubs are unrelated consultative components and are not lifecycle
  dependencies.

## Next qualifying evidence

1. Upgrade Claude to at least `2.1.177`, install the exact workspace-local
   bindings, and capture a fresh native observation for all five events.
2. Capture native Codex observations for all five configured events, including
   trust review for the workspace-local hook definitions.
3. Promote a capability only from a reviewed `native-qualified` record with
   runtime/platform identity and bounded event evidence.

Until then, readiness remains contract-validated only; neither runtime is
pilot-ready for lifecycle activation.

Walter has a separate native qualification recipe in
[`docs/walter-native-qualification.md`](walter-native-qualification.md). It
must be completed for the shared Claude/Codex handler and installation-scoped
review custody before `walter_review` or `agent_orchestration` is promoted.

## Darwin cadence status

Darwin's runtime-neutral worker contract now has explicit command deadlines,
locally qualified catalog/attendance test paths, exact occurrence binding,
non-blocking occurrence-keyed fenced execution, immutable attempt receipts with
occurrence-level idempotency, continuous/event gatekeeping and
daily/weekly/monthly cadence fixtures. Busy is an ephemeral nonterminal result,
and the shipped catalog-only/unavailable catalog cannot authorize execution.
These are local contract evidence only. Claude and Codex remain `unavailable`;
macOS and Windows scheduler templates remain disabled. A live macOS scheduler
observation would be a separate scheduler gate, not lifecycle native
qualification.
