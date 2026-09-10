---
name: excel-financial-model
description: Constrói ou reestrutura um modelo financeiro em Excel que precisa amarrar e ir a comitê — impacto de programa, business case, consolidação de iniciativas, cascata de DRE. Use para "montar o modelo de impacto", "consolidar as iniciativas numa planilha", "esse número não fecha", "reorganizar as abas do modelo", "padronizar o visual do modelo", ou antes de editar um workbook que outra pessoa também edita. Traz a arquitetura, a aba Overview obrigatória, a especificação visual completa e os modos de falha silenciosos. Não concede acesso a arquivo, não publica e não substitui a validação do dono do modelo.
---

# Modelo financeiro em Excel

Um modelo que vai a comitê tem três obrigações: **cada número tem uma única
origem rastreável**, **cada visão derivada amarra com a base na própria
planilha**, e **quem abre o arquivo entende em trinta segundos onde digitar e
onde não tocar**. Arquitetura resolve as duas primeiras; o visual resolve a
terceira, e não é decoração — é o que impede alguém de digitar por cima de uma
fórmula.

O método não adquire dados, não decide premissas de negócio e não aprova número
para uso externo. O dono do modelo permanece responsável por isso.

## Perfil de interação

Resolva o [`interaction-profile`](../interaction-profile/SKILL.md) antes de apresentar resultados. Ele muda a
profundidade da explicação, nunca a arquitetura nem a checagem.

## Entrada necessária

Pergunte, um item por vez, o que estiver faltando:

1. **Unidade de análise** — o que é uma linha da base: iniciativa, projeto,
   contrato, SKU, país. Toda a arquitetura decorre disso.
2. **Dimensões** — as colunas que classificam cada unidade (maturidade, linha da
   DRE, responsável, mês). Distinga as que são **dado** das que são **de-para**
   (atribuição que alguém decide e pode mudar).
3. **Onde o número é digitado** — quem digita, com que frequência, em qual
   granularidade.
4. **O número que tem de fechar** — a métrica que vai ao comitê e contra o que
   ela precisa amarrar (fonte externa, deck anterior, baseline).
5. **Identidade visual** — o logo do cliente (arquivo de imagem) e a **cor
   institucional em hex**. Se não vierem prontos, levante antes de formatar
   qualquer célula: é o Passo 1 do mapa de cores, na Parte 3. Sem isso o modelo
   sai com paleta genérica e parece rascunho — e refazer cor depois de 41 abas
   formatadas custa dez vezes mais que acertar antes.

Sem os itens 1 e 4, pare e pergunte. Um modelo sem checagem é uma opinião
formatada.

---

# Parte 1 — Arquitetura

**Existe uma tabela base, e todas as outras visões derivam dela por fórmula.**
Nenhuma visão de output alimenta outra visão de output.

```
abas de input (uma por unidade)  →  TABELA BASE  →  todas as visões
                                         ↑
                                  de-paras e taxonomias
```

Camadas, na ordem em que se constrói:

1. **Input, uma aba por unidade.** Único lugar onde se digita número. Cada aba
   tem: identificação, resumo por dimensão (fórmula), resumo pela cascata
   financeira (fórmula) e a tabela de detalhe (o input cru, uma linha por
   combinação de dimensões). Os resumos leem o detalhe por `SUMIFS` com critério
   no **rótulo da própria linha** — nunca por endereço fixo.
2. **Tabela base.** Uma linha por unidade, uma coluna por dimensão, mais os
   totais. Cada célula é referência à aba da unidade. Zero número digitado.
3. **Visões derivadas.** Resumo do programa, quebra por responsável, evolução
   temporal, blocos de gráfico. Todas leem a tabela base.
4. **Auxiliares.** Baseline, taxonomia das dimensões, de-paras, snapshots
   congelados, log de mudanças.

## Estrutura e nomenclatura de abas

Numeração que ordena e agrupa por papel, com o número **na frente** do nome:

| prefixo | papel | conteúdo |
|---|---|---|
| `Overview` | capa | obrigatória — ver Parte 2 |
| `1.x` | **output** | resumo do programa, a base por unidade, quebra por responsável, evolução, blocos de gráfico |
| `2.` e `2.NN` | **unidades** | índice canônico das unidades e uma aba por unidade |
| `3.x` | **auxiliares** | baseline, taxonomia, de-paras, snapshots, log |

A tabela base fica em `1.x` porque é output e input ao mesmo tempo: é a única
aba que as outras visões leem. Diga isso no subtítulo dela.

Toda aba de unidade leva um link de volta para o índice, no topo à direita.

---

# Parte 2 — A aba Overview (obrigatória)

**Todo modelo tem uma.** É a primeira aba, e existe para que alguém que nunca
viu o arquivo saiba, sem perguntar: o que o modelo faz, quais são as convenções,
o que cada aba é, quem atualiza cada uma, por onde o dado flui, quais são as
regras, em que estado o arquivo está e o que ainda está aberto.

Layout em duas colunas de blocos, com um par de blocos por faixa vertical:

| faixa | bloco esquerdo (col. B/C) | bloco direito (col. F/G/I) |
|---|---|---|
| 1 | **Description** — cliente, código do projeto, descrição curta em 2-3 linhas do que o modelo faz e o que ele alimenta | **Color codes** — a legenda das 4 cores de célula, com o quadradinho de amostra e o rótulo ao lado |
| 2 | **Tab structure** — uma linha por *grupo* de abas (`1.1 a 1.5`, `2. e 2.01-2.28`, `3.1 a 3.5`), não uma por aba | **Model information** — Currency, Units, Name of owner, Sign convention, Base period, (Others) |
| 3 | **Palette** — o mapa de cores do cliente: uma linha por papel (primária, escura, média, banda, banda leve, grade, bloco de colar), com a amostra formatada e o hex ao lado | *(livre)* |

Abaixo, blocos de largura inteira, nesta ordem:

4. **As abas do modelo** — a tabela `Aba │ O que é │ Quem atualiza`. Descreva
   **cada aba de output individualmente** e **cada auxiliar individualmente**
   (são únicas, cada uma merece uma linha). As abas de unidade entram em **uma
   linha só**, como faixa (`2.01 a 2.28`), porque são 28 cópias do mesmo layout.
   A coluna "Quem atualiza" é a mais importante da aba: ela diz `ninguém (100%
   fórmula)` nas de output, `dono de cada unidade` nas de input, e o nome do time
   nas auxiliares. É o que impede correção no lugar errado.
5. **Fluxo do dado** — a cadeia em duas ou três linhas de texto corrido, com os
   nomes reais das abas e setas: `2.01 a 2.28 (o dono digita) → resumo automático
   na própria aba → 1.2 (a base) → 1.1, 1.3, 1.4 e 1.5`. Feche com a regra
   negativa: *nenhuma aba de output tem número digitado; corrige-se sempre na aba
   da unidade e a correção sobe sozinha.*
6. **Regras do modelo** — lista numerada, uma linha cada, no imperativo. Cobrir:
   procedência de todo número digitado; onde entra mudança de premissa; regra de
   promoção entre camadas e quem autoriza; quando congelar snapshot; dimensões
   que não se confundem; qual aba é o bloco de copiar e colar; o que é linha
   lançável e o que é subtotal; convenção de sinal; **a ordem fixa das dimensões**;
   e a proibição de corrigir em aba de output.
7. **Estado deste arquivo** — contadores **vivos** (fórmula, não texto): unidades
   com dado real carregado, unidades sem responsável, de-para preenchido, e as
   duas ou três métricas de manchete. Se um contador estiver escrito à mão ele
   mente em uma semana.
8. **Pendências conhecidas** — bullets do que está aberto, com valor quando
   houver. **Apague o item quando resolver.** Pendência resolvida que continua
   listada é pior que pendência não listada: destrói a confiança na lista toda.

Larguras que fazem o layout funcionar: `A` estreita (1,7) como margem, `B` 23
para rótulos, `C` larga (60-93) para o texto corrido, `E` e `H` estreitas
(0,9-1,8) como separadores verticais entre os blocos, `F`/`G`/`I` 11-16 para o
bloco direito.

---

# Parte 3 — Especificação visual

## Topo de página, idêntico em toda aba

- **Logo do cliente ancorado em `A1`**, no canto superior esquerdo, `top=0`,
  `left=0`, cerca de **77×40 px**. Em **todas** as abas, sem exceção — inclusive
  as auxiliares. É o que faz o print de qualquer aba poder ir a slide.
- **Nome da aba em `C2`** — à direita do logo, na mesma faixa. Calibri **16**
  negrito, na cor institucional do cliente. Escreva o número junto:
  `1.  Full Summary`.
- **Subtítulo em `C3`** — uma linha dizendo o que a aba é e de onde ela lê.
  Calibri **11**, preto.
- Alturas de linha do topo: `1` = 11,3 (o logo ocupa), `2` = 21, `3` = 15,
  `4` = 15,8, `5` = 9, `6` = 13. A primeira seção começa em `L8`.

## Mapa de cores do cliente

**A paleta é do cliente, não do método.** O que se reaproveita entre modelos é o
**mapa de papéis** — quais elementos recebem cor e em que relação entre si. As
cores concretas mudam a cada projeto e são a **primeira coisa a levantar**, antes
de formatar qualquer célula.

### Passo 1 — levantar a cor institucional

Na ordem de preferência: manual de marca do cliente → paleta oficial no site de
RI → amostragem do logo em alta resolução (pegue a cor dominante do símbolo, não
do fundo). Registre em hex. Precisa de **uma cor primária**; duas ou três se a
marca tiver.

Se o cliente não tiver cor definida ou o material for interno, use a paleta da
firma. Nunca invente uma cor "que combina".

### Passo 2 — derivar os sete papéis

Da cor primária saem todos os tons, por regra fixa:

| papel | como derivar | onde aparece |
|---|---|---|
| **primária** | a cor institucional, como está | título de aba, título de seção, fonte da linha de total, borda que separa seções |
| **escura** | a primária, um passo mais escura/saturada | fundo do header de tabela (com fonte branca) |
| **média** | tom intermediário entre primária e clara | fundo de header de 2º nível, quando há tabela dentro de tabela |
| **banda** | primária a ~15% de opacidade sobre branco | fundo de linha de nome/agrupamento |
| **banda leve** | primária a ~7% | fundo de linha de total |
| **grade** | cinza neutro claro, fora da paleta do cliente | bordas da tabela |
| **bloco de colar** | neutro quente muito claro (creme/areia), fora da paleta | fundo da seção que se copia para o slide |

Duas regras que sustentam o mapa:

- **Grade e bloco de colar ficam fora da paleta do cliente, de propósito.** A
  grade tem de desaparecer na leitura, e o bloco de colar tem de se destacar como
  "isto não é análise". Se os dois entrarem na cor da marca, o olho perde a
  hierarquia.
- **Se a cor primária for clara ou muito saturada** (amarelo, laranja, verde
  limão), não a use como fundo de header: fonte branca sobre ela fica ilegível.
  Nesse caso use a versão escura como header e reserve a primária para texto.

### Passo 3 — registrar o mapa no próprio modelo

O mapa vai num bloco **Palette** na Overview, ao lado do Color codes: uma linha
por papel, com a amostra formatada e o hex escrito ao lado. Quem for editar o
modelo em seis meses precisa saber qual é o azul certo sem abrir o manual de
marca. Sem esse registro, cada pessoa que mexe adiciona um tom novo e em três
meses o modelo tem onze azuis.

> **Exemplo — operadora de saúde com identidade azul-escura.** Primária `#011E61`, escura
> `#022781`, média `#1F4E79`, banda `#D9E2F3`, banda leve `#EAEFF9`, grade
> `#BFBFBF`, bloco de colar `#F7F3E8`. Serve de referência de *relação entre os
> tons*, não de valor a copiar.

## Separação de seções

- **Título de seção**: Calibri **14** negrito na cor **primária**, fundo branco,
  com **borda inferior fina na cor primária** correndo por toda a largura da
  seção (não só sob o texto). É a linha que separa visualmente os blocos do
  modelo, e ela é da cor do cliente — borda preta faz o modelo parecer planilha
  genérica.
- **Header de tabela**: Calibri **12** negrito, fonte branca, fundo na cor
  **escura**.
- **Grade**: bordas finas na cor **grade** (cinza neutro) nos quatro lados de
  cada célula da tabela, do header até a linha de total. Fora da tabela, sem
  borda.
- **Linha de nome/agrupamento**: negrito, fundo na cor **banda**.
- **Linha de total**: negrito na cor **primária**, fundo na cor **banda leve**.
- **Gridlines do Excel desligadas em toda aba.** A grade é a da tabela, não a da
  planilha. Isso sozinho muda a percepção de "planilha" para "documento".
- Zoom 100% em todas as abas.

## Cor da guia por grupo

A guia colorida é o índice visual do arquivo — dá para navegar 41 abas sem ler
nome. Também sai do mapa de cores, por intensidade:

| grupo | tom |
|---|---|
| Overview | o mais claro da paleta — é capa, não conteúdo |
| `1.x` output | a cor **escura**, cheia: é onde o leitor deve ir |
| `2.` e `2.NN` unidades | um tom **intermediário**, para distinguir input de output à distância |
| `3.x` auxiliares | a cor **escura**, igual ao output — são o encanamento, mas são do time |
| qualquer aba que precisa de atenção | **vermelho**, fora da paleta |

O vermelho é a única cor que não vem do cliente, porque é sinalização e não
identidade: aba com pendência de revisão, ou aba descartável que ainda não saiu.
Não deixe vermelho permanente — ele para de significar alguma coisa.

> **Exemplo — a mesma paleta aplicada às abas.** Overview `#DCE6F1`, output e auxiliares `#022781`,
> unidades `#8DB4E2`, atenção `#CC0000`.

## Fonte e tamanho

**Calibri em todo o arquivo**, com quatro tamanhos e nada entre eles:

| elemento | tamanho |
|---|---|
| título da aba | **16** |
| título de seção | **14** |
| header de tabela | **12** |
| número, dado, rótulo de linha, subtítulo, nota | **11** |

A altura da linha acompanha a fonte: **15** para corpo 11, **15,75** para
header 12, **18,75** para 14, **21** para 16. Fonte maior em linha antiga corta
o texto pela metade e ninguém percebe até o print.

## Número

- **Formato único**: `#,##0;[Red]-#,##0;"-"` — negativo em vermelho, zero como
  traço. Zero visual polui e esconde o que importa.
- **Valor cheio, não em milhões**, com a unidade declarada no título da seção
  (`(R$)`). Escala abreviada é onde erro de ordem de grandeza se esconde.
- **Célula vazia em vez de zero** onde o dado alimenta gráfico — zero desenha
  linha no eixo, vazio não desenha nada:
  `=IF(ROUND(<ref>,6)=0,"",<ref>)`.
- **Ordem das dimensões fixa em todo o arquivo.** Escolha uma e não varie: se a
  ordem da maturidade for `Potencial → Planejado → Mapeado → Confirmado →
  Capturado`, é essa em toda linha, coluna e aba.

---

# Parte 4 — Color coding das células

Quatro estados, comunicados por **fundo + cor da fonte**, com a legenda visível
na Overview. Isto não é preferência estética: é o contrato que diz onde se pode
digitar.

**Estas quatro cores não variam por cliente.** Ao contrário da paleta da Parte 3,
o color coding de célula é convenção universal de modelagem financeira — qualquer
revisor externo, de qualquer firma, procura fonte azul para achar o input. Trocar
essas cores pela marca do cliente destrói a única linguagem que o arquivo tem em
comum com quem vai auditá-lo.

| estado | fundo | fonte | significado |
|---|---|---|---|
| **Assumptions** | `#FFFFCC` amarelo claro | `#0000FF` azul | Premissa digitada. Alguém escolheu esse número e ele é discutível. |
| **Historical inputs** | branco | `#0000FF` azul | Dado histórico digitado. Vem de fora, não se discute, mas também não é fórmula. |
| **Formulas / outputs** | branco | preto | Calculado. **Não digite aqui.** |
| **Important output** | `#FFC000` âmbar | preto | O número que vai ao comitê. Marca poucos por aba — se marcar dez, não marcou nenhum. |

Regras de uso:

- **Fonte azul = alguém digitou.** É a convenção universal de modelagem
  financeira e a primeira coisa que um revisor externo procura. Nunca use azul
  em célula com fórmula.
- **Amarelo = premissa, branco = fato.** A distinção entre premissa e dado
  histórico é o que permite rodar sensibilidade sem tocar no que é medido.
- **Âmbar é escasso por definição.** Um por bloco de output, no máximo.
- A legenda na Overview usa uma célula de amostra com o texto `abc` formatada em
  cada estado, e o rótulo ao lado — a amostra tem de ser formatada de verdade,
  não descrita.
- Se uma célula muda de estado (premissa que virou fórmula), **mude a cor na
  mesma edição**. Cor errada é pior que cor ausente.

---

# Parte 5 — Bloco de copiar e colar para o slide

O bloco que vai ao gráfico é uma seção própria, com fundo creme `#F7F3E8` para
dizer "isto não é análise, é fonte de gráfico". Estrutura:

- Uma **coluna por item exibido**, na ordem em que o gráfico os desenha.
- Uma coluna de **resto**, calculada como *total do programa menos os itens
  nomeados* — não como soma dos que faltam.
- Uma coluna de **total** da linha.
- As camadas abaixo da linha de água entram com **sinal invertido**, porque é
  assim que o gráfico as desenha. Escreva isso no subtítulo, senão alguém lê o
  negativo como perda.
- Linha de ID do item acima dos nomes, formatada como input (amarelo/azul):
  é ali que se escolhe quais itens aparecem.

Ponto frágil: sobrescrever a coluna de resto ou a de total com a fórmula dos
itens nomeados zera as duas, e o bloco para de fechar sem dar erro.

Se o mesmo modelo alimenta vários gráficos, faça **um bloco por corte** (por
unidade, por período mensal, por período semanal, por responsável) — todos no
mesmo formato, todos fechando no mesmo total. É melhor ter quatro blocos que
fecham do que um bloco que alguém remonta à mão a cada semana.

---

# Parte 6 — Checagem

Toda amarração vira célula visível. Um modelo que só fecha quando alguém roda um
script não fecha.

1. **Checagem por unidade.** Em cada aba de unidade, uma célula que confronta o
   resumo contra o detalhe e escreve `OK` ou `ERRO`.
2. **Checagem de cobertura.** O número de unidades classificadas em cada de-para
   soma o total de unidades: `=IF(SUM(...)=N,"OK","checar: "&SUM(...))`.
3. **Bloco de checagem no topo da visão.** Nas visões grandes, as linhas de total
   ficam **acima** do cabeçalho, não no fim — quem abre a aba vê o agregado antes
   de rolar.
4. **Caminho independente de agregação.** O mesmo número alcançável por dois
   caminhos (soma das unidades e soma por dimensão), comparados numa célula. Um
   caminho só não detecta erro de classificação.
5. **Referência externa congelada.** Quando existe um número que o modelo tem de
   reproduzir, grave-o numa linha de `Referência` ao lado do calculado, com uma
   linha de `Diferença`. A diferença é a checagem; some-a e o modelo perde a
   âncora.

## Residual explícito

Sempre que a fonte tem um bucket agregado que não pertence a nenhuma unidade
("Outros", "sem responsável"), ele entra como **linha própria** na base, com o
responsável marcado como `Sem VP` ou equivalente. Nunca distribua o residual
entre as unidades para o total fechar: o total fecha e a informação de que aquele
valor não tem dono se perde. Nas visões por responsável isso implica uma linha
extra — sem ela a soma dos responsáveis não bate com o consolidado.

## De-para é tabela, não regra em código

Toda atribuição que uma pessoa decide (unidade → responsável, bucket da fonte →
unidade, período da fonte → coluna do modelo) vive numa **tabela visível**, uma
célula por decisão, com coluna de observação para o que está por confirmar.
Regra embutida em fórmula ou em script é invisível e ninguém revisa.

Quando a fonte abre a mesma unidade em dois buckets no mesmo período, o de-para
precisa de uma **segunda coluna de bucket** e a fórmula soma os dois. Forçar
um-para-um perde valor em silêncio.

## Série temporal — congelada, não recalculada

- O período **corrente** é vivo: lê a base, muda quando o input muda.
- Os períodos **fechados** são constantes gravadas. Se ficarem como fórmula sobre
  a base, cada edição de input reescreve o passado.

Ao congelar, registre no de-para de onde cada número veio (qual corte da fonte,
qual bucket). E confirme a correspondência de datas **perguntando**, não
deduzindo por encaixe de soma: um eixo deslocado uma posição fecha igual e o erro
só aparece semanas depois.

---

# Parte 7 — Modos de falha

Todos abaixo foram observados na prática. **Nenhum gera erro na tela.**

**Reconstruir do zero um arquivo que o dono edita à mão.** Gerar o arquivo por
script destrói o trabalho manual na execução seguinte, e a perda é silenciosa.
Enquanto houver edição manual em paralelo, **edite in-place** e confirme antes
que o arquivo está salvo.

**Ligar visão derivada por posição em vez de por nome.** Blocos gerados em ordem
alfabética e ligados por posição a uma grade em outra ordem devolvem número
plausível do dono errado. Ligue por nome resolvido dinamicamente — erro de nome
devolve zero ou erro, que é ruidoso e portanto seguro. **A verificação também não
pode ser por posição**: um caractere solto digitado numa linha vazia desloca um
pareamento posicional e valida o modelo errado.

**Mover fórmula com referência relativa à própria linha.** `=SUM(C11:H11)`
copiada para outra linha continua somando a linha 11. Ao reordenar linhas:
fórmula com critério no rótulo (`SUMIFS(...,$B11)`) **se reordena sozinha** — só
troque o rótulo; fórmula com referência relativa **tem de ser regerada**.

**Apagar aba que outra aba lê.** Vira `#REF!` em massa. Religue as referências
**antes** de apagar. Renomear é seguro (o Excel atualiza tudo); apagar não é.

**Procurar erro comparando texto.** Célula com erro não devolve a string do erro
na leitura por valor. Use
`UsedRange.SpecialCells(xlCellTypeFormulas, xlErrors)`.

**Copiar intervalo entre abas esperando que as referências se ajustem.** Cópia de
**aba** preserva referência interna corretamente; cópia de **intervalo** não.
Recortar e colar atualiza as referências que apontavam para o intervalo movido;
copiar e colar não atualiza nada.

**Remendar fórmula com substituição de texto.** Regex sobre fórmula quebra
intervalo (`$B$23:$B$50` tem prefixo de aba só na primeira metade) e produz
referência entre abas diferentes, que não dá erro. Reescreva a fórmula inteira a
partir do padrão conhecido.

**Trocar formato e esquecer a altura da linha.** Aumentar a fonte sem subir a
altura corta o texto. Depois de qualquer mudança de fonte, varra as linhas e suba
as que ficaram abaixo do mínimo. Verifique também `####` em coluna estreita.

**Escala e sinal da fonte.** Confirme a unidade (milhões x cheio) e a convenção
de sinal **por camada e por período** — pode variar dentro do mesmo arquivo. Some
as camadas de um período e confronte com o total daquele período na própria fonte
antes de carregar qualquer coisa.

---

# Contrato de saída

Entregue, nesta ordem:

1. **O número que fecha**, com a amarração ao lado: qual visão foi confrontada
   com qual, e a diferença (que deve ser zero).
2. **O que foi construído ou alterado**, por aba, com o endereço das faixas.
3. **O que ficou fora e por quê** — residual sem dono, unidade sem valor
   quantificado, de-para por confirmar. Um por linha, com o valor.
4. **As decisões que dependem do dono** — cada uma como pergunta fechada,
   apontando a célula onde a resposta entra.

Nunca reporte número sem a amarração que o sustenta. E quando a checagem falhar,
**não salve**: reporte a divergência e o que a causou.

# Registro

Decisão de arquitetura, convenção de sinal, correspondência de período e
mapeamento de de-para vão para o log de decisões do caso via
[`case-decision-log-entry`](../case-decision-log-entry/SKILL.md). Método que se repetir entre modelos vai para `brain/craft/`
via [`craft-update`](../craft-update/SKILL.md). Sem isso, a próxima sessão redescobre a mesma convenção do
zero — e a redescobre errada.
