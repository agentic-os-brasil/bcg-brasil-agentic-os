---
name: brain-index
description: Compila com revisão um conjunto explícito de páginas autorais pelo MCP e consulta busca, índices, backlinks e diagnósticos na geração atual. Use sem presumir varredura global, reparo ou hook automático.
---

# Brain index

## Produto MCP e plugin

1. Confirme o workspace de `maestro_context` e leia `brain.navigation_status`.
   Escolha com o dono os nomes exatos das páginas relevantes; não enumere o
   filesystem nem procure fontes no owner brain, memória privada ou outro caso.
2. Para compilar, use `maestro_apply` com `operation=brain.compile`, `pages`
   (até 24 nomes relativos `.md`) e `purpose`. O total coletado é de até 128 KiB.
   Inclua um alvo ausente apenas se o dono pediu verificar esse nome. Apresente
   a revisão nativa: ela autoriza somente a geração derivada daquele conjunto,
   preservando todo o Markdown canônico. O plano mantém `host.trusted_review`
   como dependência exigida e não atestada; registrar as operações não prova
   que o cliente oferece essa revisão. Sem revisão disponível ou aprovada,
   mantenha a compilação pendente; não substitua por shell/Python.
3. Guarde a `generation` e a `reference=index.md` retornadas. Use
   `brain.navigate(generation, reference, purpose)` para seguir referências
   virtuais; resolva os links relativos dentro dessa árvore derivada e preserve
   o percent-encoding da referência (não decodifique nem normalize duas vezes).
   `#` usa `%23`; um `%23` literal no nome usa `%2523`. Leia as
   páginas em no máximo 4096 bytes, seguindo `next_offset` até `-1`.
4. Busque com `brain.search(generation, query, purpose)`. Use somente a geração
   atual, `offset/next_offset`, `limit` de até 24 e `max_bytes` entre 512 e 4096.
   `complete=true` encerra a busca. Cada resultado contém a página canônica,
   revisão, ponteiro e referência de navegação. Abra o texto original com
   `brain.read(page, purpose)`, também em páginas limitadas.
5. Leia `brain.diagnostics(generation, purpose)` para metadados, órfãos e links
   quebrados observados. Órfão significa sem backlinks **neste conjunto**, não
   em todo o brain. Um link fora do conjunto não foi verificado e não é
   diagnosticado como quebrado. Páginas sem metadados válidos ou superseded
   ficam fora da busca; diagnósticos nunca autorizam reparos automáticos.
6. Uma geração desatualizada ou recusada interrompe a navegação. Consulte o
   status e solicite uma nova compilação explícita; não reapresente texto antigo.
   O mesmo conjunto e bytes, em qualquer ordem, retorna `unchanged`.

A wiki privada de memória é outra superfície: `wiki.status/read/source` e
`memory.search/context` não compilam o brain autoral. Esta operação não instala
hooks nem agenda manutenção. A futura manutenção depende de autoridade própria;
consulta e Stop nunca iniciam a compilação. Não execute `brain-index.py` neste
perfil MCP, mesmo que um recurso legado mencione esse helper.

Uma conversa do produto termina nesta seção. Uma chamada ausente ou recusada
permanece indisponível; ela não seleciona o caminho ZIP.

## Distribuição ZIP legada

Use o restante deste método somente quando os metadados da instalação
identificarem positivamente o perfil ZIP e o helper gerenciado estiver admitido.

Before owner-state file operations, follow [admitted owner-state paths](../interaction-profile/resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


Compila a camada de navegação do brain a partir do frontmatter de cada página.
Tudo que ele produz é **derivado e descartável**: apagar e recompilar não perde nada.
Nenhuma página de índice é autoridade — ela aponta, e a página apontada aponta a fonte.

## Quando roda sozinho

No perfil ZIP, quando `session-stop-brain-index.sh` estiver instalado e ativo,
esse hook recompila ao fim da sessão. Sem esse registro, não há execução
automática a afirmar. O compilador leva ~1s e é
idempotente, então o índice nunca fica atrás do disco: pasta nova entra no índice de
cima, pasta que cruzou o limiar ganha índice próprio, e índice que deixou de qualificar
é removido. Nada disso exige que alguém lembre de fazer.

É por isso que **não existe** a regra "ao criar pasta, atualize o índice mais próximo":
regra que depende de disciplina falha exatamente quando o trabalho aperta. A estrutura é
derivada, não mantida à mão.

A única obrigação manual que sobra é o frontmatter na criação da página — ver
`bundles/base/brain-contract.md`. Sem ele a página é invisível ao compilador.

## Quando rodar à mão

- Quando o dono perguntar "o que eu tenho sobre X" e a resposta exigir varrer o brain.
- Depois de mover ou renomear muita coisa, para conferir antes do fim da sessão.
- Com `--check`, para inspecionar sem publicar.

Não rode a cada mensagem.

## Como rodar

```
python3 "<admitted product_root>/bundles/base/tools/brain-index.py" --product-root "<admitted product_root>"
```

Para inspecionar sem publicar — útil antes de aceitar uma mudança grande:

```
python3 "<admitted product_root>/bundles/base/tools/brain-index.py" --product-root "<admitted product_root>" --check
```

## O que ele produz

| Saída | Conteúdo |
|---|---|
| `brain/brain_index.md` | raiz: só as árvores, os casos e o que atravessa tudo. Nunca lista página |
| `brain/<pasta>/<pasta>_index.md` | um índice por pasta: as páginas dela e ponteiro para as subpastas |
| `brain/.maestro/backlinks.json` | quem aponta para cada página |
| `brain/.maestro/forward-links.json` | para onde cada página aponta |
| `brain/.maestro/diagnostics.json` | quebrados, órfãs, lacunas de contrato, parados |
| `brain/.maestro/log.md` | append-only, uma entrada por compilação, com fingerprint |

Os índices são **hierárquicos**: a raiz aponta para as árvores, cada árvore aponta
para as próprias páginas e para as subpastas, e assim por diante. Nenhum índice repete
o conteúdo do de baixo — é por isso que a raiz cabe numa tela.

O nome do arquivo carrega a pasta (`daily_index.md`, `learnings_index.md`) em vez de
ser `index.md` em toda parte — e quando o basename da pasta sozinho ainda colidiria
entre contas ou casos (`canon/` existe em vários casos), o slug da conta ou do caso
entra como prefixo (`<caso>-canon_index.md`). A regra vale para todo índice
do brain, sem exceção: nenhum arquivo de navegação repete nome com outro, gerado ou
escrito à mão. Pela mesma razão, a curadoria do dono em `learnings/`, `people/` e
`craft/` vive em `<pasta>.md` — o próprio nome da pasta, sem sufixo — e não em
`index.md`: um nome próprio por pasta em vez de um nome genérico repetido em três
lugares. Isso só é seguro porque essas são pastas de topo, únicas por construção;
o mesmo padrão não vale para uma pasta aninhada cujo nome se repete entre casos ou
contas (`canon/` de novo), onde o prefixo acima continua sendo a regra. Onde existe curadoria, o índice
gerado aponta para ela no topo. A raiz do brain segue a mesma lógica: é
`brain_index.md`, não `index.md` solto — o único índice sem pasta dona é o que mais
colidiria com qualquer curadoria de mesmo nome.

Duas regras cortam a fragmentação: uma pasta só ganha índice próprio se tiver ao menos
cinco páginas ou for fronteira de escopo (raiz de conta, raiz de caso, árvore do topo); e pasta de
passagem — um único filho que qualifica — colapsa, com o pai apontando direto para o neto.

**Conta e caso não ganham um índice à parte — a curadoria É o índice.** Nas duas
fronteiras, quando a página curada existe, nenhum arquivo `_index.md` é gerado:
- **Conta** (`brain/accounts/<conta>/<conta>.md`): não recebe nada do compilador.
  A seção `## Cases` é escrita e mantida à mão ([`account-case-setup`](../account-case-setup/SKILL.md)/[`case-agent-setup`](../case-agent-setup/SKILL.md)
  atualizam ao criar um caso) — não há navegação para gerar além dela.
- **Caso** (`brain/accounts/<conta>/cases/<caso>/<caso>.md`): recebe uma seção marcada
  (`<!-- maestro:generated:start -->…<!-- maestro:generated:end -->`) colada no fim do
  próprio brief, com a navegação para canon/decisões/tarefas/entregáveis/fontes — o
  mesmo material que antes vivia num `<caso>_index.md` separado. É o mesmo princípio do
  bloco `maestro:session-scope` nos `AGENT.md`: uma seção marcada dentro de um arquivo
  autoral é a única parte que o compilador escreve; o resto é do dono.

Cada índice tem três camadas: a **vertical** (páginas por tipo), a **lateral**
(seção Conceitos — termos que atravessam mais de um tipo de página) e o
**ranking de backlinks** (as páginas que o resto do brain mais referencia).

## Invariantes

- **Idempotente.** Mesmo conjunto de fontes, mesmo resultado e mesmo fingerprint.
- **Publicação atômica.** Compila em staging, valida tamanho mínimo, só então publica.
  Se falhar no meio, o índice anterior continua de pé.
- **Escopos não se misturam.** O índice de um caso não enxerga outro caso nem o escopo
  do dono. O índice do dono lista os casos por nome e aponta para o índice de cada um,
  sem puxar conteúdo deles.
- **Nunca reescreve conteúdo — com uma exceção declarada.** Fora do brief do caso, só lê
  páginas e escreve índices; nunca toca `<pasta>.md` de conta, craft/learnings/people ou
  qualquer outra página curada. No brief do caso, a única exceção: escreve dentro da
  seção marcada `maestro:generated`, e nunca fora dela — o mesmo contrato do bloco
  `maestro:session-scope` nos `AGENT.md`.
- **Coleta o próprio lixo.** Índice de pasta que não foi gerado na rodada é removido —
  a pasta encolheu, foi renomeada ou sumiu. `<pasta>.md` de conta e a parte autoral do
  brief de um caso nunca são tocados.

## Saúde

Depois de compilar, o hook avalia o diagnóstico e separa o que precisa de você:

- **Erro** (link quebrado, lacuna de contrato) — quebra navegação. Surface na sessão
  seguinte, sempre.
- **Curadoria** (órfã, trabalho parado há mais de 60 dias) — pode ser problema ou escolha.
  Surface no máximo uma vez por semana, sem insistir.

O marcador é `brain/.maestro/.health-requested`; a cadência semanal é controlada por
`.health-last-seen`, que o SessionStart toca ao surfaçar.

## Como ler o diagnóstico

- **`broken_links`** — o link aponta para arquivo que não existe. Sempre conserte:
  costuma ser resto de arquivo movido ou renomeado.
- **`orphans`** — nenhuma outra página de conteúdo aponta para ela. Não é erro; é sinal
  de que o material foi escrito e não foi tecido ao resto. Vale conectar ou aceitar.
- **`contract_gaps`** — página sem o frontmatter de `bundles/base/brain-contract.md`.
  Ela não aparece no índice. Conserte antes de qualquer outra coisa.
- **`stale_open_work`** — tarefa, brief ou objetivo aberto e sem toque há mais de 60 dias.

## Interaction profile

Resolva [`interaction-profile`](../interaction-profile/SKILL.md) antes de apresentar o
diagnóstico. O perfil ajusta quanto se explica de cada achado — um perfil conciso recebe
a contagem e os títulos, não a lista inteira comentada. Ele **não** ajusta o que é
reportado: um link quebrado e uma página fora do contrato aparecem em qualquer perfil,
porque a página que ficou fora do índice é trabalho gravado e invisível, e esconder isso
para ser breve é o oposto do serviço.

O perfil também não muda nada sobre a recompilação em si: ela é determinística, publica
de uma vez e preserva o último índice bom em caso de falha, independentemente de como o
resultado é narrado.

## Limites

Sem embeddings e sem busca vetorial — a spec 007 os coloca fora do V1. Conceitos são
detectados por recorrência de termo, com dois filtros: termo presente em quase todo o
escopo é estrutura de template, não conceito; e um conceito precisa aparecer em mais de
um tipo de página. Um tema que vive só nas dailys é cabeçalho repetido, não sinapse.
