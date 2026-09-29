---
name: cdc-dossier
description: Conduz entrevista estruturada para montar o dossiê semestral de CDC (Career Development Committee) de um advisee no papel de CDA — sintetiza avaliações dos últimos 6 meses + entrevistas com PPPLs/MDPs em Overall Summary, Strengths, Areas of Development, Action Plan, Career Outlook e posição na matriz de performance x potencial. Invocar com "monta o dossiê do [nome]", "cdc-dossier: [nome]" ou "preciso fazer o dossiê de CDC do [nome]".
---

# Skill: cdc-dossier

## Tipo
Atômica

## Quando usar
A cada ciclo de CDC (semestral), quando o user, no papel de Career Development Advisor (CDA), precisa montar o dossiê de um advisee para o CDC de Consultants. Distinto da skill de avaliação de performance por projeto (`cdc-performance-review`, se existir no sistema), que avalia performance em um projeto específico — este dossiê sintetiza 6 meses de avaliações + entrevistas com pessoas que trabalharam com o advisee.

## Como invocar
> "monta o dossiê do [nome]"
> "cdc-dossier: [nome]"
> "preciso fazer o dossiê de CDC do [nome]"

## Não confundir
- `cdc-performance-review` → avaliação de performance de **um projeto específico**, qualquer cargo, conduzida pelo manager/PL do projeto.
- `cdc-dossier` (esta) → síntese semestral de **múltiplos projetos e entrevistas**, para advisees no papel de CDA, sempre cargo Consultant.

---

## Processo — entrevista em etapas

Conduzir **uma etapa por vez**. Não avançar sem resposta.

### Etapa 0 — Identificação e cargo
Se o sistema host mantiver um perfil do advisee (ex: `mentees/{nome}/profile.md`), checar cargo registrado antes de perguntar.
- Se cargo já conhecido: confirmar — "Confirmo que {nome} está como {cargo}?"
- Se não: perguntar — "Qual o cargo atual de {nome}? (Associate, Sr. Associate ou Consultant)"

Esta skill assume Consultant como caso padrão (advisees de CDA são todos desse cargo). Se vier outro cargo, sinalizar: "Esse dossiê foi desenhado para o CDC de Consultants — {nome} está como {cargo}, quer que eu prossiga mesmo assim?"

---

### Etapa 1 — Tenure e janela de promoção
Perguntar:
> "Quantos meses de tenure {nome} tem no cargo de {cargo}?"

Calcular a janela de promoção. Marco de 1ª promoção por cargo:
- Associate → 24 meses
- Sr. Associate → 12 meses
- Consultant → 21 meses

Milestones de discussão: 1º milestone = marco − 3; milestones seguintes = 1º milestone + 3n (ex: Consultant, marco 21 → milestones em 18, 21, 24, 27, 30...).

A partir do 1º milestone (18 meses para Consultant), a discussão de promoção é contínua — não é um ponto único no tempo. Cada milestone tem uma janela de ±3 meses ao redor dele. Como os milestones são espaçados de 3 em 3 meses e a janela tem raio 3, normalmente **dois milestones estarão abertos ao mesmo tempo** (o mais próximo abaixo e o mais próximo acima do tenure informado) — três, se o tenure cair exatamente em cima de um milestone.

Regra: para tenure T ≥ 1º milestone, um milestone M está aberto se |T − M| ≤ 3.

Exemplo: Consultant com 25 meses de tenure → milestones abertos: **24 e 27** (não apenas o mais próximo, e não "nenhum" por não bater exato).

Sinalizar sempre que T ≥ 1º milestone:
> "{nome} está com {X} meses — janelas de discussão de promoção abertas: {lista de milestones}."

Se tenure < 1º milestone, seguir sem alarde (só registrar internamente, não bloqueia o dossiê).

---

### Etapa 2 — Dossiê anterior
Perguntar:
> "Existe um dossiê anterior de {nome} para servir de histórico? Se tiver, pode subir agora."

Se o user subir um arquivo:
- Ler o conteúdo
- Se o sistema host mantiver histórico de dossiês por advisee, salvar em local apropriado (ex: `mentees/{nome-em-kebab-case}/cdc-dossiers/YYYY-MM.md`)
- Usar como referência de evolução para Overall Summary, Strengths e Areas of Development (a nova versão deve mostrar progressão, não repetir a anterior)

Se não houver: seguir sem referência.

---

### Etapa 3 — Avaliações do ciclo
Perguntar:
> "Quais avaliações de performance por projeto já foram submetidas para {nome} neste ciclo? Pode listar projeto + avaliador + veredicto, ou colar o texto das avaliações."

Registrar cada avaliação (projeto, avaliador, veredicto/síntese). Essas avaliações entram como insumo formal para Strengths e Areas of Development, ao lado das entrevistas coletadas na Etapa 4 — avaliações de projeto trazem evidência estruturada por dimensão; entrevistas trazem contexto qualitativo adicional que o avaliador nem sempre registra formalmente. Nenhuma substitui a outra.

Se ainda não houver avaliações suficientes ou entrevistas completas no momento, seguir mesmo assim com o que houver disponível.

---

### Etapa 4 — Individuals who provided feedback
Perguntar:
> "Quem você entrevistou para esse dossiê? Lista de nomes e cargo/papel (ex: PPPL do projeto X, MDP do projeto Y)."

Gerar: lista simples, nome + papel + (opcional) contexto do projeto.

---

### Taxonomia de dimensões — granular vs. dimensão mãe

A seleção do que vira Strength ou Area of Development usa as **9 dimensões granulares** da rubrica de Consultants — é nesse nível que o user deve classificar cada relato de entrevista, porque é o nível em que o comportamento realmente aconteceu e tem evidência específica.

Na hora de escrever o bloco no dossiê, o texto é organizado pela **dimensão mãe** (taxonomia consolidada de 7 categorias) — o dossiê é uma síntese de 6 meses, não avaliação de projeto, então agrupa comportamentos correlatos sob um único título.

Mapa de agregação:

| Dimensão granular (seleção da evidência) | Dimensão mãe (título no dossiê) |
|---|---|
| Structures issues; distills insights | Problem Solving and Insights |
| Qualitative analyses | Qualitative and Quantitative Analysis |
| Quantitative analyses | Qualitative and Quantitative Analysis |
| Verbal communication | Communication and Presence (verbal) |
| Written communication | Communication and Presence (written) |
| Prioritizes work | Practicality and Effectiveness |
| Owns module | Practicality and Effectiveness |
| Client relationships | Client Interaction |
| Collaborates, takes on challenges | Collaboration and Team Contribution |

Regra: se duas evidências caem em dimensões granulares diferentes que compartilham a mesma dimensão mãe (ex: um relato sobre Qualitative analyses e outro sobre Quantitative analyses), elas podem virar **um único bloco** de Strength ou AFD, desde que a narrativa consiga amarrar as duas evidências sem forçar. Se não amarrar bem, mantém como blocos separados mesmo compartilhando dimensão mãe.

---

### Etapa 5 — Strengths
Perguntar, uma de cada vez ou em bloco se o user preferir despejar tudo de uma vez:
> "Quais foram as principais fortalezas de {nome} nesses 6 meses? Pode falar por dimensão ou me dar os relatos das entrevistas que eu organizo."

Ao ouvir cada relato, classificar internamente na dimensão **granular** correspondente (tabela acima) antes de decidir o agrupamento. Se a classificação não for óbvia, perguntar: "Isso te pareceu mais [dimensão A] ou [dimensão B]?" — não adivinhar quando houver ambiguidade real.

Gerar 2–3 blocos (o número de Strengths reais reportadas, não forçar 3), agrupados pela **dimensão mãe**. Cada bloco:
- **[Nome da dimensão mãe]:** parágrafo de 3 frases seguindo este padrão:
  1. Afirmação geral da força, no presente ("demonstrates", "consistently builds")
  2. Onde isso apareceu — contexto de projeto/situação sem viés (ex: cliente sênior, ambiente de alta pressão), incluindo evidência qualitativa das entrevistas (palavras como "maturity", "professionalism", "reliability")
  3. Evidência adicional ou confirmação (ex: outra avaliação formal, outro projeto) — mas **parar aqui**. Não adicionar frase de nuance ("this strength has been most tested in...") nem fechamento classificatório ("stable and repeatable strength", "solid foundation") — o padrão é terminar na evidência, sem interpretação/rótulo extra. Se uma nuance relevante (ex: contexto onde a força ainda não foi testada) precisar aparecer, perguntar ao user antes de incluir, não adicionar por padrão.

Ver seção **Regras de estilo — Strengths e AFD** ao final desta skill.

---

### Etapa 6 — Areas of Development
Perguntar:
> "E as principais áreas de desenvolvimento? O que precisa evoluir para o próximo nível?"

Mesma lógica de classificação granular → agrupamento por dimensão mãe da Etapa 5. Gerar 2–3 blocos. Cada bloco segue estrutura de tom suave (nunca abre no problema):
1. Abertura com reconhecimento positivo contextualizado ("demonstrates solid performance... when she is able to build on initial guidance")
2. Transição suave para a lacuna, usando linguagem de oportunidade, nunca de falha ("there is an opportunity to further strengthen/enhance...", nunca "struggles with" ou "fails to")
3. Manifestação concreta do gap, com exemplo real das entrevistas, sempre entre parênteses ou como frase natural (ex: "she has at times benefited from additional senior support to..."). **Parar aqui.** Não adicionar uma 4ª frase de fechamento ligando a melhoria ao próximo estágio de carreira ("will be important for her next stage of growth", "will be key to unlocking...") — esse fechamento acionável já existe, de forma mais apropriada, na Etapa 7 (Action Plan).

Ver seção **Regras de estilo — Strengths e AFD** ao final desta skill.

---

### Etapa 7 — Action Plan
Perguntar:
> "Para cada área de desenvolvimento, qual o plano de ação? Pode ser rascunhado que eu estruturo."

Gerar **um bloco de Action Plan por dimensão-mãe listada nas Areas of Development** — mesmo título de dimensão, na mesma ordem. Não criar dimensão nova aqui: todo Action Plan nasce de uma AFD já escrita na Etapa 5.

Cada bloco: **3 itens numerados**, cada um uma recomendação de comportamento concreta e acionável:
- Frase no imperativo/infinitivo ("Adopt...", "Practice...", "Proactively define...", "Increase engagement...", "In ambiguous situations, commit to...")
- Específica o suficiente para guiar a ação real no próximo projeto, não genérica ("be more proactive" não serve)
- Sempre que possível, descreve o comportamento-alvo E o teste/sinal de que foi bem executado (ex: "test clarity by reviewing whether the message is immediately understandable without verbal explanation")
- Os 3 itens de um bloco não se repetem entre si — cobrem ângulos distintos do mesmo gap (ex: velocidade/mindset, técnica específica, e disciplina de revisão)

Não incluir horizonte de tempo explícito — são práticas a adotar, não deadlines.

---

### Etapa 8 — Overall Summary
Sintetizar a partir do que já foi coletado (Strengths, Areas of Development, tenure/janela de promoção) — não abrir com pergunta em branco, salvo se faltar contexto.

Estrutura padrão (1 parágrafo, 4–5 frases):
1. Caracterização geral da performance (ex: "variable performance across cases" ou "consistently strong performance")
2. Onde a força aparece com mais clareza (contexto/tipo de projeto)
3. Onde a inconsistência ou lacuna aparece (contexto/tipo de projeto) — sem repetir literalmente as AFDs, sintetizar
4. Conexão com tenure atual — qual é a prioridade chave dado o momento de carreira ("At her current tenure, the key priority is...")
5. Frase de fechamento voltada ao próximo ciclo/próximo nível ("For the next cycle, consolidating this... will be critical to fully unlocking her impact at the next level")

Se houver dossiê anterior, usar como referência de formato/tamanho, não de conteúdo — a nova versão deve mostrar evolução real.

**Calibração especial para desfecho negativo** (Potential = Unlikely, Career Outlook em "Red box"):

Quando a leitura de fundo é negativa, o tom **não é** dramatizar a gravidade no texto do resumo. Estrutura correta:
1. **Abertura com uma palavra calibrada** — nem "excelente" nem "ruim", algo como "solid" (que idealmente ecoa a própria categoria de Performance já escolhida na Matrix Position, ex: "Solid, some areas for improvement"), mostrando progresso vs. o ciclo anterior.
2. **Síntese consolidada** de pontos fortes e de desenvolvimento, sem floreio nem excesso de detalhe de projeto (pode citar 1 projeto como evidência, mas não narrar).
3. **Fechamento respeitoso e simples**: reconhece que a pessoa fez um bom trabalho e é vista como valuable asset, mas ainda não atingiu o nível esperado (usar termo formal como "core team member status" — evitar gíria interna tipo "pillar" no texto escrito do dossiê). Não estender para linguagem de urgência, risco futuro, ou menção a tenure/prazo.

O peso da notícia negativa já está carregado pela Matrix Position e pelo Career Outlook (que tem sua própria mensagem pré-definida, ver `references/takeaway-library.md`) — o Overall Summary não precisa (e não deve) duplicar ou antecipar esse peso com linguagem alarmista.

---

### Etapa 9 — Career Outlook
Antes de perguntar, se o sistema host mantiver notas de probing por advisee (ex: `mentees/{nome-em-kebab-case}/probing-notes.md`), trazer à tona para o user o que os avaliadores responderam quando questionados sobre potencial (pilar / 2º recurso / substituir PL) — é insumo para a decisão do user sobre Career Outlook e Matrix Position, **nunca texto a copiar no dossiê** (o dossiê não cita avaliador nominalmente nessas respostas de probing).

**Career Outlook não é texto livre — é escolha de uma mensagem pré-definida.** Uma vez que a posição na matriz (Etapa 10) esteja definida — Etapa 9 e 10 podem ser resolvidas na ordem que fizer sentido na conversa, mas a mensagem de Career Outlook depende da posição —, consultar `references/takeaway-library.md` para localizar o bloco da posição Performance x Potential escolhida.

Regras:
1. **Usar a mensagem exatamente como está escrita no arquivo, sem parafrasear.** Só é permitido substituir os placeholders `XX`/`XXm` (cargo e mês da promoção, quando aplicável) e `(XX¹)` (nome da AfD específica a destacar, se o user quiser citar uma).
2. Se a posição tiver mais de uma variante (Promotion / CDC #2 / CDC #3 / CDC #4 / Lateral hire / Home grown), **perguntar ao user qual contexto se aplica** antes de escolher — não adivinhar (ex: "Essa é uma promoção neste ciclo, ou é CDC #2/#3/#4 sem promoção agora? Para {nome}, que é lateral hire, isso muda a frase.").
3. Se a posição da matriz não tiver bloco correspondente no arquivo (célula não coberta pela library), sinalizar ao user e registrar a resposta como texto livre.
4. Apresentar ao user a(s) frase(s) aplicável(is) e perguntar qual quer usar (ou se quer inserir uma AfD específica no placeholder), antes de finalizar.

---

### Etapa 10 — Posição na matriz (Performance x Potential)
Se o sistema host tiver `probing-notes.md` do advisee e ainda não tiver sido trazido à tona nesta sessão, relembrar o user das respostas de probing antes de perguntar (mesmo racional da Etapa 9 — sinal direto sobre o eixo Potential).

Referência de contexto institucional (não decide pelo user, só informa o pano de fundo): `references/matrix-progression.md` mostra os tracks de progressão (Fast-track, Above track, On track, Extended, Watchlist/CTP), janelas de promoção por track, e o que cada célula Performance x Potential tipicamente implica em termos de trajetória e CDC. Consultar se o user pedir contexto sobre se a posição escolhida está alinhada ao track esperado, mas a escolha final da célula é sempre do user.

Perguntar:
> "Onde {nome} se posiciona na matriz?"

Opções fixas (grid oficial do sistema de CDC):

**Performance:**
- Weak, major areas for improvement
- Solid, some areas for improvement
- Very good
- Outstanding

**Potential:**
- Very high
- High
- To be further demonstrated
- Unlikely
- Not yet assessed

Registrar a combinação escolhida (ex: "Very good / To be further demonstrated").

---

## Output final

```
INDIVIDUALS WHO PROVIDED FEEDBACK
[lista]

OVERALL SUMMARY
[texto]

STRENGTHS

1. [Dimensão]
[texto]

2. [Dimensão]
[texto]

AREAS OF DEVELOPMENT

1. [Dimensão]
[texto]

2. [Dimensão]
[texto]

ACTION PLAN

[Dimensão mãe da AFD 1]

1. [texto]
2. [texto]
3. [texto]

[Dimensão mãe da AFD 2]

1. [texto]
2. [texto]
3. [texto]

CAREER OUTLOOK
[texto]

MATRIX POSITION
Performance: [X]
Potential: [Y]
```

Se o sistema host mantiver histórico por advisee, salvar cópia em local apropriado (ex: `mentees/{nome-em-kebab-case}/cdc-dossiers/YYYY-MM.md`) ao final, junto com o output formatado.

---

## Regras de estilo — Strengths e AFD

Regras destiladas de ~2 anos de calibração real de dossiês escritos por CDA experiente (Principal BCG):

1. **Blocos de 3 frases, ponto.** Não adicionar 4ª frase de nuance/classificação em Strengths, nem 4ª frase de fechamento acionável em AFDs. O bloco termina na evidência concreta.
   - Strength: evitar fechamentos tipo "This is a stable and repeatable strength", "solid foundation", "This strength has been most rigorously tested in..."
   - AFD: evitar fechamentos tipo "Closing this gap will be important for her next stage of growth", "will be key to unlocking...". Esse fechamento acionável já tem lugar próprio no Action Plan (Etapa 7).

2. **Tom sempre suave em AFDs — nunca abrir no problema.** Cada bloco de AFD abre com uma frase de reconhecimento positivo/contexto antes de entrar no gap. Linguagem de oportunidade ("there is an opportunity to..."), nunca de falha ("struggles with", "fails to").

3. **Nunca usar travessão (—) em texto gerado.** Em nenhum contexto — vale para Overall Summary, Strengths, AFD, Action Plan, Career Outlook. Usar vírgula, ponto ou parênteses no lugar. Bullets com hífen simples ("-") como marcador de lista não são travessão e continuam permitidos.

4. **Exemplos concretos entre parênteses**, nunca soltos ou introduzidos com travessão.

5. **Output final em inglês** (padrão BCG para CDCs).

6. **Nunca inventar exemplos.** Texto gerado deve ser específico e baseado apenas no que o user informou nas entrevistas.

7. **Se houver dossiê anterior**, a nova versão deve refletir evolução real, não repetição de formato/conteúdo.

---

## Regras operacionais
- Nunca pular etapas.
- Se existir `probing-notes.md` do advisee no sistema host, consultar nas Etapas 9 e 10 como contexto de decisão para o user — nunca como texto citável no dossiê.
- Cargo padrão é Consultant; sinalizar se vier diferente.
- Strengths e Areas of Development devem usar dimensões diferentes entre si sempre que possível.
- Overall Summary nunca repete literalmente Strengths/AFD — sintetiza e eleva.
