---
name: dream-memory
description: Run or inspect professional memory consolidation through the BCG Brasil Agentic OS memory engine. Use for session or daily closure, weekly deep dreaming, memory status, lifetime promotion explanations, missed-cycle catch-up, or requests such as "consolide a memória", "fecha o dia", "fecha a semana" and "dreaming".
---

# Dream Memory

Operate directly on the workspace memory tree under `data/memory/` (memória recente, memória semanal, memória de médio prazo e memória permanente). All reads and writes go through the Read, Write and Edit tools, following the invariants below.

## Interaction profile

Resolve the canonical `interaction-profile` skill before responding. Ajustar o tom e o nível de detalhe ao perfil do usuário antes de apresentar qualquer resultado visível. A operação de memória, a política e o comportamento de segurança nunca variam por perfil; apenas a explicação e o detalhe opcional variam.

- `standard`: state the result, what changed and one safe next action.
- `advanced`: add the relevant cycle rationale, diagnostics and drill-down
  pointers when useful.
- `power`: add a detalhamento da origem de cada memória e quando foi registrada, os limites de capacidade de cada camada e os trade-offs operacionais, sob pedido ou quando afetam materialmente uma decisão.

## Choose the cycle

- Use **daily light** for session or day closure. It may capture sanitized signals and update memória recente only.
- Use **weekly deep** for week closure or an overdue weekly cycle. It may update memória semanal and memória de médio prazo and promote eligible lifetime memory.
- Use **status** when the user asks what is remembered, why a promotion occurred or whether a cycle was missed.

## Auto-trigger (SessionStart)

When invoked automatically at session start (the `⚠️ Dreaming pendente` block was present in session context), run the **daily light** cycle without prompting the user. After the cycle completes successfully:

1. Delete `data/memory/.dream-requested` (the marker written by `session-stop-dream.sh`).
2. Report the result in one paragraph — do not wait for the user to ask.

If the cycle fails or the memory tree is missing, report the failure and delete the marker anyway so it does not repeat on every session start.

## Workflow

1. Resolve the active workspace identity by reading `data/profile/identity.json` and the memory root at `data/memory/`.
2. Confirm the memory tree exists and verify the effective capacity limits for each memory layer. If either is missing, stop and report the safe next action.
3. For capture, persist only signals classified as sanitized in the source (Session Start, hook output, prior memória recente entry). Never write raw credentials, client files or unrestricted prompt history into `data/memory/`.
4. Execute exactly one cycle per invocation. Hooks, schedules and manual requests all follow the same idempotent read, synthesize, stage, commit sequence.
5. For weekly lifetime promotion, require a named eligibility policy in `data/memory/policies/lifetime.json`. If it is missing, stop: lifetime activation must fail closed.
6. Return the cycle, period, origem e momento de registro de cada memória, activated layers, lifetime eligibility reason and any skipped or missing layers.
7. If the required policy or budget files are missing, report the capability as unavailable rather than emulating dreaming with ad-hoc edits.
8. **Marker cleanup (auto-trigger only):** if `data/memory/.dream-requested` exists at invocation time, delete it after the cycle — success or failure — so the trigger fires only once per session stop.

## Invariants

- O ciclo diário não pode escrever na memória semanal, memória de médio prazo ou memória permanente.
- O ciclo semanal prepara todos os outputs e os torna disponíveis de uma vez, de forma consistente.
- Uma síntese vazia, inválida ou interrompida não altera nada visível.
- O sistema usa apenas o estado mais recente totalmente válido; nenhum estado parcial de memória semanal, de médio prazo ou permanente é injetado.
- Um histórico de memória totalmente inválido é reportado como corrompido, nunca como memória vazia.
- Um bloqueio por espaço de trabalho impede que ciclos diários e semanais concorram sobre a memória compartilhada.
- Atualizações de memória permanente exigem rastreabilidade de origem, critério de elegibilidade, histórico de versões e nunca sobrescrita direta.
- O contexto é montado como memória permanente → médio prazo → semanal → recente, com limites de capacidade independentes e ponteiros de detalhamento.
- As capturas de origem permanecem somente-leitura e isoladas por espaço de trabalho.

## Current delivery boundary

The managed bundle contains this canonical skill and the memory capacity and policy contracts under `data/memory/`. If those files are absent in the current workspace, report dreaming as unavailable and point the user at the setup skill rather than claim execution.
