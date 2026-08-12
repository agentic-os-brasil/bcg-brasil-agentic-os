---
name: find-prior-work
description: Recover explicitly requested prior professional material (past decks, documents, project artifacts) that already lives inside the Maestro workspace. Use only when the user explicitly asks to find a past deliverable or artifact. Not for general research, browsing or speculative context gathering.
---

# Find Prior Work

Skill de recuperação de material profissional prévio dentro da workspace Maestro. Ativa apenas quando o pedido é explícito ("acha o deck que fiz para o cliente X", "onde ficou aquele documento de kickoff do projeto Y", "recupera o material anterior sobre Z"). Não ativar para pesquisa geral, exploração de contexto ou dúvidas sobre SharePoint.

## Contrato de comunicação

- Sem "você", "tu" ou "te". Preferir impessoal ou 3ª pessoa.
- Sem em-dash em texto externo. Vírgula, dois pontos ou parênteses.
- Nunca pedir terminal, script, edição de JSON ou configuração de credenciais.
- Nunca mencionar `bcgos` nem binário de instalador.

## Interaction profile

Resolver `interaction-profile` se presente. Ajusta vocabulário e profundidade, nunca as regras de escopo abaixo.

## Escopo desta release

O Maestro atual (distribuição ZIP) indexa apenas o que já está local na workspace do usuário, sob `${CLAUDE_PROJECT_DIR}/data/`. Fontes cobertas:

- `data/memory/` — memória de longo prazo do Maestro (L1/L2/L3/lifetime) com decisões, contexto de projeto e sínteses de sessão.
- `data/workspaces/` — projetos ativos, notas, drafts e artefatos que o usuário mantém dentro da workspace.
- `data/agents/` — estado e histórico dos agentes (relevante para recuperar deliverables que passaram por um agent específico).

**Fora de escopo nesta release:** varredura automática de SharePoint, OneDrive corporativo, Teams, email ou qualquer fonte remota. A integração nativa com SharePoint entra em uma release futura do Maestro. Até lá, se o material está no SharePoint e não na workspace local, a skill orienta o usuário a trazer o arquivo para dentro de `data/workspaces/` (por exemplo, sincronizando pelo OneDrive local ou salvando uma cópia manual) e reativar a busca.

## Workflow

1. **Extrair intenção da pergunta.** Identificar sinais explícitos: cliente, projeto, tema, ano, tipo de artefato (deck, doc, análise, brief). Se o pedido for vago demais, fazer uma única pergunta de esclarecimento antes de buscar.

2. **Buscar dentro de `data/`.** Usar Grep sobre `${CLAUDE_PROJECT_DIR}/data/memory/**`, `${CLAUDE_PROJECT_DIR}/data/workspaces/**` e `${CLAUDE_PROJECT_DIR}/data/agents/**` com os termos identificados. Combinar busca por nome de arquivo (Glob) e por conteúdo (Grep). Priorizar matches em títulos, frontmatter e primeiras linhas.

3. **Ranquear resultados.** Ordenar por: (a) match direto no termo mais específico do pedido, (b) freshness (mtime mais recente), (c) camada de memória (lifetime > L3 > L2 > L1 quando aplicável).

4. **Ler apenas o necessário.** Para cada candidato do topo do ranking, ler as primeiras seções via Read para confirmar relevância. Não abrir arquivo grande inteiro sem necessidade.

5. **Devolver ponteiros, não conteúdo bruto.** Para cada item retornado, informar: título ou identificador, caminho absoluto dentro de `data/`, uma linha de descrição (client / project / theme / year quando presente), data da última modificação. Não colar o conteúdo do deck ou documento no chat, apenas resumir e apontar.

## Regras de recuperação

- Só ativar sob pedido explícito de prior work. Não usar como busca geral.
- Restringir a busca à `data/` da workspace atual. Nunca ler fora dessa raiz.
- Tratar títulos, nomes de cliente, nomes de projeto e caminhos como metadata restrito local. Não replicar em resposta mais do que o necessário para o usuário identificar o item.
- Retornar até 5 candidatos ranqueados. Se houver muito mais, pedir um termo adicional que estreite (cliente, ano, tema).
- Se a busca vier vazia, dizer isso direto, sem inventar plausíveis. Sugerir termos alternativos apenas se houver base concreta (variações de nome já vistas na workspace).

## Quando o material só existe fora da workspace

Se o usuário confirma que o material existe mas está no SharePoint, OneDrive corporativo ou email:

1. Explicar em uma frase que a integração com fontes remotas está prevista para uma release futura do Maestro.
2. Orientar o caminho manual desta release: salvar ou sincronizar uma cópia do arquivo dentro de `${CLAUDE_PROJECT_DIR}/data/workspaces/<projeto>/` (por exemplo, arrastando pelo Finder a partir da pasta local do OneDrive) e pedir a busca de novo.
3. Não abrir browser, não pedir credencial, não tentar conector alternativo, não sugerir workaround técnico.

## O que esta skill nunca faz

- Não busca em fontes remotas (SharePoint, OneDrive corporativo, Teams, Outlook, Graph).
- Não pede credencial, token, cookie ou configuração de conector.
- Não escreve, move ou apaga nada dentro de `data/`. É read-only.
- Não retorna resultados quando o pedido não é explicitamente prior work.
- Não replica o conteúdo completo de decks ou documentos no chat; retorna ponteiros e uma linha de descrição.
