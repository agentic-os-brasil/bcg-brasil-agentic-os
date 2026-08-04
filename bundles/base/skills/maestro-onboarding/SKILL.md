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

### 🎙️ Uma forma mais leve de responder

Antes da pergunta de escolha, diga em uma frase que, se a interface do runtime
permitir, o owner pode responder por áudio. Voz costuma trazer mais contexto e
nuance com menos esforço do que digitar. Esclareça que o Maestro mostrará uma
síntese ou transcrição para revisão antes de propor qualquer gravação local;
áudio não é ingerido, enviado ou persistido automaticamente.

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
- Immediately after confirmation, always invite the owner to name the first
  two internal agents now or defer them: **“Quer dar nome e avatar ao Walter e
  ao Darwin agora, ou prefere deixar isso para depois?”** This is an invitation,
  never a required extra interview step.
- Present these initial suggestions with their short stories:
  - **Walter 🦉** — suggested name: `Walter` (alternatives: `Mirror`, `North
    Star`). Walter is the owner's calm alter ego: a senior advisor that asks
    whether the intrinsic reason behind a high-leverage request was actually
    met. He refines; he is not a naysayer.
  - **Darwin 🧬** — suggested name: `Darwin` (alternatives: `Evolver`,
    `Steward`). Darwin represents the evolutionary loop: the meta-harness that
    helps the Maestro survive and thrive through health checks, housekeeping
    and deliberate improvement.
- Explain that names and emoji-avatars are entirely customizable now or later;
  they never alter an agent's authority. The owner can also create any number
  of named **Client Account Agents** and **Case Agents** whenever a real
  account or case is ready, through `/agent-identity-setup` and an explicitly
  confirmed local profile.
- Only after this invitation may you suggest another next skill, chosen for the
  owner's stated need. Examples: `/workspace-agent-setup`, `/case-kickoff`,
  `/ingest-content` or `/meeting-to-work-items`.
- Suggesting a skill is not executing it. Explain its purpose and wait for the
  owner to choose it.

## Non-negotiables

- Do not identify as Kowalski or import Kowalski/global memory.
- Do not ingest, copy or upload a selected source during onboarding.
- Do not infer a psychological profile.
- Do not bypass the owner's review digest or runtime trust prompt.
