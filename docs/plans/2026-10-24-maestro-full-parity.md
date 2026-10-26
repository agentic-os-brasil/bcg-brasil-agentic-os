# Maestro completo — checklist de paridade MCP, plugins e Desktop

Preparado em 2026-10-23. Baseline publicada: `d9da6e3dcb581e998c0d16ca59af6e6365b98082`.

Plano incorporado ao repositório em 2026-10-24 após autorização para construir
e abrir PRs. A primeira entrega é a inspeção somente leitura do data home,
definida na [Spec 055](../../specs/055-data-home-inspection.md) e explicada no
[guia de diagnóstico](../development/data-home-inspection.md). Ela prepara
B01; os checkboxes de paridade integral continuam abertos até o aceite inteiro.
O experimento MCP/plugin está em `dev/spikes/maestromcp`; sua implementação e
seus recibos não substituem a adaptação dos contratos do produto.

Objetivo: disponibilizar todas as funcionalidades do Maestro da baseline por
uma instalação e atualização coerentes, preservando estado, políticas e
experiência de trabalho nos clientes-alvo. O kit 0.0.4 é uma prova técnica de
três ferramentas MCP; não é a migração do produto completo.

## Como marcar pronto

Cada item só fecha com implementação, dependências entregues, teste funcional,
evidência do cliente/plataforma e preservação dos dados. `Em catálogo`,
`contrato implementado`, `arquivo no pacote`, `CLI instalado` e `execução nativa`
são estados diferentes. Os checkboxes abaixo representam a paridade completa,
por isso começam abertos; provas do experimento estão registradas à parte.

As skills e identidades permanecem conteúdo governado; operações e validações
de domínio passam pelo motor comum exposto por MCP. A adaptação ao Desktop
precisa demonstrar como o método é disponibilizado e executado, pois instalar
o MCPB não instala os hooks e subagentes do Code.

Clientes-alvo: Claude Code CLI, Code tab do Desktop e chat do Claude Desktop.
Preservar contratos e restrições Codex existentes; paridade Codex completa e
transporte remoto/web/mobile exigem escopo/qualificação próprios.

## Inventário conferido

| Superfície | Quantidade | O que a contagem prova |
|---|---:|---|
| Skills base | 53 | Catálogo coincide com 53 arquivos SKILL.md |
| Skills Tech Core | 12 | Catálogo coincide com 12 arquivos SKILL.md |
| Agentes canônicos | 7 | Catálogo e caminhos do manifesto |
| Handlers do bundle | 9 | Eventos declarados no manifesto; projeções runtime adicionais inventariadas nas 13 capacidades |
| Helpers declarados | 7 | Tools explícitos do manifesto; recursos transitivos ainda precisam do fechamento de empacotamento |
| Capacidades runtime | 13 | Declarações de capacidade; não são 13 provas nativas |
| Rotinas de manutenção | 18 | Jobs declarados; catálogo está unavailable sem enrollment/qualificação |

## Provas técnicas já concluídas — escopo limitado

- [x] Três ferramentas MCP com checkpoint/pause/resume e validação de revisão/tentativa.
- [x] Code → Desktop → conversa original do Code, com recusa exata da escrita antiga, em fixture sintético.
- [x] Atualização manual da extensão preservando o estado externo.
- [x] Instalador/updater experimental, feed assinado e gerações locais.
- [x] Atualização 0.0.3 → 0.0.4 pelo instalador real e CLI isolado, com readback e estado preservado no Mac ARM64.
- [x] Oito verificações SDK de instalação, continuidade, atualização e negativos.
- [x] Quatro revisões internas e CI pré/pós-merge das entregas experimentais.
- [ ] Publicação e sincronização em biblioteca SharePoint real — destino ainda não informado.
- [ ] Migração e paridade do Maestro completo — checklist de trabalho abaixo.

Essas provas não fecham funcionalidades que ainda não foram conectadas ao
MCP/plugin de produto nem qualificam computadores dos colegas ou Windows.

## Checklist de construção e aceite

### A. Contrato de paridade e arquitetura

- [ ] **A01 — Inventariar o produto completo.** Aceite: Catálogos, arquivos e dependências têm IDs estáveis, versão/hash e destino. Nenhum recurso desaparece na mudança de embalagem.
- [ ] **A02 — Definir a matriz Code CLI, Code tab e chat Desktop.** Aceite: Para cada função: nativa, adaptada ou pendente, com teste e evidência por cliente; transporte remoto/web/mobile é decisão separada.
- [ ] **A03 — Preservar um único motor de domínio.** Aceite: MCP e plugins usam os mesmos contratos e stores; não criar outra memória, outro ledger ou políticas duplicadas.
- [ ] **A04 — Reconciliar contratos vigentes e propostas antigas.** Aceite: Resolver explicitamente divergências de data-home, topologia de agentes e comportamento de hooks antes do freeze de produto.
- [ ] **A05 — Versionar a API e as compatibilidades.** Aceite: Contratos de entrada/saída, erros, versões mínimas, compatibilidade de schema e política de mudança documentados e testados.

### B. Estado, contextos e migração

- [ ] **B01 — Resolver managed core, HOME/brain e workspace.** Aceite: Um resolver aprovado identifica origem, instalação, owner, conta e caso; sem caminhos de fábrica ou fixture sintético no produto.
- [ ] **B02 — Preservar dados durante instalação e atualização.** Aceite: Dados, customizações, credenciais, logs e fontes permanecem fora dos pacotes; comparar snapshots antes/depois.
- [ ] **B03 — Migrar do ZIP preservando o original.** Aceite: Plano, execução, validação, retomada de falha e rollback observados; migração única, sem coexistência ambígua de hooks.
- [ ] **B04 — Manter namespaces e aliases legados admitidos.** Aceite: Material antigo continua acessível e identificável; aliases permitidos funcionam e papéis rejeitados não são reintroduzidos.
- [ ] **B05 — Separar múltiplos casos, contas e usuários.** Aceite: Contexto de um caso não entra no outro; troca de escopo exige seleção autorizada e deixa evidência local.
- [ ] **B06 — Cobrir concorrência e recuperação.** Aceite: Revisões, tentativas, locks, interrupção de processo, reexecução idempotente e rollback preservam fontes e estado válido.

### C. API MCP funcional completa

- [ ] **C01 — Contexto e navegação de escopo.** Aceite: Consultar contexto atual, workspaces, contas/casos admitidos e estado de onboarding sem varredura indiscriminada.
- [ ] **C02 — Perfil e contexto do owner.** Aceite: Ler/refinar identidade, estilo, trajetória, aspirações e facetas profissionais com confirmação e origem.
- [ ] **C03 — Execução completa.** Aceite: Criar trabalho/contrato, critérios, iniciar, inspecionar, checkpoint, pausar, retomar, bloquear, concluir, evidências e exportar conforme o ledger.
- [ ] **C04 — Memória e conhecimento.** Aceite: Expor captura elegível, leitura, consolidação, busca, correção, exclusão e retenção pelos stores existentes.
- [ ] **C05 — Agentes e revisão.** Aceite: Expor preparação/validação de pacotes de delegação e revisão quando necessários, sem transformar uma skill em concessão de autoridade.
- [ ] **C06 — Ingestão e integrações.** Aceite: Operações locais e fontes conectadas usam consentimento, proveniência, limites, estados indisponíveis e conectores já autorizados.
- [ ] **C07 — Diagnóstico e manutenção.** Aceite: Status, versão, capacidades, drift, jobs e reparos permitidos ficam acessíveis pelo mesmo motor.
- [ ] **C08 — Aplicar limites no servidor.** Aceite: Todas as mutações validam escopo, tamanho, permissão e path; revisão/tentativa são exigidas quando previstas no contrato da operação. Descrições de ferramentas não substituem validação.
- [ ] **C09 — Produzir contexto limitado e erros úteis.** Aceite: Paginação, orçamento de contexto, erro tipado e recuperação segura; nenhum retorno despeja o brain inteiro ou credenciais.

### D. Plugin completo do Code

- [ ] **D01 — Empacotar as 53 skills base.** Aceite: Cada ID do inventário tem conteúdo, recursos transitivos, descoberta, invocação e teste funcional no pacote instalado.
- [ ] **D02 — Empacotar as 12 skills Tech Core.** Aceite: Seleção técnica explícita, dependência da base, namespaces/IDs sem colisões e inventário consistente.
- [ ] **D03 — Empacotar os sete agentes e templates admitidos.** Aceite: Identidades, registries e políticas chegam íntegros; nomes personalizados e contexto permanecem locais.
- [ ] **D04 — Transportar orientação do CLAUDE.md e AGENTS.md.** Aceite: Mapear cada instrução para bootstrap, skill ou política canônica; nenhuma regra depende de estar na raiz do ZIP antigo.
- [ ] **D05 — Preservar os nove handlers do bundle.** Aceite: Eventos, matchers, criticidade, stdin/stdout/exit, codificação e orçamento do host cobertos por contratos.
- [ ] **D06 — Resolver dependências dos helpers.** Aceite: Os sete tools declarados e demais recursos têm runtime entregue/descoberto; não pressupor Python/Node disponíveis nem instalar software silenciosamente.
- [ ] **D07 — Qualificar Code CLI e Code tab separadamente.** Aceite: Descoberta, hooks, skills, MCP, permissão, delegação e versão carregada observados em cada superfície.

### E. Desktop com capacidade equivalente

- [ ] **E01 — Criar extensão de produto vinculada ao estado do participante.** Aceite: MCPB por plataforma, configuração mínima e seleção de escopo; substituir o binding case-a do experimento.
- [ ] **E02 — Entregar descoberta e orientação no chat.** Aceite: O usuário encontra capacidades e aciona o método correto; provar o mecanismo suportado para disponibilizar skills/instruções no Desktop.
- [ ] **E03 — Adaptar workflows sem presumir hooks do Code.** Aceite: Abertura, contexto por mensagem, fechamento e manutenção têm mecanismo explícito no Desktop; testar o efeito equivalente.
- [ ] **E04 — Adaptar delegação e revisão ao host.** Aceite: Definir execução nativa ou adaptada dos agentes; se houver execução de modelo no servidor, declarar provedor, autenticação, custo, timeout e autoridade.
- [ ] **E05 — Manter controles do servidor mesmo sem PreToolUse.** Aceite: Isolamento e validação das operações Maestro funcionam no Desktop; não alegar sandbox para ferramentas externas do host.
- [ ] **E06 — Retomar trabalhos entre clientes em ambas as direções.** Aceite: Code → Desktop e Desktop → Code recuperam o mesmo trabalho, memória e próximo passo; escritor antigo é rejeitado.

### F. Memória, brain e aprendizado

- [ ] **F01 — Entregar L1, L2, L3 e lifetime.** Aceite: Captura, síntese, promoção, leitura e limites seguem o motor e a elegibilidade aprovados; não confundir checkpoint com memória completa.
- [ ] **F02 — Injetar contexto correto nas sessões.** Aceite: Owner, conta, caso, memória, trabalho aberto e snapshots dos agentes são limitados, atuais e com proveniência.
- [ ] **F03 — Entregar dreaming leve e profundo.** Aceite: Cadência, autorização, execução determinística/model-backed e reentrância testadas; jobs ausentes continuam explícitos até implementação.
- [ ] **F04 — Preservar aprendizado sobre o owner.** Aceite: Observações, hipóteses e propostas ficam distintos de fatos confirmados; learn-from-logs, feedback, métodos e estilo respeitam consentimento.
- [ ] **F05 — Implementar correção, exclusão e retenção.** Aceite: Mudanças propagam aos índices e derivados; conteúdo removido não reaparece em memória, busca ou wiki.
- [ ] **F06 — Manter snapshots dos agentes e histórico permitido.** Aceite: Estado de agente e prompt history opt-in têm orçamento, origem, escopo e controles de reversão/deleção.

### G. Métodos profissionais e cadência

- [ ] **G01 — Onboarding rápido e completo.** Aceite: Entrevista, retomada sem repetição, perfis e preparo de ambiente passam pelo pacote instalado.
- [ ] **G02 — Rotina diária e semanal.** Aceite: start-day, eod e retro leem/escrevem o escopo correto e respeitam os modos atendido e agendado existentes.
- [ ] **G03 — Conta, case e canon.** Aceite: Criação, kickoff, fontes, mapeamento SharePoint, ingestão de canon e decisões preservam referências e isolamento.
- [ ] **G04 — Análise e entregas.** Aceite: Qualitativa, quantitativa, entrevistas, reuniões, storyline, slides, Excel, escrita e gates funcionam com entradas/saídas rastreáveis.
- [ ] **G05 — Desenvolvimento profissional.** Aceite: Feedback, upward feedback, FoDAIS, trajetória, métodos e aprendizados preservam validação individual e privacidade.
- [ ] **G06 — Qualidade e engenharia.** Aceite: As 12 skills técnicas, investigate, QA, Gamma e Yoda produzem evidência proporcional e mantêm limites de ação.

### H. Documentos, SharePoint, caseOS e atlas

- [ ] **H01 — Ingestão local com runtimes verificáveis.** Aceite: Docling/MarkItDown e fallbacks têm dependências entregues, formatos, limites e falhas explicados; contrato sem runtime não fecha o item.
- [ ] **H02 — Fontes SharePoint de owner e case.** Aceite: Mapear/enrolar apenas pastas escolhidas, materializar rationales, manter origem e watermarks e permitir revogação.
- [ ] **H03 — Busca de trabalho anterior.** Aceite: Índice privado, consulta explícita, atualização incremental e exclusão corretas; coletor nativo qualificado por runtime.
- [ ] **H04 — caseOS com consentimento e escopo.** Aceite: Conexão, consulta/tutorial, preparo de ingestão e enriquecimento preservam revisão local e não fazem publicação implícita.
- [ ] **H05 — Navegação do brain e wiki/OKF.** Aceite: Índice, backlinks, órfãos, links quebrados, tarefas e atlas derivados reproduzíveis; fontes continuam autoritativas.
- [ ] **H06 — Delta de pastas de conhecimento.** Aceite: Manifestos de fontes, checkpoints de leitura e reconcile-drop preservam comparabilidade sem retenção indevida de conteúdo.
- [ ] **H07 — Preservar restrições específicas de runtime.** Aceite: Por exemplo, coleta SharePoint em Codex continua vedada onde a política atual determina; um novo MCP não amplia essa permissão.

### I. Agentes, governança e manutenção

- [ ] **I01 — Maestro como ponto de entrada.** Aceite: Roteamento e critérios de materialidade preservados; o empacotamento não altera autoridade ou acesso a ferramentas.
- [ ] **I02 — Case e Client Account.** Aceite: Framing, execução, validação e retorno têm pacote delimitado, contexto correto e continuidade.
- [ ] **I03 — PA Expert, Gamma e Yoda.** Aceite: Expert registry FPA/IPA, qualidade longitudinal e revisão interna têm evidências reais e limites de acesso.
- [ ] **I04 — Darwin operacional e estrutural.** Aceite: Diagnóstico e reparos admitidos são reversíveis; evolução estrutural permanece proposta quando esse é o contrato.
- [ ] **I05 — Declarar e entregar as 18 rotinas de manutenção.** Aceite: Cada job tem executor, gatilho, consentimento, status e prova próprios; catálogo default_enabled não comprova ativação.
- [ ] **I06 — Agendador e recuperação de presença.** Aceite: Não sobrepor ocorrências, respeitar timezone, recuperar jobs vencidos e distinguir wake-up, execução e commit.
- [ ] **I07 — Observabilidade e melhoria federada.** Aceite: Canary, métricas e propostas federadas seguem enrollment/revogação e metadata-only; funções propostas ficam separadas das já implementadas.
- [ ] **I08 — Tratar telemetry export proposta em trilha própria.** Aceite: Spec 054 e seu receptor/anonimização entram no inventário de horizonte, sem vender uma proposta como funcionalidade atual.

### J. Distribuição completa e atualização

- [ ] **J01 — Construir release completo por plataforma.** Aceite: Base, Tech Core, helpers, servidor MCP, extensão, políticas e recursos pertencem a uma versão coerente.
- [ ] **J02 — Preparar publicação e bootstrap confiáveis.** Aceite: Autoridade organizacional, chaves, verificação pré-execução, assinatura de plataforma e proveniência do primeiro instalador têm responsáveis definidos.
- [ ] **J03 — Publicar no SharePoint escolhido.** Aceite: Biblioteca, permissões de publicador/leitura, sincronização, disponibilidade local e payload antes do ponteiro validados; URL ainda pendente.
- [ ] **J04 — Instalar e atualizar por entrada simples.** Aceite: Uma ação prepara/instala o que cada host suporta; confirmação Desktop e pré-requisitos ficam claros, sem edição manual de JSON.
- [ ] **J05 — Comprovar N → N+1 no pacote completo.** Aceite: Verificar versão carregada, hooks, skills, agentes e MCP novos com estado e customizações preservados.
- [ ] **J06 — Implementar rollback e recall de produto.** Aceite: Retornar a versão íntegra, recuperar instalação interrompida e documentar limites de offline/sessões abertas; defesa de replay não equivale a rollback.
- [ ] **J07 — Desinstalar e exportar sem apagar o brain.** Aceite: Remover runtime/projeções com controle separado para dados do usuário e recursos do host.
- [ ] **J08 — Qualificar Mac Apple Silicon, Mac Intel e Windows.** Aceite: Execução real, usuário não elevado, políticas, espaços, UTF-8 e proteções normais; cross-compilation não fecha a qualificação.

### K. Aceite ponta a ponta e piloto de produto

- [ ] **K01 — Cenário de primeiro valor em máquina limpa.** Aceite: Instalar → onboarding → criar caso → ingerir fonte → produzir entrega → salvar → retomar por outro cliente.
- [ ] **K02 — Cenário de memória e rotina.** Aceite: Fechar dia → consolidar elegivelmente → nova sessão → consumir memória correta → abrir dia/semana.
- [ ] **K03 — Cenário de agentes e revisão.** Aceite: Pedido realista → Case/Account/Expert conforme contrato → Gamma/Yoda quando cabível → resultado e evidência.
- [ ] **K04 — Cenário de atualização e recuperação.** Aceite: Publicar N+1 → sincronizar → atualizar → ler versão ativa → retomar trabalho → rollback quando solicitado.
- [ ] **K05 — Negativos de escopo e integridade.** Aceite: Cross-case, revisão/tentativa antiga, fonte não autorizada, pacote incompleto, assinatura inválida, replay, indisponibilidade e interrupção.
- [ ] **K06 — Revisar com quatro lentes.** Aceite: Especificação, standards, segurança/autoridade e interoperabilidade sobre a mesma árvore e os artefatos exatos.
- [ ] **K07 — Executar piloto de produto com participantes.** Aceite: 2–3 participantes de perfis/plataformas definidos, suporte/feedback, critérios de sucesso e evidências de primeira instalação; ainda não iniciado.
- [ ] **K08 — Fechar a matriz inteira antes de declarar paridade.** Aceite: Todo item da baseline tem teste nos clientes-alvo; exceção exige decisão explícita, não exclusão silenciosa do inventário.

## Ordem de execução

1. A + B: inventário fechado, contrato de destino e resolver/state/migração em fixtures.
2. C + D: ampliar motor/API e empacotar produto completo do Code, com guard e dependências reais.
3. E: adaptar o chat Desktop e comprovar workflows equivalentes, sem presumir hooks/skills nativos.
4. F a I: conectar memória, métodos, integrações, agentes e manutenção; cada função indisponível exige implementação/qualificação própria.
5. J + K: release completo, SharePoint real, recuperação e piloto de produto nas plataformas.

As sondas corporativas F0 podem ocorrer em paralelo à preparação isolada.
Congelar F1 e promover F2/F3/F4 segue os gates do epic #428. A assinatura
experimental do kit não substitui autoridade/assinatura de produção. A ausência
temporária da URL SharePoint não bloqueia a construção local das etapas A–I.

## Inventário nominal das skills

Para cada linha: empacotar conteúdo e dependências, disponibilizar o método no
Code e no Desktop por mecanismo suportado, executar um cenário coerente com a
skill e guardar a evidência por host. A existência no catálogo é a baseline,
não declaração de execução nativa da função inteira.

### base — 53 skills

- [ ] `account-case-setup` — Conta e Projeto.
- [ ] `agent-identity-setup` — Identidade dos Agentes.
- [ ] `bcg-case-kickoff` — Kickoff de Case.
- [ ] `bcg-deck` — Deck e Storyline.
- [ ] `brain-index` — Índice do brain.
- [ ] `case-agent-setup` — Case Agent.
- [ ] `case-canon-ingest` — Canon do Case.
- [ ] `case-decision-log-entry` — Registro de Decisão do Case.
- [ ] `case-sharepoint-ingest` — SharePoint do Case.
- [ ] `case-sharepoint-map` — Mapa do SharePoint do Case.
- [ ] `caseos-connect` — caseOS MCP.
- [ ] `caseos-prepare-enrichment` — caseOS — preparar enriquecimento.
- [ ] `caseos-prepare-ingest` — caseOS — preparar ingestão.
- [ ] `caseos-tutorial` — caseOS — aprender e consultar.
- [ ] `client-delivery-gate` — Gate de Entrega ao Cliente.
- [ ] `craft-update` — Registro de Método.
- [ ] `deck-drill` — Ensaio de Deck.
- [ ] `deck-review` — Review de Deck.
- [ ] `dream-memory` — Consolidação de Memória.
- [ ] `eod` — Fechar o Dia.
- [ ] `excel-financial-model` — Modelo financeiro em Excel.
- [ ] `execution-continuity` — Execution Continuity.
- [ ] `expert-interview-guide` — Guia de Entrevista com Expert.
- [ ] `feedback-capture` — Registro de Feedback.
- [ ] `find-prior-work` — Buscar Trabalho Anterior.
- [ ] `fodais-performance-review` — Avaliação FoDAIS.
- [ ] `ingest-content` — Ingestão de Conteúdo.
- [ ] `interaction-profile` — Perfil de Interação.
- [ ] `investigate` — Investigar.
- [ ] `learn-from-logs` — Aprender com os Logs.
- [ ] `learnings-bridge` — Ponte de Aprendizados.
- [ ] `maestro-doctor` — Diagnóstico do Maestro.
- [ ] `maestro-environment-setup` — Preparar Ambiente Maestro.
- [ ] `maestro-knowledge-pill` — Maestro Knowledge Pill.
- [ ] `maestro-onboarding` — Onboarding do Maestro.
- [ ] `maestro-operator` — Operador do Maestro.
- [ ] `maestro-runtime-checkup` — Checkup do Maestro.
- [ ] `maestro-setup-update` — Instalação e Atualização do Maestro.
- [ ] `meeting-close` — Fechamento de Reunião.
- [ ] `meeting-to-work-items` — Reunião em Itens de Trabalho.
- [ ] `owner-background` — Background do dono.
- [ ] `qa-gate` — Gate de QA.
- [ ] `qualitative-analysis` — Análise Qualitativa.
- [ ] `quantitative-analysis` — Análise Quantitativa.
- [ ] `retro` — Retrospectiva da Semana.
- [ ] `sharepoint-ingest` — Ingestão do SharePoint.
- [ ] `slide-summary` — Resumo de Slides.
- [ ] `start-day` — Abrir o Dia.
- [ ] `upward-feedback` — Upward Feedback.
- [ ] `wayfinder` — Wayfinder.
- [ ] `workspace-agent-setup` — Alias Legado do Case Agent.
- [ ] `writing-tighten` — Apertar a escrita.
- [ ] `yoda` — Yoda — pressure-test.

### tech-core — 12 skills

- [ ] `coverage-diagnose` — Diagnóstico de cobertura.
- [ ] `data-pipeline-quality` — Qualidade de pipeline de dados.
- [ ] `data-science-evaluation` — Avaliação de ciência de dados.
- [ ] `decision-log-entry` — Decision Log Entry.
- [ ] `pr-quality-loop` — Quality loop de PR.
- [ ] `pr-review` — Revisão de pull request.
- [ ] `reproducible-data-run` — Execução de dados reproduzível.
- [ ] `review-explain-change` — Revisar e explicar mudança.
- [ ] `spec-driven-delivery` — Entrega orientada por especificação.
- [ ] `test-and-evidence` — Teste e evidência.
- [ ] `unit-test-wave` — Onda de testes unitários.
- [ ] `xfail-bug-capture` — Captura de bug via teste.

## Inventário nominal dos agentes

- [ ] `case-agent` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.
- [ ] `client-account-agent` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.
- [ ] `darwin` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.
- [ ] `gamma-guardian` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.
- [ ] `maestro` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.
- [ ] `pa-expert` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.
- [ ] `yoda` — identidade, contexto, registro, ferramentas/autoridade, rota, retorno e teste no host.

O alias `workspace-agent-setup` continua no catálogo de skills por compatibilidade. Templates/aliases legados exigem a política canônica: não transformar antigos practice/subject em agentes novos admitidos.

## Inventário dos nove handlers

| Pendente | Evento | Handler | Criticidade |
|---|---|---|---|
| [ ] | `session_start` | `first-run-scaffold.sh` | required |
| [ ] | `session_start` | `session-start-memory-inject.sh` | required |
| [ ] | `user_prompt_submit` | `context-inject-userprompt.sh` | recommended |
| [ ] | `pre_tool_use` | `block-cross-case-writes.sh` | required |
| [ ] | `pre_tool_use` | `announce-agent-dispatch.sh` | recommended |
| [ ] | `stop` | `session-stop-dream.sh` | recommended |
| [ ] | `stop` | `session-stop-agent-check.sh` | recommended |
| [ ] | `stop` | `session-stop-brain-index.sh` | recommended |
| [ ] | `stop` | `session-stop-eod-check.sh` | recommended |

Em cada handler: portabilidade Bash/PowerShell, evento/matcher, stdin/stdout/exit e efeito real. Onde o Desktop não fornece o evento, implementar e testar uma adaptação explícita.

## Helpers declarados e recursos transitivos

- [ ] `brain-index.py` — compilador de índice, backlinks, visão de tarefas e índice de roteamento.
- [ ] `brain-route.py` — roteia a mensagem do dono para páginas do brain, lendo só o índice.
- [ ] `skill-route.py` — roteia a mensagem do dono para as skills que a resolvem.
- [ ] `agent-route.py` — decide qual spoke despachar, a partir de activation-policy.json.
- [ ] `session-memory-emit.py` — monta os blocos de memória do SessionStart num spawn só, dentro do orçamento.
- [ ] `paths-check.py` — verifica que todo caminho citado pelo produto existe.
- [ ] `migrate-data-to-brain.py` — migra uma workspace data/ da versão anterior para brain/.

Também fechar a árvore de recursos, schemas, políticas, templates, índices e executáveis referenciados por skills/agentes; os sete helpers não esgotam dependências transitivas.

## Capacidades runtime — declarações da baseline

| Pendente | ID | Estado declarado Claude | Estado declarado Codex |
|---|---|---|---|
| [ ] | `runtime_discovery` | native | native |
| [ ] | `sharepoint_work_local_query` | native | native |
| [ ] | `sharepoint_work_collection` | unavailable | unavailable |
| [ ] | `local_ingestion_markitdown` | unavailable | unavailable |
| [ ] | `agent_orchestration` | operational_beta | unavailable |
| [ ] | `session_start` | operational_beta | unavailable |
| [ ] | `pre_action_guard` | operational_beta | unavailable |
| [ ] | `post_action_observe` | operational_beta | unavailable |
| [ ] | `stop_finalize` | operational_beta | unavailable |
| [ ] | `context_injection` | operational_beta | unavailable |
| [ ] | `native_subagent_start` | operational_beta | unavailable |
| [ ] | `native_subagent_stop` | operational_beta | unavailable |
| [ ] | `darwin_housekeeping` | unavailable | unavailable |

Esses estados são do catálogo de produto, não readback de instalação nova. O chat Desktop precisa de sua própria coluna/evidência; a declaração `claude` não comprova as duas superfícies.

## Rotinas de manutenção — inventário completo

| Pendente | Job | Executor | Estado declarado |
|---|---|---|---|
| [ ] | `memory-l1-capture` | local_adapter | unavailable |
| [ ] | `memory-checkpoint` | deterministic | unavailable |
| [ ] | `memory-light-dream` | deterministic | unavailable |
| [ ] | `memory-deep-dream` | model_adapter | unavailable |
| [ ] | `memory-retention-check` | deterministic | unavailable |
| [ ] | `wiki-incremental-sync` | deterministic | unavailable |
| [ ] | `wiki-reconcile` | deterministic | unavailable |
| [ ] | `wiki-integrity-check` | deterministic | unavailable |
| [ ] | `skills-index-refresh` | deterministic | unavailable |
| [ ] | `runtime-health-check` | deterministic | unavailable |
| [ ] | `capability-recheck` | deterministic | unavailable |
| [ ] | `runtime-drift-check` | deterministic | unavailable |
| [ ] | `self-observation-capture` | local_adapter | unavailable |
| [ ] | `update-check` | local_adapter | unavailable |
| [ ] | `darwin-housekeeping-daily` | local_adapter | unavailable |
| [ ] | `darwin-deep-weekly` | local_adapter | unavailable |
| [ ] | `yoda-self-review-weekly` | model_adapter | unavailable |
| [ ] | `darwin-structural-evolution-proposal` | local_adapter | unavailable |

Preservar o motivo de unavailable e exigir enrollment/execução/commit próprios. default_enabled não é execução; jobs com modelo precisam de autorização e configuração válidas.

## Funcionalidades propostas e trabalho paralelo

Spec 054 (telemetria anonimizada), mecanismos de delta/manifests e demais
propostas precisam de rastreio próprio: inventariar e decidir o horizonte de
implementação sem apresentá-los como funcionalidades já prontas. O pacote
publicado contém base + Tech Core. Trabalho paralelo não publicado, incluindo
pm-core, não entra silenciosamente na baseline; deve ser reconciliado antes de
um próximo freeze caso a intenção seja incluí-lo.

## Definition of done do Maestro completo nesta arquitetura

- [ ] Todas as 65 skills têm pacote, descoberta, dependências e evidência funcional nos clientes-alvo.
- [ ] Os sete agentes, nove handlers e suas projeções adicionais têm comportamento preservado ou adaptação explicitamente validada.
- [ ] Contexto, execução, memória, conhecimento, owner e manutenção usam autoridades/stores únicos.
- [ ] Funções inicialmente unavailable são entregues e qualificadas quando incluídas no escopo final; nenhuma é marcada pronta por estar listada.
- [ ] Instalação, atualização, rollback e remoção preservam dados e customizações em máquina limpa.
- [ ] Cenários completos atravessam Code/Desktop, atualização e recuperação sem regressão de contexto/isolamento.
- [ ] Release de produto tem autoridade, plataforma, suporte e piloto próprios; aprovação do experimento não é reaproveitada como aprovação de produto.
- [ ] Não resta lacuna sem decisão explícita na matriz. A contagem desta checklist não é uma porcentagem automática da missão.

## Fontes e rastreabilidade

Baseline: main `d9da6e3dcb581e998c0d16ca59af6e6365b98082`; árvore publicada
`1bbaaa1ca666b7f6540897ea82ec66ca9d9519f1`, idêntica à árvore local auditada.
Catálogos e manifesto: bundles/base/manifest.json, bundles/catalog/catalog.json,
bundles/base/skills/catalog.json, bundles/tech-core/skills/catalog.json,
bundles/base/agents/catalog.json, bundles/base/runtime/capabilities.json e
bundles/base/runtime/maintenance.json. Contratos: specs/006, 010, 013, 029,
031 (orchestration), 034, 035 (bundles/evidence), 036 (maintenance), 037
(prior-work), 038, 044–049, 052–054 e proposta 004.

Provas técnicas: PRs #446, #447 e #448 e os recibos em dev/spikes/maestromcp/evidence/.
Este documento prepara a construção; não altera código, release, biblioteca
SharePoint ou estado de usuário.
