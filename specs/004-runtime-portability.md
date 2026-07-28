# Spec 004 - Runtime portability

Status: direction accepted; canonical capability manifest and diagnostic report
implemented; Claude lifecycle wiring implemented with native evidence pending.

## Objective

Run the same BCG Brasil Agentic OS semantics on Claude and Codex while acknowledging differences in their native lifecycle, hook and configuration mechanisms.

Claude is the primary runtime and reference coverage target. The canonical architecture remains independent from both runtimes.

## Primary development runtime contract

Claude Code is the first-class contributor surface. This is an executable contract, not only a documentation preference:

- every canonical development skill under `dev/skills/` must have a native thin projection under `.claude/skills/`;
- `.claude/skill-routing.json` must route every canonical development skill to at least one contributor intent;
- `CLAUDE.md` must identify Claude as primary, load the routing contract and name every native `$skill`;
- Claude SessionStart must inject the effective golden path, while PostToolUse for Skill and UserPromptExpansion record successful automatic loading and direct `/skill-name` expansion respectively;
- PreToolUse must block Bash, Edit and Write when the required skill has not been recorded for that Claude session;
- Claude SessionStart, UserPromptExpansion, PreToolUse and PostToolUse hooks must remain structurally configured with their exact commands and matchers;
- the development harness and CI must reject drift in those requirements.

The project settings declare, and the interactive bootstrap detects, the minimum Claude Code version needed by this hook contract. If Go is missing, the clone is an unsupported state: hooks fail closed and direct the contributor back to external onboarding instead of promising an in-repository skill can repair the harness.

Codex compatibility remains required through the shared canonical contracts. It must not weaken or replace Claude's native discovery, routing or hook path.

## Canonical core

The shared core owns:

- agent and skill definitions;
- policies and governance rules;
- semantic lifecycle events;
- state, memory and observability schemas;
- injection precedence and context budgets;
- feature criticality and fallback behavior.

The core must not depend on Claude- or Codex-specific paths, hook names or payload formats.

The managed agent catalog under `bundles/base/agents/` defines Maestro, Walter,
Darwin and the lean delegation invariants. A catalog definition is not runtime
activation. `agent_orchestration` remains unavailable until an adapter proves
no-tool enforcement, one active branch, one child per agent and role-gated
depth-two delegation.

## Runtime adapters

Adapters own only:

- runtime and version discovery;
- installation and configuration wiring;
- translation between native hooks and semantic lifecycle events;
- payload normalization;
- capability reporting.

Adapters must not duplicate business rules or create competing state formats.

## Semantic lifecycle

The initial canonical event vocabulary is:

```text
session_start
pre_action_guard
post_action_observe
stop_finalize
context_inject
```

Exact names may evolve before implementation, but adapters must map to one shared vocabulary.

## Capability states

Each runtime capability is reported as one of:

- `native`: the runtime provides the mechanism directly;
- `emulated`: the adapter reproduces the invariant through another reliable mechanism;
- `degraded`: the feature remains useful but has a documented behavioral gap;
- `unavailable`: the invariant cannot be provided safely.

Critical security or governance capabilities cannot degrade silently. When they are unavailable, initialization or execution must fail closed or declare the workspace unsupported.

## Executable compatibility matrix

A versioned manifest is the single source for capability ID, semantic contract, criticality, required mechanisms, compatibility range and fallback behavior. `bcgos init`, `bcgos doctor` and startup derive their reports from this manifest.

Every canonical capability requires conformance fixtures against both adapters. Documentation may render the matrix, but must not become a separately maintained source of truth.

## Initial implementation

`bundles/base/runtime/capabilities.json` is the versioned source of truth for
the current capability inventory. `schemas/runtime-capabilities.schema.json`
defines its portable shape, and `internal/runtimecap` validates and reports the
same capability IDs, semantic events and criticality for Claude and Codex.
`bcgos doctor` reports the detected runtime plus its derived capability view.

The Claude and Codex adapters map the complete initial lifecycle behind the
neutral contract and record metadata-only local delivery receipts. Codex uses
its native command-hook surface for the same five semantic events. Every
product event remains `unavailable` in the capability manifest until the
relevant thin adapter and qualifying native-session conformance evidence exist;
local configuration, direct-command tests and development hooks are never
evidence of product hook support.

Every future product adapter must also conform to the non-blocking execution
policy in Spec 019. A native lifecycle mechanism may translate and emit a
semantic event, but it may not turn the hook into a synchronous worker.

## Update guarantees

CLI, bundle and adapter versions declare compatible ranges. `bcgos update` must reject incompatible combinations, preserve the active version and provide rollback.
