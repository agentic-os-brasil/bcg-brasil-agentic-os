---
name: sharepoint-ingest
description: Lê as pastas SharePoint autorizadas em `brain/memory/sharepoint-config.json`, ingere os materiais recentes via ingest-content e generaliza conceitos/contexto do trajeto, gravando dentro do caso ativo quando há um — o material fica junto do trabalho que ele descreve, nunca numa árvore de conhecimento separada. Use quando o pedido for "ingerir SharePoint", "ler as pastas autorizadas", "puxar racionais das pastas do projeto" ou equivalente.
---

# SharePoint Ingest

Ingerir materiais das pastas SharePoint selecionadas no onboarding e materializar racionais internos + um índice de conceitos que generaliza o contexto do trajeto de pastas. A leitura acontece **em sessão**, usando o conector SharePoint MCP do próprio Claude Code; sem coletor externo, sem enrollment paralelo.

## Interaction profile

Resolver [`interaction-profile`](../interaction-profile/SKILL.md) se disponível. O perfil ajusta profundidade de explicação e sugestões opcionais, nunca o envelope de segurança nem o destino da escrita.

## Contrato de comunicação

- Uma pergunta por vez.
- Sem "você", "tu" ou "te". Impessoal ou 3ª pessoa.
- Sem em-dash em texto externo.
- Nunca copiar corpo bruto de documento para dentro da workspace.
- Nunca enviar conteúdo do documento para provedor remoto como fallback.

## Pré-checagem de MCP (upfront, obrigatória)

Antes de qualquer leitura, verificar se um conector SharePoint MCP está ativo nesta sessão Claude Code. Sinais aceitos:

- Tools MCP com prefixo `mcp__*sharepoint*` ou `mcp__*microsoft*` disponíveis no ambiente.
- Ou tool genérica de fetch autenticada capaz de resolver URLs `sharepoint.com`.

Se nenhum sinal está presente, parar imediatamente e orientar em uma linha:

> "Pra ler as pastas do SharePoint, ativar o conector no Claude Code (Settings → Connectors → SharePoint / Microsoft 365). Depois abrir Maestro de novo e pedir `sharepoint-ingest`."

Não tentar leitura sem conector. Não propor coletor externo. Não gravar nada.

## Onde o material aterrissa — decidido antes de qualquer escrita

Conhecimento não tem uma pasta própria no `brain/`: ele mora junto do que ele
descreve. Material de um cliente fica na pasta daquele cliente, conhecimento do
próprio dono fica nas árvores do dono. Uma pasta de SharePoint de projeto,
portanto, aterrissa **dentro do caso** que aquele projeto é — não numa camada
de conhecimento paralela, que ficaria fora do guard de isolamento entre
clientes e invisível para quem abre o caso.

Antes do passo 1, decida o destino e diga qual é:

- **Com caso ativo** (`brain/accounts/.active`, formato `<conta>/<caso>`) — o
  destino é o próprio caso:
  - racionais por documento em
    `brain/accounts/<conta>/cases/<caso>/sources/sharepoint-rationales/<doc-slug>.md`;
  - índice de conceitos em
    `brain/accounts/<conta>/cases/<caso>/sources/sharepoint-rationales/_index.md`.

  `sources/` é exatamente para isto: ponteiro de fonte autorizada, nunca corpo
  de documento de cliente. Um racional promovido a artefato revisado de canon é
  um ato separado, por [`case-canon-ingest`](../case-canon-ingest/SKILL.md),
  com confirmação individual — esta skill nunca escreve em `canon/`.
- **Sem caso ativo, pasta pessoal do dono** — o destino é
  `brain/memory/sharepoint-rationales/<folder-slug>/`, com o `_index.md` na
  mesma pasta dos racionais que ele indexa. Os ponteiros do índice apontam para
  arquivos irmãos, então separar os dois só quebraria os links.
- **Sem caso ativo e a pasta é claramente de um projeto de cliente** — pare.
  Não grave material de cliente em memória global do dono: aquilo não fica sob
  o guard de isolamento e não aparece para quem abrir o caso depois. Diga em
  uma linha que o caso precisa existir primeiro (`/case-agent-setup`) e ofereça
  retomar em seguida.

Se o caso ativo mudar no meio de um pass, pare: o destino foi decidido no
início e continuar escreveria material de um cliente dentro de outro.

## Fluxo

1. **Confirmar workspace e config.** Ler `${CLAUDE_PROJECT_DIR}/brain/memory/sharepoint-config.json`. Se ausente ou `status != "selected"`, orientar: "as pastas ainda não foram selecionadas; rodar [`maestro-onboarding`](../maestro-onboarding/SKILL.md) primeiro" e parar.

2. **Pré-checagem de MCP.** Ver seção acima. Fail-closed com orientação.

3. **Confirmar escopo.** Listar em uma linha as pastas registradas em `folder_urls` e pedir confirmação: "Ingerir dessas pastas agora? (sim / ajustar / cancelar)". Ajustar redireciona pro [`maestro-onboarding`](../maestro-onboarding/SKILL.md). Cancelar para tudo sem escrever.

4. **Pass bounded pelas pastas.** Para cada `folder_url` autorizada, listar somente os materiais **modificados nos últimos 90 dias** (padrão bounded). Para cada item:
   - Delegar a síntese ao [`ingest-content`](../ingest-content/SKILL.md) apontando a URL como fonte, com destino `<destino>/<doc-slug>.md`, onde `<destino>` é a pasta decidida na seção acima — nunca escolhido aqui.
   - Rationale gravado tem obrigatoriamente: título, `Origem: <URL SharePoint>`, data de modificação, 3–8 bullets, decisões/números citáveis, linha final "Ver original em: <URL>".
   - Nunca copiar o corpo bruto. Se um item é imagem/binário sem texto extraível, registrar o pointer só com metadata e marcar `content: pointer_only`.

5. **Generalização — índice de conceitos do trajeto.** Após o pass, produzir **um** arquivo `_index.md` na mesma pasta dos racionais do pass (`<destino>/_index.md`) com:
   - Nome da pasta + URL raiz.
   - Data do pass.
   - 5–10 conceitos recorrentes atravessando os racionais (temas, stakeholders, entregáveis).
   - Para cada conceito: 1 frase de contexto + até 3 pointers (`<doc-slug>.md`) que sustentam.
   - Gaps observados: pastas/subpastas mencionadas nos racionais mas não autorizadas ainda.

6. **Atualizar config.** Escrever em `brain/memory/sharepoint-config.json`:
   - `last_ingest_at: <ISO-8601>`
   - `last_ingest_summary: { folders: N, rationales: M, concepts: K }`
   - `status` permanece `"selected"`.

7. **Confirmar.** Reportar em 2 linhas: pastas processadas, racionais gerados, caminho do índice de conceitos. Não colar o índice no chat salvo se solicitado.

## Generalização — o que o índice é e não é

- **É:** camada leve de conceitos e pointers, permite ao Maestro rodar [`find-prior-work`](../find-prior-work/SKILL.md) e [`wayfinder`](../wayfinder/SKILL.md) com contexto sem re-ler o SharePoint.
- **Não é:** substituto do SharePoint, memória canônica de decisão, nem base para citação em entregável de cliente sem re-verificação na fonte.

## Failure modes & atomicity

- Escrita per-doc é idempotente por `<doc-slug>.md`: reingerir o mesmo documento substitui o rationale anterior sem tocar nos vizinhos.
- `_index.md` só é escrito **após** todos os per-doc do pass terem sucesso. Não há `_index.md` parcial.
- Falha parcial (ex: doc 7 de 12 falha no [`ingest-content`](../ingest-content/SKILL.md)): os per-doc já escritos permanecem no lugar (retomáveis no próximo pass), `_index.md` não é escrito, e `sharepoint-config.json` recebe `ingest_status: "partial"` com `pending_docs: [<doc-slug>, ...]` para o próximo pass consumir.
- Pass completo com sucesso zera `ingest_status` de volta para `"complete"` e regenera `_index.md` do zero.
- Reentrância no mesmo `folder-slug`: pass novo assume o estado corrente da pasta remota, sobrescreve per-doc por slug e regenera o índice; não mescla índices antigos.

## Invariantes

- Nunca lê pasta fora de `folder_urls`.
- Nunca escreve fora do destino decidido no início do pass: `sources/sharepoint-rationales/` do caso ativo, ou `brain/memory/sharepoint-rationales/` quando não há caso e a pasta é do próprio dono.
- Nunca escreve em `canon/`. Promover um racional a artefato de canon é ato separado e confirmado, por [`case-canon-ingest`](../case-canon-ingest/SKILL.md).
- Nunca cria uma árvore de conhecimento paralela no topo de `brain/`. Conhecimento mora junto do que descreve.
- Sem conector MCP presente, a skill não grava nada.
- Nenhuma chamada a provedor remoto além do próprio conector MCP autorizado pelo owner.

## Fora do escopo

- Descoberta ampla de SharePoint fora das `folder_urls` registradas.
- Ingestão de anexos de email, Teams messages, ou fontes que não sejam as pastas selecionadas.
- OCR de PDF escaneado — herda o mesmo limite do [`ingest-content`](../ingest-content/SKILL.md).

## Encerramento

Uma linha com: pastas processadas, número de racionais, caminho do `_index.md` gerado. Se nada foi gravado, dizer explicitamente por que (config ausente, MCP ausente, owner cancelou) e qual é o próximo passo seguro.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.
