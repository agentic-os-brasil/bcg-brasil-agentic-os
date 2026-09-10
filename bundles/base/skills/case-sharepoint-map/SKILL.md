---
name: case-sharepoint-map
description: Percorre a árvore de pastas do SharePoint do caso ativo em profundidade limitada, amostra um arquivo por pasta para reconhecer o tipo de conteúdo, e grava um mapa de navegação único em brain/accounts/<conta>/cases/<caso>/sources/sharepoint-map.md. Use quando o pedido for "mapeia o sharepoint do caso", "monta um mapa da pasta do projeto", "onde fica cada coisa no sharepoint desse caso" ou equivalente.
---

# Case SharePoint Map

Produzir um guia de navegação do SharePoint do caso ativo: não o conteúdo dos
documentos, só onde cada tipo de informação provavelmente mora. É reconhecimento
de estrutura, não ingestão — a leitura de conteúdo real, documento por
documento, continua sendo trabalho do [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md).

## Por que isto existe, e o que não é

Sem mapa, a única forma de achar algo no SharePoint do caso é saber o nome
exato da pasta ou cair em busca por palavra-chave livre — que varre o tenant
inteiro e traz de volta material de outras contas ou de pastas pessoais de
terceiros no mesmo site ([`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md) documenta esse risco). O mapa
resolve isso: da próxima vez que o dono, o [`find-prior-work`](../find-prior-work/SKILL.md) ou o
[`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md) precisarem saber onde procurar um tipo de informação,
consultam este arquivo primeiro, que já aponta a pasta certa.

O mapa é um guia, não um índice de conteúdo. Ele **envelhece**: pastas mudam,
documentos somem, times reorganizam. Toda leitura do mapa carrega a data da
varredura consigo, e uma resposta construída a partir dele nunca é tratada como
garantia — é o melhor palpite atual de onde procurar, a confirmar abrindo a
pasta de verdade.

## Interaction profile

Resolver [`interaction-profile`](../interaction-profile/SKILL.md) se disponível. O perfil ajusta profundidade de
explicação, nunca o envelope de segurança nem o destino da escrita.

## Contrato de comunicação

- Uma pergunta por vez.
- Sem "você", "tu" ou "te". Impessoal ou 3ª pessoa.
- Sem em-dash em texto externo.
- Nunca copiar corpo de documento para dentro do mapa — só a classificação do
  tipo de conteúdo e, no máximo, o nome de um arquivo de exemplo.
- Nunca enviar conteúdo do documento para provedor remoto como fallback.

## Pré-checagem de MCP (upfront, obrigatória)

Antes de qualquer leitura, verificar se um conector SharePoint/Microsoft 365
MCP está ativo nesta sessão. Sinais aceitos:

- Tools MCP com prefixo `mcp__*sharepoint*` ou `mcp__*microsoft*` (ex.:
  `sharepoint_search`, `sharepoint_folder_search`, `read_resource`).

Se nenhum sinal está presente, parar imediatamente e orientar em uma linha:

> "Pra mapear o SharePoint deste caso, ativar o conector no Claude Code
> (Settings → Connectors → SharePoint / Microsoft 365). Depois pedir
> `case-sharepoint-map` de novo."

Não tentar leitura sem conector. Não gravar nada.

## Escopo: só o caso ativo

1. Ler `brain/accounts/.active` (`<conta>/<caso>`). Se ausente, parar e
   orientar [`case-agent-setup`](../case-agent-setup/SKILL.md) primeiro.
2. Esta skill nunca escreve fora de
   `brain/accounts/<conta>/cases/<caso>/sources/sharepoint-map.md`. Nenhuma
   leitura ou escrita cruza para outro caso ou para memória do dono.

## Fluxo

1. **Pré-checagem de MCP.** Ver seção acima. Fail-closed com orientação.
2. **Confirmar caso e raiz.** Perguntar a URL ou o nome do site/pasta raiz do
   SharePoint do caso, se ainda não foi dita nesta sessão (reaproveitar a raiz
   já usada por um [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md) anterior, se houver uma).
3. **Declarar o orçamento da varredura antes de começar**: profundidade padrão
   de dois níveis (subpastas da raiz, e um nível abaixo de cada uma) e um teto
   de 30 pastas visitadas no total. Se o teto for atingido antes de cobrir a
   árvore inteira, dizer isso no relatório final em vez de estourar
   silenciosamente — nunca aumentar o teto sem o dono pedir.
4. **Resolver a árvore só por pasta nomeada.** `sharepoint_folder_search` para
   achar a raiz e as subpastas, `read_resource` no URI da pasta para listar o
   conteúdo. Nunca `sharepoint_search` com termo livre como forma de
   descoberta — mesma regra do [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md), pelo mesmo motivo:
   evitar varrer o tenant inteiro.
5. **Dobrar pasta pessoal em uma linha só.** Uma subpasta cujo nome sugere
   propriedade individual (ex. "Personal Folders", ou qualquer pasta nomeada
   com iniciais de pessoa dentro de uma árvore compartilhada) não é expandida.
   Ela vira uma única linha no mapa: "material pessoal de membros do time, não
   oficial do caso, não detalhado" — sem abrir a subárvore de ninguém.
6. **Dobrar coleção repetitiva em uma linha só.** Uma pasta cujos filhos
   seguem um padrão de nomenclatura repetido — numeração sequencial ("CTM 1",
   "CTM 2", ...) ou uma subpasta por ocorrência de reunião, nomeada por data
   ("20260204 Touchpoint...", "20260205 Alinhamento...") — não é expandida
   item por item. Ela conta como **uma pasta visitada** contra o teto do passo
   3, mesmo tendo dezenas ou centenas de filhos, e vira uma linha só no mapa
   descrevendo o padrão (o que se repete, o intervalo de datas ou numeração
   observado, a contagem aproximada). Listar cada ocorrência individualmente
   nunca é o objetivo — isso é o oposto de um guia rápido, e cada nome de
   reunião carrega detalhe de stakeholder que o mapa não precisa expor. Achado
   ao testar esta skill pela primeira vez contra um caso real: pastas de
   reunião (cliente e interna) são exatamente assim, uma subpasta por
   instância, e sem esta regra o teto de pastas do passo 3 estoura em duas
   pastas só.
7. **Amostrar, não ler.** Para cada pasta visitada que não foi dobrada nos
   passos 5 ou 6, listar os até 3 itens mais recentes (nome, tipo, data de
   modificação) e ler uma prévia leve de **um** deles — o mais recente, ou o
   mais representativo pelo nome — o suficiente para reconhecer o tipo de
   conteúdo. Nunca ler o documento inteiro nem todos os itens da pasta; isso é
   reconhecimento de estrutura, não extração.
8. **Classificar cada pasta em uma linha**: o tipo de conteúdo que ela
   provavelmente guarda (ex. "decks de pesquisa de mercado", "atas e decks de
   reunião com cliente", "modelo financeiro e planilhas de suporte", "normas de
   time e material administrativo"). Quando a amostra for insuficiente ou
   ambígua, dizer isso na própria linha em vez de inventar uma classificação
   confiante.
9. **Mostrar o mapa completo antes de gravar.** Uma rodada de confirmação
   cobre o mapa inteiro (diferente do [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md), que confirma
   documento por documento): aqui o conteúdo gravado é metadado de estrutura,
   não síntese de material de stakeholder, então uma confirmação por pasta
   seria atrito sem ganho de segurança proporcional. Perguntar: "Gravar este
   mapa? (sim / ajustar / cancelar)".
10. **Gravar o arquivo único** em
    `brain/accounts/<conta>/cases/<caso>/sources/sharepoint-map.md` (ver
    formato abaixo). Se o arquivo já existir, reler antes de sobrescrever: uma
    nota manual do dono no arquivo anterior é preservada como uma seção à parte
    ("## Notas do dono"), nunca descartada silenciosamente.
11. **Fechar a rodada.** Reportar: pastas visitadas, pastas dobradas por serem
    pessoais ou repetitivas, se o teto de 30 foi atingido, caminho do arquivo
    gravado.

## Formato do mapa

```markdown
---
id: accounts/<conta>/cases/<caso>/sources/sharepoint-map
title: "Mapa do SharePoint — <nome do caso>"
summary: "Guia de navegação de onde cada tipo de informação mora no SharePoint do caso, não um índice de conteúdo."
type: source
scope: account/<conta>/case/<caso>
status: active
sensitivity: client-confidential
source: "<URL da raiz mapeada>"
updated: YYYY-MM-DD
---

# Mapa do SharePoint — <nome do caso>

> Guia de onde procurar, não garantia do que está lá. Última varredura:
> YYYY-MM-DD, profundidade 2, teto de 30 pastas ({atingido|não atingido}).

## Estrutura

| Pasta | Tipo de conteúdo | Exemplo | Observado em |
| --- | --- | --- | --- |
| `<caminho relativo à raiz>` | <classificação em uma linha> | `<nome do arquivo>` | YYYY-MM-DD |

## Pastas dobradas (não expandidas item por item)
- `<caminho>` — material pessoal de membros do time, não oficial do caso.
- `<caminho>` — coleção repetitiva (ex.: uma subpasta por reunião ou por número
  sequencial); padrão observado, intervalo e contagem aproximada, sem listar
  cada ocorrência.

## Notas do dono
<!-- preservado entre varreduras; nunca sobrescrito por esta skill -->

## Relacionado
- Use `case-sharepoint-ingest` para trazer um documento específico ao canon do caso.
```

## Como outras skills usam este mapa

- **[`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md)** lê este arquivo, quando existir, antes de
  perguntar a pasta ao dono: se o pedido nomear um tema que bate com uma linha
  do mapa, sugere a pasta em vez de perguntar às cegas.
- **[`find-prior-work`](../find-prior-work/SKILL.md)** consulta este arquivo quando uma busca local não
  encontra nada e o caso tem um mapa: em vez de dizer apenas "não encontrado",
  aponta a pasta onde o mapa sugere que o material provavelmente está, e
  orienta o [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md) como próximo passo.
- Nenhuma das duas trata o mapa como fonte de verdade sobre o conteúdo — só
  como direção de busca.

## Failure modes & atomicity

- Escrita é de arquivo único e idempotente: uma nova varredura substitui o
  mapa anterior por inteiro, preservando apenas `## Notas do dono`.
- Falha no meio da varredura (ex. pasta 15 de 30 falha ao listar): reportar
  quais pastas foram cobertas e quais ficaram pendentes; não gravar um mapa
  parcial como se fosse completo — perguntar se grava o que foi coberto até
  ali, marcado como parcial, ou cancela.
- Nenhuma leitura de conteúdo de documento é persistida fora da classificação
  de uma linha e do nome do arquivo de exemplo.

## Fora do escopo desta release

- Ler o conteúdo completo de qualquer documento — isso é [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md).
- Expandir pastas pessoais de terceiros.
- OneNote, pela mesma limitação de conector documentada em
  [`case-sharepoint-ingest`](../case-sharepoint-ingest/SKILL.md).
- Descoberta por palavra-chave fora de navegação por pasta.
- Varredura recorrente automática. Esta skill roda sob pedido; se o dono
  quiser refrescar o mapa periodicamente, isso é uma decisão separada de
  agendamento, não um comportamento default desta skill.

## Invariantes

- Nunca lê ou escreve fora do caso ativo declarado em `brain/accounts/.active`.
- Nunca escreve fora de `brain/accounts/<conta>/cases/<caso>/sources/sharepoint-map.md`.
- Sem conector MCP presente, a skill não grava nada.
- Descoberta é sempre por pasta nomeada, nunca por busca de palavra-chave livre
  no tenant.
- Pasta pessoal de terceiro nunca é expandida, sempre dobrada em uma linha.
- Coleção repetitiva (padrão de nomenclatura sequencial ou uma subpasta por
  ocorrência de reunião) nunca é expandida item por item, sempre dobrada em
  uma linha, e conta como uma única pasta contra o teto.
- Teto de pastas visitadas é explícito e nunca estourado em silêncio.
- Corpo de documento nunca é copiado para o mapa; apenas classificação e nome
  de exemplo.

## Encerramento

Uma linha com: raiz mapeada, pastas visitadas, pastas dobradas, se o teto foi
atingido, caminho do arquivo gravado. Se nada foi gravado, dizer explicitamente
por que (conector ausente, caso não ativo, dono cancelou) e qual é o próximo
passo seguro.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.
