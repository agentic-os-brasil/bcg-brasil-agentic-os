# 🎼 Roteiro funcional — Maestro Canary

Execute este roteiro em `/loop` até finalizar todas as etapas.

Antes de começar:

1. informe a previsão total de duração — inicialmente, **60–90 minutos**;
2. apresente a quantidade de etapas;
3. diga qual etapa será iniciada;
4. atualize o progresso e o tempo restante depois de cada etapa.

Continue autonomamente entre as tarefas. Pause somente quando:

- precisar de uma resposta do owner;
- chegar ao teste que exige fechar e reabrir a sessão;
- uma ação externa, destrutiva ou irreversível exigir autorização;
- encontrar um bloqueio que impeça objetivamente a continuação.

Não encerre o roteiro após um erro. Registre `FAIL`, `PARTIAL` ou `UNAVAILABLE`,
explique brevemente o motivo e siga para a próxima tarefa segura.

Mensagem inicial esperada:

> 🎼 **Maestro Canary iniciado**
>
> Vou executar 12 etapas em `/loop`, com duração estimada de **60–90 minutos**.
> Conduzirei o teste autonomamente e atualizarei o progresso e a previsão
> restante após cada etapa. Só vou interromper quando sua participação for
> realmente necessária.
>
> **Progresso:** 0/12  
> **Tempo restante estimado:** 60–90 minutos  
> **Agora:** identidade, instalação e workspace

## Objetivo

Validar se o Maestro instalado funciona como produto profissional:

- reconhece sua identidade e o workspace correto;
- conduz onboarding e personalização;
- organiza contexto, tarefas e continuidade;
- encontra e executa skills;
- decide a profundidade adequada para cada tarefa;
- utiliza Client Account Agent, Case Agent e Walter quando aplicável;
- observa sua saúde com Darwin;
- mantém hooks, receipts e estado entre sessões;
- permanece amigável e pouco intrusivo para usuários leigos.

## Regras do Canary

- Use somente este workspace e conteúdo sintético.
- Nunca mencione Kowalski ou outro sistema pessoal/local.
- Não acesse clientes, credenciais, OneDrive, SharePoint ou fontes externas.
- Você está autorizado a criar artefatos apenas dentro de `canary/`.
- Prossiga normalmente com leitura, escrita e organização local reversível.
- Peça confirmação somente antes de rede, publicação, exclusão relevante ou
  alteração externa ao workspace.
- Diferencie `configurado`, `observado em runtime`, `native-qualified` e
  `unavailable`.
- Não simule agentes, hooks, receipts ou skills indisponíveis.
- Registre o relatório progressivamente em
  `canary/MAESTRO-CANARY-REPORT.md`.

Ao terminar cada tarefa:

1. explique brevemente o que foi testado;
2. classifique como `PASS`, `PARTIAL`, `FAIL` ou `UNAVAILABLE`;
3. mostre a evidência observada;
4. atualize `Progresso: N/12` e `Tempo restante estimado`;
5. siga automaticamente, salvo quando houver um dos motivos de pausa acima.

---

## Tarefa 1 — Identidade, instalação e workspace

Confirme quem você é, o papel do Maestro, o caminho do workspace, a versão
instalada (bundle), onde está o workspace, se o workspace é válido, a estrutura
criada, o resultado de `/maestro-doctor` e os runtimes/adapters configurados.
Verifique se não existe referência ou memória proveniente de outro sistema
pessoal.

**Aprovação:** identidade correta, workspace isolado, runtime encontrado e
diagnóstico explicado em linguagem simples.

## Tarefa 2 — Onboarding e fontes iniciais

Verifique o estado real do onboarding. Se pendente, execute
`/maestro-onboarding`. Antes da primeira pergunta, ofereça entrevista curta e
completa com estimativas de tempo e as opções de começar sem fontes, fornecer
arquivos locais ou indicar fontes públicas para pesquisa futura.

Explique que podem ser úteis CV do BCG, currículo, LinkedIn, bio, portfólio,
publicações, descrição de cargo, avaliações de desempenho, MBTI, Big Five,
leadership profile e outros testes. Esses materiais são autodescrição, não
diagnóstico.

Use somente uma fixture fictícia em `canary/fixtures/perfil-sintetico.md`.
Conduza a trilha curta, uma pergunta por vez, resumindo e confirmando cada
resposta antes de persistir.

**Aprovação:** trilha persistida, entrevista avança corretamente e fontes são
oferecidas nas trilhas curta e completa.

## Tarefa 3 — Personalização e ownership

Execute, se disponíveis, `/interaction-profile` e `/agent-identity-setup`.
Configure nomes, emojis, papéis e ownership de Maestro, Client Account Agent,
Case Agent, Walter e Darwin. PA Experts permanecem especialistas funcionais e
industriais consultivos, podendo continuar como stubs.

**Aprovação:** personalização compreensível e persistida sem alterar os
contratos arquiteturais.

## Tarefa 4 — Descoberta e chamada de skills

Liste as skills realmente instaladas, agrupadas em onboarding, organização,
análise, reuniões, casos, apresentações, qualidade e ingestão. Confirme se
existem skills chamadas `AfD` ou `CDC`; não invente esses IDs. Se ausentes,
mostre as skills atuais de spec-driven delivery, testes e evidências, revisão,
PR review, quality loop, cobertura, unit tests e QA gate.

**Aprovação:** catálogo corresponde ao instalado e nenhum `SKILL.md` é tratado
automaticamente como capability qualificada.

## Tarefa 5 — Reunião para decisões e tarefas

Use estas notas sintéticas:

> A CEO Marina aprovou um piloto de seis semanas. O CFO Rafael quer o business
> case até sexta-feira. Ana enviará a baseline de custos. O principal risco é a
> baixa qualidade dos dados. A próxima reunião será na terça-feira.

Execute `/meeting-to-work-items` ou `/meeting-close`. Extraia decisões, tarefas,
responsáveis, prazos, dependências, riscos, perguntas abertas e checkpoint.
Salve em `canary/outputs/meeting-close.md`.

**Aprovação:** fato, decisão, tarefa e inferência estão separados, sem inventar
responsáveis ou datas.

## Tarefa 6 — Caso simples e decisão de profundidade

Solicitação:

> Organize as notas da reunião anterior em uma mensagem curta para o time.

Antes de executar, informe importância estratégica, necessidade de visão de
stakeholders, profundidade, agentes realmente chamados e motivo de qualquer
skip. O caminho esperado para tarefa simples é
`User → Maestro → Case Agent → Maestro → User`, ou skill direta quando esse for
o contrato disponível.

**Aprovação:** tarefa simples não recebe cerimônia desnecessária.

## Tarefa 7 — Caso estratégico e orquestração

Cenário:

> A Aurora Mobility precisa decidir se apresenta ao conselho um programa de
> redução de custos de 15% com impacto potencial sobre operações e pessoas.
> Prepare uma recomendação executiva considerando CEO, CFO, COO, riscos de
> execução e mensagem ao conselho.

Classifique materialidade e explique a rota. Quando disponível, use:

`User → Maestro → Client Account → Maestro → Case → Maestro → Client Account
validation → Maestro → Walter → Maestro → User`

A validação de Client Account só ocorre se esse agente foi chamado na ida.
Walter atua como senior advisor, proxy contextual do owner e refiner, avaliando
se a intenção intrínseca foi atendida. Se a orquestração nativa estiver
indisponível, não a simule: mostre o plano e registre `UNAVAILABLE`.

**Aprovação:** profundidade proporcional e distinção entre plano contratual e
agentes realmente executados.

## Tarefa 8 — Ingestão e análise

Use somente `canary/fixtures/perfil-sintetico.md`. Execute `/ingest-content` se
a capability estiver qualificada. Produza fatos, inferências, lacunas,
proveniência e pontos para confirmação. Se indisponível, não instale
dependências nem improvise adapters.

**Aprovação:** arquivo selecionado não vira autorização automática e nenhuma
síntese é tratada como verdade permanente sem revisão.

## Tarefa 9 — Hooks e continuidade entre sessões

Verifique `session-start`, `context-injection`, `pre-action-guard`,
`post-action-receipt` e `stop-finalization`.

Crie esta tarefa aberta:

> Preparar versão 2 da recomendação da Aurora Mobility incorporando uma
> sensibilidade de impacto financeiro.

Registre um checkpoint e pause para o owner fechar a sessão e abrir uma sessão
nova no mesmo workspace. Na nova sessão, valide identidade, onboarding já
concluído, tarefa aberta, checkpoint, próxima ação e ausência de conteúdo de
outro workspace.

**Aprovação:** continuidade observada numa sessão nativa nova; configuração ou
comando manual não bastam como prova.

## Tarefa 10 — Guard amigável

Crie `canary/sandbox/nota.md`, edite, renomeie e apague somente esse arquivo
sintético. Observe se o trabalho local comum ocorre sem bloqueios excessivos.
Não execute comandos destrutivos amplos; apenas explique como uma remoção
inequívoca de raiz protegida deveria ser tratada.

**Aprovação:** operações comuns seguem o runtime; hard stops ficam restritos a
ações claramente destrutivas, externas ou fora do escopo.

## Tarefa 11 — Darwin e manutenção

Verifique catálogo, status das rotinas, jobs disponíveis/indisponíveis,
LaunchAgent, última execução, receipts e trabalhos pendentes. Se houver rota
attended autorizada, execute somente uma avaliação local bounded. Não afirme
que houve manutenção semanal, dreaming ou revisão de states com base apenas em
configuração ou catálogo.

**Aprovação:** Darwin diferencia housekeeping, gatekeeping, survive, thrive,
evolução semanal/mensal e tarefas indisponíveis.

## Tarefa 12 — Relatório executivo final

Finalize `canary/MAESTRO-CANARY-REPORT.md` com:

- resumo executivo em texto corrido;
- matriz por área com resultado, evidência, fricção e próxima ação;
- defeitos com severidade, reprodução, esperado, observado, evidência,
  workaround e componente provável;
- tempo total real versus previsão inicial;
- um dos vereditos abaixo:
  - `READY FOR NEXT CANARY`;
  - `READY WITH KNOWN LIMITATIONS`;
  - `HOLD FOR FIX`;
  - `UNAVAILABLE — INSUFFICIENT EVIDENCE`.

Não use "tudo certo" se alguma capacidade essencial estiver apenas configurada
ou indisponível.
