---
name: dream-memory
description: Inspect or consolidate reviewed professional memory, navigate its private wiki, or manage recurring upkeep. Use for daily closure, weekly dreaming, remembered concepts and sources, memory status, lifetime promotion questions, or missed cycles.
---

# Dream Memory

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

## MCP product memory

Resolve [interaction-profile](../interaction-profile/SKILL.md) for presentation.
Keep the admitted workspace and the user's selected text as the scope.

1. For remembered context or missed-cycle questions, use `maestro_query` with
   `memory.list`, `memory.search`, `memory.read` or `memory.context`. Discover
   capture IDs before reading or correcting them; preserve pagination and current
   capture / visibility revisions. `maintenance.status` reports job state and
   qualification, separately from memory contents.
2. Capture only text the user explicitly selects through `maestro_apply`
   `memory.capture` with `text`. The trusted review presents that exact text.
   Correction, deletion and retention use `memory.correct`, `memory.delete` and
   `memory.retention` with their current revisions and exact trusted review.
   Product memory does not use raw daily journals, direct Read/Write/Edit,
   automatic SessionStart dreaming or request-marker deletion.
3. For an explicit daily or weekly closure, use `maestro_apply` with
   `{"operation":"memory.consolidate","cycle":"daily","date":"YYYY-MM-DD"}`
   or `cycle: "weekly"`, selecting the intended date. Daily synthesis writes L1
   from eligible attested captures. Weekly deterministic synthesis writes L2/L3;
   an eligible lifetime candidate needs canonical generation evidence and trusted
   review of its exact text. Missing or declined lifetime review retains ordinary
   L2/L3 and reports `lifetime_status: confirmation_unavailable` or
   `confirmation_declined`. Report those layers separately from lifetime status.
4. For recurring upkeep, follow the reviewed
   [maintenance workflow](../maestro-runtime-checkup/SKILL.md). The relevant jobs
   are `memory-light-dream`, `memory-deep-dream`, `memory-checkpoint`,
   `memory-retention-check`, `wiki-incremental-sync`, `wiki-reconcile` and
   `wiki-integrity-check`. Qualification executes once; enrollment is separate.
   Bootstrap checkpoint qualification with a real reviewed capture and qualified
   light dream. A retention diagnostic never deletes captures. Deep-dream job
   enrollment authorizes its L2/L3 schedule, never future lifetime text.
5. Read back bounded `memory.context` and the resulting metadata. For deep dream
   or retention, `maintenance.report` with the exact `job_id` exposes the domain
   result. Status is not proof that a new synthesis or background wake occurred.
   Report the requested period, activated layers and any actual limitation.
6. For concept navigation, query `wiki.status` first. Read `wiki.read` with the
   exact returned `generation` and a registered `reference` such as `index.md`;
   follow only concept/layer/backlink references actually returned by that graph.
   Resolve a returned source reference with `wiki.source` and the same generation.
   Pages accept `offset` and `max_bytes` from 4 to 4096; preserve `next_offset`.
   Source changes, correction, deletion or pending retirement can make every old
   page unavailable. Reread status; never browse private filesystem paths or use
   raw captures to reconstruct a denied view. Querying never builds the wiki.
   An explicitly requested, reviewed `wiki-incremental-sync` qualification uses
   a real pending source event; `wiki-reconcile` builds current committed memory.
   `wiki-integrity-check` checks the current graph and persists a body-free report.
   Historical readable content or a no-op does not qualify a new compiler build.

If capture/correct/delete/retention review is unavailable, the explicit local
operator uses the exact request JSON on stdin:

~~~text
maestro-mcp memory-review --product-root <product> --user-home <owner> --workspace <workspace>
maestro-mcp memory-apply --product-root <product> --user-home <owner> --workspace <workspace> --review-digest <exact-review-digest> --confirm
~~~

The operator reviews the private text, then repeats the same binding and stdin
with the exact review digest and explicit confirmation. These four mutation
fallbacks do not implement `memory.consolidate`; do not invent a local
consolidation command or treat an operator flag as a model approval argument.

## Legacy ZIP only — explicit runtime scope

Everything below applies only to the positively identified legacy ZIP runtime.
It is not an MCP recovery path. Use its verified installed helpers and existing
legacy authority; product operations above never fall through to these steps.

Before owner-state file operations, follow [admitted owner-state paths](../interaction-profile/resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


Operate directly on the workspace memory tree under `brain/memory/` (memória recente, memória semanal, memória de médio prazo e memória permanente). All reads and writes go through the Read, Write and Edit tools, following the invariants below.

Paths beginning `brain/memory/` below are owner defaults. For an active-case
request, resolve the confirmed case first and use only that case's `memory/`
tree, schema, budgets and policies. Never borrow owner configuration or silently
copy policies into a case. If required case configuration is missing, dreaming
is unavailable and its request remains pending.

## Interaction profile

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) skill before responding. Ajustar o tom e o nível de detalhe ao perfil do usuário antes de apresentar qualquer resultado visível. A operação de memória, a política e o comportamento de segurança nunca variam por perfil; apenas a explicação e o detalhe opcional variam.

- `standard`: state the result, what changed and one safe next action.
- `advanced`: add the relevant cycle rationale, diagnostics and drill-down
  pointers when useful.
- `power`: add a detalhamento da origem de cada memória e quando foi registrada, os limites de capacidade de cada camada e os trade-offs operacionais, sob pedido ou quando afetam materialmente uma decisão.

## Choose the cycle

- Use **daily light** for session or day closure. It may capture sanitized signals and update memória recente only. The day's entry is `brain/memory/recent/<YYYY-MM-DD>.md`, one file per date being closed.
- Use **weekly deep** for week closure or an overdue weekly cycle. It may update memória semanal and memória de médio prazo and promote eligible lifetime memory.
- Use **status** when the user asks what is remembered, why a promotion occurred or whether a cycle was missed.

## Auto-trigger (SessionStart)

When invoked automatically at session start (the `⚠️ Dreaming pendente` block was present in session context), run the **daily light** cycle without prompting the user. After the cycle completes successfully:

1. Acknowledge only the exact request fingerprint read before synthesis, using the matching-ack procedure below.
2. Report the result in one paragraph — do not wait for the user to ask.

If the cycle fails, is unavailable, empty or interrupted, retain the request and report the limitation. A later SessionStart retries it. Never delete a marker to suppress a failed cycle.

## Workflow

1. Resolve the active workspace identity by reading `brain/owner/identity.json` and the memory root at `brain/memory/`.
2. Confirm the memory tree exists and verify the effective capacity limits for each memory layer. If either is missing, stop and report the safe next action.
2a. **Schema-version gate (GAP-D).** Read `brain/memory/.schema-version`. The current expected schema is `1`. If the marker is missing, report the tree as uninitialized and stop; if `schema_version` differs from the expected value, stop and route the user to `/maestro-setup-update`. Never mutate the memory tree when the schema does not match — a stale reader against a newer schema corrupts the layers.
3. For capture, persist only signals classified as sanitized in the source (Session Start, hook output, prior memória recente entry). Never write raw credentials, client files or unrestricted prompt history into `brain/memory/`.
4. Execute exactly one cycle per invocation. Hooks, schedules and manual requests all follow the same idempotent read, synthesize, stage, commit sequence.
5. For weekly lifetime promotion, require a named eligibility policy in `brain/memory/policies/lifetime.json`. If it is missing, stop: lifetime activation must fail closed.
6. Return the cycle, period, origem e momento de registro de cada memória, activated layers, lifetime eligibility reason and any skipped or missing layers.
7. If the required policy or budget files are missing, report the capability as unavailable rather than emulating dreaming with ad-hoc edits.
8. **Matching acknowledgement:** after successful persisted synthesis, acknowledge the request fingerprint captured before the cycle. Changed requests stay pending. Never remove request files directly.

## Invariants

- O ciclo diário não pode escrever na memória semanal, memória de médio prazo ou memória permanente.
- A date file is not a freshness watermark. Later checkpoints on that date create a new request fingerprint and remain pending until matching successful synthesis.
- O ciclo semanal prepara todos os outputs e os torna disponíveis de uma vez, de forma consistente.
- Uma síntese vazia, inválida ou interrompida não altera nada visível.
- O sistema usa apenas o estado mais recente totalmente válido; nenhum estado parcial de memória semanal, de médio prazo ou permanente é injetado.
- Um histórico de memória totalmente inválido é reportado como corrompido, nunca como memória vazia.
- Um bloqueio por espaço de trabalho impede que ciclos diários e semanais concorram sobre a memória compartilhada.
- Atualizações de memória permanente exigem rastreabilidade de origem, critério de elegibilidade, histórico de versões e nunca sobrescrita direta.
- O contexto é montado como memória permanente → médio prazo → semanal → recente, com limites de capacidade independentes e ponteiros de detalhamento.
- As capturas de origem permanecem somente-leitura e isoladas por espaço de trabalho.

## Current delivery boundary

The managed bundle contains this canonical skill and the memory capacity and policy contracts under `brain/memory/`. If those files are absent in the current workspace, report dreaming as unavailable and point the user at the setup skill rather than claim execution.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.

## ZIP scope and matching acknowledgement

Resolve the same owner or confirmed active-case scope as maestro-operator.
Requests and generated recent pages stay under that scope's `memory/`; never
copy case content into owner memory. Read the selected
`<scope>/memory/.dream-requested` fingerprint before selecting daily sources.
The daily journal is selected agent-authored context, not a capture-v2 source or
semantic sanitization attestation. It cannot bypass a required engine producer,
policy or eligibility check; when unavailable retain the request.

After successful synthesis has persisted the selected work, use the verified
helper with `daily-dream-ack --root ROOT` and stdin JSON
`{"scope":"owner","digest":"<64-character fingerprint read before synthesis>"}`.
For a case use `account/<account>/case/<case>`. Bash: pipe the JSON to
`maestro_runtime "$PWD" daily-dream-ack --root "$PWD"` after sourcing
`.claude/hooks/lib/maestro-runtime.sh`. PowerShell: pass the JSON via
`Invoke-MaestroRuntime -Root $PWD.Path -RuntimeArgs @('daily-dream-ack','--root',$PWD.Path) -Payload $ackJson`
after sourcing `.claude/hooks/lib/maestro-runtime.ps1`.

The shared writer lock compares and acknowledges atomically; if a newer
checkpoint arrived meanwhile, the command fails and the new request survives.
The fingerprint represents cumulative durable work, so replaying an older
checkpoint cannot roll it back. Legacy 0.1.x timestamp requests remain pending:
run `daily-stop` to normalize them under the same lock, preserving the original
timestamp in the scope's `memory/.dream-legacy-request`, then read the normalized
fingerprint before synthesis. Missing migration readiness blocks normalization,
logging and acknowledgement; valid history remains available through `daily-context`.
An acknowledgement is an agent assertion of success, not independent proof of
synthesis. No-op Stop does not request another cycle. Do not use ordinary daily
logging as authorization to perform user-confirmed day closure.
