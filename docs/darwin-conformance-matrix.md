# Darwin 🧬 conformance matrix

This matrix separates the deterministic contract from runtime-native evidence.
Headless housekeeping is a mode of Darwin, not a second agent.

```mermaid
flowchart LR
    Packet["bounded health packet"] --> Identity["darwin · 🧬"]
    Identity --> Scope["health/maestro-system grants"]
    Scope --> Plan["deterministic plan"]
    Plan --> Modes{"interactive or headless_housekeeping"}
    Modes --> Execute["same fail-closed executor"]
    Execute --> Receipt["metadata-only receipt"]
    Receipt --> Native{"native session evidence?"}
    Native -->|no| Unavailable["runtime capability unavailable"]
    Native -->|yes| Qualified["candidate for qualification"]
```

| Implemented contract | Local evidence | Native evidence | State |
| --- | --- | --- | --- |
| Stable identity and emoji | `internal/darwin/contract.go`, catalog and identity tests | Claude/Codex native agent identity receipt | implemented / native unavailable |
| Registered scoped grant | `internal/agentcatalog`, `internal/agentorchestration/controller_test.go` | Native adapter grant receipt | contract-tested / native unavailable |
| No forged identity or target | shared adapter adversarial fixtures | Native event observation | contract-tested / native unavailable |
| One branch, no Darwin delegation | shared controller and Darwin grant tests | Native lifecycle event sequence | contract-tested / native unavailable |
| Same interactive/headless contract | `internal/darwin/runtime.go`, `contract_test.go`, conformance fixture | Native scheduler wake invoking Darwin | implemented / native unavailable |
| Metadata-only repair receipt | `internal/darwin/store.go`, CLI test | Native session receipt | contract-tested / native unavailable |
| Recoverable failed housekeeping | scheduler `RunDue` contract plus Darwin store receipt | Native scheduler retry evidence | implemented / native unavailable |
| Bounded worker command and lease | `internal/maintenance`, `internal/scheduler/lease.go`, cadence fixture | Native wake invoking worker | contract-tested / native unavailable |
| Daily, weekly and monthly cadence | scheduler cadence tests and maintenance catalog | Native scheduler execution | contract-tested / native unavailable |
| Continuous event gate | maintenance command/gate tests; signal-only fixture | Native lifecycle event observation | contract-tested / native unavailable |
| Monthly structural evolution | proposal-only command receipt; no tool invocation | Approved human application record | proposal-only / application unavailable |
| Claude native invocation | adapter docs and lifecycle probe | qualifying Claude session | unavailable: native observation pending |
| Codex native invocation | adapter docs and lifecycle probe | qualifying Codex session | unavailable: native observation pending |

No row is promoted by configuration, unit tests or an adapter-command receipt
alone. Native qualification requires an observed session in the target runtime.

## Local invocation

The deterministic CLI exposes the same contract for an operator or a headless
runner:

```sh
bcgos agent darwin assess --stdin
bcgos agent darwin housekeeping --stdin
```

Housekeeping requires three separately supplied capability values through
`BCGOS_MAESTRO_CAPABILITY`, `BCGOS_DARWIN_CAPABILITY` and
`BCGOS_RECOVERY_CAPABILITY`. They authenticate the branch and recovery path;
they are never accepted in the health packet and never appear in a receipt.
