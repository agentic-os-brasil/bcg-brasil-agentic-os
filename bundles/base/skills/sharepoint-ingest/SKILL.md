---
name: sharepoint-ingest
description: Lê as pastas SharePoint autorizadas em `data/memory/sharepoint-config.json`, ingere os materiais recentes via ingest-content e generaliza conceitos/contexto do trajeto. Use quando o pedido for "ingerir SharePoint", "ler as pastas autorizadas", "puxar racionais das pastas do projeto" ou equivalente.
---

# SharePoint Ingest

Ingerir materiais das pastas SharePoint selecionadas no onboarding e materializar racionais internos + um índice de conceitos que generaliza o contexto do trajeto de pastas. A leitura acontece **em sessão**, usando o conector SharePoint MCP do próprio Claude Code; sem coletor externo, sem enrollment paralelo.

## Interaction profile

Resolver `interaction-profile` se disponível. O perfil ajusta profundidade de explicação e sugestões opcionais, nunca o envelope de segurança nem o destino da escrita.

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

## Fluxo

1. **Confirmar workspace e config.** Ler `${CLAUDE_PROJECT_DIR}/data/memory/sharepoint-config.json`. Se ausente ou `status != "selected"`, orientar: "as pastas ainda não foram selecionadas; rodar `maestro-onboarding` primeiro" e parar.

2. **Pré-checagem de MCP.** Ver seção acima. Fail-closed com orientação.

3. **Confirmar escopo.** Listar em uma linha as pastas registradas em `folder_urls` e pedir confirmação: "Ingerir dessas pastas agora? (sim / ajustar / cancelar)". Ajustar redireciona pro `maestro-onboarding`. Cancelar para tudo sem escrever.

4. **Pass bounded pelas pastas.** Para cada `folder_url` autorizada, listar somente os materiais **modificados nos últimos 90 dias** (padrão bounded). Para cada item:
   - Delegar a síntese ao `ingest-content` apontando a URL como fonte, com destino `data/memory/sharepoint-rationales/<folder-slug>/<doc-slug>.md`.
   - Rationale gravado tem obrigatoriamente: título, `Origem: <URL SharePoint>`, data de modificação, 3–8 bullets, decisões/números citáveis, linha final "Ver original em: <URL>".
   - Nunca copiar o corpo bruto. Se um item é imagem/binário sem texto extraível, registrar o pointer só com metadata e marcar `content: pointer_only`.

5. **Generalização — índice de conceitos do trajeto.** Após o pass, produzir **um** arquivo `brain/knowledge/sharepoint-rationales/<folder-slug>/_index.md` com:
   - Nome da pasta + URL raiz.
   - Data do pass.
   - 5–10 conceitos recorrentes atravessando os racionais (temas, stakeholders, entregáveis).
   - Para cada conceito: 1 frase de contexto + até 3 pointers (`<doc-slug>.md`) que sustentam.
   - Gaps observados: pastas/subpastas mencionadas nos racionais mas não autorizadas ainda.

6. **Atualizar config.** Escrever em `data/memory/sharepoint-config.json`:
   - `last_ingest_at: <ISO-8601>`
   - `last_ingest_summary: { folders: N, rationales: M, concepts: K }`
   - `status` permanece `"selected"`.

7. **Confirmar.** Reportar em 2 linhas: pastas processadas, racionais gerados, caminho do índice de conceitos. Não colar o índice no chat salvo se solicitado.

## Generalização — o que o índice é e não é

- **É:** camada leve de conceitos e pointers, permite ao Maestro rodar `find-prior-work` e `wayfinder` com contexto sem re-ler o SharePoint.
- **Não é:** substituto do SharePoint, memória canônica de decisão, nem base para citação em entregável de cliente sem re-verificação na fonte.

## Failure modes & atomicity

- Escrita per-doc é idempotente por `<doc-slug>.md`: reingerir o mesmo documento substitui o rationale anterior sem tocar nos vizinhos.
- `_index.md` só é escrito **após** todos os per-doc do pass terem sucesso. Não há `_index.md` parcial.
- Falha parcial (ex: doc 7 de 12 falha no `ingest-content`): os per-doc já escritos permanecem no lugar (retomáveis no próximo pass), `_index.md` não é escrito, e `sharepoint-config.json` recebe `ingest_status: "partial"` com `pending_docs: [<doc-slug>, ...]` para o próximo pass consumir.
- Pass completo com sucesso zera `ingest_status` de volta para `"complete"` e regenera `_index.md` do zero.
- Reentrância no mesmo `folder-slug`: pass novo assume o estado corrente da pasta remota, sobrescreve per-doc por slug e regenera o índice; não mescla índices antigos.

## Invariantes

- Nunca lê pasta fora de `folder_urls`.
- Nunca escreve fora de `data/memory/sharepoint-rationales/` e `brain/knowledge/sharepoint-rationales/`.
- Sem conector MCP presente, a skill não grava nada.
- Nenhuma chamada a provedor remoto além do próprio conector MCP autorizado pelo owner.

## Fora do escopo

- Descoberta ampla de SharePoint fora das `folder_urls` registradas.
- Ingestão de anexos de email, Teams messages, ou fontes que não sejam as pastas selecionadas.
- OCR de PDF escaneado — herda o mesmo limite do `ingest-content`.

## Encerramento

Uma linha com: pastas processadas, número de racionais, caminho do `_index.md` gerado. Se nada foi gravado, dizer explicitamente por que (config ausente, MCP ausente, owner cancelou) e qual é o próximo passo seguro.
