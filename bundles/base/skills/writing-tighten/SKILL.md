---
name: writing-tighten
description: Aperta um texto já escrito — corta preâmbulo, nominalização, hedge empilhado e repetição, calibrado pela voz registrada do próprio dono em `brain/owner/self/voice.md` (texto externo) ou `communication-style.md` (texto interno). Use para "deixa isso mais direto", "enxuga esse texto", "corta o excesso", "isso ficou prolixo", "revisa a escrita disso", ou antes de mandar um e-mail, mensagem ou memo que saia com o nome do dono. NÃO use para escrever de zero, para revisar storyline de deck (isso é `deck-review`), para checar evidência de uma recomendação (isso é `yoda`), nem para resumir um documento longo — apertar não é resumir.
---

# Apertar a Escrita

Um texto já existe e está mais longo do que precisa. O trabalho aqui é cortar o
que não carrega significado, **sem tocar no que ele afirma** — e mostrar o corte,
em vez de devolver um texto novo e pedir confiança.

## Apertar não é resumir

Esta é a única distinção que importa, e é onde uma passada de concisão estraga
um texto:

- **Resumir** decide o que sai do conteúdo. Perde informação de propósito.
- **Apertar** mantém cada afirmação, cada ressalva e cada número, e corta as
  palavras que não estão fazendo trabalho nenhum.

Se o corte muda o que a frase afirma — o grau de certeza, o escopo, quem disse —
não era excesso: era conteúdo. Devolva a frase.

## Antes de cortar: a voz é do dono, não uma casa de estilo

Leia a faceta que corresponde ao destino do texto. É isso que separa esta skill
de um guia de estilo genérico: o alvo é a voz que o dono já registrou, não uma
noção externa de boa prosa.

- **Texto que sai com o nome do dono** — e-mail a cliente, mensagem a sócio,
  memo, post — leia `brain/owner/self/voice.md`.
- **Texto interno, para o próprio dono** — nota, página de caso, resumo de
  sessão — leia `brain/owner/self/communication-style.md`.

Se a faceta não existir, siga com as regras gerais abaixo e **diga em uma linha
que está sem calibração**: a trilha curta do onboarding deixa `voice`
deliberadamente para depois, então essa ausência é comum e não é erro. Nunca
invente uma voz a partir do próprio texto que está sendo apertado — o texto pode
justamente não estar na voz que o dono quer.

Se a faceta existir e contradisser uma regra geral, **a faceta ganha**. Um dono
que registrou que prefere abrir com contexto antes do pedido não está errado
sobre a própria voz.

## O que cortar

Dez padrões. Cada um vira uma edição concreta, não um comentário.

| # | Padrão | Exemplo → corte |
|---|---|---|
| 1 | Preâmbulo antes do ponto | "Queria te trazer uma questão sobre o prazo" → "O prazo escorregou" |
| 2 | Nominalização — verbo virado substantivo | "fizemos uma análise de" → "analisamos" |
| 3 | Par redundante | "claro e transparente", "planejamento e organização" → escolha a palavra que carrega |
| 4 | Hedge empilhado | "pode ser que talvez", "aparentemente parece" → um hedge, ou nenhum |
| 5 | Meta-comentário sobre o documento | "Neste memo vamos explorar" → comece pelo assunto |
| 6 | Intensificador vazio | "muito importante", "extremamente relevante" → o adjetivo sozinho, ou o fato |
| 7 | Passiva onde o ator importa | "foi decidido" → quem decidiu |
| 8 | Transição de aquecimento | "Dito isso", "Nesse sentido", "Vale destacar que" → corte a frase inteira |
| 9 | Repetir a pergunta antes de responder | "Sobre se conseguimos entregar até sexta: sim" → "Sim" |
| 10 | Fechamento que repete o que foi dito | último parágrafo que não acrescenta → corte |

## O que nunca cortar

Cinco coisas parecem excesso e não são. Cortar qualquer uma delas transforma
concisão em imprecisão:

- **Hedge que marca incerteza real.** "Provavelmente" sobre um número não
  conferido é precisão, não enchimento. Cortar transforma estimativa em fato.
- **Qualificador que limita a afirmação.** "nos três casos medidos", "no
  cenário base" — sem ele a frase afirma mais do que se sabe.
- **Atribuição.** "segundo o cliente", "na leitura do time" — cortar
  transforma relato em asserção própria.
- **Número, nome, data, unidade, citação.** Preservados exatamente, incluindo
  a precisão decimal como está escrita.
- **Concessão que sustenta o argumento.** O "é verdade que X, mas" que
  antecipa a objeção é o que faz o resto convencer.

Na dúvida entre corte e conteúdo, **mantenha e aponte**: mostre a frase ao dono
e diga por que ficou.

## Fluxo

1. Confirme o destino do texto — externo (sai com o nome do dono) ou interno.
   Isso decide qual faceta calibra. Se não estiver claro pelo pedido, pergunte
   em uma linha; é a única pergunta desta skill.
2. Leia a faceta correspondente. Registre em uma frase o que ela pede, para
   aplicar de forma consistente e para o dono poder discordar da leitura.
3. Passe o texto contra os dez padrões, marcando cada candidato.
4. Passe os candidatos contra a lista do que nunca se corta. O que sobreviver
   às duas listas é edição; o resto volta.
5. Apresente **o texto apertado e o que mudou**, nesta ordem:
   - o texto final, pronto para usar;
   - abaixo, os cortes por categoria, em uma linha cada — não um diff palavra
     por palavra, e não um ensaio: "cortei três aquecimentos de transição",
     "troquei duas nominalizações por verbo";
   - e, separado, o que **deixou de cortar** e por quê, quando houver. Essa
     lista é a mais útil das três: é onde o dono vê que a passada entendeu o
     texto em vez de só encurtar.
6. Diga a redução em números — de quantas palavras para quantas. Um texto que
   encurtou 8% não valeu a rodada, e é honesto dizer isso.

## Não-negociáveis

- Nunca altere o que o texto afirma: grau de certeza, escopo, atribuição,
  número ou nome. Apertar é uma operação sobre palavras, não sobre conteúdo.
- Nunca grave nada. Esta skill devolve texto na conversa; quem decide usar é o
  dono, e quem guarda uma calibração de estilo durável é
  [`craft-update`](../craft-update/SKILL.md), num ato separado.
- Nunca invente a voz do dono a partir do texto sendo apertado.
- Nunca aperte um texto que o dono não escreveu nem colou nesta conversa. Ler
  um arquivo para apertá-lo é um pedido explícito, com o caminho dito pelo dono.
- Nunca transforme a passada em reescrita. Se o texto precisa de outra
  estrutura, diga isso em uma frase e pare — reestruturar sem pedir é devolver
  um texto que o dono não reconhece.
- Nunca aplique isto a material de cliente para além do texto apresentado. O
  documento de origem continua onde está.

## Interaction profile

Resolva [`interaction-profile`](../interaction-profile/SKILL.md) antes de
apresentar. O perfil ajusta quanto se explica sobre cada corte — um perfil
conciso recebe as três listas mais curtas, não menos cortes e nunca sem a lista
do que ficou. O que o perfil calibra é a explicação, nunca o critério de corte
nem a exigência de mostrar o que mudou.
