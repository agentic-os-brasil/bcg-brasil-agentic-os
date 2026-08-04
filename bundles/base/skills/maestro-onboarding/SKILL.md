---
name: maestro-onboarding
description: Start Maestro's owner interview with an explicit quick or complete track, one question at a time, and a reviewed local profile.
---

# Maestro Onboarding

Run this skill when a newly installed Maestro workspace receives its first
guided-onboarding prompt. The goal is a useful, consented professional baseline
— not a long system explanation and not an unreviewed memory import.

## Before the first reply

1. Read `CLAUDE.md` and preserve the Maestro workspace identity.
2. Resolve the canonical `interaction-profile` before choosing language,
   explanation depth or optional technical detail. It does not choose the
   onboarding track, grant authority or change the review requirement.
3. Run `bcgos owner onboarding status` to inspect the deterministic local
   state. Do not infer that onboarding exists from files or prior messages.
4. Do not start a professional task, read a selected memory source, execute an
   unrelated skill or grant runtime trust globally.

## Opening response

Respond in Brazilian Portuguese with this compact, welcoming shape:

### 🎼 Bem-vindo ao Maestro

One sentence: Maestro is the owner's professional second brain for context,
execution and evidence in this local workspace.

### ✨ O que já está preparado

- A new local workspace, separate from existing projects.
- Initial areas for context, decisions, people and work.
- Local hooks and maintenance configuration; describe them as configured, not
  as observed native runtime behavior.

### 🧭 Escolha como quer começar

Present exactly two explicit options, then wait for a choice. Do not ask the
first identity question in the same message.

| Opção | Tempo estimado | O que estabelece | Implicação |
| --- | --- | --- | --- |
| **Curta** | **~7 minutos** | Papel profissional, estilo de colaboração e limites de trabalho | Você começa mais rápido, mas voz, preferências e regras de decisão serão refinadas em conversas futuras; as sugestões iniciais serão menos personalizadas. |
| **Completa** | **~25 minutos** | As seis facetas acima, incluindo voz, preferências e regras de decisão | Leva mais tempo agora, mas o Maestro começa com uma leitura mais fiel de como você trabalha e decide. |

Ask only: **“Você prefere a entrevista curta ou a completa?”**

## After the owner chooses

1. Confirm the exact selected track once and persist it only with:

   ```sh
   bcgos owner onboarding select --track quick|complete --confirm
   ```

2. Ask one interview question at a time. Use the next question returned by
   `bcgos owner onboarding status`; do not invent extra mandatory questions.
3. Before proposing any write to a facet, show the concise draft and obtain the
   owner's agreement. Never claim that an answer has been saved or that the
   track is complete until the local review is confirmed.
4. When the status becomes `review_required`, show the owner the profile
   facets that were included in the selected track. Ask for an explicit review,
   then use the exact digest returned by the status command:

   ```sh
   bcgos owner onboarding confirm --digest SHA256 --confirm
   ```

## Completion and follow-through

- A confirmed **quick** track is a valid baseline, not a claim that the full
  identity is known. Offer the complete track later only when it is useful;
  never nag or silently upgrade it.
- A confirmed **complete** track has the full initial professional baseline.
- Only after confirmation may you suggest a next skill, chosen for the owner's
  stated need. Examples: `/agent-identity-setup`, `/workspace-agent-setup`,
  `/case-kickoff`, `/ingest-content` or `/meeting-to-work-items`.
- Suggesting a skill is not executing it. Explain its purpose and wait for the
  owner to choose it.

## Non-negotiables

- Do not import prior persona, project or memory context that is outside this
  Maestro workspace. Keep the conversation focused on the owner's
  professional work.
- Do not ingest, copy or upload a selected source during onboarding.
- Do not infer a psychological profile.
- Do not bypass the owner's review digest or runtime trust prompt.
