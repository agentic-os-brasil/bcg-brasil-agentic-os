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

> **Estado atual:** o Maestro é distribuído como um ZIP privado e operado
> inteiramente pelo Claude Code — não há CLI externo nem binário para instalar.
> A distribuição de piloto, a assinatura dos artefatos e o runtime local de
> ingestão ainda precisam de evidência própria. Se `/maestro-doctor` reportar
> `unavailable`, isso é um estado seguro e honesto — não uma falha a ser
> contornada instalando Python, `pip`, chaves ou scripts externos.
>
> Para recuperar trabalhos anteriores em pastas SharePoint autorizadas, use o
> [onboarding específico de prior-work](sharepoint-prior-work-onboarding.md).
> A regra corporativa é explícita: Claude pode ser qualificado para coletar;
> Codex não se conecta ao SharePoint e usa somente o índice local verificado.

## 0. Escolha a jornada correta

| Você é | Use | Comandos principais | O que não fazer |
| --- | --- | --- | --- |
| **Participante de piloto ou usuário autorizado** | Este guia e um release privado verificado (ZIP). | `/maestro-onboarding`, `/maestro-doctor` e as skills liberadas pelo release. | Não clonar o repositório nem usar o harness de desenvolvimento para instalar o produto. |
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
| Slash-commands | Skills nativas invocadas pelo Claude Code: `/maestro-onboarding`, `/maestro-doctor`, `/execution-continuity`, `/ingest-content` etc. | Use apenas skills do bundle gerenciado; não improvise comandos externos. |
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

As duas jornadas começam por identidade básica: como o owner prefere ser
chamado e qual contexto pessoal, se houver, ele autoriza o Maestro a respeitar
no trabalho. “Nenhum por enquanto” é uma resposta válida; não se pede história
pessoal, saúde, fé ou outros identificadores por padrão.

O percurso completo cobre depois oito facetas: papel profissional, estilo de
comunicação, preferências de trabalho, voz externa, motivações profissionais,
barra de qualidade/QA, regras de decisão e limites de trabalho. A jornada curta
cobre identidade, contexto pessoal autorizado, papel, comunicação, preferências
e barra de qualidade; começa mais rápido, mas demora mais para absorver voz,
motivações, regras de decisão e limites de trabalho.

Personality/psychological profile, história pessoal, fé, avaliações e identidade
visual ficam fora da entrevista padrão. Só entram por uma etapa local,
explicitamente autorizada, com finalidade e revisão próprias.

Isso não significa que o Maestro conheça apenas o cargo. Depois da revisão do
percurso escolhido, ele pode oferecer camadas opcionais — propósito e não
negociáveis, contexto pessoal ampliado, personalidade/avaliações ou identidade
visual — sempre com finalidade, fonte, leitores e retenção explícitos. Essas
camadas não são necessárias para começar um trabalho profissional, não entram em
contexto de cliente por padrão e permanecem `unavailable` quando não houver um
adaptador local qualificado.

Escolha entre:

- **Curta — cerca de 10 minutos:** baseline operacional com identidade e limites
  pessoais mínimos para começar com segurança.
- **Completa — cerca de 30 minutos:** perfil profissional mais fiel para
  recomendações, comunicação e revisão de qualidade desde o início.

Em ambos os casos, o Maestro resume o que entendeu, espera sua correção e só
depois apresenta o digest para confirmação final.

## 5. Primeira configuração: percurso recomendado

Faça a primeira configuração em um workspace de teste. O roteiro abaixo mostra
o destino da experiência; um comando que ainda não estiver liberado deve ser
reportado como `unavailable`, nunca emulado manualmente.

### Passo 1 — Instale e verifique

Extraia o ZIP privado verificado para uma pasta local (ex: `~/maestro-workspace`)
e abra essa pasta no Claude Code. O scaffold de primeira sessão cria
automaticamente os arquivos de configuração necessários. Em seguida, invoque:

```text
/maestro-doctor
```

O diagnóstico deve explicar, em linguagem simples:

- versão do bundle instalado e compatibilidade;
- sistema operacional e runtime detectado;
- capacidades `supported`, `degraded`, `blocked` ou `unavailable`;
- se o workspace está dentro dos limites esperados;
- qual é a próxima ação segura.

Se o Claude Code não reconhecer `/maestro-doctor`, pare. Não instale nada
encontrado na internet, não rode `pip install`, não aceite scripts sem
assinatura e não forneça credenciais no chat. O próximo passo é pedir o
release privado autorizado ao responsável pela distribuição.

### Passo 2 — Inicialize o workspace

Depois de confirmar o caminho da pasta, o scaffold automático do ZIP já terá
criado a estrutura inicial. Para confirmar o estado e calibrar o runtime:

```text
/maestro-doctor
/maestro-onboarding
```

O scaffold é idempotente: reabrir a pasta não apaga configuração e dados
existentes. `/maestro-doctor` explica diagnósticos e capacidades disponíveis;
`/maestro-onboarding` reúne calibração, tarefas abertas, checkpoint, memória,
manutenção e evidência de runtime já existentes, propondo a próxima ação segura.
Nenhum desses comandos cria clientes, projetos ou conteúdo de trabalho sem
pedido explícito.

### Passo 2.1 — Verifique a configuração do runtime

O ZIP entrega os hooks locais e deixa no workspace uma orientação completa e
legível (`CLAUDE.md` e `AGENTS.md`) com os blocos do OS, além das skills reais
do bundle em `.claude/skills/`. É possível abrir esses arquivos diretamente no
editor. A instalação preserva texto que já exista; se uma skill gerenciada
tiver sido alterada manualmente, ela para com `conflict` em vez de
sobrescrevê-la.

Execute `/maestro-doctor` para confirmar que o adapter está configurado
corretamente. A configuração local prepara o runtime, mas não prova que uma
capability nativa já foi qualificada. Para isso, é necessária uma observação
em uma sessão nativa nova; consulte a
[matriz de evidências do lifecycle](../../specs/035-lifecycle-evidence-matrix.md).

### Passo 3 — Escolha a profundidade de interação

Durante o `/maestro-onboarding`, o Maestro pergunta qual nível de interação
é preferido. O perfil muda a quantidade de explicação e sugestões, não muda permissões:

- **standard:** uma recomendação segura, linguagem simples e o próximo passo;
- **advanced:** a mesma rota, mais justificativa, diagnósticos e opções úteis;
- **power:** detalhes técnicos, premissas e trade-offs explícitos.

Nenhum perfil autoriza upload remoto, ignora verificação, pede uma credencial
em chat ou amplia escopo de acesso.

### Fontes para acelerar o onboarding

Antes da primeira pergunta de identidade, o Maestro oferece a mesma escolha em
qualquer trilha:

- enviar arquivos locais, como CV do BCG, exportação do LinkedIn, bio,
  portfólio, job description, publicações, avaliação de desempenho, MBTI/Big
  Five ou leadership profile;
- indicar fontes públicas, como LinkedIn público, site pessoal, artigos,
  entrevistas, palestras ou repositórios;
- começar sem fontes e anexar material depois.

Essa escolha não transforma uma trilha curta em completa. A curta usa o
material autorizado para criar um baseline e deixa algumas facetas para depois;
a completa usa a mesma evidência auxiliar nas oito facetas profissionais.
Sempre há uma síntese revisável antes de qualquer persistência.

Arquivos permanecem na origem escolhida e só são lidos por um adapter local
qualificado após autorização explícita. Links públicos exigem um plano de
pesquisa com temas minimizados, domínios permitidos e aprovação antes da busca.
Se a capacidade estiver `unavailable`, o onboarding continua sem simular
extração ou pesquisa. Avaliações de personalidade e liderança são fontes de
autodescrição, não diagnóstico nem regra determinística de agente.

### Passo 4 — Inicialize o contexto profissional, se autorizado

O `/maestro-onboarding` conduz a inicialização do contexto do owner e do atlas
em uma entrevista guiada. O contexto do owner é local, inspecionável e limitado
às facetas autorizadas. O atlas navega fontes derivadas e governadas; ele não
deve ser preenchido com segredos, dumps de conversa ou conteúdo de cliente sem
política e consentimento.

Depois da confirmação inicial, o Maestro pode oferecer, dentro da mesma
conversa, uma única pergunta para aprofundar ou atualizar uma faceta profissional
desconhecida ou antiga. A resposta vira um rascunho local somente com seu
consentimento e sua declaração de que não contém dados de cliente. O Maestro
mostra o rascunho inteiro e o digest; só a confirmação exata atualiza a faceta.
Ele nunca deduz uma resposta, preenche lacunas sozinho ou altera o perfil
psicológico.

Se uma sessão for interrompida antes de confirmar um rascunho, ao retomar o
Maestro informará o rascunho pendente e oferecerá revisá-lo antes de qualquer
nova pergunta. O digest de expansão não deve ser reutilizado fora do fluxo de
onboarding.

### Passo 4.1 — Escolha as fontes SharePoint do projeto

Depois que a entrevista do owner estiver revisada e confirmada **dentro do
workspace recém-criado**, o Maestro pergunta uma vez se você quer indicar as
pastas autorizadas do SharePoint deste projeto ou começar sem essa fonte. O
instalador não pede essa fonte antes de criar o workspace: o conteúdo derivado
será lido e organizado dentro dele. Você pode adiar sem perder funcionalidade;
essa escolha fica registrada e o Maestro não repete a pergunta em toda sessão.

Para inspecionar o estado e selecionar fontes, diga ao Maestro que deseja
conectar pastas SharePoint — ele guiará o processo de seleção e registro dentro
da conversa. Se preferir começar sem a fonte, diga "quero adiar a conexão
SharePoint" e a escolha ficará registrada.

Essa primeira etapa registra somente ponteiros exatos em armazenamento privado
local. Com um passe one-and-done ativo, a seleção vincula o fingerprint exato a
esse passe e o Maestro não pergunta novamente na mesma sessão ou em leituras
do mesmo escopo. Um coletor Claude qualificado lê apenas o escopo aprovado e
envia o lote assinado para ingestão.

O Maestro grava apenas racionais derivados em
`brain/knowledge/sharepoint-rationales/`, ordenados pelos materiais mais
recentes, com o link SharePoint, item, digest e data de modificação em cada
registro. O corpo bruto do documento não é copiado e o SharePoint continua
sendo a fonte de verdade. Sem enrollment, qualificação nativa ou runtime local
disponível, a ingestão falha fechada e nada é criado. Codex não coleta
SharePoint; ele pode consultar apenas um índice local já verificado.

### Roadmap de ativação: transforme o workspace em um sistema de trabalho

O `init` prepara o scaffold técnico, mas o primeiro valor profissional exige
agentes de negócio com escopo real. Para trabalho de cliente, siga esta ordem:

```text
Client Account Agent → Case Agent vinculado → primeira entrega → checkpoint
```

Comece dizendo ao Maestro algo como:

> “Quero criar o Client Account Agent da Aurora Mobility para organizar o
> relacionamento e o contexto autorizado da conta.”

Depois, crie o caso concreto:

> “Crie o Case Agent do projeto de pricing 2026 da Aurora Mobility, vinculado à
> conta, com objetivo de preparar uma recomendação para o steering de setembro.”

O Client Account Agent cuida do contexto e da validação da conta; o Case Agent
cuida da execução e da entrega do projeto. Em seguida, peça uma primeira
entrega pequena e revisável, por exemplo:

> “Transforme as três fontes aprovadas em um briefing de uma página com fatos,
> hipóteses, lacunas e próxima decisão. Mostre o plano antes de produzir.”

Finalize pedindo:

> “Registre o checkpoint, indique a próxima ação e me mostre como retomar este
> caso na próxima sessão.”

Use o [roadmap detalhado de ativação dos agentes](agent-activation-roadmap.md)
como checklist. O scaffold automático não é um substituto para um Client
Account Agent e um Case Agent nomeados, vinculados e usados em uma primeira
entrega. Para um trabalho interno sem conta de cliente, o caminho direto para
Case Agent continua permitido.

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

Para trabalhos que atravessam sessões, use `/execution-continuity` como uma
receita conversacional. O runtime registra a tarefa diretamente em
`brain/tasks/`, liga-a ao projeto ou entrega correspondente e acrescenta um
checkpoint bounded quando o owner pausa ou muda a próxima ação. Não exponha
run IDs, revisões, JSON ou comandos do ledger ao owner durante o fluxo normal.

Ao abrir uma nova sessão, leia os artefatos de tarefa e checkpoint do workspace,
confirme o escopo e continue pelo último próximo passo revisado. Se não houver
item ativo, diga isso e ofereça criar um; se houver mais de um, pergunte qual
deve continuar. O `/execution-continuity` é a entrada canônica para registrar e retomar trabalho
novo; use-o como primeira opção.

Um runtime configurado ou um receipt local pode aparecer como `configured` ou
`adapter_observed`; isso não significa `native_qualified`. `unavailable`
permanece explícito até existir evidência nativa atendida para a versão exata.
O contador `attested_capture_files` mede apenas capturas de memória que foram
explicitamente atestadas e persistidas pelo produtor autorizado. Contexto base
injetado por SessionStart, `CLAUDE.md` ou skills não cria captura atestada e,
portanto, não deve elevar esse contador.
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

O guard de remoção permanece deliberadamente conservador: um `rm` isolado e
bounded pode ser avaliado, mas remoções encadeadas ou shell complexo podem ser
negadas quando a análise não consegue provar o alvo com segurança. Reexecute a
operação em uma chamada simples e confirme o caminho; isso é uma limitação de
análise do guard, não uma falha de instalação.

## 7. Ingestão de documentos e fontes

O objetivo da ingestão é transformar uma fonte autorizada em conhecimento local
estruturado, preservando proveniência e classificação de fidelidade.

```text
fonte local → checagem de política/tamanho → Docling local
→ resultado estruturado + Markdown → validação → downstream autorizado
```

Um adapter de fallback opensource, ZIP-embeddable, será selecionado
posteriormente para formatos simples cobertos por conversão determinística.
No momento nenhum fallback é distribuído com o base bundle.

Exemplo de invocação:

```text
/ingest-content
```

O Maestro pergunta a fonte, o workspace autorizado e o adapter disponível.

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
| `case-agent-setup` | Cria ou atualiza o Case Agent de um projeto por entrevista, pesquisa pública autorizada e evidência com fonte. | Não concede acesso amplo, não representa o Client Account Agent e não pesquisa fora do escopo. |
| `workspace-agent-setup` (alias de migração) | Redireciona instalações/projeções antigas para `$case-agent-setup`. | Não é um tipo de agente atual e não cria um workspace agent ou practice agent. |
| `agent-identity-setup` | Define identidade, ownership e personalização de agente de forma revisável. | Não transforma persona em autoridade. |
| `bcg-deck` | Organiza uma narrativa profissional orientada à decisão a partir de evidência aprovada. | Não autoriza ler, editar ou apresentar material sem escopo. |
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
correto e a ação é reversível?”. Se uma resposta for não, pause e execute
`/maestro-doctor` ou consulte o responsável pelo ambiente.

## 10. Rotina recomendada de adoção

### Primeiro dia — provar o caminho

1. Extrair o ZIP e abrir a pasta no Claude Code.
2. Executar `/maestro-doctor` e confirmar capacidades.
3. Escolher o perfil de interação via `/maestro-onboarding`.
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
| Slash-commands não reconhecidos pelo Claude Code | O ZIP não foi extraído corretamente ou a pasta não foi aberta no Claude Code. | Feche e reabra a pasta; se persistir, solicite o release privado correto. |
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

- [ ] Release privado (ZIP) e autorizado confirmado.
- [ ] ZIP extraído e pasta aberta no Claude Code.
- [ ] Workspace local escolhido e confirmado.
- [ ] `/maestro-doctor` executado e capacidades revisadas.
- [ ] `/maestro-onboarding` concluído (calibração e perfil de interação).
- [ ] Primeira tarefa pequena concluída com critério.
- [ ] Checkpoint, pause e resume testados via `/execution-continuity`.
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
