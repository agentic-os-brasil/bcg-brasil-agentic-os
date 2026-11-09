---
name: maestro-runtime-checkup
description: Inspect Maestro runtime and maintenance status, qualify a named job or manage its reviewed enrollment. Use for runtime evidence, bounded Darwin diagnosis/proposals, attended signed-source checks, due work, pause/resume/revoke or readiness; native scheduling remains separate.
---

# Maestro Runtime Checkup

## Select the installed route

Use the admitted MCP product route first: read `maestro_context` and inspect the
current `maestro_query` / `maestro_apply` schemas. Product operations derive the
owner and workspace binding; model arguments cannot choose filesystem roots or
supply approval. A missing tool, rejected binding or failed call stays
unavailable and keeps the requested operation pending. It never selects legacy.

Use the legacy section only after the installed runtime is positively identified
as the ZIP distribution by its installation metadata and verified managed helper.
A product conversation stops after its product workflow below. Recipe selection
and diagnostic findings grant no shell, repair, installation, provider access,
native scheduling or new enrollment authority.

## MCP product checkup and maintenance

Resolve [interaction-profile](../interaction-profile/SKILL.md) for presentation.
Start with read-only `maestro_query` `maintenance.status`. Inspect the requested
job's `state`, `revision`, `qualification_state`, schedule, write set and reason.
For an existing domain result, query `maintenance.report` with its exact `job_id`.
A checkup reports and offers the next supported action; it does not repair wiring
or initialize state as a side effect.

### Review a named maintenance action

Only when the owner requests that action, use `maestro_apply` with the operation
below. Derive `expected_revision` from the current status. Each transition needs
the trusted human review of its exact job, workspace, schedule, write set and
current implementation/configuration. Missing, declined or stale review grants
no transition; reread current status before reconciling a conflict.

| Operation | Model arguments after `operation` | Effect |
| --- | --- | --- |
| `maintenance.qualify` | `job_id`, `expected_revision` | Attended one-shot execution and verified domain result / receipt; no recurring enrollment |
| `maintenance.enroll` | `job_id`, `expected_revision`, `timezone` | Enroll this qualified job with the reviewed timezone, cadence, write set and no-backfill boundary |
| `maintenance.pause` | `job_id`, `expected_revision` | Pause dispatch while preserving pending work |
| `maintenance.resume` | `job_id`, `expected_revision` | Resume under the existing bounded catch-up policy |
| `maintenance.revoke` | `job_id`, `expected_revision` | Deny further dispatch/publication under the revoked revision; retain its tombstone |
| `maintenance.run` | none | Execute active due jobs in the foreground under their existing reviewed authority |

Use only the implemented jobs: `memory-light-dream`, `memory-checkpoint`,
`memory-deep-dream`, `memory-retention-check`, `skills-index-refresh`,
`runtime-health-check`, `capability-recheck`, `runtime-drift-check`,
`wiki-incremental-sync`, `wiki-reconcile`, `wiki-integrity-check`,
`darwin-housekeeping-daily`, `darwin-deep-weekly`,
`darwin-structural-evolution-proposal` and `update-check`. Other
catalog entries retain their unavailable reasons. The catalog does not activate
jobs. Enrollment requires current qualification; changed build/configuration
requires fresh qualification. Use the owner's timezone; resolve an ambiguous
choice before enrollment. The service selects cadence, not a model-supplied
schedule: light dream/checkpoint every three hours, deep dream and wiki reconcile
weekly on Sunday, Darwin deep review weekly on Sunday, wiki incremental only
for an eligible committed-source event, and retention/index/runtime diagnosis
and Darwin housekeeping daily. Recurring jobs retain bounded catch-up. Darwin
monthly proposals and update checks are attended-only and cannot enroll.
Incremental
sync never invents a calendar occurrence. Reconcile qualification requires an
actual current-build compilation; a verified no-op can run normally but cannot
mint qualification. Integrity checks the entire current graph without repair.

Run the foreground due pass only for an explicit request. It cannot select an
inactive job, perform arbitrary repair, install a native wake task, download an
interpreter or access a model/provider. Deep-dream enrollment approves L2/L3
work, never future lifetime candidate text; the exact candidate review remains
in [dream-memory](../dream-memory/SKILL.md).

### Darwin local health and proposals

Inspect exact job status and its existing report first. Status distinguishes
absent/current/stale/corrupt/foreign/unsafe/unavailable cache state without writes.
For an owner-requested first creation, daily qualification reviews exact absent
parent and current actual source, requires trusted confirmation, publishes
exclusively and reads back exact bytes. Native observation and unbound document
coverage stay unavailable.

For a stale daily cache, show `manual_refresh_required`. Automatic replacement
and compensation remain unavailable under the physical-preservation gate.
Preserve state and the request for explicit reconciliation; the MCP operation
supplies no archive/delete/command repair route. Failed readback retains actual
partial bytes and recovery evidence. Report that limit without changing state.

Weekly/monthly work may publish a bounded immutable assessment/proposal while
preserving existing cache bytes. Keep proposal, execution qualification and
structural approval distinct. Daily/weekly enrollment is separately reviewed;
monthly cannot enroll or apply structural changes. Unchanged replay preserves
actual evidence rather than qualifying time/occurrence churn.

Windows Darwin publication remains unavailable; a registered job is not native
qualification. Follow the read-only diagnosis and supported next action while
keeping automatic repair, client loading and rollout acceptance pending.

### Attended signed-source check

`update-check` qualifies only an explicitly reviewed one-shot observation of the
installed owner's enrolled signed source. Its declared effects are source custody,
bounded candidate staging/plan and local diagnosis. It never enrolls recurring
work or activates a release. Show authenticated source/candidate diagnosis and
unavailable eligibility separately. Retain historical report/plan provenance;
only a separately canonically verified `current_pending` projection identifies
a current unconfirmed plan. Clock-only replay preserves genuine qualification
or remains unqualified. If the MCP source launch is unavailable, use the existing
independent installer recovery route; never reset SourceControl/Previous or
substitute a root/key/executable to make the check pass.

### Read the result accurately

`maintenance.report` supports `memory-deep-dream`, `memory-retention-check`,
`skills-index-refresh`, `runtime-health-check`, `capability-recheck` and
`runtime-drift-check`, `wiki-incremental-sync`, `wiki-reconcile` and
`wiki-integrity-check`, all three named Darwin jobs and `update-check`.
Darwin reports contain bounded cache/source/effect/proposal identities, counts
and coverage; signed-source reports contain bounded historical diagnosis and
separate verified current-pending metadata. Wiki reports contain source/graph identities and counts,
never private concept or capture bodies. A historical report does not establish
current graph readability. Inspect `wiki.status` for the current view; reads and
status do not compile, enroll or recover anything. Light dream/checkpoint qualification metadata and domain
references are in `maintenance.status`; do not invent unsupported report IDs.

Keep `inactive`, `active`, `paused` and `revoked` separate from qualification
`unqualified`, `qualified` and `stale`. A diagnostic may successfully execute and
publish a verified report whose finding remains missing/unavailable. Report
execution never promotes native health or capability. A foreground due pass,
local qualification, installed catalog or packet does not establish OS wake,
Code/Desktop native qualification, signing or pilot readiness. Preserve partial
results and unavailable findings; optional document conversion or background
maintenance does not block healthy ordinary work.

### Explicit local operator fallback

When trusted host review is unavailable, the operator supplies the exact intent
JSON on stdin to the installed executable:

~~~text
maestro-mcp maintenance-status --product-root <product> --user-home <owner> --workspace <workspace>
maestro-mcp maintenance-review --product-root <product> --user-home <owner> --workspace <workspace>
maestro-mcp maintenance-apply --product-root <product> --user-home <owner> --workspace <workspace> --review-digest <exact-review-digest> --confirm
maestro-mcp maintenance-run --product-root <product> --user-home <owner> --workspace <workspace>
~~~

`maintenance-review` and `maintenance-apply` consume the same JSON, for example
`{"operation":"maintenance.qualify","job_id":"runtime-health-check","expected_revision":0}`
only when current status actually reports revision zero. Review the complete
current envelope before applying; repeat for each transition. Enrollment adds
`timezone` to that exact intent. `maintenance-status` / `maintenance-run` do not
consume transition JSON. There is no local `maintenance-report` command.
These are attended operator routes, not permission for the model to run shell,
supply `--confirm` or approve future lifetime text. Refusal preserves the request;
inspect current status and use the supported next step without clearing markers,
replacing roots or silently repairing anything.

## Legacy ZIP only — explicit runtime scope

Everything below applies only to the positively identified legacy ZIP runtime.
It is not an MCP recovery path. Use its verified installed helpers and existing
legacy authority; product operations above never fall through to these steps.

Run a quiet, bounded health pass. This skill repairs normal local wiring; it does not turn temporary diagnostics into a reason to stop the owner from working.

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) skill before responding. Ajustar o tom e o nível de detalhe da resposta ao perfil do usuário antes de explicar o resultado. Isso afeta apenas o detalhe e o ritmo da conversa, nunca o que o Maestro inspeciona ou corrige.

## Check and reconcile

1. Verificar o ambiente de execução do Maestro e confirmar que o espaço de trabalho está acessível. Se o espaço ainda não foi preparado, usar `$maestro-environment-setup` primeiro.
2. Inspect workspace and runtime status. If the projection or hook wiring is missing, run the same idempotent `setup apply` consolidation that first-run uses. Do not ask the owner to copy commands or identify hook files.
3. Verify the result through the owning status surfaces. Re-run once only when the local cause changed. Preserve the workspace, local context and prior receipts; never replace a managed root just to make a check appear green.
4. On macOS, inspect Darwin's user-level LaunchAgent binding. The visual installer owns enrollment and repair of that schedule; if it needs attention, hand off to `$maestro-setup-update` rather than rewriting it from a runtime conversation. If a different managed schedule is already bound, explain that Maestro needs a choice rather than overwriting it. On other systems, report only the useful maintenance state and leave ordinary work available.
5. Verificar se o componente de leitura avançada de documentos está disponível. Until a versioned managed pack ships, descrevê-lo como uma adição opcional futura. Sua ausência afeta apenas a conversão de documentos, nunca o restante do espaço de trabalho.

## Explain the result

Use a short result with four human categories:

- **Pronto para trabalhar** — the workspace and normal runtime route are set.
- **Ajustado** — Maestro repaired a local integration and preserved the work.
- **Um detalhe para concluir depois** — an optional component or background schedule is not ready; explain its owner-visible impact only.
- **Preciso de uma escolha sua** — the action would change another workspace, replace a bound scheduler, use credentials, publish externally or remove data.

Never present raw receipts, command output, absolute paths, internal state labels or diagnostics unless the owner explicitly asks for a technical view.
