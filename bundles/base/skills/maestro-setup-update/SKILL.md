---
name: maestro-setup-update
description: Guide an explicitly requested Maestro installation, signed-source check, reviewed update, recovery or rollback. Select the admitted MCP product or positively identified legacy ZIP route before acting.
---

# Maestro Setup and Update

## Select the installed route

For the admitted MCP product, use the workflow below and stop before the legacy
ZIP sections. Read `maestro_context` and the current three core tool schemas.
The method supplies procedure only: it grants no shell, path selection, source
enrollment, activation, trust replacement or approval authority. Startup and
doctor stay read-only. A blocked MCP uses the independently admitted local
installer recovery path; never select another home or execute a synced file.

## MCP product: signed local source and exact review

1. Query `maintenance.status` and an existing `maintenance.report` for
   `update-check` when available. Reports describe a bounded check, not current
   native health or an activated release. Ordinary model arguments cannot select
   the source directory, signing registry, binary or private installation roots.
2. For an explicitly requested check of the already enrolled source, request
   `maintenance.qualify` with `job_id: update-check` and its current revision.
   The trusted form must show the exact signed generation, installation, source,
   recalls and source-custody/candidate-plan/diagnosis effects. A decline or stale
   form grants nothing. This is attended-only and cannot enroll recurring work.
3. Read the bounded `maintenance.report`. `current_signed_release` is an
   authenticated current check; `candidate_staged_activation_unconfirmed` means
   private candidate bytes and a canonical plan exist. Neither activates code.
   `reviewed_no_change` retains an existing qualification and never creates one
   from observation-time plan-ID churn. `historical_update_check` preserves the
   report's original plan; a separate `current_pending: verified_unconfirmed`
   readback identifies the current reauthenticated plan without approving it.
   Keep unavailable/partial checks explicit: accepted high-water and recalls can
   persist even when the payload is missing or the signed metadata is expired.
4. Source enrollment and installation mutation use the independently admitted
   local installer through its exact operator review. The operator reviews
   `source-review`, then confirms `source-check` or `stage` with that digest.
   First enrollment requires the selected hydrated local folder and exact
   source/channel/platform tuple; later checks use the enrolled fixed binding.
   `update-review` shows the exact pending plan. `update` requires that plan ID
   and explicit confirmation, followed by actual installed executable/bundle
   readback. A method or model assertion never supplies the confirmation.
5. Recovery uses the same installer: `reconcile` names the exact pending plan;
   rollback has its own `rollback-review` and separately confirmed `rollback`.
   Only a compatible authenticated nonrecalled previous release can return.
   Rollback preserves source sequence, known recalls, expiry policy and reader
   floor. Existing schema3 cannot enroll without genuine signed current proof
   and an actual compatible executable; unavailable migration stays no-write.
6. Use the installed participant-local Manage Maestro launcher for explicitly
   requested client setup, update or recovery. Its first console run collects the
   admitted workspace, local packet destinations and selected Code scope, creates
   the opaque carrier ID, and presents exact source/materialization reviews.
   It grants no model shell, arbitrary path selection or confirmation authority.
   The ordinary release-bound packets retain their exact pins. Stable-local-v1
   packets use a separate current materialization record: activation or compatible
   rollback requires checked regeneration from the actual authenticated current
   server. Between phases, report activation-applied/client-materialization-pending.
   A metadata-only refresh preserves unchanged active client inputs and needs no
   regeneration. Read current materialization separately from Code registration,
   loaded sessions and functional evidence. Code registration uses normal scoped
   plugin operations with JSON readback and preserves disabled state. Never
   uninstall/reinstall as an update shortcut, edit native profiles, or infer loading
   from an exit acknowledgement. Desktop still needs its normal manual extension
   prompts. An exact compatible stable bundle may be reused after checked current
   regeneration; external-launcher support remains pending native qualification.
   A changed carrier/native configuration requires the manual updated-MCPB flow.
   Opening a file, operator installation confirmation, connection and functional
   operation are separate evidence. Restart/reload/reconnect is separately needed.
   Explicit clients-reconcile uses its exact review to verify actual packets and
   host readback, reclaiming only an installation lock with a verified dead owner;
   startup/status never recover it. Unknown ownership preserves files and refuses.
   Expired/recalled source or a changed running executable requires the
   independent recovery route; local file presence does not establish freshness.
   Exhausted recall custody stays denied after newer metadata or rollback; source
   diagnosis remains usable, while clearing that denial needs a separate supported
   repair. A missing/corrupt active executable permits independent source refresh
   but payload replacement remains `payload_repair_unavailable`, preserving it.

Present a plain-language outcome and one concrete next action. Host installation,
native qualification, production signing and team publication remain separate
evidence. The operator guide supplies exact local command syntax; a recipe does
not authorize the model to run those commands or invent a distribution location.

## Legacy ZIP only

Continue below only after actual installation metadata positively identifies the
legacy ZIP runtime. The legacy procedure never repairs an MCP product by copying
a ZIP or its owner data over the managed installation.

Before owner-state file operations, follow [admitted owner-state paths](../interaction-profile/resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


Guia conversacional de instalação, atualização e reparo. Oriente a extração em pasta nova e execute as verificações autorizadas pelas ferramentas do host. Não transfira ao usuário trabalho de terminal ou edição manual que o Maestro pode realizar com segurança. Permissões obrigatórias do host nunca são contornadas.

## Interaction profile

Resolver [`interaction-profile`](../interaction-profile/SKILL.md) se disponível. Ajustar vocabulário e ritmo, jamais o envelope de segurança (uma pergunta por vez, sem terminal, sem edit manual).

## Contrato de comunicação

- Uma pergunta por vez.
- Sem "você", "tu" ou "te". Preferir impessoal ou 3ª pessoa.
- Sem em-dash ("—") em texto externo. Usar vírgula, dois pontos ou parênteses.
- Sem jargão de shell, JSON ou chmod.
- Se o usuário pedir "voltar versão", ser transparente: o Maestro atual não faz rollback automático. Se o ZIP anterior foi guardado, reinstalar a versão antiga seguindo o `README-INSTALL.md` resolve. Caso contrário, pedir o link ao time BCG Brasil AI.
- Nunca mencionar `bcgos`, `bcgos doctor`, `bcgos update` ou qualquer binário de instalador. Esse caminho foi encerrado.

## Roteamento inicial

Perguntar apenas qual desfecho o usuário quer, em uma frase:

> "É primeira instalação, atualização ou reparo?"

Se o pedido for rollback, tratar como caso de "reparo com ZIP anterior" (ver seção Rollback abaixo).

## Fluxo: primeira instalação

1. **Confirmar posição.** Verificar se `${CLAUDE_PROJECT_DIR}/VERSION` existe.
   - Se existir, informar: "a pasta Maestro já está aberta aqui, versão v<X.Y.Z>. Podemos seguir com o onboarding."
   - Se não existir, orientar: "abra no Claude Code a pasta Maestro que foi extraída do ZIP. Feche esta janela e reabra pela pasta correta."

2. **Confirmar estado.** Obter os paths pelo helper verificado. Se já estiverem
   admitidos, seguir com o brain retornado. Caso seja uma instalação nova sem
   estado anterior, confirmar essa escolha e executar a inicialização local
   explícita `home init`; os hooks só preenchem o brain depois da admissão.
   Estado legado, conflito ou origem migrada não verificada exige diagnóstico;
   não reabrir a janela para criar uma árvore alternativa nem mesclar conteúdo.

3. **Delegar identidade.** Encaminhar para a skill [`maestro-onboarding`](../maestro-onboarding/SKILL.md) sem duplicar o trabalho dela. Frase-ponte sugerida: "com a pasta pronta, vamos à apresentação e captura de identidade. Ativando o onboarding."

## Fluxo: atualização

Contexto: o time BCG Brasil AI envia um email com o link do ZIP novo. O usuário baixa e segue o ritual do `README-INSTALL.md` na raiz da pasta Maestro, que é a fonte única desse processo. Esta skill entra depois disso, para verificar.

Leia `UPDATE-CONTRACT.json` e `UPDATE-RUNBOOK.md` da instalação de destino antes de agir. São a autoridade do contrato v2; não invente uma cópia local de campos ou checks. O kit contém receitas e núcleo gerenciado, nunca o Maestro pessoal completo.

1. Confirme raiz canônica, host real (Claude Code ou Codex) e versão local/esperada. Não assuma CLAUDE_PROJECT_DIR no Codex. Nunca extraia sobre a instalação em uso.
2. Siga o ritual em README-INSTALL.md: preserve a instalação anterior inteira, copie a árvore de origem (data/ em 0.1.11/0.1.12) para uma extração nova e reconcile personalizações fora dela. Não mova/apague a origem. Dual trees sem recibo válido exigem diagnóstico antes de escrever.
3. Após confirmação da atualização legada coberta, execute explicitamente a migração v1 pelo motor gerenciado e valide seu recibo imutável. Startup e inspeção não a executam; isso não é migração externa para HOME. Prove leitura dos namespaces retidos usando o resolver; existência de pasta não prova consumo.
4. Abra ou retome o recibo em `brain/.maestro/updates/update-<versão>.json`. Valide todas as bindings do contrato antes de reutilizar PASS: ZIP exato, núcleo, baseline, raiz, host/plataforma e recibo da migração. Divergências preservam a tentativa antiga como stale e exigem nova tentativa.
5. Prove todos os checks obrigatórios e a reconciliação das personalizações. Configurado, observado pelo adaptador e executado no host nativo são estados diferentes. Claude possui nove handlers; Codex usa seu próprio conjunto. Não exija comandos slash do Claude em outro host.
6. Rode maestro-doctor e chame Yoda de verdade pelo mecanismo de subagents do host. Aguarde o retorno; registre indisponibilidade sem imitá-lo. MCP/caseOS e Python opcionais indisponíveis não bloqueiam checks locais, mas não podem ser relatados como conectados/instalados.
7. Continue do primeiro check não PASS enquanto houver ação segura autorizada. Salve checkpoint antes de interrupção/compactação. Use goal somente onde suportado; a receita não remove limites de contexto, orçamento, permissões ou políticas corporativas. Prova impossível recebe FAIL ou UNAVAILABLE e próximo passo concreto, não conclusão fictícia.
8. `status: pass` exige todos os checks obrigatórios e Yoda em PASS. Só então reconcilie brain/.maestro-version e brain/.upgrade-pending. Estados de checks: PASS/FAIL/UNAVAILABLE. Estados da tentativa: in_progress/pass/fail/unavailable. Recibo contém somente metadados.

Edição autoral posterior pode produzir `state=committed, verification=target_evolved, current=false`. Preserve a edição e o recibo histórico. Nova qualificação combina prova histórica da cópia com verificações frescas do estado final e runtime, sem recópia obrigatória nem bloqueio permanente. Escapes e adulteração de plano/recibo continuam bloqueantes.

## Migração incremental de schema

O bundle carrega dois marcadores de schema em `brain/`:

- `brain/.maestro-version` — versão do bundle instalado.
- `brain/memory/.schema-version` — schema efetivo da árvore de memória.

Quando um upgrade muda o schema de memória, o release notes do time BCG Brasil AI indica explicitamente. Nessa situação, além do fluxo de atualização acima:

1. Confirmar que o release notes menciona mudança de schema de memória.
2. Delegar para [`dream-memory`](../dream-memory/SKILL.md) a validação — a skill lê `brain/memory/.schema-version` e recusa qualquer escrita se o schema esperado não bater. Não migrar manualmente.
3. Se o schema exigir atualização, o release notes explicita o novo valor. Só então atualizar `brain/memory/.schema-version` via Edit para o valor indicado. Sem release notes explícito, não tocar.

## Fluxo: reparo

1. **Delegar diagnóstico.** Ativar [`maestro-doctor`](../maestro-doctor/SKILL.md). Aguardar o veredicto de uma linha e a lista de pontos.

2. **Mapear cada achado à ação certa.** O [`maestro-doctor`](../maestro-doctor/SKILL.md) reporta em linguagem simples; a tabela mental abaixo traduz cada caso para a orientação ao usuário.

   - **Arquivos core ausentes** (`VERSION`, `CLAUDE.md`, `.claude/`, `bundles/`): "a instalação está incompleta. Baixe o ZIP mais recente indicado no último email do time BCG Brasil AI e siga o `README-INSTALL.md`. O HOME e qualquer estado legado permanecem separados do núcleo e preservados."
   - **Estado ausente, core presente:** conferir o diagnóstico canônico; uma instalação nova pode ser inicializada explicitamente após a escolha do dono. Não inferir perda, reparar HOME ou recriar brain no produto por reabrir o host.
   - **Hooks presentes mas sem permissão de execução (Mac/Linux):** "reinstale seguindo o `README-INSTALL.md`. A extração de uma pasta nova restaura as permissões corretas."
   - **`brain/` corrompida ou com conteúdo perdido:** ser direto. "o Maestro não guarda backup automático da sua workspace. Se há backup autorizado, restaure em outra pasta e compare, preservando ambas as cópias. Nunca sobrescreva a árvore atual. Sem backup, o conteúdo perdido não é recuperável pelo Maestro."
   - **`VERSION` presente mas fora do formato `X.Y.Z`:** tratar como install corrompido, apontar para o `README-INSTALL.md`.

3. **Confirmar recuperação.** Após qualquer ação, sugerir rodar [`maestro-doctor`](../maestro-doctor/SKILL.md) de novo para confirmar veredicto "Tudo funcionando".

## Rollback

Não há rollback automático. Se o usuário pediu para voltar a uma versão anterior:

1. Perguntar: "o ZIP da versão anterior foi guardado localmente?"
2. **Sim:** reabrir a instalação anterior intacta preservada pelo kit. Revogar uma migração preserva ambas as árvores e retorna restored_runtime: false; não reinstala o núcleo anterior. Nunca copie brain/ para um runtime 0.1.11 que espera data/. Sem backup íntegro, diagnóstico e reconciliação assistida.
3. **Não:** informar honestamente que o Maestro atual não faz rollback automático e sugerir pedir o link do ZIP anterior ao time BCG Brasil AI pelo canal oficial.

## O que esta skill nunca faz

- Não sugere abrir terminal, rodar script ou editar JSON.
- Não invoca `bcgos` nem qualquer binário de instalador (esse caminho foi encerrado).
- Não promete rollback automático.
- Não altera conteúdo autoral em `brain/`. Depois de uma atualização validada,
  pode reconciliar somente os marcadores de lifecycle explicitados neste
  contrato (`.maestro-version` e `.upgrade-pending`).
- Não repete o trabalho de `maestro-onboarding` (identidade) nem de `maestro-doctor` (diagnóstico). Delega.

## Encerramento

Ao terminar, resumir em uma linha o desfecho, a versão ativa (lida em `VERSION`) e o caminho absoluto da workspace (`<admitted brain_root>/`). Se houver ação pendente do lado do time BCG Brasil AI (email com link, versão a confirmar), deixar explícito.
