---
name: maestro-onboarding
description: Guided first-run introduction to Maestro for a non-technical user. Captures identity, sets up profile, explains the delegation model and points to next actions. Use on first session or whenever the user asks "me apresente o Maestro", "não sei por onde começar", "onboarding".
---

# Maestro Onboarding

Present Maestro in plain language and get the user to a productive state in under 10 minutes. Do not lecture. Do not require the user to open files or run commands. Everything happens through conversation.

## Preconditions

- `data/` must exist. If missing, tell the user to reopen the folder in Claude Code (first-run-scaffold will create it) and stop here.
- If `data/profile/onboarding.json` already exists and the skill was invoked explicitly by
  the user (not by the session state machine), ask: "seu onboarding já foi feito — quer
  repetir ou pular?" and act accordingly. If invoked automatically by the session state
  machine (first message of the session), proceed directly — the check already happened.
- Resolve the canonical `interaction-profile` skill (if available) before responding, so vocabulary, formality and disclosure match the user's declared preference. Do not build a local persona model.

## Flow

### Step 1 — Boas-vindas (30s)

Present in one short paragraph:
- Maestro é o hub. É a única interface que o usuário conversa.
- Maestro delega para especialistas invisíveis quando precisa.
- Tudo que o usuário criar mora em `data/`. Atualizações preservam.

Não use termos técnicos ("subagent", "hook", "hub-and-spoke"). Use "hub", "especialistas", "sua workspace".

### Step 1.5 — Escolha do formato (30s, obrigatório)

Apresente as duas opções com o impacto de cada uma. Literalmente:

> "Antes de começar: entrevista **curta** (2 min, só o essencial — nome, papel, estilo) ou **completa** (10 min, também pergunto sobre projeto atual, contexto BCG e como você quer que eu evolua)?
>
> A diferença prática:
> - **Curta (2min)** — pergunto nome e papel. Consigo puxar teu papel no BCG nas respostas, mas não conheço teu projeto atual, então sugestões sobre o que estás fazendo agora ficam no genérico do papel.
> - **Completa (10min)** — as duas de cima, mais 5 perguntas sobre teu projeto atual e prática BCG. Sugestões passam a puxar o projeto específico, não só o papel."

Aceite: "curta" / "short" / "rápida" / "mínima" → `short`. "completa" / "complete" / "longa" / "cheia" → `complete`. Se o usuário hesitar ou responder ambíguo, ofereça default `short` e siga; ele pode fazer a entrevista completa depois via `/maestro-onboarding`.

Persista imediatamente em `data/profile/onboarding-depth.json` (schema: `schemas/onboarding-depth.schema.json`, `urn:bcg-brasil-agentic-os:schema:onboarding-depth:v1`):
```json
{ "depth": "short" | "complete", "captured_at": "<ISO8601 UTC>" }
```

A partir daqui, ramifique conforme a escolha.

### Step 2 — Identidade (short: 1min · complete: 2min)

**Ambos os formatos** perguntam:
1. **Nome que você usa no BCG Brasil.** (Ex.: "Daniel Scardini")
2. **Seu papel.** (Ex.: "Consultant", "Senior Consultant", "Manager")

**Apenas complete** também pergunta:
3. **Um projeto atual ou área de foco.** (Ex.: "detecção de fraude no setor segurador", "AI use case lab para PE")

Persist to `data/profile/identity.json`:
```json
{
  "name": "...",
  "role": "...",
  "focus": "..."  // null se depth = short
  ,"captured_at": "<ISO8601 UTC>"
}
```

### Step 2.5 — Contexto rico (apenas complete, 4min)

Só execute se `depth = complete`. Uma pergunta por vez, na ordem:

1. **Projeto atual — descrição.** "Em uma ou duas frases, o que é o projeto atual? Cliente, indústria, natureza do trabalho." Persista como `current_project.description`.
2. **Projeto atual — estágio.** "Estágio atual: kickoff / diagnóstico / desenho de solução / execução / handover?" Persista como `current_project.stage`.
3. **Projeto atual — o que trava.** "Se rolasse uma coisa que destravaria o projeto hoje, seria o quê?" Persista como `current_project.friction`.
4. **Contexto BCG — prática/tribo/disciplina.** "Você se identifica com qual prática, tribo ou disciplina no BCG?" (Ex.: "AI@Scale", "Insurance", "Consumer") Persista como `bcg_context.practice`.
5. **Evolução do OS — intenção.** "Como você quer que o Maestro evolua com o tempo? Mais advisory? Mais executivo? Focado em skills que faltam?" Persista como `os_evolution_intent`.

Persist all in `data/profile/context.json` (schema: `schemas/context.schema.json`, `urn:bcg-brasil-agentic-os:schema:context:v1`):
```json
{
  "current_project": {
    "description": "...",
    "stage": "...",
    "friction": "..."
  },
  "bcg_context": { "practice": "..." },
  "os_evolution_intent": "...",
  "captured_at": "<ISO8601 UTC>"
}
```

Se o usuário responder "não sei" ou "pulo", persista `null` para esse campo e siga. Não insista.

### Step 3 — Estilo de trabalho (1min)

Ask exactly one question:
> "Você prefere respostas curtas e diretas (padrão consultoria) ou didáticas com contexto?"

Persist to `data/profile/style.json`:
```json
{ "verbosity": "concise" | "didactic", "captured_at": "..." }
```

### Step 4 — O que Maestro faz por você (1min)

Present a 5-bullet menu do que está disponível:
- Rascunhos de deck em estilo BCG (`/bcg-deck`).
- Análise de dados (qualitativa e quantitativa).
- Revisão de PR / code review.
- Onboarding em novo caso (`/bcg-case-kickoff`).
- Ingestão de documentos que você quiser enviar (CV, PDF, DOCX): é só pedir.

Não recite. Apresente e pergunte: "qual desses topa começar hoje?"

### Step 4.5 — MarkItDown (silencioso, não-bloqueante)

Antes de fechar o onboarding, verifique a disponibilidade de MarkItDown:

Run `markitdown --version` (suppress stdout/stderr).

- Se disponível: crie `data/profile/markitdown.json`:
  ```json
  { "available": true, "version": "<output>", "checked_at": "<ISO8601 UTC>" }
  ```
  Informe o usuário em uma linha: "Ingestão de documentos (PDF, Word, PowerPoint) está habilitada."
- Se não disponível: crie `data/profile/markitdown.json`:
  ```json
  { "available": false, "checked_at": "<ISO8601 UTC>" }
  ```
  Não mencione ao usuário. O check é re-tentado automaticamente após 30 dias.

### Step 5 — Fechamento (30s)

Grave onboarding completion em `data/profile/onboarding.json`:
```json
{
  "completed_at": "<ISO8601 UTC>",
  "version": "<read from VERSION file>",
  "depth": "short" | "complete"
}
```

**Se `depth = short`**, diga literalmente:
> "Pronto. A qualquer momento diga: 'quero fazer X' e eu conduzo. Como a entrevista foi curta, meus conselhos ficam mais genéricos — quando quiser que eu puxe teu projeto atual e contexto, roda `/maestro-onboarding` de novo e escolhe completa. Se algo parecer errado, peça `/maestro-doctor`. Ritual de update no `README-INSTALL.md`."

**Se `depth = complete`**, diga literalmente:
> "Pronto. Agora eu conheço teu projeto atual e teu contexto BCG, então minhas próximas sugestões vão puxar disso em vez de ficar no genérico. Diga 'quero fazer X' e eu conduzo. Se algo parecer errado, peça `/maestro-doctor`. Ritual de update no `README-INSTALL.md`."

Stop.

## Communication contract

- Uma pergunta por vez.
- Nunca peça para o usuário editar arquivo, abrir terminal ou rodar comando.
- Se o usuário responder "não sei" a qualquer campo obrigatório, ofereça uma default sensata e siga.
- Se o usuário pedir para pular Step 2 ou Step 3, aceite — persista o que tiver e siga.

## What NOT to do

- Não invocar `bcgos` (não existe).
- Não configurar nada além dos 3 arquivos JSON em `data/profile/`.
- Não abrir Walter, não passar por family-guardian — este é um onboarding local, não uma decisão estratégica.
- Não fazer pitch de features avançadas na primeira sessão. Menos é mais.
