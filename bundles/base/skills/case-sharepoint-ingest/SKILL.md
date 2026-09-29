---
name: case-sharepoint-ingest
description: Lê uma pasta SharePoint do caso ativo (tipicamente a árvore "Modules/" do projeto) e propõe cada documento recente como candidato a canon do caso, um a um, com o módulo inferido da subpasta. Use quando o pedido for "ingere as notas do SharePoint pro caso", "puxa os alinhamentos da pasta de módulos", "processa as notas de reunião do projeto" ou equivalente.
---

# Case SharePoint Ingest

Ler documentos de uma pasta SharePoint pertencente ao caso ativo e materializar
cada um como candidato a artefato de canon do caso, nunca como memória do
dono. A leitura acontece **em sessão**, usando o conector SharePoint MCP do
próprio Claude Code; sem coletor externo, sem enrollment paralelo.

## Como esta skill se diferencia das duas vizinhas

- **[`sharepoint-ingest`](../sharepoint-ingest/SKILL.md)** é do dono: generaliza pastas inteiras em conceitos e
  racionais soltos, em `brain/memory/`. Esta skill é do caso: cada documento
  vira (ou não) um artefato de canon nomeado, ligado ao caso e ao módulo. Uma
  pasta de projeto processada aqui nunca aparece em
  `brain/memory/sharepoint-rationales/`, e uma pasta pessoal do dono
  processada por [`sharepoint-ingest`](../sharepoint-ingest/SKILL.md) nunca aparece no canon de um caso.
- **[`case-canon-ingest`](../case-canon-ingest/SKILL.md)** recebe um documento já identificado e revisado na
  conversa. Esta skill faz a descoberta dentro de uma pasta autorizada e
  propõe candidatos em sequência, mas delega a cada um o mesmo template do
  tipo de canon, a mesma exigência de confirmação individual e a mesma
  proibição de copiar corpo bruto. Nenhuma das duas escreve em lote sem
  confirmação por item.

## Interaction profile

Resolver [`interaction-profile`](../interaction-profile/SKILL.md) se disponível. O perfil ajusta profundidade de
explicação e sugestões opcionais, nunca o envelope de segurança nem o destino
da escrita.

## Contrato de comunicação

- Uma pergunta por vez.
- Sem "você", "tu" ou "te". Impessoal ou 3ª pessoa.
- Sem em-dash em texto externo.
- Nunca copiar corpo bruto de documento para dentro do canon; sempre síntese
  com ponteiro para a fonte.
- Nunca enviar conteúdo do documento para provedor remoto como fallback.

## Pré-checagem de MCP (upfront, obrigatória)

Antes de qualquer leitura, verificar se um conector SharePoint/Microsoft 365
MCP está ativo nesta sessão. Sinais aceitos:

- Tools MCP com prefixo `mcp__*sharepoint*` ou `mcp__*microsoft*` (ex.:
  `sharepoint_search`, `sharepoint_folder_search`, `read_resource`).

Se nenhum sinal está presente, parar imediatamente e orientar em uma linha:

> "Pra ler a pasta do SharePoint deste caso, ativar o conector no Claude Code
> (Settings → Connectors → SharePoint / Microsoft 365). Depois pedir
> `case-sharepoint-ingest` de novo."

Não tentar leitura sem conector. Não propor coletor externo. Não gravar nada.

## Escopo: só o caso ativo

1. Ler `brain/accounts/.active` (`<conta>/<caso>`). Se ausente, parar e
   orientar [`case-agent-setup`](../case-agent-setup/SKILL.md) primeiro.
2. Esta skill nunca escreve fora de
   `brain/accounts/<conta>/cases/<caso>/canon/`. Nenhuma leitura ou escrita
   cruza para outro caso ou para memória do dono.

## Fluxo

1. **Pré-checagem de MCP.** Ver seção acima. Fail-closed com orientação.
2. **Consultar o mapa antes de perguntar.** Se
   `brain/accounts/<conta>/cases/<caso>/sources/sharepoint-map.md` existir
   (gerado por [`case-sharepoint-map`](../case-sharepoint-map/SKILL.md)), ler a tabela de estrutura primeiro. Se o
   tema pedido bater com uma linha do mapa, sugerir a pasta em vez de
   perguntar às cegas — mas deixar claro que é sugestão de um guia que pode
   estar desatualizado, e ainda pedir confirmação do passo 3. Se o mapa não
   existir, seguir direto para o passo 3; não é pré-requisito desta skill.
3. **Confirmar caso e pasta.** Perguntar a URL ou o nome da pasta raiz do
   SharePoint do caso (ex.: a árvore "Modules/" do site do projeto) se ainda
   não foi dita, ou confirmar a sugestão do passo 2. Aceitar múltiplas pastas
   na mesma rodada.
4. **Escopo obrigatoriamente restrito à pasta nomeada.** Resolver a pasta com
   `sharepoint_folder_search` pelo nome, depois listar o conteúdo dessa pasta
   via `read_resource` no URI retornado. Nunca usar `sharepoint_search` com
   termo livre como forma de descobrir documentos — essa busca é sobre o
   tenant inteiro e traz de volta material de outras contas ou de pastas
   pessoais de terceiros no mesmo site. Busca por texto dentro de um
   documento já identificado pela pasta é permitida.
5. **Confirmar escopo antes de processar.** Listar em uma linha as pastas a
   processar e pedir: "Processar essas pastas agora? (sim / ajustar /
   cancelar)". Cancelar para tudo sem escrever.
6. **Pass bounded pela pasta.** Listar os documentos modificados nos últimos
   90 dias (mesmo padrão do [`sharepoint-ingest`](../sharepoint-ingest/SKILL.md)). Para cada um:
   - **Inferir o módulo** a partir do nome da subpasta imediata sob a pasta
     raiz processada (ex.: `Modules/2. Pesquisa/...` → módulo "Pesquisa"). Se
     o documento está direto na raiz, sem subpasta, o módulo fica vazio e só
     é perguntado ao dono se parecer relevante.
   - **Classificar o tipo de canon** — mesmo vocabulário do
     [`case-canon-ingest`](../case-canon-ingest/SKILL.md): `interview` para nota de reunião ou alinhamento
     (nome ou conteúdo sugerindo ata, alinhamento, kick-off, 1:1, reunião),
     `data` ou `framework` para material mais estruturado (deck de
     referência, benchmark, metodologia aplicada). Caso ambíguo, perguntar ao
     dono, um documento por vez — nunca decidir por adivinhação.
   - **Mostrar o rascunho de canon antes de gravar**: título, tipo, módulo,
     fonte (URL SharePoint + data de modificação), conteúdo sintetizado no
     formato do tipo escolhido (ver "Formato do artefato de canon" abaixo).
     Pedir confirmação por documento, nunca em lote.
   - Na confirmação, gravar em
     `brain/accounts/<conta>/cases/<caso>/canon/<type>-<slug>.md`.
   - Recusado ou pulado: seguir para o próximo documento sem perguntar de novo
     na mesma rodada.
   - Item binário sem texto extraível (imagem, `.one` de OneNote, formato não
     suportado): registrar como pulado com o motivo, nunca em silêncio.
7. **Fechar a rodada.** Reportar: pasta(s) processada(s), quantos candidatos
   propostos, quantos confirmados e gravados, quantos recusados ou pulados
   (com motivo), caminho de cada arquivo gravado.

## Formato do artefato de canon

Mesmo template por tipo que o [`case-canon-ingest`](../case-canon-ingest/SKILL.md) declara em "Canon artifact
types" (`hypothesis`, `interview`, `data`, `framework`, `benchmark`) — ler
aquela seção antes de sintetizar. O frontmatter segue o contrato do brain:

```markdown
---
id: accounts/<conta>/cases/<caso>/canon/<type>-<slug>
title: "<Título>"
summary: "<uma linha, o que este documento contém>"
type: <hypothesis|interview|data|framework|benchmark>
scope: account/<conta>/case/<caso>
status: active
sensitivity: client-confidential
source: "<URL SharePoint> — modificado em <data>"
tags: [module:<slug-do-módulo>]
updated: YYYY-MM-DD
---

# <Título>

<síntese no formato do tipo, ver "Canon artifact types" em case-canon-ingest>

## Fonte
- Ver original em: <URL SharePoint>
```

Omitir `tags` quando não houver módulo inferido nem confirmado.

## Failure modes & atomicity

- Escrita per-documento é idempotente por `<type>-<slug>.md`: reingerir o
  mesmo documento oferece atualizar (nova seção datada) em vez de duplicar,
  seguindo a mesma regra do [`case-canon-ingest`](../case-canon-ingest/SKILL.md) para um slug já existente.
- Falha parcial (ex.: documento 5 de 12 falha na leitura): os candidatos já
  gravados permanecem no lugar. Reportar quais ficaram pendentes para a
  próxima rodada, sem inventar um estado de sucesso total.
- Reentrância na mesma pasta: uma rodada nova assume o estado corrente da
  pasta remota; documentos já gravados aparecem de novo como candidato só se
  modificados desde a última passagem.

## Fora do escopo desta release

- OneNote. O conector Microsoft 365 ativo nesta sessão não expõe leitura de
  página de notebook (`.one`) — só arquivo de SharePoint/OneDrive, página de
  SharePoint, email, calendário, Teams e transcrição de reunião. Se o dono
  pedir, dizer isso direto e sugerir exportar a página como Word antes de
  reingerir.
- Descoberta por palavra-chave fora de uma pasta nomeada.
- Ingestão de anexos de email, mensagens de Teams, ou qualquer fonte que não
  seja a pasta SharePoint indicada.
- Criar uma árvore `modules/` dedicada dentro do caso. O módulo vive como tag
  no canon existente; nenhuma estrutura nova é inventada sem pedido explícito
  do dono.

## Invariantes

- Nunca lê ou escreve fora do caso ativo declarado em `brain/accounts/.active`.
- Nunca escreve fora de `brain/accounts/<conta>/cases/<caso>/canon/`.
- Sem conector MCP presente, a skill não grava nada.
- Descoberta de documento é sempre por pasta nomeada, nunca por busca de
  palavra-chave livre no tenant.
- Cada candidato é confirmado individualmente antes da escrita. Nenhuma
  escrita em lote, mesmo quando o dono aprova o processamento da pasta
  inteira de uma vez.
- Corpo bruto do documento nunca é copiado para o canon; apenas síntese com
  ponteiro para a fonte.

## Encerramento

Uma linha com: pasta(s) processada(s), candidatos propostos, quantos gravados
e onde, quantos recusados ou pulados e por quê. Se nada foi gravado, dizer
explicitamente por que (conector ausente, caso não ativo, dono cancelou) e
qual é o próximo passo seguro.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.
