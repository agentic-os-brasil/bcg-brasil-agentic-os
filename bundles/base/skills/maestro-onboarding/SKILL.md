---
name: maestro-onboarding
description: Start Maestro's owner interview with an explicit quick or complete track, one question at a time, and a reviewed local profile.
---

# Maestro Onboarding

Run this skill when a newly installed Maestro workspace receives its first
guided-onboarding prompt. The goal is a useful, consented professional baseline
— not a long system explanation and not an unreviewed memory import.

## Resolve the Maestro CLI before any command

Never invoke a bare `bcgos` command. Desktop runtimes do not inherit the
owner's shell `PATH`, and a missing PATH entry must not be reported as a
permission or onboarding-state failure. Use the exact executable path emitted
by the Maestro `SessionStart` context and shown in the managed orientation's
**Comandos úteis** section. If that pointer is unavailable, use the platform
managed install location (`~/Library/Application Support/Maestro/bin/bcgos` on
macOS or `%LOCALAPPDATA%\\Maestro\\bin\\bcgos.exe` on Windows); use `PATH` only
as a final fallback. Call the resolved path as
`<maestro-cli>` in the commands below; this is a placeholder to substitute,
never a literal command to execute. If no executable can be resolved, stop and
report the concrete missing path; do not substitute another runtime or pretend
the command succeeded.

## Before the first reply

1. Read `CLAUDE.md` and preserve the Maestro workspace identity.
2. Resolve the canonical `interaction-profile` before choosing language,
   explanation depth or optional technical detail. It does not choose the
   onboarding track, grant authority or change the review requirement.
3. Run `<maestro-cli> owner onboarding status` to inspect the deterministic local
   state. Do not infer that onboarding exists from files or prior messages.
4. Do not start a professional task, read a selected memory source, execute an
   unrelated skill or grant runtime trust globally.

The canonical owner context is private to the Maestro installation's data root,
outside the workspace. The SessionStart directive prints that exact root and
the canonical `owner/self/` destination. Never create or edit `owner/` or
`owner/self/` inside the workspace: those files are not authoritative and will
not advance the CLI state. After the owner approves the concise reflection,
save it with the one-shot bounded command below. It writes the correct facet,
records the audit receipt and returns the next deterministic interview state:

```sh
<maestro-cli> owner onboarding answer --facet <facet-id> --body "<reviewed Markdown>" --confirm
```

Use `--stdin` instead of `--body` when the runtime can provide standard input.
The lower-level `owner refine submit/apply` pair remains available for more
complex or separately reviewed refinements.

The onboarding track and final profile confirmation remain separate gates. The
CLI registry, review digest and audit receipt are the source of truth; a
workspace-local Markdown file alone is never evidence of completion.

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
| **Curta** | **~7 minutos** | Papel profissional, comunicação, preferências de trabalho e qualidade/QA | Você começa mais rápido, mas voz externa, motivações, regras de decisão e limites serão refinados em conversas futuras; as sugestões iniciais serão menos personalizadas. |
| **Completa** | **~25 minutos** | As oito facetas profissionais, incluindo voz, preferências, motivações, qualidade/QA, regras de decisão e limites | Leva mais tempo agora, mas o Maestro começa com uma leitura mais fiel de como você trabalha, decide e define qualidade. |

Ask only: **“Você prefere a entrevista curta ou a completa?”**

### 🎙️ Uma forma mais leve de responder

Antes da pergunta de escolha, diga em uma frase que, se a interface do runtime
permitir, o owner pode responder por áudio. Voz costuma trazer mais contexto e
nuance com menos esforço do que digitar. Esclareça que o Maestro mostrará uma
síntese ou transcrição para revisão antes de propor qualquer gravação local;
áudio não é ingerido, enviado ou persistido automaticamente.

## What the interview is calibrating

The interview is a guided construction of the owner's **professional self** —
not a personality test and not a request to import another system's private
memory. The complete track covers eight explicit, reviewable facets:

- `professional-role`: the work the owner is accountable for and where Maestro
  should create leverage;
- `communication-style`: how the owner wants reasoning, detail, language and
  recommendations presented;
- `voice`: how the owner's external work should sound;
- `preferences`: tools, formats, rhythms and collaboration habits;
- `motivations`: the professional impact and outcomes that make work matter;
- `quality-bar`: what must be checked before something is called ready,
  including QA, evidence and finish level;
- `decision-rules`: principles, trade-offs and decisions that remain with the
  owner;
- `working-boundaries`: scope, confidentiality, sources, people and external
  communication that require authorization.

The quick track covers `professional-role`, `communication-style`,
`preferences` and `quality-bar`. It is a useful operating baseline, but it
intentionally leaves external voice, motivations, decision rules and working
boundaries for later refinement. Psychological/personality material, personal
history, faith, assessments and visual identity are not inferred or imported by
either track; they require a separate, explicit local consent path.

## Sugestão técnica orientada pela função

Depois que o owner responder qual é sua função, use a recomendação determinística
do runtime:

```sh
bcgos bundles recommend --function "<resposta declarada pelo owner>"
```

Se o resultado for `recommended`, explique que engineering, data ou AI foram
identificados somente na resposta declarada e pergunte se a pessoa quer incluir
o bundle opcional `tech-core`. Se o resultado for `ask`, faça a mesma pergunta
sem presumir que a função é técnica. Nunca ative o bundle automaticamente: a
seleção de uma trilha técnica e a confirmação do owner continuam sendo a única
forma de projetar as skills. O `tech-core` é um bundle único e inclui engineering,
data, AI e métodos de qualidade.

## Camadas opcionais de identidade

The first interview must not pretend that a professional baseline is the whole
person. After the selected track is reviewed, offer (do not start automatically)
these optional layers when they are useful:

- **Propósito e não negociáveis** — values, long-term direction and personal
  constraints that the owner explicitly wants the professional system to
  respect. Keep this private and out of client/case packets by default.
- **Contexto pessoal autorizado** — only the minimum family, faith or life
  context the owner deliberately chooses to share, with a declared purpose and
  reader scope. It is never required for ordinary professional work.
- **Personalidade ou avaliação** — a local owner-authored synthesis or an
  explicitly selected assessment source. Never diagnose, infer or turn a score
  into an agent rule; a source that cannot be reviewed remains unavailable.
- **Identidade visual** — colors, references and presentation preferences for
  owner-facing artifacts only. It changes presentation, never authority or
  routing.

For every optional layer, ask for the purpose, source, allowed readers, retention
and explicit confirmation before writing. If the runtime has no qualified local
adapter for the chosen layer, report `unavailable` and continue with the
professional baseline; do not emulate ingestion from conversation.

## After the owner chooses

1. Confirm the exact selected track once and persist it only with:

   ```sh
   <maestro-cli> owner onboarding select --track quick|complete --confirm
   ```

2. Ask one interview question at a time. Use the next question returned by
   `<maestro-cli> owner onboarding status`; do not invent extra mandatory questions.
3. After each answer, reflect back a concise interpretation and ask whether it
   is accurate. Only then propose the corresponding facet draft. This is the
   quality loop for onboarding: the owner corrects meaning before anything is
   written.
4. Before proposing any write to a facet, show the concise draft and obtain the
   owner's agreement. Never claim that an answer has been saved or that the
   track is complete until the local review is confirmed.
5. When the status becomes `review_required`, show the owner the profile
   facets that were included in the selected track. Ask for an explicit review,
   then use the exact digest returned by the status command:

   ```sh
   <maestro-cli> owner onboarding confirm --digest SHA256 --confirm
   ```

   Never try to manufacture this digest with `shasum`, `cat | openssl`, `awk`
   or shell command substitution. Those commands are intentionally outside the
   bounded hook grammar. If the digest is missing or stale, run
   `<maestro-cli> owner onboarding status` again and use its current
   `review_digest`. `owner onboarding review` is accepted as a read-only alias
   for `status` when a runtime presents that wording.

## Completion and follow-through

- A confirmed **quick** track is a valid baseline, not a claim that the full
  identity is known. Offer the complete track later only when it is useful;
  never nag or silently upgrade it.
- A confirmed **complete** track has the full initial professional baseline.
- The workspace bootstrap is always first. Never ask for, resolve or ingest a
  SharePoint source before the new workspace has been initialized and the
  Maestro session is running inside that workspace. The source question below
  is deliberately a post-bootstrap onboarding step because all derived
  content must be read and organized from within the workspace.
- After either track is confirmed, inspect the deterministic project-source
  state with `<maestro-cli> prior-work source status --workspace <workspace>`. If it is
  `selection_required`, ask exactly one question and wait: **“Você quer indicar
  as pastas autorizadas do SharePoint deste projeto agora ou prefere começar
  sem essa fonte?”**
  - If the owner chooses SharePoint, make the two-stage contract explicit:
    selecting folders records the exact scope, but **does not yet authorize a
    read**. Review the canonical folder URLs with the owner, then send strict
    JSON (`schema_version: 1`, `folder_urls`) through standard input to
    `<maestro-cli> prior-work source select --workspace <workspace> --stdin --confirm`.
    Immediately after selection, ask whether the owner authorizes a bounded
    recent-material pass: **“Posso ler os materiais mais recentes dessas
    pastas e criar racionais internos rastreáveis no workspace?”** Explain
    plainly that the pass reads only the selected folders through the qualified
    Claude collector, writes concise derived racionais under
    `brain/knowledge/sharepoint-rationales/`, keeps the SharePoint link and
    modification date on every rationale, and never copies the raw document
    body. If the owner authorizes it, run the explicit rationale-ingestion
    command only when signed enrollment and the qualified local ingestion
    runtime are available:
    `<maestro-cli> prior-work rationale ingest --workspace <workspace> --stdin --confirm`.
    The batch is deterministic: newest source modifications first, then stable
    item reference as tie-breaker. If the collector/runtime is unavailable,
    report that honestly and leave the source selected but not ingested.
  - If the owner prefers to start clean, record the choice with
    `<maestro-cli> prior-work source defer --workspace <workspace> --confirm` and do
    not ask again automatically.
  - A selection is not enrollment or collection authority. SharePoint remains
    authoritative; only a signed enrollment plus a qualified Claude collector
    can read the selected roots and produce the bounded rationale batch. Codex
    collection remains `unavailable/corporate_policy` and no fallback is
    allowed. The local rationale layer is a derived convenience, never a
    replacement for the SharePoint source.
- Immediately after confirmation, always invite the owner to name the first
  two internal agents now or defer them: **“Quer dar nome e avatar ao Walter e
  ao Darwin agora, ou prefere deixar isso para depois?”** This is an invitation,
  never a required extra interview step.
- Present these initial suggestions with their short stories:
  - **Walter 🦉** — suggested name: `Walter`. He is the owner's calm alter
    ego: a senior advisor that asks whether the intrinsic reason behind a
    high-leverage request was actually met. He refines; he is not a naysayer.
    If the owner explicitly asks for a reference-based alternative, examples
    include `Virgil` (guide through complexity), `Iroh` (mentor sereno),
    `Athena` (estratégia prudente) and `Jarvis` (advisor técnico elegante).
  - **Darwin 🧬** — suggested name: `Darwin`. He represents the evolutionary
    loop: the meta-harness that helps the Maestro survive and thrive through
    health checks, housekeeping and deliberate improvement. If the owner
    explicitly asks for a reference-based alternative, examples include `TARS`
    (resiliência pragmática), `Ariadne` (arquitetura de complexidade), `EVE`
    (sinais de futuro) and `Data` (aprendizado contínuo).
- The full repertoire lives in `/agent-identity-setup`. Before suggesting a
  reference-based name, ask one optional question: **“Que presença você quer
  desses agentes: guia sereno, estrategista, parceiro firme, advisor técnico,
  arquiteto de sistemas ou observador de evolução?”** Use only the preferences
  the owner explicitly states to offer at most three relevant choices and say
  why each was suggested. Do not derive a personality, role fit or psychological
  profile from past conversations. `HAL` remains available only if the owner
  chooses it deliberately; never suggest it by default.
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

- Do not import prior persona, project or memory context that is outside this
  Maestro workspace. Keep the conversation focused on the owner's
  professional work.
- Do not read a selected source until the owner gives the second, explicit
  rationale-ingestion authorization. After that authorization, never copy raw
  source bodies; only materialize bounded derived racionais with a source
  pointer and freshness metadata.
- Do not discover SharePoint broadly, resolve a selected folder, call a
  collector or claim that an index exists during onboarding.
- Do not infer a psychological profile.
- Do not bypass the owner's review digest or runtime trust prompt.
