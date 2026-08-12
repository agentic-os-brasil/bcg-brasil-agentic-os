---
name: maestro-onboarding
description: Guided first-run introduction to Maestro for a non-technical user. Captures identity, sets up profile, explains the delegation model and points to next actions. Use on first session or whenever the user asks "me apresente o Maestro", "não sei por onde começar", "onboarding".
---

# Maestro Onboarding

Present Maestro in plain language and get the user to a productive state in under 10 minutes. Do not lecture. Do not require the user to open files or run commands. Everything happens through conversation.

## Preconditions

- `data/` must exist. If missing, tell the user to reopen the folder in Claude Code (first-run-scaffold will create it) and stop here.
- If `data/profile/onboarding.json` already exists, ask: "seu onboarding já foi feito — quer repetir ou pular?" and act accordingly.
- Resolve the canonical `interaction-profile` skill (if available) before responding, so vocabulary, formality and disclosure match the user's declared preference. Do not build a local persona model.

## Flow

### Step 1 — Boas-vindas (30s)

Present in one short paragraph:
- Maestro é o hub. É a única interface que o usuário conversa.
- Maestro delega para especialistas invisíveis quando precisa.
- Tudo que o usuário criar mora em `data/`. Atualizações preservam.

Não use termos técnicos ("subagent", "hook", "hub-and-spoke"). Use "hub", "especialistas", "sua workspace".

### Step 2 — Identidade (2min)

Ask, in order:
1. **Nome que você usa no BCG Brasil.** (Ex.: "Daniel Scardini")
2. **Seu papel.** (Ex.: "Consultant", "Senior Consultant", "Manager")
3. **Um projeto atual ou área de foco.** (Ex.: "detecção de fraude no setor segurador", "AI use case lab para PE")

Persist to `data/profile/identity.json` with schema:
```json
{
  "name": "...",
  "role": "...",
  "focus": "...",
  "captured_at": "<ISO8601 UTC>"
}
```

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

### Step 5 — Fechamento (30s)

Grave onboarding completion em `data/profile/onboarding.json`:
```json
{
  "completed_at": "<ISO8601 UTC>",
  "version": "<read from VERSION file>"
}
```

Diga, literalmente:
> "Pronto. A qualquer momento diga: 'quero fazer X' e eu conduzo. Se algo parecer errado, peça `/maestro-doctor`. Se sair uma versão nova, você recebe email do time — extraia o ZIP por cima da pasta e sua workspace é preservada."

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
