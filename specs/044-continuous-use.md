# Spec 044 - Maestro continuous use

Status: deterministic local projection, CLI status and lifecycle wiring
implemented and locally contract-tested. Native Claude/Codex qualification and
Windows maintenance activation remain unavailable.

## Objective

Make Maestro useful after installation and across session boundaries without
turning the prompt, transcript or a new continuity store into authority.

The observable loop is:

```text
configure -> orient -> work -> capture bounded signal -> checkpoint explicitly
          -> stop with metadata only -> re-orient -> resume safely
```

## Authority boundary

CONTINUOUS USE is a read-only projection. It does not own a second state
machine and never repairs or advances its source authorities while rendering
status:

- Owner Context owns onboarding and calibration state;
- the Execution Ledger owns active work, bounded checkpoint bodies, fencing and
  completion;
- the memory engine owns attested capture-v2 inputs and generated commits;
- the maintenance plane owns checkpoint/light-dream occurrences and receipts;
- runtime projection and lifecycle receipts own configured and
  adapter-observed evidence;
- the native-session qualification protocol alone owns `native-qualified`.

No prompt, transcript, objective, checkpoint body, tool payload, error body,
client name, path or generated output enters the continuous-use projection.

## Projection contract

`bcgos maestro status [workspace]` returns one versioned projection containing:

- calibration state and track, with one deterministic next action;
- open-work state: `available`, `unavailable` or `ambiguous`, plus only the
  portable `bcgos://execution/active` pointer when unambiguous;
- work state and checkpoint presence without item, attempt, objective, summary
  or artifact content;
- generated-memory state and the count of HMAC-attested capture-v2 files;
- maintenance enrollment/observation state without paths or receipt bodies;
- one evidence row per runtime with separate `configured`,
  `adapter_observed`, `native_qualified` and `unavailable` fields; and
- a bounded ordered list of closed next-action IDs and safe CLI commands.

Top-level `bcgos status` embeds the same projection rather than inventing a
coarser capability label. Corrupt or ambiguous source state fails closed and
becomes an explicit unavailable/action-required result.

## Lifecycle behavior

Session Start reads the projection only. It may state onboarding/calibration,
open-task count, one active-work/checkpoint state, current generated-memory
availability and the first safe next action. It never resolves the execution
pointer or injects a checkpoint body automatically.

When exactly one active item has a checkpoint, the runtime may explicitly run
`bcgos work next --active --workspace <workspace>` and then resume with a new
fenced attempt. When an active item has no checkpoint, the projection requires
an explicit bounded `bcgos work checkpoint` before a durable handoff; a hook
cannot fabricate that summary.

`UserPromptSubmit` preserves the existing pointer-only packet and may append
only HMAC-attested selected-skill IDs to capture-v2. `PostToolUse` and `Stop`
persist only the existing allowlisted adapter-command lifecycle receipts.
Every hook remains non-blocking and performs no model/network call, rollup,
checkpoint synthesis, wiki work or worker wait.

## Evidence vocabulary

- `configured`: the exact managed runtime projection and five lifecycle
  bindings are installed and integrity-checked for the workspace.
- `adapter_observed`: a bounded local adapter command emitted an allowlisted
  receipt. It is not proof that the native runtime invoked the hook.
- `native_qualified`: a fresh attended supported-runtime session produced the
  accepted native evidence for the exact installed adapter and lifecycle.
- `unavailable`: the evidence needed for the next layer is missing, invalid or
  ambiguous. Unavailable never becomes configured or observed by inference.

Local contract tests are reported separately from all four lifecycle fields.

## Non-goals

- raw transcript or prompt retention for continuity;
- automatic model-written checkpoint bodies;
- scheduler-driven execution-item mutation;
- weekly deep dreaming, L2/L3/lifetime promotion or model-backed self review;
- changes to canonical Owner Context interview facets or promotion rules;
- business task synchronization, notification or external publication;
- native/runtime claims from configuration, fixtures or direct CLI execution.

## Acceptance

- a fresh workspace reports onboarding before work and one safe next action;
- a single active item is pointer-only and distinguishes missing versus valid
  checkpoint state; multiple active items fail closed as ambiguous;
- Session Start shows the same deterministic state and never exposes execution
  content;
- context injection stores no prompt and only attested selected-skill IDs;
- adapter receipts can set only `adapter_observed`; native remains unavailable;
- top-level and Maestro status use the same projection;
- Claude and Codex contract fixtures remain semantically equivalent; and
- the full development harness passes without changing deep dreaming, Owner
  Context facets or provider authority.
