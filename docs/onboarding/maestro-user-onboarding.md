# Onboarding do usuário do Maestro

## O que você vai ganhar

O Maestro é um sistema operacional profissional local para trabalhos que não
cabem em uma única conversa. Ele organiza contexto, execução, evidências,
continuidade e limites de acesso em torno do trabalho real — sem exigir que o
usuário transforme cada interrupção em um novo briefing e sem transformar
material de cliente em memória genérica.

Este guia serve para o usuário de uma instalação autorizada do Maestro. Ele não
é o guia para contribuir com o código-fonte. Se você está clonando o
repositório, use [`CONTRIBUTING.md`](../../CONTRIBUTING.md), o
[guia do development harness](../development-harness.md) e, quando aplicável,
o fluxo de contribuidor descrito em
[`windows-contributor-prompt.md`](windows-contributor-prompt.md).

> **Estado atual:** o repositório tem a camada de contratos, CLI e adaptadores
> em evolução. A distribuição de piloto, a assinatura dos artefatos e o
> runtime local de ingestão ainda precisam de evidência própria. Se `doctor`
> reportar `unavailable`, isso é um estado seguro e honesto — não uma falha a
> ser contornada instalando Python, `pip`, chaves ou scripts externos.
>
> Para recuperar trabalhos anteriores em pastas SharePoint autorizadas, use o
> [onboarding específico de prior-work](sharepoint-prior-work-onboarding.md).
> A regra corporativa é explícita: Claude pode ser qualificado para coletar;
> Codex não se conecta ao SharePoint e usa somente o índice local verificado.

## 0. Escolha a jornada correta

| Você é | Use | Comandos principais | O que não fazer |
| --- | --- | --- | --- |
| **Participante de piloto ou usuário autorizado** | Este guia e um release privado verificado. | `bcgos version`, `init`, `status`, `doctor` e os comandos liberados pelo release. | Não clonar o repositório nem usar o harness de desenvolvimento para instalar o produto. |
| **Contribuidor do repositório** | [`CONTRIBUTING.md`](../../CONTRIBUTING.md) e o [development harness](../development-harness.md). | `go run ./dev/harness doctor`, `setup`, `validate` e `validate --full`. | Não tratar um gate local como CI verde, review concluído, mergeabilidade ou autorização de piloto. |

### Estados que precisam permanecer separados

O resultado de um comando é evidência de uma superfície específica. A escada
abaixo não é um atalho entre superfícies:

| Estado | Evidência mínima | Não significa |
| --- | --- | --- |
| **Contrato validado** | Core determinístico, schemas, testes e harness local passam. | Runtime nativo qualificado ou release confiável. |
| **Runtime qualificado** | Sessão nativa nova invoca o adapter instalado e produz evidência limitada e reproduzível. | Configuração local, teste direto ou receipt `adapter_command`. |
| **CI** | Workflow hospedado obrigatório executa e passa para o commit exato. | `validate` local, workflow sem steps ou execução ignorada. |
| **Review** | Revisor humano avalia o diff e suas evidências. | Merge automático ou branch mergeável. |
| **Mergeabilidade** | Branch remoto atualizado, checks/review/regras satisfeitos e estado remoto compatível. | Que o merge ocorreu ou que o produto está pronto para usuários. |
| **Pilot-ready** | Release assinado, aceitação em dispositivos limpos, suporte/incidente e gate de piloto aprovados. | Uma instalação de teste, contrato ou documentação isolada. |

No estado atual, o repositório está em **contract-validated**. A qualificação
nativa, a distribuição assinada e o gate **pilot-ready** exigem evidência
própria; uma capacidade `unavailable` deve permanecer fechada e explícita.

## 1. A promessa em linguagem simples

Uma sessão comum responde a uma pergunta. O Maestro ajuda a conduzir um
trabalho:

```text
definir o resultado → trabalhar com evidência → pausar com checkpoint
→ retomar sem perder o fio → revisar → melhorar o modo de trabalhar
```

Os benefícios principais são:

| Benefício | Como aparece na prática | Por que importa |
| --- | --- | --- |
| Continuidade | Objetivo, revisão, tentativa, checkpoint e próxima ação ficam em um ledger local. | O trabalho sobrevive a interrupções, trocas de sessão e handoffs. |
| Governança | Ações e conclusões importantes passam por contratos estreitos, evidência e, quando aplicável, revisão autenticada do Walter. | Automação ganha rastreabilidade sem receber autoridade ilimitada. |
| Privacidade | Conteúdo de cliente, credenciais e material profissional permanecem fora do bundle gerenciado e da telemetria. | O produto melhora sem transformar trabalho confidencial em dataset compartilhado. |
| Portabilidade | Claude é a projeção primária e Codex segue contratos runtime-neutral. | A política do Maestro não fica presa à implementação de um único runtime. |
| Ingestão local | Docling é a rota primária planejada; fallbacks determinísticos aprovados podem atender formatos específicos. | Documentos entram no fluxo sem API key nem upload remoto implícito. |
| Melhoria com evidência | O canary usa sinais tipados e metadados, não prompts ou documentos. | A evolução do produto pode ser priorizada por fricção observada. |

## 2. Antes de começar

### Você precisa de

- uma instalação ou release privada explicitamente autorizada;
- um runtime suportado pelo piloto, quando o caso de uso exigir integração
  nativa;
- uma pasta local escolhida por você para o workspace;
- autorização para usar os arquivos e fontes que serão processados;
- tempo para completar a primeira jornada em um workspace de teste, sem dados
  sensíveis.

### Você não precisa de

- Git, Go, Python, Node ou Docker para usar uma instalação de piloto;
- uma API key para o caminho local;
- uma pasta de cliente dentro do repositório do Maestro;
- copiar documentos para o bundle gerenciado;
- fornecer senha, token ou código de recuperação no chat.

O usuário final recebe um artefato verificado. O repositório e o harness de
contribuição pertencem ao time que desenvolve o produto, não ao onboarding
normal do piloto.

### Escolha o workspace certo

Prefira uma pasta local, fora do OneDrive ou de outra sincronização automática,
com um nome que não exponha o cliente. Confirme o caminho antes de inicializar.
O workspace é privado e separado do core gerenciado; ele não deve ser usado
como lixeira de documentos ou como cópia do repositório.

## 3. O modelo mental do Maestro

| Camada | Responsabilidade | Regra do usuário |
| --- | --- | --- |
| `bcgos` | CLI local para inicialização, diagnóstico, trabalho, perfil e inspeção. | Use os comandos do release autorizado. |
| Core gerenciado | Contratos, políticas, schemas, skills e agentes aprovados. | Não edite nem substitua manualmente. |
| Workspace | Contexto e artefatos privados do trabalho atual. | Mantenha apenas material autorizado e necessário. |
| Adaptador | Traduz contratos do Maestro para Claude ou Codex. | Configuração local não prova que um runtime nativo está qualificado. |
| Runtime pack | Componentes pesados, como extração local e modelos aprovados. | Instale apenas pelo fluxo verificado do Maestro. |
| Ledger | Estado de trabalho, checkpoints, evidências e receipts locais. | Consulte-o para retomar; não o trate como transcript completo. |
| Atlas/wiki | Navegação derivada e governada sobre fontes autorizadas. | Não é um segundo depósito informal de memória. |
| Wiki organizacional de prior-work | Catálogo separado para recuperar trabalhos anteriores após pedido explícito. | Não ativa em toda conversa e não concede SharePoint ao Codex. |

Essa separação é o que permite vender continuidade sem prometer memória
ilimitada, automação irrestrita ou compartilhamento invisível.

## 4. Entrevista inicial: calibrar o self profissional

Na primeira sessão, o Maestro não começa uma tarefa profissional. Ele conduz
uma entrevista, uma pergunta por vez, e mostra a interpretação de cada resposta
antes de propor qualquer gravação local. O objetivo é construir um self
profissional corrigível — não inferir personalidade nem importar silenciosamente
o conteúdo de outro sistema.

O percurso completo cobre oito facetas: papel profissional, estilo de
comunicação, preferências de trabalho, voz externa, motivações profissionais,
barra de qualidade/QA, regras de decisão e limites de trabalho. A jornada curta
cobre papel, comunicação, preferências e barra de qualidade; começa mais rápido,
mas demora mais para absorver voz, motivações, regras de decisão e limites.

Personality/psychological profile, história pessoal, fé, avaliações e identidade
visual ficam fora da entrevista padrão. Só entram por uma etapa local,
explicitamente autorizada, com finalidade e revisão próprias.

Isso não significa que o Maestro conheça apenas o cargo. Depois da revisão do
percurso escolhido, ele pode oferecer camadas opcionais — propósito e não
negociáveis, contexto pessoal autorizado, personalidade/avaliações ou identidade
visual — sempre com finalidade, fonte, leitores e retenção explícitos. Essas
camadas não são necessárias para começar um trabalho profissional, não entram em
contexto de cliente por padrão e permanecem `unavailable` quando não houver um
adaptador local qualificado.

Escolha entre:

- **Curta — cerca de 7 minutos:** baseline operacional para começar com segurança.
- **Completa — cerca de 25 minutos:** perfil profissional mais fiel para
  recomendações, comunicação e revisão de qualidade desde o início.

Em ambos os casos, o Maestro resume o que entendeu, espera sua correção e só
depois apresenta o digest para confirmação final.

## 5. Primeira configuração: percurso recomendado

Faça a primeira configuração em um workspace de teste. O roteiro abaixo mostra
o destino da experiência; um comando que ainda não estiver liberado deve ser
reportado como `unavailable`, nunca emulado manualmente.

### Passo 1 — Verifique a instalação

```text
bcgos version
bcgos doctor <workspace>
```

O diagnóstico deve explicar, em linguagem simples:

- versão do CLI e compatibilidade do bundle;
- sistema operacional e runtime detectado;
- capacidades `supported`, `degraded`, `blocked` ou `unavailable`;
- se o workspace está dentro dos limites esperados;
- qual é a próxima ação segura.

Se `bcgos` não existir, pare. Não instale uma versão encontrada na internet,
não rode `pip install`, não aceite um binário sem assinatura e não forneça
credenciais no chat. O próximo passo é pedir o release privado autorizado ao
responsável pela distribuição.

### Passo 2 — Inicialize o workspace

Depois de confirmar o caminho:

```text
bcgos init <workspace>
bcgos status <workspace>
bcgos doctor <workspace>
bcgos maestro status <workspace>
```

O `init` deve ser idempotente: executar novamente não pode apagar ou substituir
configuração e dados existentes. O `status` mostra a situação; o `doctor`
explica diagnósticos. `maestro status` reúne a calibração, tarefas abertas,
checkpoint, memória, manutenção e evidência de runtime já existentes, com uma
próxima ação segura. Nenhum desses comandos cria clientes, projetos ou conteúdo
de trabalho sem pedido explícito.

### Passo 2.1 — Conecte o Maestro ao seu runtime

Depois do `init`, escolha o runtime que você usa no workspace:

```text
bcgos adapter install --runtime claude <workspace>
# ou
bcgos adapter install --runtime codex <workspace>
```

Esse passo instala os hooks locais e também deixa no próprio workspace uma
orientação completa e legível (`CLAUDE.md` ou `AGENTS.md`) com os blocos do OS,
além das skills reais do bundle em `.claude/skills/` ou `.codex/skills/`.
Você pode abrir esses arquivos diretamente no editor. A instalação preserva
texto que já exista; se uma skill gerenciada tiver sido alterada manualmente,
ela para com `conflict` em vez de sobrescrevê-la.

Confira o resultado:

```text
bcgos adapter status --runtime claude <workspace>
```

Troque `claude` por `codex` quando aplicável. A configuração local prepara o
runtime, mas não prova que uma capability nativa já foi qualificada. Para isso,
é necessária uma observação em uma sessão nativa nova; consulte a
[matriz de evidências do lifecycle](../../specs/035-lifecycle-evidence-matrix.md).

### Passo 3 — Escolha a profundidade de interação

```text
bcgos profile show
```

O perfil muda a quantidade de explicação e sugestões, não muda permissões:

- **standard:** uma recomendação segura, linguagem simples e o próximo passo;
- **advanced:** a mesma rota, mais justificativa, diagnósticos e opções úteis;
- **power:** detalhes técnicos, premissas e trade-offs explícitos.

Nenhum perfil autoriza upload remoto, ignora verificação, pede uma credencial
em chat ou amplia escopo de acesso.

### Passo 4 — Inicialize o contexto profissional, se autorizado

```text
bcgos owner init
bcgos atlas init <workspace>
bcgos atlas status <workspace>
bcgos skills index
```

O contexto do owner é local, inspecionável e limitado às facetas autorizadas.
O atlas navega fontes derivadas e governadas; ele não deve ser preenchido com
segredos, dumps de conversa ou conteúdo de cliente sem política e consentimento.

Depois da confirmação inicial, o Maestro pode oferecer uma única pergunta para
aprofundar ou atualizar uma faceta profissional desconhecida ou antiga:

```text
bcgos owner expand status
bcgos owner expand next
```

Você pode responder por texto ou voz; a versão de áudio é apenas uma formulação
curta da mesma pergunta. A resposta vira um rascunho local somente com seu
consentimento e sua declaração de que não contém dados de cliente. O Maestro
mostra o rascunho inteiro e o digest; só a confirmação exata atualiza a faceta.
Ele nunca deduz uma resposta, preenche lacunas sozinho ou altera o perfil
psicológico.

### Passo 4.1 — Escolha as fontes SharePoint do projeto

Depois que a entrevista do owner estiver revisada e confirmada, o Maestro
pergunta uma vez se você quer indicar as pastas autorizadas do SharePoint deste
projeto ou começar sem essa fonte. Você pode adiar sem perder funcionalidade;
essa escolha fica registrada e o Maestro não repete a pergunta em toda sessão.

Para inspecionar o estado:

```text
bcgos prior-work source status --workspace <workspace>
```

Se você escolher conectar, o Maestro mostra as URLs canônicas das pastas para
revisão e envia um JSON estrito por entrada padrão para:

```text
bcgos prior-work source select --workspace <workspace> --stdin --confirm
```

Se preferir começar sem a fonte:

```text
bcgos prior-work source defer --workspace <workspace> --confirm
```

Essa primeira etapa registra somente ponteiros exatos em armazenamento privado
local; selecionar não é o mesmo que autorizar leitura. Em seguida, o Maestro
pergunta explicitamente se pode ler os materiais mais recentes dessas pastas e
criar racionais internos rastreáveis. Com essa autorização, um coletor Claude
qualificado lê apenas o escopo aprovado e envia um lote assinado para:

```text
bcgos prior-work rationale ingest --workspace <workspace> --stdin --confirm
```

O Maestro grava apenas racionais derivados em
`brain/knowledge/sharepoint-rationales/`, ordenados pelos materiais mais
recentes, com o link SharePoint, item, digest e data de modificação em cada
registro. O corpo bruto do documento não é copiado e o SharePoint continua
sendo a fonte de verdade. Sem enrollment, qualificação nativa ou runtime local
disponível, a ingestão falha fechada e nada é criado. Codex não coleta
SharePoint; ele pode consultar apenas um índice local já verificado.

### Passo 5 — Faça uma primeira tarefa pequena

Escolha uma tarefa que possa ser verificada em menos de uma hora: organizar um
briefing interno, comparar duas fontes autorizadas ou produzir uma decisão com
critérios explícitos. Comece declarando:

```text
Objetivo: o que precisa estar decidido ou entregue?
Escopo: quais fontes e workspace estão autorizados?
Critério: como saberemos que ficou bom?
Limite: o que o Maestro não pode fazer?
Próxima ação: qual é o menor passo reversível?
```

O objetivo é provar continuidade e qualidade, não testar todas as features no
primeiro dia.

## 6. Trabalhos longos: como pausar e retomar

Para trabalhos que atravessam sessões, use o ledger local. O fluxo conceitual é:

```text
create → start → checkpoint → evidence → pause
                         ↓
                       resume → inspect → complete/export
```

Comandos disponíveis na superfície do CLI:

```text
bcgos work create --workspace <workspace> --stdin
bcgos work start --workspace <workspace> --item <id> --revision <n>
bcgos work checkpoint --workspace <workspace> --item <id> --revision <n> --attempt <id> --stdin
bcgos work evidence --workspace <workspace> --item <id> --revision <n> --attempt <id> --criterion <id>
bcgos work pause --workspace <workspace> --item <id> --revision <n> --attempt <id>
bcgos work resume --workspace <workspace> --item <id> --revision <n>
bcgos work inspect --workspace <workspace> --item <id>
bcgos work export --workspace <workspace> --item <id>
```

Ao abrir uma nova sessão, o Maestro apresenta a mesma projeção de
`bcgos maestro status <workspace>`. Se houver um único trabalho ativo, ele
mostra apenas `bcgos://execution/active`, o estado e se existe checkpoint. Para
ler a próxima ação privada, resolva o ponteiro explicitamente:

```text
bcgos work next --active --workspace <workspace>
```

Um runtime configurado ou um receipt local pode aparecer como `configured` ou
`adapter_observed`; isso não significa `native_qualified`. `unavailable`
permanece explícito até existir evidência nativa atendida para a versão exata.
Nenhuma sessão fabrica checkpoint, injeta transcript ou promove memória para
deep dreaming/lifetime automaticamente.
Esse status não acumula sessões nem receipts: ele é recalculado em um formato
fechado de até 4 KiB. O histórico continua no ledger versionado, e retenção e
compactação continuam sob responsabilidade do ledger, da memória e dos stores
de receipts correspondentes.

Boas práticas:

1. Faça checkpoint quando o objetivo, a evidência ou a próxima ação mudar.
2. Escreva ponteiros e critérios suficientes para uma pessoa continuar; não
   replique o transcript inteiro.
3. Registre evidência por critério, não apenas uma frase “parece pronto”.
4. Ao retomar, leia o estado anterior e confirme o escopo antes de executar.
5. Se a tentativa anterior foi invalidada, trate-a como histórico; não a
   misture silenciosamente com a nova tentativa.
6. Exporte apenas o artefato autorizado, não o banco local completo.

O ledger é uma memória operacional auditável. Ele não é uma licença para
persistir tudo, nem substitui revisão humana em decisões de alto risco.

## 7. Ingestão de documentos e fontes

O objetivo da ingestão é transformar uma fonte autorizada em conhecimento local
estruturado, preservando proveniência e classificação de fidelidade.

```text
fonte local → checagem de política/tamanho → Docling local
→ resultado estruturado + Markdown → validação → downstream autorizado
```

Para formatos cobertos por um fallback aprovado, o route selector pode usar o
MarkItDown local. Ele é um componente delimitado do runtime pack, não uma
instalação Python para o usuário.

Exemplo da superfície prevista:

```text
bcgos ingest --workspace <workspace> --source <arquivo-local> --adapter markitdown
```

O resultado deve informar rota, fidelidade, proveniência, retenção e limitações.
As garantias são:

- a fonte original permanece onde o usuário a colocou;
- o conteúdo bruto não entra no bundle gerenciado, Git ou atlas compartilhado;
- falha de extração não altera memória, wiki ou conhecimento compartilhado;
- serviço remoto nunca é fallback implícito;
- runtime pack ausente significa `unavailable` e mostra o próximo passo seguro.

Não trate “Markdown gerado” como prova de qualidade. Em documentos complexos,
confira tabelas, ordem, notas, imagens, OCR e referências antes de usar o
resultado em uma decisão.

## 8. Skills e agentes: o que o Maestro pode organizar

As skills são capacidades versionadas e governadas, não prompts soltos. Entre as
principais superfícies descritas no bundle estão:

| Superfície | Valor para o usuário | Limite |
| --- | --- | --- |
| `maestro-setup-update` | Orienta setup, update, verificação e recuperação com uma confirmação clara. | Não aceita release sem confiança nem contorna bloqueios. |
| `ingest-content` | Conduz ingestão local com política, rota, proveniência e fidelidade. | Não instala runtime ad hoc nem envia dados remotamente. |
| `workspace-agent-setup` | Cria ou atualiza um agente de workspace por entrevista, pesquisa pública autorizada e evidência com fonte. | Não concede acesso amplo nem pesquisa fora do escopo. |
| `agent-identity-setup` | Define identidade, ownership e personalização de agente de forma revisável. | Não transforma persona em autoridade. |
| `deck-storyline` | Organiza uma narrativa profissional orientada à decisão a partir de evidência aprovada. | Não autoriza ler, editar ou apresentar material sem escopo. |
| `qualitative-analysis` / `quantitative-analysis` | Estrutura análise com fontes, premissas, incerteza e contraevidência. | Não concede acesso a dados nem autorização para publicar. |
| `pr-review` / `pr-quality-loop` | Reduz risco de mudança por gates, evidência e revisão. | Não substitui a decisão humana de merge. |
| `dream-memory` | Inspeciona e consolida memória profissional em camadas governadas. | Permanece indisponível até adapters, retenção e direitos de correção/remoção estarem prontos. |

A progressão `standard → advanced → power` determina como a capacidade é
explicada, nunca quais dados ou ações ficam autorizados.

## 9. Privacidade, segurança e limites de autoridade

O Maestro foi desenhado para ser útil justamente onde autonomia sem controle
seria perigosa:

- **local-first:** a rota padrão não envia material para um provedor remoto;
- **fail-closed:** capacidade crítica não degrada em silêncio;
- **separação de raízes:** core gerenciado, owner, workspace e memória têm
  limites próprios;
- **receipts metadata-only:** observabilidade mede eventos tipados, resultados e
  intervenções, não conteúdo profissional;
- **humano no circuito:** revisão, aprovação e publicação continuam tendo
  responsáveis identificáveis;
- **sem autoridade implícita:** configuração ou prompt não concede acesso a
  uma nova ferramenta, raiz ou agente.

Antes de qualquer uso, pergunte: “a fonte está autorizada, o destino está
correto e a ação é reversível?”. Se uma resposta for não, pause e use
`bcgos doctor` ou o responsável pelo ambiente.

## 10. Rotina recomendada de adoção

### Primeiro dia — provar o caminho

1. Configurar um workspace vazio.
2. Rodar `status` e `doctor`.
3. Escolher o perfil de interação.
4. Completar uma tarefa pequena com critério de sucesso.
5. Fazer pause/resume e confirmar que o próximo passo ficou legível.

### Primeira semana — criar hábito

1. Use um único caso de uso de alto valor.
2. Faça checkpoints em decisões e handoffs.
3. Registre somente fricções reproduzíveis, sem conteúdo de cliente.
4. Teste ingestão com fixtures não sensíveis antes de fontes reais.
5. Compare tempo até o primeiro resultado, retrabalho e intervenções manuais.

### Após a primeira semana — decidir expansão

Promova o uso apenas se houver evidência de valor e segurança: sucesso de
retomada, qualidade aceitável, instalação estável, suporte compreensível e
ausência de vazamento de conteúdo. “Usamos bastante” é um sinal; não é, sozinho,
critério de produção.

## 11. Diagnóstico e recuperação

| Situação | Interpretação | Próxima ação |
| --- | --- | --- |
| `bcgos` não encontrado | O release não está instalado ou não está autorizado. | Pare e solicite o canal privado correto. |
| `unavailable` | A capacidade existe no contrato, mas falta runtime/evidência. | Não emule; registre a limitação e aguarde a instalação aprovada. |
| `blocked` | Política, escopo, assinatura ou pré-condição impediu a ação. | Leia o motivo, corrija a condição autorizada e tente novamente. |
| `degraded` | A ação é possível com uma lacuna explícita. | Revise a fidelidade e confirme antes de usar o resultado. |
| workspace fora da raiz permitida | O limite de segurança foi acionado. | Escolha um workspace dentro do escopo; não force path ou symlink. |
| update falhou | A ativação não passou pela verificação ou compatibilidade. | Preserve a versão ativa e acione rollback/support; não substitua binários manualmente. |
| resultado de ingestão estranho | Conversão não garante fidelidade sem revisão. | Inspecione estrutura, tabelas e proveniência; reclassifique ou pare. |

Nunca apague o workspace para “destravar”. Nunca rode comandos destrutivos de
Git como recuperação de uma instalação. Preserve o estado e encaminhe a
mensagem de erro, versão, sistema operacional e etapa — sem credenciais,
documentos, nomes de cliente ou conteúdo de prompt.

## 12. Checklist de conclusão

- [ ] Release privado e autorizado confirmado.
- [ ] `bcgos version` executado.
- [ ] Workspace local escolhido e confirmado.
- [ ] `bcgos init`, `status` e `doctor` concluídos.
- [ ] Perfil de interação entendido.
- [ ] Primeira tarefa pequena concluída com critério.
- [ ] Checkpoint, pause e resume testados.
- [ ] Ingestão testada apenas quando o runtime pack estiver qualificado.
- [ ] Limitações e próximos passos registrados sem conteúdo sensível.
- [ ] Responsável por suporte e incidente conhecido.

## 13. Documentos relacionados

- [Onboarding completo de recuperação de trabalhos anteriores no SharePoint](sharepoint-prior-work-onboarding.md)

- [Development harness para contribuidores](../development-harness.md)

- [README do Maestro](../../README.md)
- [Roadmap técnico](../../ROADMAP.md)
- [Roadmap de evolução do produto](../roadmap/maestro-evolution-roadmap.md)
- [Trial de instalação](local-installation-trial.md)
- [Prompt de onboarding de piloto](pilot-user-prompt.md)
- [Conformidade de hooks](pilot-hook-conformance.md)
- [Spec de ingestão local](../../specs/010-local-ingestion-runtime.md)
- [Perguntas em aberto](../OPEN-QUESTIONS.md)

## Assinatura do projeto

Maestro — BCG Brasil Agentic OS

Desenvolvido por:

- Daniel Scardini
- Julia Ribeiro
- Marcelho Sanches
