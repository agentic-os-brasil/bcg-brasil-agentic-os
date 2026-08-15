---
name: maestro-knowledge-pill
description: Gera uma pílula de conhecimento curta e engajante sobre o Maestro OS (skills, hooks, agentes, memória, boas práticas) em formato Slack Block Kit para postar em canais internos. Use para "pílula de conhecimento", "cria pílula do Maestro", "knowledge pill", "gera post pro slack sobre Maestro" ou pedidos equivalentes. Retorna JSON Block Kit + preview em markdown; não posta em canal, não inventa skills que não existem no catálogo.
---

# Maestro Knowledge Pill

Gera **uma** mensagem curta pra Slack apresentando uma boa prática, conceito ou skill do Maestro OS. Formato fixo, engajante, sem inventar recursos que não existem.

## Contrato de saída

Duas coisas, nesta ordem:

1. **JSON Block Kit** — pronto pra colar no [Block Kit Builder](https://app.slack.com/block-kit-builder/) ou postar via API.
2. **Preview em markdown** — pra o owner conferir o texto antes de mandar pro canal.

## Formato locked

Estrutura obrigatória — não alterar sem instrução explícita.

### Header
- Tipo `header`, texto `plain_text`, `emoji: true`.
- **Bookends 💊 no título — e só 💊.** Nunca `:maestro:`, nunca outro emoji nos bookends.
- Título ≤ 60 caracteres (contando os dois 💊 e os espaços).
- Formato: `💊 <título curto e afirmativo> 💊`.

### Body
- Tipo `section`, texto `mrkdwn`.
- **Tudo em itálico** — envolver cada frase e cada bullet com `_..._`.
- ≤ 900 caracteres no total do body.
- Emojis temáticos **dentro** do texto, 1 por parágrafo/bullet, com moderação. Escolher emojis que ancoram o conceito (🎯 gatilho, ⚡ velocidade, 🌙 fim de dia, 🚀 kickoff, 📊 deck, 🧠 memória, 🔒 gate, 🪝 hook, 🤖 agente, 📖 index, 🧭 wayfinder, etc.).
- Bullets em `•` (não `-`, não `*`).
- Nomes de skills, arquivos e comandos entre backticks — dentro do itálico: `_\`skill-name\`_`.

### Footer
- Tipo `context`, elemento `mrkdwn`.
- Formato exato: `:maestro: _Pílula #NNN · Maestro OS_ :maestro:`
- `NNN` = número sequencial de 3 dígitos (001, 002…). Se o owner não indicar número, perguntar ou usar `NNN` como placeholder e alertar.
- **`:maestro:` só no footer.** Nunca no header, nunca no body.

### Template JSON
```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "💊 <TÍTULO> 💊",
        "emoji": true
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "<EMOJI> _<abertura em uma frase>._\n\n<EMOJI> _<parágrafo ou bullet 1>_\n\n• _<bullet 2>_ <EMOJI>\n• _<bullet 3>_ <EMOJI>\n\n<EMOJI> _<fechamento com pointer para arquivo/comando real>._"
      }
    },
    {
      "type": "context",
      "elements": [
        {
          "type": "mrkdwn",
          "text": ":maestro: _Pílula #NNN · Maestro OS_ :maestro:"
        }
      ]
    }
  ]
}
```

## Anti-alucinação — catálogo verificado

**Regra dura:** só citar skills, hooks, agentes, arquivos e comandos que existem no repo agentic-os. Se em dúvida, checar com `ls bundles/base/skills/` e ler o `SKILL.md` antes de citar.

### Skills reais (bundles/base/skills/) — 41 skills
Cada linha: `nome · gatilho canônico curto`.

- `account-case-setup` · monta conta + Case Agent do projeto
- `agent-identity-setup` · nomeia e personaliza agentes governados
- `bcg-case-kickoff` · plano dos primeiros dias de um caso ("kickoff de caso novo")
- `bcg-deck` · storyline e plano de deck decision-led
- `case-agent-setup` · interview + pesquisa pública de um case
- `case-canon-ingest` · compila insight revisado no canon do case
- `case-decision-log-entry` · registra decisão estrutural no log do case
- `client-delivery-gate` · gate de 3 lentes antes de mandar pro cliente
- `craft-update` · documenta método ou preferência do owner
- `deck-drill` · ensaia o deck contra perguntas do público
- `deck-review` · revisa storyline e evidência dos slides ("revisa esse deck")
- `dream-memory` · consolida memória diária/semanal ("fecha o dia", "dreaming")
- `eod` · fecha o dia útil ("eod", "fechando o dia")
- `execution-continuity` · checkpoint e retomada entre sessões
- `expert-interview-guide` · guia de entrevista com expert/stakeholder
- `feedback-capture` · captura feedback recebido → objetivos
- `find-prior-work` · recupera deliverable anterior no workspace
- `fodais-performance-review` · CDC de FoDAIS/Sr. FoDAIS BCG X
- `ingest-content` · registra PDF/office/web na memória local
- `interaction-profile` · resolve perfil de interação do owner
- `investigate` · root cause de output errado ("por que isso está errado")
- `learnings-bridge` · promove learnings do daily pra conhecimento durável
- `maestro-doctor` · health check do install
- `maestro-environment-setup` · prepara workspace pós-install
- `maestro-onboarding` · interview inicial do owner
- `maestro-operator` · método operacional carregado no SessionStart
- `maestro-runtime-checkup` · repara runtime/hooks/Darwin
- `maestro-setup-update` · install / update / rollback via ZIP
- `meeting-close` · fecha reunião em packet reviewable
- `meeting-to-work-items` · extrai decisões e tasks das notas
- `qa-gate` · classifica qualidade de mudança com evidência
- `qualitative-analysis` · sintetiza evidência qualitativa
- `quantitative-analysis` · analisa evidência quantitativa
- `retro` · retrospectiva semanal contra objetivos
- `sharepoint-ingest` · ingere pastas SharePoint autorizadas
- `slide-summary` · mapeia texto de deck em arco narrativo
- `start-day` · abre o dia útil com briefing ("bom dia")
- `upward-feedback` · prepara feedback pra sênior
- `yoda` · pressure-test interno antes do owner (persona: Yoda 🧙 — Mestre Yoda; triggers: "yoda check")
- `wayfinder` · quebra problema fuzzy em issue tree
- `workspace-agent-setup` · alias legado do case-agent-setup

Se o owner pedir uma pílula sobre algo fora dessa lista (ou sobre hook/agente/arquivo), **verificar antes** com `ls`/`grep` no repo. Não presumir existência.

## Temas sugeridos (rotação)

Girar entre categorias pra não repetir tópico:

1. **Skill em ação** — 1 skill real, o gatilho e o "sem isso, o que dói".
2. **Fluxo do dia** — `start-day` → trabalho → `eod` → `dream-memory`.
3. **Governança de qualidade** — `yoda` (persona Yoda 🧙), `client-delivery-gate`, `qa-gate`.
4. **Memória viva** — `case-canon-ingest`, `ingest-content`, `learnings-bridge`, `dream-memory`.
5. **Case lifecycle** — `account-case-setup` → `case-agent-setup` → `bcg-case-kickoff` → `bcg-deck` → `deck-review` → `deck-drill`.
6. **Meta-conceito** — o INDEX.md como mapa; SessionStart do `maestro-operator`; interaction-profile; separação owner/case/account.
7. **Boas práticas de owner** — nunca postar sem `client-delivery-gate`; usar `investigate` antes de "consertar"; `yoda` (Yoda 🧙) antes de output alto risco.

### Nota de persona — Yoda 🧙
Yoda é a persona canônica em todo o OS — Mestre Yoda de Star Wars, calmo, denso, sem teatro. Pílulas usam "Yoda" no texto humano e `\`yoda\`` só quando referenciam a skill/agente tecnicamente. Doc canônica: `docs/personas.md`.

## Passos de geração

1. **Escolher tema** — se o owner não indicou, rotacionar a partir da categoria menos usada nas últimas pílulas conhecidas.
2. **Escrever título** — afirmativo, curto, com verbo forte. Ex: "Skills atendem no gatilho certo", "Fecha o dia, ganha o próximo", "Yoda mira o que não pode voltar".
3. **Redigir body** — abertura (1 frase) + 2–3 bullets ou 2 parágrafos curtos + fechamento com pointer real (`bundles/base/skills/INDEX.md`, `maestro-doctor`, etc.).
4. **Checar catálogo** — cada skill/arquivo citado existe? Se não, remover ou substituir.
5. **Aplicar formato** — 💊 nos bookends do título, itálico em tudo no body, `:maestro:` só no footer, número da pílula preenchido ou marcado como placeholder.
6. **Validar limites** — título ≤ 60 chars, body ≤ 900 chars, 1 emoji por parágrafo/bullet.
7. **Entregar** — JSON Block Kit + preview em markdown, nessa ordem.

## Anti-padrões

- ❌ Citar skill que não está no catálogo acima.
- ❌ Usar `:maestro:` no título ou no body.
- ❌ Emoji diferente de 💊 nos bookends do título.
- ❌ Body sem itálico ou com itálico parcial.
- ❌ Mais de 1 emoji por bullet/parágrafo.
- ❌ Título terminando em ponto final.
- ❌ Pílula longa (≥ 900 chars no body) — pílula é curta.
- ❌ Postar em canal — a skill só gera, quem posta é o owner.
- ❌ Inventar número de pílula sem contexto.

## Exemplo de saída

```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "💊 Skills atendem no gatilho certo 💊",
        "emoji": true
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "🎯 _Toda skill do Maestro tem *triggers* — frases que fazem ela pular na frente automaticamente._\n\n⚡ _Não precisa lembrar do nome. Basta descrever o que quer:_\n\n• _`kickoff de caso novo` → dispara `bcg-case-kickoff`_ 🚀\n• _`revisa esse deck` → dispara `deck-review`_ 📊\n• _`fechar o dia` → dispara `eod`_ 🌙\n\n📖 _Ver todos: `bundles/base/skills/INDEX.md`._"
      }
    },
    {
      "type": "context",
      "elements": [
        {
          "type": "mrkdwn",
          "text": ":maestro: _Pílula #001 · Maestro OS_ :maestro:"
        }
      ]
    }
  ]
}
```
