# Maestro agent governance - visual guide

This guide shows the agent-orchestration foundation implemented in the managed
bundle and development harness. The catalog, definitions, validation and
Session Context Packet pointer exist today. Native Claude and Codex activation
does not: `agent_orchestration` remains fail-closed and `unavailable`.

## What was implemented

The user has one conversational surface. Maestro can select one governed branch
at a time, but has no filesystem, shell, web, messaging or external-system
tools. Darwin 🧬 is the one governance exception: a non-user-facing surgeon
with signed, reversible maintenance grants limited to `health/maestro-system`.
The catalog allows depth two only through named role edges.

```mermaid
flowchart TB
    User["User"] <--> Maestro["Maestro<br/>sole direct interface · no tools"]

    subgraph Core["Core governance leaves"]
        Walter["Walter<br/>pressure-test"]
        Darwin["Darwin 🧬<br/>governance surgeon · scoped health tools"]
        Errand["Errand helper<br/>basic · reversible"]
    end

    subgraph WorkspaceChain["Workspace or account chain"]
        Workspace["Workspace or account agent"] --> Capability["Capability specialist"]
    end

    subgraph PracticeChain["Practice chain"]
        Practice["Practice agent"] --> Subject["Subject specialist"]
    end

    Maestro --> Walter
    Maestro --> Darwin
    Maestro --> Errand
    Maestro --> Workspace
    Maestro --> Practice
```

The graph is a registration policy, not permission to run all branches. The
runtime contract remains one active branch, one active child per agent and
maximum depth two.

Headless housekeeping is not a new agent or a parallel taxonomy. The scheduler
invokes the same Darwin contract with `mode=headless_housekeeping`, the same
identity and grants, and the same metadata-only receipt path used by an
interactive health episode.

```mermaid
flowchart LR
    Packet["Bounded health packet"] --> Plan["Darwin 🧬 Plan"]
    Plan --> Interactive["interactive"]
    Plan --> Headless["headless_housekeeping"]
    Interactive --> Execute["same scoped Execute"]
    Headless --> Execute
    Execute --> Guard["shared fail-closed grants"]
    Guard --> Receipt["metadata-only receipt"]
    Guard -.-> Denied["client/workspace/release denied"]
```

## How sequential delegation works

A second chain starts only after the first one returns to Maestro. Walter may
review a material recommendation after specialist work because the specialist
branch is already closed.

```mermaid
sequenceDiagram
    actor User
    participant Maestro
    participant Owner as Owning agent
    participant Leaf as Allowed leaf specialist
    participant Walter

    User->>Maestro: request
    Maestro->>Owner: minimum work packet
    Owner->>Leaf: one bounded child task
    Leaf-->>Owner: result
    Owner-->>Maestro: scoped result and evidence
    Note over Maestro: active branch closed
    Maestro->>Walter: sealed review packet when material
    Walter-->>Maestro: approved or refine-and-return
    Maestro-->>User: final synthesis
```

## How workspace and practice knowledge stay separate

Workspace agents can receive authorized workspace context. Practice agents own
a professional subject canon and cannot read raw workspace context. If both
chains contribute, Maestro mediates a minimum sanitized packet between their
results; the agents never browse or message each other.

```mermaid
flowchart LR
    subgraph Private["Workspace-private boundary"]
        Raw["Raw workspace context"] --> Workspace["Workspace agent"]
    end

    Workspace --> Scoped["Scoped result"]
    Scoped --> Maestro["Maestro mediation"]
    Maestro --> Sanitized["Minimum sanitized packet"]

    subgraph Managed["Managed practice boundary"]
        Sanitized --> Practice["Practice agent"]
        Canon["Bounded professional canon"] --> Practice
    end

    Raw -.->|default deny| Practice
```

## What is live and what remains gated

The current implementation stops at runtime-neutral definitions and bounded
discovery. Runtime activation requires native adapter enforcement plus shared
conformance evidence.

```mermaid
flowchart LR
    Decision["Implemented<br/>decisions HUBS and BRCH"] --> Catalog["Implemented<br/>managed catalog and definitions"]
    Catalog --> Validation["Validated<br/>role graph and invariants"]
    Validation --> Packet["Implemented<br/>Session Context Packet pointer"]
    Packet --> Darwin["Implemented<br/>Darwin scoped contract + headless executor"]
    Darwin --> Adapter["Pending<br/>Claude and Codex native invocation"]
    Adapter --> Conformance["Pending<br/>cross-runtime fixtures"]
    Conformance --> Active["Pending<br/>runtime activation"]
    Packet -.->|current capability| Unavailable["Unavailable<br/>fails closed"]
```

## Evidence map

| Concern | Canonical evidence |
| --- | --- |
| Product and architecture decisions | `docs/decisions/decision-log.md` (`HUBS`, superseded by `BRCH`) |
| Role graph and limits | `bundles/base/agents/catalog.json` |
| Core definitions | `bundles/base/agents/maestro/AGENT.md`, `walter/AGENT.md`, `darwin/AGENT.md` |
| Deterministic validation | `internal/agentcatalog/`, `internal/dev/mermaiddoc/`, `dev/harness` |
| Darwin contract and receipts | `internal/darwin/`, `specs/018-maestro-core-agents.md`, `specs/032-canary-observability.md` |
| Session discovery | `specs/015-session-context-packet.md`, `internal/sessionctx/` |
| Runtime activation boundary | `specs/004-runtime-portability.md`, `adapters/claude/`, `adapters/codex/` |
| Reusable visual workflow | `dev/skills/visualize-change/` |

The canonical architecture contract remains
[`specs/018-maestro-core-agents.md`](../../specs/018-maestro-core-agents.md).
