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
> "Como prefere que eu trabalhe: **padrão** (respostas curtas, direto ao ponto), **avançado** (mais contexto e nuance quando útil), ou **power** (assume familiaridade total, máxima densidade)?"

Mapeie a resposta para um dos três valores canônicos: `standard`, `advanced`, `power`.

Persist to `data/profile/style.json`:
```json
{ "interaction_profile": "standard" | "advanced" | "power", "captured_at": "..." }
```

Este campo é lido pela skill `interaction-profile` como preflight de todas as demais skills. Não use outras chaves (`verbosity`, `mode`, etc.).

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
  "version": "<read from VERSION file>"
}
```

Diga, literalmente:
> "Pronto. A qualquer momento diga: 'quero fazer X' e eu conduzo. Se algo parecer errado, peça `/maestro-doctor`. Se sair uma versão nova, você recebe email do time — o ritual de update está no `README-INSTALL.md` da sua pasta."

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
