---
name: maestro-onboarding
description: Guided first-run introduction to Maestro for a non-technical user. Captures identity, sets up profile, explains the delegation model and points to next actions. Use on first session or whenever the user asks "me apresente o Maestro", "não sei por onde começar", "onboarding".
---

# Maestro Onboarding

Present Maestro in plain language and get the user to a productive state in under 10 minutes. Do not lecture. Do not require the user to open files or run commands. Everything happens through conversation.

## Preconditions

- `data/` must exist. If missing, create directories inline (data/, data/profile/) and proceed.
- If `data/profile/onboarding.json` already exists and the skill was invoked explicitly by
  the user (not by the session state machine), ask: "seu onboarding já foi feito — quer
  repetir ou pular?" and act accordingly. If invoked automatically by the session state
  machine (first message of the session), proceed directly — the check already happened.

## Flow

### Step 1 — Boas-vindas (30s)

Present in one short paragraph:
- Maestro é o hub. É a única interface que o usuário conversa.
- Maestro delega para especialistas invisíveis quando precisa.
- Tudo que o usuário criar mora em `data/`. Atualizações preservam.

Não use termos técnicos ("subagent", "hook", "hub-and-spoke"). Use "hub", "especialistas", "sua workspace".

### Step 2 — Escolha da trilha (1min)

Logo após as boas-vindas, apresente as duas trilhas com o impacto de cada uma:

> "Para configurar o Maestro, há duas formas:
>
> **Trilha rápida (~3 min)** — nome, papel e estilo de resposta. Começamos a trabalhar imediatamente. O Maestro se calibra com o tempo conforme trabalhamos juntos — mas nas primeiras sessões pode precisar pedir contexto que a trilha completa já teria.
>
> **Trilha completa (~8 min)** — além do básico, capturo seu projeto atual, o que é importante para você profissionalmente e o que consome mais energia agora. Com isso, sugestões e análises são personalizadas desde a primeira sessão: menos perguntas de clarificação, próximos passos mais alinhados ao que importa, e o Maestro já sabe puxar os temas certos ao longo do tempo.
>
> Qual prefere?"

- Se **trilha rápida**: execute Step 3 (só Nome e Papel) → Step 4 → Step 5 (resumido) → Step 5.5 → Step 6.
- Se **trilha completa**: execute Step 3 → Step 3.5 → Step 4 → Step 5 → Step 5.5 → Step 6.

### Step 3 — Identidade (1–2min)

Ask, in order:
1. **Nome que usa no BCG Brasil.** (Ex.: "Daniel Scardini")
2. **Papel.** (Ex.: "Consultant", "Senior Consultant", "Manager")

Trilha completa: continue para Step 3.5 antes de persistir.
Trilha rápida: persista agora e siga para Step 4.

Persist to `data/profile/identity.json` with schema:
```json
{
  "name": "...",
  "role": "...",
  "focus": "...",
  "work_energy": "...",
  "quality_bar": "...",
  "track": "quick" | "complete",
  "captured_at": "<ISO8601 UTC>"
}
```

Campos `focus`, `work_energy`, `quality_bar` ficam em branco na trilha rápida.

### Step 3.5 — Contexto profissional (completo apenas, 3–4min)

Ask in order, one at a time:
3. **Projeto atual ou área de foco.** (Ex.: "detecção de fraude no setor segurador", "AI use case lab para PE")
4. **O que define trabalho bem feito para você neste projeto?** (Ex.: "análise que convence o board", "código que sobrevive sem o autor", "entrega dentro do prazo com zero retrabalho")
5. **O que mais consome energia agora?** (Ex.: "preparar apresentações", "alinhar com o cliente", "revisar código dos outros")

Persist ao `data/profile/identity.json` os campos `focus`, `quality_bar` e `work_energy`.

### Step 4 — Estilo de trabalho (1min)

Ask exactly one question:
> "Como prefere que eu trabalhe: **padrão** (respostas curtas, direto ao ponto), **avançado** (mais contexto e nuance quando útil), ou **power** (assume familiaridade total, máxima densidade)?"

Mapeie a resposta para um dos três valores canônicos: `standard`, `advanced`, `power`.

Persist to `data/profile/style.json`:
```json
{ "interaction_profile": "standard" | "advanced" | "power", "captured_at": "..." }
```

Este campo é lido pelo skill canônico `interaction-profile` como preflight de todas as demais skills. Não use outras chaves (`verbosity`, `mode`, etc.).

### Step 5 — O que Maestro faz por você (1min)

Present o que está disponível. **Trilha rápida**: mencione apenas 3 itens principais. **Trilha completa**: apresente os 5.

- Rascunhos de deck em estilo BCG.
- Análise de dados (qualitativa e quantitativa).
- Revisão de código e PR.
- Onboarding em novo caso de cliente.
- Ingestão de documentos (CV, PDF, DOCX): é só pedir.

**Trilha completa — adicione**: "A cada sessão, proponho próximos passos ligados ao seu projeto, ao seu desenvolvimento profissional e à saúde do Maestro — para você nunca precisar lembrar o que perguntar."

Pergunte: "qual desses topa começar hoje?"

### Step 5.5 — MarkItDown (silencioso, não-bloqueante)

Run `markitdown --version` silently (suppress stdout/stderr).

- Disponível → crie `data/profile/markitdown.json`:
  ```json
  { "available": true, "version": "<output>", "checked_at": "<ISO8601 UTC>" }
  ```
  Informe em uma linha: "Ingestão de documentos (PDF, Word, PowerPoint) está habilitada."
- Não disponível → crie `data/profile/markitdown.json`:
  ```json
  { "available": false, "checked_at": "<ISO8601 UTC>" }
  ```
  Não mencione ao usuário.

### Step 6 — Fechamento (30s)

Grave em `data/profile/onboarding.json`:
```json
{
  "completed_at": "<ISO8601 UTC>",
  "track": "quick" | "complete",
  "version": "<read from VERSION file>"
}
```

Diga:
> "Pronto. A qualquer momento diga: 'quero fazer X' e eu conduzo. Se algo parecer errado, leia `skills/maestro-doctor/SKILL.md` para diagnóstico. Se sair uma versão nova, o ritual de update está no `README-INSTALL.md` da sua pasta."

Stop.

## Communication contract

- Uma pergunta por vez.
- Nunca peça para o usuário editar arquivo, abrir terminal ou rodar comando.
- Se o usuário responder "não sei" a qualquer campo obrigatório, ofereça uma default sensata e siga.
- Se o usuário pedir para pular qualquer step, aceite — persista o que tiver e siga.
- Se o usuário escolher trilha rápida mas depois quiser completar, aceite — retome do Step 3.5.

## What NOT to do

- Não invocar skills por nome de tool — leia o SKILL.md correspondente diretamente.
- Não configurar nada além dos arquivos JSON em `data/profile/`.
- Não fazer pitch de features avançadas antes do Step 5.
- Não apresentar as duas trilhas como "mais fácil vs mais difícil" — são diferentes em **impacto**, não em esforço.
