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
3. Read `data/profile/onboarding.json` to inspect the local onboarding state. Do not infer that onboarding exists from prior messages.
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

### 🎙️ Uma forma mais leve de responder

Antes da pergunta de escolha, diga em uma frase que, se a interface do runtime
permitir, o owner pode responder por áudio. Voz costuma trazer mais contexto e
nuance com menos esforço do que digitar. Esclareça que o Maestro mostrará uma
síntese ou transcrição para revisão antes de propor qualquer gravação local;
áudio não é ingerido, enviado ou persistido automaticamente.

### 🧭 Escolha como quer começar

Present exactly two explicit options, then wait for a choice. Do not ask the
first identity question in the same message.

| Opção | Tempo estimado | O que estabelece | Implicação |
| --- | --- | --- | --- |
| **Curta** | **~10 minutos** | Seu nome preferido, contexto pessoal de base (coletado por padrão; opt-out disponível), papel profissional, comunicação, preferências de trabalho e qualidade/QA | Você começa mais rápido, mas voz externa, motivações, regras de decisão e limites de trabalho serão refinados em conversas futuras. |
| **Completa** | **~30 minutos** | Identidade básica, contexto pessoal de base (coletado por padrão; opt-out disponível) e as oito facetas profissionais, incluindo voz, preferências, motivações, qualidade/QA, regras de decisão e limites | Leva mais tempo agora, mas o Maestro começa com uma leitura mais fiel de como você trabalha, decide e quais limites pessoais autorizou. |

Ask only: **"Você prefere a entrevista curta ou a completa?"**

## What the interview is calibrating

The interview is a guided construction of the owner's **professional self** —
not a personality test and not a request to import another system's private
memory. Both tracks begin with two explicit, reviewable identity facets:

- `owner-identity`: the name the owner wants Maestro to use. No unnecessary
  identifiers are requested.
- `personal-context`: a short, purpose-bound statement of personal context
  Maestro should respect at work. **Collected by default in both tracks**; the
  owner may explicitly opt out. When the owner opts out, the facet file records
  the opt-out decision with a timestamp (not silence, not "none for now").

The complete track then covers eight explicit, reviewable professional facets:

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

The quick track covers those two identity facets plus
`professional-role`, `communication-style`, `preferences` and `quality-bar`.
It is a useful operating baseline, but it intentionally leaves external voice,
motivations, decision rules and working boundaries for later refinement.

**Personal-context policy (default-on with opt-out):** the personal-context
facet is collected by default in both tracks. Ask a short, bounded question
such as *"Quer registrar um contexto pessoal curto que o Maestro deve
respeitar no trabalho? (ex.: fuso, restrições de agenda, algo relevante para
priorização). Você pode fazer opt-out."* Never require disclosure of family,
health, faith or private history: the owner may share only the minimum
necessary or opt out. When the owner opts out, write
`data/owner/self/personal-context.md` with an explicit opt-out record
(timestamp + "opt-out registrado pelo owner"), not an ambiguous "none for
now". Psychological/personality material, assessments and visual identity
are not inferred or imported by either track; they require a separate,
explicit local consent path.

## Sugestão técnica orientada pela função

Depois que o owner responder qual é sua função, avalie diretamente se a resposta
contém indicação clara de engenharia, data ou AI. Se sim, explique o que é o
bundle opcional `tech-core` e pergunte se a pessoa quer incluí-lo. Se a resposta
for ambígua, faça a mesma pergunta sem presumir que a função é técnica. Nunca
ative o bundle automaticamente: a seleção de uma trilha técnica e a confirmação
do owner continuam sendo a única forma de projetar as skills. O `tech-core` é um
bundle único e inclui engineering, data, AI e métodos de qualidade.

## Camadas opcionais de identidade

The first interview must not pretend that a professional baseline is the whole
person. After the selected track is reviewed, offer (do not start automatically)
these optional layers when they are useful:

- **Propósito e não negociáveis** — values, long-term direction and personal
  constraints that the owner explicitly wants the professional system to
  respect. Keep this private and out of client/case packets by default.
- **Contexto pessoal ampliado** — anything beyond the short baseline the owner
  deliberately chooses to share, with a declared purpose and reader scope. It
  is never required for ordinary professional work.
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

1. Confirm the exact selected track once and write the selection to
   `data/profile/onboarding.json` (fields: `track`, `status: "in_progress"`).

2. Ask one interview question at a time, following the sequence for the
   selected track; do not invent extra mandatory questions.
3. After each answer, reflect back a concise interpretation and ask whether it
   is accurate. Only then propose the corresponding facet draft. This is the
   quality loop for onboarding: the owner corrects meaning before anything is
   written.
4. Before proposing any write to a facet, show the concise draft and obtain the
   owner's agreement. Never claim that an answer has been saved or that the
   track is complete until the local review is confirmed.
5. When all facets for the selected track are reviewed and confirmed, write each
   confirmed profile file: `data/profile/identity.json`, `data/profile/style.json`,
   and `data/profile/onboarding.json` with `status: "complete"`. Ask the owner for an
   explicit final review before marking complete. **Canonical filenames — do not
   rename or split**: the profile layer has exactly three files: `identity.json`,
   `style.json` (persists the interaction profile per `schemas/style.schema.json`;
   never write a parallel `preferences.json`), and `onboarding.json`. The word
   "preferences" appears in this doc as a facet label (`owner/self/preferences.md`)
   and must not be mirrored as a `data/profile/preferences.json` file.
6. In addition to the profile JSON files, write each confirmed facet to
   `data/owner/self/<facet-name>.md` using the reviewed draft content. The facet
   file names match the canonical facets used by the scaffold: `owner-identity`,
   `personal-context`, `professional-role`, `communication-style`, `voice`,
   `preferences`, `motivations`, `quality-bar`, `decision-rules`,
   `working-boundaries`. Overwrite only the placeholders the scaffold created;
   never write to a facet the owner did not confirm in this session. Each file
   uses the layout `# <facet>\n\n## Current\n\n<reviewed draft>\n`. This is the
   canonical location the `session-start-memory-inject.sh` hook reads to inject
   owner SELF context into future sessions — without this step, subsequent
   sessions silently lose the owner context even though `profile/` is correct.
   For the **quick** track, write only the six facets covered by the track and
   leave the remaining four as scaffold placeholders.
7. After the profile and facet writes succeed, close the owner control-tree so
   downstream skills see a consistent state:
   - Update `data/owner/registry.json`: set `initialized: true` (the scaffold
     writes it as `false`).
   - Update `data/owner/interview/confirmations.json`: append the completed
     track to `completed_tracks` (e.g. `["quick"]`) and set `last_updated` to
     the current ISO 8601 UTC timestamp.
   Without this step the profile files are written but the owner tree still
   reports `initialized: false`, which breaks Doctor/Darwin consistency checks
   and any consumer that reads `registry.json` as the entry pointer.

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
- After either track is confirmed, read `data/memory/sharepoint-config.json`
  to check the project-source state. If the file is absent or `status` is
  `selection_required`, ask exactly one question and wait: **"Você quer indicar
  as pastas autorizadas do SharePoint deste projeto agora ou prefere começar
  sem essa fonte?"**
  - If the owner chooses SharePoint, review the canonical folder URLs with
    the owner and write the confirmed selection to
    `data/memory/sharepoint-config.json` (fields: `schema_version: 1`,
    `folder_urls`, `status: "selected"`).
  - Before proposing ingestion, check upfront whether a SharePoint MCP
    connector is configured in this Claude Code session. If none is present,
    orient the owner in one line: **"Pra ler as pastas na próxima etapa,
    ativar o conector SharePoint no Claude Code (Settings → Connectors).
    Sem ele, a seleção fica gravada e a ingestão roda quando o conector
    estiver ativo."** Do not attempt reads without the connector.
  - With the connector active, offer the in-session ingestion via the
    `sharepoint-ingest` skill. The skill reads only the selected folders
    through the owner's own SharePoint access, writes bounded per-document
    rationales under `data/memory/sharepoint-rationales/` and a generalized
    concept index under `brain/knowledge/sharepoint-rationales/`, keeps the
    SharePoint link and modification date on every rationale, and never
    copies the raw document body.
  - If the owner prefers to start clean, write `status: "deferred"` to
    `data/memory/sharepoint-config.json` and do not ask again automatically.
  - SharePoint remains authoritative. The local rationale layer is a
    derived convenience, never a replacement for the SharePoint source.

### 📎 MarkItDown — ingestão de documentos

After the profile is confirmed and before suggesting any next skill, run
`markitdown --version` silently to detect availability.

**Se disponível:** confirme em uma linha — "Ingestão de documentos (PDF, Word,
PowerPoint, Excel) está habilitada via MarkItDown."

**Se não disponível:** explique de forma direta e peça autorização:

> "O MarkItDown é a ferramenta que habilita a ingestão de documentos (PDF, Word,
> PowerPoint, Excel e outros formatos) no Maestro. Com ele instalado, basta enviar
> um arquivo na conversa para que o conteúdo seja incorporado ao workspace — sem
> cópia manual, sem ctrl+v.
>
> Para instalar, o caminho canônico é via **pipx** (funciona no macOS, Linux e
> Windows sem esbarrar em ambiente Python gerenciado):
>
> ```
> pipx install markitdown
> ```
>
> No Windows, se `pipx` não estiver disponível, `pip install markitdown` também
> funciona diretamente. No macOS com Python do Homebrew e na maioria das distros
> Linux modernas, `pip install` retorna `error: externally-managed-environment`
> (PEP 668) — nesses casos use `pipx`, que já vem via `brew install pipx` ou
> `python3 -m pip install --user pipx`.
>
> Posso guiar a instalação agora, ou você prefere instalar depois?"

If the owner authorizes installation now, provide the exact command
(`pipx install markitdown`, com `pip install markitdown` como fallback Windows)
and confirm once the owner reports success by running `markitdown --version`
again. Do not attempt to run the install autonomously; the owner must execute it
themselves or explicitly delegate terminal access.

### 🤝 Agentes internos — identidade e personalização

Immediately after MarkItDown confirmation (or deferral), always invite the owner
to name the internal agents now or defer them:
**"Quer dar nome e avatar ao Walter, ao Darwin e ao Gamma Guardian agora, ou
prefere deixar isso para depois?"** This is an invitation, never a required
extra interview step.

Present these initial suggestions with their short stories:

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
- **Gamma Guardian 🧪** — suggested name: `Gamma Guardian`. It is the
  system-known longitudinal quality/QA guardian: a direct Maestro spoke that
  reviews bounded workspace heads and returns advisory evidence, never a
  naysayer, Case child, merge authority or native-runtime qualification. The
  owner may customize its display name and emoji, but not its
  `quality_guardian` role, `quality_longitudinal` scope, read-only boundary or
  Maestro routing. If an adapter or independent runtime evidence is absent,
  Gamma reports `UNAVAILABLE`/`BLOCKED`; it does not infer readiness.

The full repertoire lives in `/agent-identity-setup`. Before suggesting a
reference-based name, ask one optional question: **"Que presença você quer
desses agentes: guia sereno, estrategista, parceiro firme, advisor técnico,
arquiteto de sistemas ou observador de evolução?"** Use only the preferences
the owner explicitly states to offer at most three relevant choices and say
why each was suggested. Do not derive a personality, role fit or psychological
profile from past conversations. `HAL` remains available only if the owner
chooses it deliberately; never suggest it by default.

Explain that names and emoji-avatars are entirely customizable now or later;
they never alter an agent's authority. Gamma's identity is known by the
system even when its runtime is unavailable. The owner can also create any
number of named **Client Account Agents** and **Case Agents** whenever a real
account or case is ready, through `/agent-identity-setup` and an explicitly
confirmed local profile.

Only after this invitation may you suggest another next skill, chosen for the
owner's stated need. Examples: `/case-agent-setup`, `/case-kickoff`,
`/ingest-content` or `/meeting-to-work-items`. Suggesting a skill is not
executing it. Explain its purpose and wait for the owner to choose it.

## Non-negotiables

- Do not import prior persona, project or memory context that is outside this
  Maestro workspace. Keep the conversation focused on the owner's
  professional work.
- Do not read a selected source until the owner gives the second, explicit
  rationale-ingestion authorization. After that authorization, never copy raw
  source bodies; only materialize bounded derived racionais with a source
  pointer and freshness metadata.
- Do not discover SharePoint broadly during onboarding, resolve or read a
  selected folder, or claim rationales exist before `sharepoint-ingest` has
  run with an active MCP connector.
- Do not infer a psychological profile.
- Do not bypass the owner's profile review or skip writing confirmed profile files.
- Do not run `pip install` or any installation command autonomously; always
  present the command and wait for the owner to execute or explicitly
  authorize terminal delegation.
