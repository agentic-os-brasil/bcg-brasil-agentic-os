---
name: maestro-setup-update
description: Guia conversacional para instalar, atualizar ou reparar o Maestro a partir do ZIP distribuído pelo time BCG Brasil AI. Use sempre que o pedido envolver install, primeira instalação, update, atualização, upgrade, reparo, recuperação, "voltar versão" ou rollback do Maestro.
---

# Maestro Setup and Update

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

2. **Confirmar workspace inicial.** Verificar se `brain/` e `brain/.initialized` existem.
   - Se ausentes, orientar: "feche a pasta no Claude Code e reabra. Na próxima abertura o Maestro cria a workspace pessoal automaticamente."
   - Se presentes, seguir.

3. **Delegar identidade.** Encaminhar para a skill [`maestro-onboarding`](../maestro-onboarding/SKILL.md) sem duplicar o trabalho dela. Frase-ponte sugerida: "com a pasta pronta, vamos à apresentação e captura de identidade. Ativando o onboarding."

## Fluxo: atualização

Contexto: o time BCG Brasil AI envia um email com o link do ZIP novo. O usuário baixa e segue o ritual do `README-INSTALL.md` na raiz da pasta Maestro, que é a fonte única desse processo. Esta skill entra depois disso, para verificar.

Leia `UPDATE-CONTRACT.json` e `UPDATE-RUNBOOK.md` da instalação de destino antes de agir. São a autoridade do contrato v2; não invente uma cópia local de campos ou checks. O kit contém receitas e núcleo gerenciado, nunca o Maestro pessoal completo.

1. Confirme raiz canônica, host real (Claude Code ou Codex) e versão local/esperada. Não assuma CLAUDE_PROJECT_DIR no Codex. Nunca extraia sobre a instalação em uso.
2. Siga o ritual em README-INSTALL.md: preserve a instalação anterior inteira, copie a árvore de origem (data/ em 0.1.11/0.1.12) para uma extração nova e reconcile personalizações fora dela. Não mova/apague a origem. Dual trees sem recibo válido exigem diagnóstico antes de escrever.
3. Execute a migração pelo motor gerenciado e valide seu recibo imutável. Prove leitura dos namespaces retidos usando o resolver; existência de pasta não prova consumo.
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

   - **Arquivos core ausentes** (`VERSION`, `CLAUDE.md`, `.claude/`, `bundles/`): "a instalação está incompleta. Baixe o ZIP mais recente indicado no último email do time BCG Brasil AI e siga o `README-INSTALL.md`. A workspace `brain/` é preservada pelo ritual."
   - **`brain/` ausente, core presente:** "feche o Claude Code e reabra a pasta Maestro. Na próxima abertura a workspace é recriada automaticamente."
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

Ao terminar, resumir em uma linha o desfecho, a versão ativa (lida em `VERSION`) e o caminho absoluto da workspace (`${CLAUDE_PROJECT_DIR}/brain/`). Se houver ação pendente do lado do time BCG Brasil AI (email com link, versão a confirmar), deixar explícito.
