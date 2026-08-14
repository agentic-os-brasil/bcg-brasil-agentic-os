---
name: fodais-performance-review
description: Conduz entrevista estruturada e gera o texto de avaliação de performance (CDC) de membro do time BCG X — Forward Deployed AI Scientist (FoDAIS) ou Sr. Forward Deployed AI Scientist (Sr. FoDAIS) — aplicando a rubrica de Associates/Sr. Associates (FoDAIS) ou Consultants (Sr. FoDAIS). Invocar com "escreve a avaliação do [nome], ele(a) é FoDAIS", "CDC do [nome], BCG X" ou "avaliação do FoDAIS/Sr. FoDAIS [nome]".
---

# Skill: fodais-performance-review

## Tipo
Atômica

## Quando usar
Ao escrever avaliação de performance (CDC) de membro do time BCG X — Forward Deployed AI Scientist (FoDAIS) ou Sr. Forward Deployed AI Scientist (Sr. FoDAIS). Conduz uma entrevista estruturada com Julia e gera o texto final formatado.
Rubrica de referência: `Associates & Consultants - Grading Rubrics 2023.pdf` (Brazil Career Development, Out/2023) — FoDAIS usa a rubrica de Associates/Sr. Associates; Sr. FoDAIS usa a rubrica de Consultants. Dimensão **Expertise** foi adicionada por Julia em 2026-08-06 — não existe no PDF original; passou a usar uma lista de comportamentos de referência fornecida por Julia em 2026-08-13 (substitui o rascunho grade-a-grade inicial do Tamagoshi).

## Como invocar
> "escreve a avaliação do [nome], ele(a) é FoDAIS"
> "fodais-performance-review: [nome]"
> "preciso fazer o CDC do [nome], BCG X"

---

## Processo — entrevista em etapas

Conduzir a entrevista **uma etapa por vez**. Não avançar para a próxima antes de receber resposta. Ao final de todas as etapas, gerar o output completo.

### Etapa 0 — Identificação
Perguntar:
> "Qual o nome completo da pessoa e qual é o cargo dela? (FoDAIS ou Sr. FoDAIS)"

Registrar internamente: `{nome}`, `{cargo}` → determina qual rubrica aplicar (FoDAIS → rubrica de Associates/Sr. Associates; Sr. FoDAIS → rubrica de Consultants).

---

### Etapa 0.5 — Avaliação anterior
Perguntar:
> "Você tem uma avaliação anterior de {nome} para usar como base? Se tiver, pode subir agora — assim a nova vai ser uma evolução daquela."

Se Julia subir um arquivo:
- Ler o conteúdo
- Salvar em `professional/affiliation/cdcs/{nome-em-kebab-case}/` com nome no formato `YYYY-MM-[projeto-ou-ciclo].md`
- Confirmar: "Salvo. Vou usar essa avaliação como referência durante a entrevista."
- Carregar como contexto de fundo para todas as etapas seguintes — especialmente Strengths, Areas of Development e Overall Summary, que devem refletir progressão em relação à avaliação anterior

Se Julia não tiver ou quiser pular:
- Prosseguir normalmente sem referência anterior

---

### Etapa 0.6 — Individuals who provided input
*(adicionada por Julia em 2026-08-13)*

Perguntar:
> "Além de você, alguém mais deu input para essa avaliação? (outros PLs, gestores ou colegas consultados — pode dizer que não houve)"

Gerar: lista de nomes, ou "N/A" se não houver.

---

### Etapa 1 — Project Description
Perguntar:
> "Me dá uma descrição curta do objetivo do projeto."

Gerar: 2–3 frases descrevendo o objetivo central do engajamento.

---

### Etapa 2 — Project Structure and Team Members
Perguntar:
> "Quais eram os membros do time e os cargos de cada um? (pode ser lista simples)"

Gerar: lista formatada com nome e cargo de cada membro.

---

### Etapa 3 — Individual Role and Experience Gained
Perguntar:
> "Descreve as responsabilidades de {nome} no projeto — o que ela/ele fez, quais módulos/builds técnicos tocou, com quem interagiu."

Depois perguntar:
> "Agora, quais habilidades ou aprendizados você acha que ela/ele desenvolveu nesse projeto?"

Gerar: **dois parágrafos**.
- §1: responsabilidades e atuação no projeto
- §2: habilidades e experiências desenvolvidas

---

### Etapa 3.5 — Skill demands on this project
*(adicionada por Julia em 2026-08-13)*

Perguntar:
> "Qual foi o nível de demanda desse projeto em cada uma dessas frentes? (heavy/medium/light)
> - Conceptual framing
> - Quantitative analysis
> - Qualitative analysis
> - Written output
> - Client interaction and management
> - Formal presentations and/or meeting facilitation"

Gerar: tabela com as 6 frentes e o nível (heavy/medium/light) de cada uma. Serve para dar ao leitor da avaliação uma noção do contexto de demanda do projeto antes do grading, não é avaliada nota a nota.

---

### Etapa 4 — Grading dimensão por dimensão

Conduzir **uma dimensão por vez** — nunca em lote, nunca agrupando dimensões correlatas. Julia rejeitou explicitamente a abordagem agrupada em avaliações formais: quer garantir que cada nota tenha racional isolado, sem uma dimensão mais forte "carregar" o julgamento de uma mais fraca no mesmo bloco.

**Escala de notas (ambos os cargos):** 1 a 4, onde **1 é a nota mais forte** ("top choice to staff") e **4 é a mais fraca** (ainda não é nota negativa — "important AfDs, but does not mean negative leverage"). A escala é invertida em relação à leitura intuitiva — atenção ao comunicar isso a Julia se houver ambiguidade.

Dimensões, nesta ordem:
1. Expertise (conhecimento técnico) — **dimensão nova, adicionada por Julia**; sem subnotas; avaliada a partir de uma lista de comportamentos de referência gerais, não grade-a-grade (ver Rubrica de referência)
2. Problem solving & Insight — **com subnotas** (Frames and structures issues; Leverages BCG knowledge; Ensures high quality analyses; Pushes for insight)
3. Communication and Presence — **com subnotas** (Verbal communication; Presence; Written communication)
4. Practicality & effectiveness — **com subnotas** (Prioritizes work effectively, demonstrates business sense and adaptability; Is reliable and timely, manages team expectations; Identifies next steps proactively, owns module)
5. Client Interaction — sem subnotas
6. Team contribution — sem subnotas
7. Role model — sem subnotas (adicionada por Julia em 2026-08-13, ver fonte abaixo)

Ordem alterada por Julia em 2026-08-07: Expertise vem primeiro, mesmo que Problem Solving continue sendo a dimensão que define o piso de qualidade da nota Overall (regra de consistência corrigida por Julia em 2026-08-13, ver abaixo).

Estrutura de subnotas definida por Julia em 2026-08-11: Communication & Presence — Verbal e Communication & Presence — Written deixam de ser duas dimensões separadas e passam a ser 2 das 3 subnotas de uma única dimensão "Communication and Presence" (a terceira subnota, Presence, é nova). Practicality & effectiveness e Problem solving & Insight também passam a ter subnotas próprias, listadas na seção "Rubrica de referência" abaixo, em "Subnotas por dimensão".

Para cada dimensão, repetir:
a. Buscar na seção "Rubrica de referência" abaixo os comportamentos dessa dimensão, no cargo de {cargo} (FoDAIS → tabela Associates/Sr. Associates; Sr. FoDAIS → tabela Consultants). Expertise e Role model não têm comportamentos por cargo — usar a lista de comportamentos de referência geral (ambos os cargos) da seção correspondente.
b. Se a dimensão tiver subnotas definidas (ver "Subnotas por dimensão" na Rubrica de referência — Problem solving & Insight, Communication and Presence, e Practicality & effectiveness têm subnotas; Expertise, Client Interaction, Team contribution e Role model não têm), perguntar e registrar a nota de cada subnota (1-4) separadamente antes de sintetizar a nota geral da dimensão. Para as demais dimensões, seguir direto para a nota geral.
c. Para dimensões com descritores por grade (todas exceto Expertise e Role model): perguntar, citando a rúbrica **na íntegra e sem alterar nenhuma palavra**, todas as 4 notas (não resumir, não citar apenas grade 2 e 3):
   > "**[Dimensão]**. Pra {cargo}:
   > 4 = [texto literal da rúbrica para grade 4]
   > 3 = [texto literal da rúbrica para grade 3]
   > 2 = [texto literal da rúbrica para grade 2]
   > 1 = [texto literal da rúbrica para grade 1]
   > Como foi {nome} aqui? Nota (1/2/3/4)?"
   Para Expertise e Role model (sem descritores por grade): apresentar a lista de comportamentos de referência da dimensão, sem quebra por nota, e perguntar a nota direto:
   > "**[Dimensão]**. Sinais de referência pra essa dimensão: [lista de comportamentos]. Como foi {nome} aqui? Nota (1/2/3/4)?"
d. Aguardar resposta. Para dimensões com descritores por grade, se a nota não bater claramente com os comportamentos descritos na rúbrica, sinalizar o desvio antes de seguir (ex.: "isso soa mais grade 3 do que grade 2 pela rúbrica — confirma ou me dá mais contexto?") — não aceitar a nota sem checagem quando houver descolamento aparente. Para Expertise e Role model (sem descritores por grade), aceitar a nota de Julia sem essa checagem.
e. Depois de fechar a(s) nota(s), escrever um resumo em inglês da dimensão para {nome}, sintetizando o racional por trás da nota. Ordem fixa dentro da dimensão: nota por subdimensão (quando aplicável) → nota geral da dimensão → resumo em inglês. Esses resumos alimentam a síntese de Strengths, Areas of Development e Overall Summary nas etapas seguintes, **e são sempre incluídos no output final**, na seção "DIMENSION SUMMARIES" (regra de Julia em 2026-08-13 — nunca omitir essa seção).
   O resumo deve ter **dois parágrafos**: o primeiro focado nos pontos fortes da dimensão, o segundo nos pontos de desenvolvimento. Se não houver ponto de desenvolvimento claro (nota 1 sem ressalvas), o segundo parágrafo dá um "spin" construtivo, algo para {nome} pensar como próximo passo de carreira em vez de uma lacuna de performance — mas só se esse spin for genuíno. Não forçar um segundo parágrafo: se a pessoa já demonstrou o comportamento que seria sugerido como próximo passo (Julia vai sinalizar isso), remover o parágrafo e deixar o resumo com um único parágrafo.
   Quando houver ponto de desenvolvimento real (não spin), o segundo parágrafo segue esta estrutura fixa: (i) o que precisa melhorar; (ii) exemplo(s) concreto(s) de situação em que isso aconteceu, se houver (não inventar se Julia não der um); (iii) como fazer para melhorar esse ponto, de forma acionável.
   Em **cada frase** do resumo (não só em Strengths/AfD): generalizar o ponto primeiro, e só então trazer o exemplo concreto entre parênteses no formato `(e.g., ...)`. Nunca deixar um exemplo específico solto na frase sem antes generalizar o comportamento que ele ilustra.
   Dimensões com subnotas (Problem solving & Insight, Communication and Presence, Practicality & effectiveness) têm **um só resumo consolidado** cobrindo todas as subnotas daquela dimensão — perguntar e registrar a nota de cada subnota separadamente (passos b-d, uma por vez), mas só escrever o resumo em inglês depois de fechar todas as subnotas da dimensão.
f. Só então avançar para a próxima dimensão.

Ao final das 7 dimensões, perguntar a **Overall grade**:
> "Considerando o conjunto, qual seria a nota Overall (1 a 4) de {nome} nesse projeto?"

**Validar consistência** (ler em termos de qualidade, não do valor numérico bruto, dada a escala invertida; aplicada às 6 notas gerais de dimensão, não às subnotas individuais):
- **Overall** não pode ser numericamente maior (mais fraca) que a nota de Problem Solving & Insight — ou seja, Overall ≤ Problem Solving. Problem Solving define o piso de qualidade da Overall (a Overall nunca pode ser pior que Problem Solving, mas pode ser igual ou melhor). *(Correção de Julia em 2026-08-13 — a regra anterior estava na direção errada.)*
- **Todas as demais dimensões (incluindo Expertise)** podem ser no máximo 1 ponto numericamente maior (ou seja, uma nota mais fraca) que a Overall — nunca duas ou mais.

Se alguma nota violar a regra, sinalizar a Julia antes de fechar a etapa (ex.: "Communication and Presence ficou 2 notas mais fraca que a Overall — pela regra, o máximo é 1. Confirma os números ou ajustamos?").

Gerar ao final: tabela com dimensão → nota, mais a Overall grade.

---

### Etapa 5 — Strengths
Perguntar:
> "Quais foram as 3 principais fortalezas de {nome} nesse projeto? Me dá exemplos concretos se tiver — depois eu estruturo."

Gerar: 3 blocos, cada um com:
- **Dimensão:** [nome da dimensão]
- **Descrição:** parágrafo descrevendo a fortaleza com evidências específicas do projeto

---

### Etapa 6 — Areas of Development
Perguntar:
> "E as 3 principais áreas de desenvolvimento? O que ela/ele precisa melhorar ou aprofundar?"

Gerar: 3 blocos, cada um com:
- **Dimensão:** [nome da dimensão]
- **Descrição:** parágrafo seguindo a estrutura fixa: (i) contexto positivo/pontos fortes da pessoa nessa dimensão antes de entrar no ponto de melhoria; (ii) o ponto de desenvolvimento em si, com exemplo concreto se houver; (iii) como melhorar, de forma acionável. Quando o ponto for mais um "spin" de carreira do que uma lacuna real (ver regra em Etapa 4), pode flexibilizar essa estrutura, mas mantendo o tom construtivo.

---

### Etapa 7 — Overall Summary
Por padrão, sintetizar a partir do que já foi coletado nas etapas anteriores (Individual Role, Strengths, Areas of Development, notas por dimensão, Overall grade) — não abrir com pergunta em branco. Só perguntar diretamente a Julia se: (a) faltar contexto suficiente para sintetizar, ou (b) ela pedir para adicionar algo que ainda não apareceu na entrevista.

Se houver uma avaliação anterior (do mesmo projeto/pessoa ou de referência indicada por Julia) disponível, usá-la como referência de **formato e tamanho** (número de parágrafos, tom de fechamento) — não de conteúdo.

Gerar: síntese de tom positivo e construtivo, integrando fortalezas + desenvolvimento + trajetória esperada. Nunca repete literalmente o que já foi dito — eleva. Comprimento segue a referência de formato quando houver uma; na ausência dela, 1 parágrafo é o padrão.

---

### Etapa 8 — Additional Comments (opcional)
Perguntar:
> "Há alguma circunstância especial ou comentário adicional que deva constar na avaliação? (ex: contexto difícil, desafio atípico, licença, etc.) — pode pular se não houver."

Se houver: incluir campo preenchido.
Se não houver: incluir campo com "N/A".

---

## Output final

Gerar o texto completo formatado para cópia direta no sistema de avaliação:

```
INDIVIDUALS WHO PROVIDED INPUT
[lista de nomes ou N/A]

PROJECT DESCRIPTION
[texto gerado]

PROJECT STRUCTURE AND TEAM MEMBERS
[lista de membros]

INDIVIDUAL ROLE AND EXPERIENCE GAINED
[§1 — responsabilidades]

[§2 — habilidades desenvolvidas]

SKILL DEMANDS ON THIS PROJECT
| Frente                                            | Nível  |
|----------------------------------------------------|--------|
| Conceptual framing                                  | [heavy/medium/light] |
| Quantitative analysis                               | [heavy/medium/light] |
| Qualitative analysis                                | [heavy/medium/light] |
| Written output                                      | [heavy/medium/light] |
| Client interaction and management                   | [heavy/medium/light] |
| Formal presentations and/or meeting facilitation    | [heavy/medium/light] |

GRADING
| Dimension / Subnota                                              | Grade |
|--------------------------------------------------------------------|-------|
| Expertise                                                          | [X]   |
| Problem solving & Insight (overall)                                | [X]   |
| — Frames and structures issues                                     | [X]   |
| — Leverages BCG knowledge                                           | [X]   |
| — Ensures high quality analyses                                    | [X]   |
| — Pushes for insight                                                | [X]   |
| Communication and Presence (overall)                                | [X]   |
| — Verbal communication                                              | [X]   |
| — Presence                                                          | [X]   |
| — Written communication                                             | [X]   |
| Practicality & effectiveness (overall)                              | [X]   |
| — Prioritizes work effectively, demonstrates business sense and adaptability | [X] |
| — Is reliable and timely, manages team expectations                | [X]   |
| — Identifies next steps proactively, owns module                   | [X]   |
| Client Interaction                                                  | [X]   |
| Team contribution                                                   | [X]   |
| Role model                                                          | [X]   |
| **Overall**                                                         | [X]   |

DIMENSION SUMMARIES

[Nome da dimensão 1]
[resumo em inglês gerado na Etapa 4, passo e — 1 ou 2 parágrafos, conforme a regra daquele passo]

[Nome da dimensão 2]
[resumo]

[... uma entrada por dimensão avaliada, na mesma ordem da Etapa 4. Dimensões com resumo consolidado (ex.: Communication and Presence) entram como uma única entrada.]

STRENGTHS DEMONSTRATED ON THIS PROJECT

1. [Dimensão]
[texto]

2. [Dimensão]
[texto]

3. [Dimensão]
[texto]

AREAS OF DEVELOPMENT IDENTIFIED ON THIS PROJECT

1. [Dimensão]
[texto]

2. [Dimensão]
[texto]

3. [Dimensão]
[texto]

OVERALL SUMMARY
[texto]

ADDITIONAL COMMENTS AND SPECIAL CIRCUMSTANCES
[texto ou N/A]
```

---

## Rubrica de referência

Fontes:
- `Associates & Consultants - Grading Rubrics 2023.pdf` (Brazil Career Development, Out/2023), salvo em `C:\Users\Ribeiro Julia\OneDrive - The Boston Consulting Group, Inc\Private\Training\Associates & Consultants - Grading Rubrics 2023.pdf` — rubrica com descritores de grade 1-4 por dimensão (não inclui Role model nem as subnotas de Communication and Presence / Practicality & effectiveness).
- `Competencies in Action vFINAL (2).pdf`, salvo em `C:\Users\Ribeiro Julia\OneDrive - The Boston Consulting Group, Inc\Private\Training\Case Leader Readiness\Competencies in Action vFINAL (2).pdf` — confirma as subnotas oficiais de Problem solving & Insight, Communication and Presence e Practicality & effectiveness, e é a fonte da dimensão **Role model** (subitens abaixo). Competências são idênticas entre Associate e Consultant, exceto onde uma nota de rodapé indicar divergência.

Todo o texto abaixo (exceto a dimensão Expertise, que não existe em nenhum dos dois PDFs) é **citação literal**, sem tradução ou paráfrase — citar sempre assim para Julia, palavra por palavra.

### Escala de notas — Associates / Sr. Associates
| Grade | Descrição |
|------|------------------|
| 4 | Important AfDs, but does not mean negative leverage; interest in staffing upon proper team balancing |
| 3 | Strong contributor; happy to staff in any project (not as pilar) |
| 2 | Very Strong; would be among my top choices; could be staffed as pillar of the any team |
| 1 | Top choice to staff; pillar of the team in any project; expect to perform exceptionally in all situations and dimensions (among the best in the cohort) |

### Escala de notas — Consultants
| Grade | Descrição |
|------|------------------|
| 4 | Important AfDs, but does not (necessarily) mean negative leverage; interest in staffing upon proper team balancing |
| 3 | Strong contributor; happy to staff in any project (not as pilar) |
| 2 | Very Strong; among my top choices; could be pillar of any team and able to contribute significantly beyond own module |
| 1 | Top choice to staff, pillar of the team on any project; carries the case on his/her back; able to clearly substitute the P/PL |

**Regras de consistência (texto literal do PDF, ambos os cargos):**
- Problem Solving grade ≥ Overall grade
- Other dimensions maximum one grade lower than Overall grade
- (Aplicada por Julia também à dimensão Expertise, que não está no PDF original.)

---

### Subnotas por dimensão (quebra oficial, ambos os cargos)
Estrutura confirmada por Julia em 2026-08-07 (Problem solving & Insight) e 2026-08-11 (Communication and Presence; Practicality & effectiveness) — fonte distinta do PDF de grading rubrics (modelo de competências BCG). Cada subnota abaixo recebe sua própria nota (1-4) na entrevista e aparece como linha própria na tabela de grading; a nota geral da dimensão é sintetizada a partir do conjunto de subnotas. Expertise, Client Interaction e Team contribution **não têm subnotas** — só nota geral.

**Problem solving & Insight:**
- **Frames and structures issues**: demonstrates understanding of the client's overall problem and the analytic steps required to solve; identifies key issues and linkages by scoping module and prioritizing analysis through hypothesis-driven approach
- **Leverages BCG knowledge**: leverages and quickly adapts BCG knowledge
- **Ensures high quality analyses**: identifies and collects relevant data efficiently; presents a resourceful and tenacious approach to problem solving; conducts complex quantitative analysis and modeling independently and efficiently; applies reality checks to analysis
- **Pushes for insight**: distills insights from analysis, goes above and beyond the obvious; translates the output of analyses into practical recommendations

**Communication and Presence:**
- **Verbal communication**: qualidade da comunicação falada em si (clareza, estrutura, articulação de ideias)
- **Presence**: postura e confiança no espaço da conversa/reunião (subnota nova, adicionada por Julia em 2026-08-11 — não existe separada no PDF, que trata presence como parte dos comportamentos de "Verbal"; sem rubrica literal própria por grade ainda, calibrar com Julia com o tempo)
- **Written communication**: qualidade do material escrito (slides, e-mails, storylines)

**Practicality & effectiveness:**
- **Prioritizes work effectively, demonstrates business sense and adaptability**
- **Is reliable and timely, manages team expectations**
- **Identifies next steps proactively, owns module**
(Essas 3 subnotas de Practicality & effectiveness também não têm rubrica literal própria por grade no PDF de Grading Rubrics — o PDF só descreve a dimensão como um todo. Usar a rubrica geral da dimensão, abaixo, como referência para as 3, até Julia fornecer descritores específicos por subnota.)

---

### Role model (dimensão sem subnotas, ambos os cargos)
Adicionada por Julia em 2026-08-13, fonte `Competencies in Action vFINAL (2).pdf` — não existe no PDF de Grading Rubrics (que só cobre as outras 6 dimensões). Usa a mesma escala 1-4 e a mesma regra de consistência das demais dimensões (máximo 1 nota mais fraca que a Overall). Sem subnotas — uma única nota geral, mesmo havendo 6 comportamentos-chave listados no PDF de origem (idênticos entre Associate e Consultant form):

- Performs role with highest level of integrity, generating trust and protecting client interests
- Treats all others with respect regardless of background, position or performance
- Displays perseverance and tenacity in the face of obstacles; responds calmly to stressful situations
- Seeks and acts on feedback for self-development
- Contributes to positive working environment
- Handles confidential data responsibly and takes appropriate security precautions

Sem descritores literais por grade (1-4) no PDF de origem — usar os 6 comportamentos acima como referência qualitativa até Julia calibrar descritores específicos por grade.

---

### Expertise (dimensão sem subnotas, ambos os cargos)
Adicionada por Julia em 2026-08-06 especificamente para BCG X — não existe na rubrica oficial de 2023. Lista de comportamentos de referência fornecida por Julia em 2026-08-13, substituindo o rascunho grade-a-grade inicial do Tamagoshi. Igual a Role model: usa a mesma escala 1-4 e a mesma regra de consistência das demais dimensões, mas **sem descritores por grade** — os comportamentos abaixo servem só para dar ao avaliador uma noção geral do que está sendo avaliado, não para quebrar nota a nota.

- Leverages and quickly adapts expertise and BCG knowledge
- Viewed as an expert (in a specific area) by relevant clients
- Shares expertise with the team; generates interest around the topic area
- Applies expertise that contributes to the overall success of the module
- Contributes to BCG's intellectual capital (develops and shares new ideas / analytical / methodologies / approaches)

---

### FoDAIS (rubrica base: Associates / Sr. Associates)

**Problem solving & Insight**
- 4: Struggles to frame the problem; Difficulty understanding or executing tasks without help; Uncomfortable with quantitative modeling; Has hard time with qualitative tasks; Poor business sense; Multiple errors or illogical steps; May get stuck constantly; Inappropriate level of detail; Brings problems, not suggestions; Does not understand or articulate implications of analyses
- 3: Understands overall problem and implications of module; Asks insightful questions; Develops frameworks that communicate options, after few interactions with the PPL; Executes against expected analysis well with limited errors; Good sanity checks and business sense; Models functional and effective but may require refinement; Appropriate level of detail; Knows the important #s; Translates analyses to insights / recommendations
- 2: Develops frameworks that communicate action options; Uses hypothesis to structure analysis; Independently identifies and collects relevant data; Tenacious and creative in proposing options; Sophisticated analysis with clean design; Models are functional and effective; Leverages BCG knowledge; Clean and clear layout/approach; Pushes to 2nd order insights / recommendations; Knows the important #s and can justify (almost) everything
- 1: Develops hypotheses to structure approach independently; Proactively brings ideas, insights, triangulation or analysis; Draws parallels to other relevant work (leverages available information to support powerful conclusions); Creates tools / analyses easily handed off / used by clients; Very sophisticated quantitative skills; Error free on critical/final outcomes; Takes risks (to right extent); Derives key takeaways from qualitative interactions; Constantly "Wow's" OPPL / client; Gets to 2nd/3rd order implications/insights

**Expertise** — ver seção compartilhada "Expertise" acima (sem descritores por cargo).

**Communication and Presence** *(uma dimensão, com subnotas Verbal communication / Presence / Written communication — ver "Subnotas por dimensão" acima)*

*Verbal communication:*
- 4: P/PL not comfortable to send alone to most meetings or to let him/her present in relevant contexts; May make naive or immature remarks; Gets visibly nervous; Presentations not focused on key points; Low or inadequate participation in CTMs; Passive engagement style
- 3: Clear & logical, but usually sticks to slides; Well prepared with point of view; Active listener who absorbs facts well; Reasonably confident communication; May undermine knowledge by being too casual or quiet
- 2: Can go to some clients alone (but not on too controversial topics or clients); Able to explain/present structured ideas; Adequate verbal participation on meetings; Has command of discussion related to module; Can provide comments on modules related to theirs
- 1: Can go to most clients alone (but not on too controversial topics or clients); P/PL is comfortable in letting A conduct meetings alone or present to mid-management clients; Able to explain/present complex ideas; Good conversation share; Verbal contributions beyond module; Able to flex communication to suit audience / situation; Incorporates new facts on the spot into conversation; P/PL is comfortable in letting A conduct meetings alone or present to some senior clients

*Presence:* sem rubrica literal própria (ver nota em "Subnotas por dimensão").

*Written communication:*
- 4: Materials require significant iteration (P/PL must be very directive and frequently decides to do it by him/herself in order to save time); Slides with errors/inconsistency or lacking logic; Poor format or visual, including "disturbing" content; Word choice immature and/or not thoughtful
- 3: Materials requires some iteration (more than P/PL would like) to refine them; Consistent and accurate slides; Format based on templates, but capturing key analysis and messages; Some difficulties with more abstract concepts / storylines; Professional and efficient communication (slides and e-mail wording)
- 2: Client-ready slides that require little iteration (completeness); Frames content around key points; Slides executed beyond templates; Able to develop logical messaging for individual/small set of slides/analysis independently
- 1: Refined choice of words (slides and e-mail wording); Crispy & visually appealing slides designed on first attempt; Able to develop good storylines based on initial discussion with P/PL; Wording tailored & refined to audience (putting him/herself in the client's shoes); In a position where he/she could train others in slide writing

**Practicality & effectiveness**
- 4: Does not provide updates on work/status; Lack of clear "to do list" / priorities; Significantly underestimates tasks duration; Slow to output (P/PL reviews the scope frequently); Misses' deadlines more than once; Doesn't follow on all agreed actions; Waits to receive next steps
- 3: Provides updates on work and status (however, P/PL still feels the need to ensure path is in the right direction); Prioritizes work well; Good speed to output; Communicates timelines, delivers on time; Identifies more immediate next steps
- 2: Drives work forward proactively; Effective 80/20 to prioritize and focus on key issues; Very fast to output; Supports other modules/team members; P/PL is very comfortable A is "doing the right thing" in between interactions
- 1: Creates tools and invests time to help clients implement in the smoothest way possible (enables the client); Develops / acts on work plan up front with minimal input; Incredibly fast to create high quality output; Identifies potential blockages and solutions, having a pragmatic approach; Displays very strong ownership of module including "so what" and next steps

**Client Interaction**
- 4: Purely transactional; May rely on emails for interaction; Shies away from opportunities for client interaction; Does not establish good rapport/respect from client; Not recognized by clients as the "owner" of the module
- 3: May be transactional occasionally; Establishes good rapport/respect with junior clients; Comfortable and productive interacting with more junior clients independently; May shy away in larger settings and with senior executives (internal and/or external)
- 2: Proactively tries to build relationships beyond content with junior clients; Establishes good rapport/respect with middle management; Operates comfortably with middle management clients; Understands client perspectives and considers when interacting
- 1: Builds personal relationships with junior clients; Participates actively in large/senior client settings; Gains respect of senior clients; Clients complain when A rotates; Clients specifically ask /call him/her; Able to draw insight from client dynamics

**Team contribution**
- 4: Does not raise red flags in advance and/or ask for help when needed; Requires constant interaction with P/PL; Goes "Missing in Action"; Misses or is late to meetings; May require other A/Cs to jump in to help; May be defensive about tasks
- 3: Notifies in advance of delays; May require more follow-up interactions and clarifications than P/PL would like; "Can do" attitude; teams with others; Carries own weight, but rarely helps others significantly; Helps beyond module upon request
- 2: Drives works independently; Clearly "on top of things"; Excellent upward communication; Pro-actively offers to help other modules; Teams with others to leverage existing knowledge base
- 1: Contributes significantly to team 'spirit'; Acts very independently early in the case; Constantly goes the "extra mile"; Actively contributes to other modules during CTMs or client discussions; Helps identify issues on other modules; "Right arm" of P/PL on unexpected tasks; Helps reduce stress for team

---

### Sr. FoDAIS (rubrica base: Consultants)

**Problem solving & Insight**
- 4: Understands overall problem and implications of module, but struggles to frame the problem and link module to key issues; Executes aligned tasks, but with errors or realignment need; Uncomfortable with quantitative modeling; need help with the model structuring and operationalization; Models are functional, but may require several refinement and may contain errors on important numbers and analysis; Sometimes (~50%) misses' takeaways from qualitative tasks; Good business sense, but not able to leverage it effectively on analysis and insights; May get stuck or slow-down in case of complexity/subjectivity; Inappropriate balance of detail; loses big picture or is shallow; Brings more problems than suggestions or misses relevant ones; Delivers analysis and data but not insight or "so what"; Translates analyses to insights / recommendations but shallow
- 3: Develop frameworks that clearly communicate action options; Uses hypothesis to structure analyses; Independently identifies and collects relevant data; Leverages BCG knowledge; Models are functional and effective; Executes against analyses well with limited error; Knows the important #s and can justify (almost) everything; Thorough sanity checks, based on strong business sense; Appropriate level of detail; Pushes insights beyond first order of implications
- 2: Develops hypotheses to structure approach very independently; Proactively brings new ideas, insights, triangulations and/or new analysis; Draws parallels to other work (leverage available info to support conclusions); Creates tools / analysis that easily can be handed off / used by clients; Takes risks (to right extent); Sophisticated analysis with clean design; Very sophisticated quantitative skills; Derives key takeaways from qualitative interactions; Error free on critical/final outcomes; Gets to 2nd/3rd order implications/insights
- 1: Demonstrates understanding of the problem and independently finds new methodologies and approach to generate framework for the module; Acts as a thinking partner to OPPL, beyond own module; PPL confident that C is ready to support case & team mgmt; PPL might feel redundant in the problem-solving cycle; Challenges status quo and applies business judgment to client issues; shows adaptability; Tackles very complex issues with little support, with effective upward management; Generates value for client; ensures buy-in & understanding; Identifies and anticipates implementation challenges; translates analyses into implementable solutions; Effectively transfers capabilities to client teams

**Expertise** — ver seção compartilhada "Expertise" acima (sem descritores por cargo).

**Communication and Presence** *(uma dimensão, com subnotas Verbal communication / Presence / Written communication — ver "Subnotas por dimensão" acima)*

*Verbal communication:*
- 4: Clear & logical, but usually sticks to slides; Sometimes has a point of view to share; Might get visibly nervous; May undermine knowledge by being too casual or quiet; Low or inadequate participation in CTMs and client meetings
- 3: Able to explain/present structured ideas; Well prepared with point of view; Adequate verbal participation on meetings; Has command of discussion related to module; Active listener who absorbs facts well; Verbal contributions beyond module; Can go to most clients alone (but not on too controversial topics or clients)
- 2: P/PL is comfortable in letting C conduct meetings alone or present to some senior clients; Confident presence; Able to explain/present complex ideas; Good conversation share; Speaks up confidently beyond module; Able to talk off slide and engage in debate with clients; Modulates depth to reflect audience & prioritization; Incorporates new facts into learned on the spot into presentation/conversation
- 1: P/PL totally comfortable in letting C conduct great majority of meetings alone; Able to articulate ideas, talk off slide and engage in debate with more senior clients; Occupies spaces during meetings smoothly; Clearly commands discussion and engages audience; Moderates' discussions to productive outcomes; P/PL totally comfortable with C presenting in any setting (would even prefer, at times); Able to articulate ideas, talk off slide and engage in debate with more senior clients; Speaks up confidently about any project content & process

*Presence:* sem rubrica literal própria (ver nota em "Subnotas por dimensão"). O PDF menciona "Confident presence" como parte dos comportamentos de grade 2 em Verbal.

*Written communication:*
- 4: Materials require some iteration (more than P/PL would like) to refine them; Slides with errors/ inconsistency, lacking clear logic; Poor format or visual, "disturbing" content; Some difficulties with more abstract concepts/storylines; Word choice professional but not on most efficient matter
- 3: Consistent and accurate slides that require little iteration; Crispy & visually appealing slides after few interactions; Frames content around key points; Able to develop very good initial versions of storylines
- 2: Refined choice of words (slides and e-mail wording); Crispy & visually appealing slides designed on first attempt; Able to develop very good storylines, connecting to other modules and overall issue; Client-ready decks that require very little iteration (completeness); Wording tailored & refined to audience (putting him/herself in the client's shoes)
- 1: Pro actively develops robust storylines and conceptual frameworks; Independently creates client-ready exec. summaries and decks on first attempt (P/PL barely has to touch it); In a position where he/she could train others in slide writing and story lining

**Practicality & effectiveness**
- 4: Waits to receive next steps or only identifies the more immediate ones; Not very effective upward management; Lack of clear "to do list" / priorities; Often underestimates duration of tasks; Slower than expected to output; Misses' deadlines more than once; Does not follow through on all agreed action items
- 3: Effective 80/20 to prioritize and focus on key issues; P/PL is very comfortable C is "doing the right thing" in between interactions; Drives work forward proactively; Provides updates on work (P/PL still feels the need); Leverages previous work experience; Good speed to output; Communicates timelines, delivers on time; Tends to focus on immediate next steps
- 2: Develops / acts on work plan up front with little input; Very fast to create high quality output; Identifies potential blockages and solutions, having a pragmatic approach; Supports other modules/team members; Displays very strong ownership of module including "so what" and next steps
- 1: Creates tools / invests time to help clients implement (enables the client); Cracks "80% of the case" early on; Creates shell deck for module up-front; Incredibly fast to create high quality, client-ready output; PPL's "right arm" in managing the project; Constantly structures beyond immediate module tasks; PPL confident that C could plan almost scope of the case; PPL knows things will get done (and they do even better) without any need to follow-on

**Client Interaction**
- 4: Tends to be mostly transactional; Establishes good rapport/respect with junior clients; Comfortable in interacting with clients if they are more jr; May shy away in larger settings and with Sr clients/MDPs; Not always recognized by clients as the module "owner"
- 3: Proactively tries to build relationships beyond content with junior clients; Establishes good rapport/respect with middle management clients; Operates comfortably and productively with middle management clients; Understands client perspectives, but may not grasp nuances of organization dynamics
- 2: Proactively tries to build relationships beyond content with mid-management clients; Participates actively in large/senior client settings; Gains respect of senior clients; Operates comfortably with a range of clients and settings; Creates opportunities to engage with clients; Able to draw insight from client dynamics
- 1: Seen as the "BCG face" (go-to person); Builds personal relationships with middle-mgmt clients; Senior clients specifically ask/call him/her; Senior clients complain when C rotates; Able to draw insight from client dynamics; Anticipates clients needs

**Team contribution**
- 4: Often does not raise red flags in advance and/or asks for help when needed; Requires frequent interaction with P/PL; May miss or be late to meetings; May require other A/Cs to jump in to help; May be defensive about tasks; Usually isolates, does not often connect with team to share knowledge
- 3: Helps beyond module only if directly requested; "On top" of things; "Can do" attitude; Carries own weight driving module independently after few interactions; May require a bit more follow-up interactions than P/PL would like; Support to others can vary from slight to pro-actively doing so; Teams with others to leverage existing knowledge base; Strong upward comms.; notifies in advance of delays
- 2: Acts very independently early in the case; Actively contributes to other modules during CTMs or client discussions; Helps identify issues on other modules; Helps reduce stress and contributes to the "team spirit"; Excellent upward & lateral communication
- 1: Constantly goes the "extra mile"; "Right arm" of P/PL on unexpected tasks; Carries the project on his/her back; Delivers things P/PL wasn't expecting in a consistent and recurring way; Checks for case linkages; eager to help across modules; Proactively identifies issues, brings in BCG lead as needed; Mentors junior team members; Great at "acting PL" (if given opportunity); Is the team's "center of gravity", actively supporting the P/PL manage the team's mood

---

## Regras
- Nunca pular etapas — a entrevista é linear.
- Perguntas devem ser diretas e conversar, não ser formulários.
- Se Julia der uma resposta curta, extrair o máximo dela antes de gerar — não inventar exemplos não mencionados.
- Strengths e Areas of Development devem usar dimensões diferentes entre si sempre que possível.
- Overall Summary nunca repete literalmente o que já foi dito nas seções anteriores — sintetiza e eleva.
- Output final em inglês (padrão BCG para CDCs).
- Texto gerado deve ser profissional, específico e baseado apenas no que Julia informou.
- Exemplos concretos citados no texto (resumos de dimensão, Strengths, Areas of Development, etc.) devem sempre generalizar o ponto primeiro e trazer o exemplo entre parênteses, no formato `(e.g., ...)` — nunca soltos na frase ou introduzidos com travessão.
- **Nunca usar travessão (—) em nenhum texto gerado por essa skill** — usar vírgula, parênteses ou ponto e vírgula no lugar.
- A dimensão **Expertise** foi adicionada por Julia especificamente para BCG X e não existe na rubrica oficial de 2023 — usa uma lista de comportamentos de referência fornecida por Julia em 2026-08-13, sem descritores por grade (ver seção "Expertise" na Rubrica de referência).
- Regra de consistência entre notas (Overall ≤ Problem Solving, ou seja, Overall nunca pode ser mais fraca que Problem Solving; demais dimensões, incluindo Expertise, no máximo 1 nota mais fraca que a Overall) deve ser checada explicitamente na Etapa 4 antes de fechar a tabela final.
