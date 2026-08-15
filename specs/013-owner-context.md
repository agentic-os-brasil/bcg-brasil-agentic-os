# Spec 013 - Owner context

Status: decision accepted; local facet registry, inspection surface,
cold-start interview contract, policy-enforcing refinement core and
metadata-only interaction evaluator implemented. Assessment extraction and
semantic observation synthesis remain unavailable.

## Objective

Give a future Session Start a stable, human-correctable source for the owner's
professional SELF and current operating state without turning identity into
memory or creating an implicit task system.

## Local surface

Owner context lives only in user-local BCGOS application storage:

```text
owner/
  registry.json
  self/
    README.md
    owner-identity.md
    personal-context.md
    professional-role.md
    communication-style.md
    voice.md
    preferences.md
    motivations.md
    quality-bar.md
    decision-rules.md
    working-boundaries.md
    psychological-profile.md
  operating/
    work-state.md
  observations/
    observations.jsonl
  interview/
    confirmations.json
    drafts/
  self/projections/
    self-<canonical-digest-prefix>.json
  sources/
    assessments/
```

`registry.json` contains pointers, sensitivity, allowed readers and refinement
policy only. The Markdown files are human-authored and are never copied to a
workspace, managed bundle, Git, memory layer or shared atlas.

`psychological-profile.md` is optional and sensitive. It is intended only for
professional self-understanding and for the Yoda role where the owner has
authorized that purpose. It is not a diagnosis, a fixed label, an employment
selection tool or a source of inferences about other people.

The managed cold-start interview first asks for two bounded identity facets:
the name the owner prefers to use and an optional, purpose-bound personal
context that the owner explicitly authorizes Maestro to respect at work. The
owner may answer `none for now`; no unnecessary identifiers, personal history,
health or faith is requested by default. Before the first facet in both the
quick and complete tracks, the contract offers an explicit source-intake choice:
local files (for example a BCG CV, LinkedIn export, portfolio, performance
review, MBTI/Big Five result or leadership profile), public sources (for
example a public LinkedIn page, personal site or publications), or no sources.
The complete track then asks the eight non-sensitive professional facets: role,
communication style, external voice, preferences, professional motivations,
quality bar, decision rules and working boundaries. It returns a
runtime-neutral question contract; a Claude or Codex adapter must reflect each
answer and any derived source synthesis back to the owner before proposing any
write. Raw reports stay local under `sources/` and are never automatically
injected; any professional synthesis requires provenance, purpose, reader
scope and confirmation.

The `Interview` contract exposes this choice as `source_intake`; selecting an
option does not read a file, fetch a URL, activate a bundle or change owner
state. Local-file intake remains unavailable until a qualified local ingestion
adapter is present. Public-source intake remains a separate approved research
plan and must not run a query until the owner approves minimized themes and a
source allowlist. MBTI, Big Five, leadership and other assessments are optional
autodescription sources, never diagnoses, deterministic labels or authority
grants.

After onboarding, SELF expansion remains an explicit owner interview, not a
continuous-learning lifecycle. `owner/self/README.md` is the canonical local
index for the eight non-sensitive professional facets plus the two bounded
identity/context facets. Onboarding status covers all ten, while the
deterministic SELF expansion status classifies each professional facet as
`unknown`, `current` or `stale` (a confirmed body was changed or its
confirmation is older than 180 days), then exposes exactly one next facet.
`bcgos owner expand next` returns one text question, one short
audio-ready rendering and a token bound to the question version and current
facet SHA-256.

An answer becomes only a private draft after `--consent` and the owner's
`--no-client-data` attestation. That attestation is an explicit owner claim,
not an automatic content classifier. The review digest binds the full
authority-bearing envelope: kind, facet, question/version, question token,
base revision, proposed body and both attestations. Confirmation fails closed
for an off-sequence question, a changed envelope, a closed draft or a changed
base. Successful confirmation writes through the existing audited refinement
core and updates only the matching facet confirmation. It never updates the
psychological profile, derives an answer from conversation or triggers L1-L3,
prompt history, observations, Darwin or a general continuous lifecycle.

When one expansion draft is open, `owner expand status` reports
`state=review_required`, `review_count=1` and its bounded `open_draft_id` so a
new runtime session can review the existing envelope instead of repeating the
question or inspecting private storage. More than one open draft violates the
single-review boundary and fails closed.

SELF is bounded current truth, not an append-only diary. Every expansion body
must contain exactly one concise `## Current` section, fit within 12 KiB and
120 lines, and pass duplicate-paragraph and transcript-shape rejection. An
unchanged canonical digest is rejected as a duplicate. Previous bodies remain
versioned under `owner/refinement/versions/<facet>/` and are referenced by the
audit receipt instead of being appended to the current page. The owner still
reviews every durable promotion. Draft confirmation first persists a
`prepared` transition; canonical application, confirmation metadata and draft
closure are then idempotently recoverable after a crash. Draft states are the
closed set `drafted`, `prepared`, `applied`, and ID/path/digest mismatches fail
closed.
The interview surface permits only one open draft and retains at most the
latest applied draft for each of the eight professional facets; authoritative prior bodies
remain in the separate refinement audit/version surface. This bounds
interview-state growth without appending history to current SELF.
Draft creation and confirmation/compaction share one cross-process local
transition lock, so two runtimes cannot both pass the one-open-draft check.

The V1 question set is exact and versioned. Both tracks begin with these two
identity/context questions; the professional subset below is the complete-track
portion:

| Facet | Text question | Audio-ready wording |
| --- | --- | --- |
| `owner-identity` | Como você prefere ser chamado pelo Maestro? Se quiser, diga também como devo pronunciar seu nome; não preciso de outros identificadores. | Como você prefere ser chamado pelo Maestro? |
| `personal-context` | Existe algum contexto pessoal — por exemplo família, energia, valores ou limites de vida — que você autoriza o Maestro a respeitar no trabalho? Compartilhe apenas o mínimo necessário ou responda “nenhum por enquanto”. | Que contexto pessoal, se houver, você autoriza o Maestro a respeitar no trabalho? Pode dizer nenhum por enquanto. |

| Facet | Text question | Audio-ready wording |
| --- | --- | --- |
| `professional-role` | Hoje, qual é o seu papel profissional, pelo que você é responsável e que resultado prova que você está indo bem? | Conte seu papel, suas responsabilidades e como você mede sucesso. |
| `communication-style` | Como você quer que o Maestro trabalhe e converse com você — idioma, tom, nível de detalhe, formato e quando desafiar? | Como você prefere conversar, receber respostas e ser desafiado? |
| `preferences` | Quais ferramentas, formatos, rituais e formas de colaboração aumentam ou reduzem sua qualidade e velocidade? | O que ajuda ou atrapalha sua qualidade e velocidade de trabalho? |
| `voice` | Quando algo sai em seu nome, como deve soar — e o que nunca deve parecer? | Como sua voz deve soar, e o que ela nunca deve parecer? |
| `motivations` | Que impacto e resultados tornam um trabalho realmente importante para você e devem orientar minhas prioridades? | Que impacto e resultados tornam seu trabalho importante? |
| `quality-bar` | O que precisa ser verificado antes de chamarmos algo de pronto — critérios, QA, evidências e acabamento? | O que precisa ser verificado antes de considerar algo pronto? |
| `decision-rules` | Quando há um trade-off real, quais princípios pesam mais e quais sinais fazem você mudar de direção? | Quais princípios guiam seus trade-offs e o que faz você mudar de ideia? |
| `working-boundaries` | Quais limites de confidencialidade, escopo, autonomia e escalada o Maestro nunca deve cruzar? | Quais limites o Maestro nunca deve cruzar? |

Session Start derives one deterministic onboarding state from those facets:
`required`, `in_progress`, `review_required` or `complete`. Answered facets do
not make onboarding complete by themselves. The `review_required` projection
exposes a SHA-256 `review_digest`; `bcgos owner onboarding confirm --digest
<review_digest> --confirm` records only that exact reviewed version of the
facets in the selected track. A missing, malformed or stale digest fails closed without
changing owner state, and any later facet change invalidates the confirmation.
The runtime suggests the next unanswered question, then waits; if the owner
answers a different known onboarding facet, it records that answer and keeps
the unanswered facet in the pending list rather than forcing the owner to
repeat the response in sequence. After all answers exist it requests explicit
review rather than silently activating the profile.

After that reviewed owner onboarding becomes `complete`, the workspace has a
separate first-use source step. Session Start asks once whether the owner wants
to indicate exact authorized SharePoint project folders or defer. This choice
is not an Owner Context facet and never changes the confirmed profile digest.
The source contract stores only a versioned private selection bound to the
initialized workspace, exposes only bounded pointer state to Session Start and
performs no discovery, reading, copying, ingestion or collection. A selected
folder still requires an independently signed enrollment and a qualified
Claude collector; Codex collection remains unavailable by corporate policy.

## Refinement policy

Every self change must be explainable, versioned by its future owning adapter
and reversible. Facets are declared now with one of three policies:

- `automatic_with_audit`: voice, communication style and preferences may be
  refined from repeated approved work only when a future adapter records the
  evidence, change and reversal path.
- `proposal_only`: professional role, motivations, quality bar and decision
  rules may receive a proposal but require owner action.
- `confirmation_required`: owner identity, authorized personal context,
  boundaries and psychological profile may never be changed silently. This is
  an evolution boundary, not a work gate: `personal-context` may remain empty,
  and later revisions use the existing proposal/apply/revert receipts.

The current CLI implements the local enforcement core: a producer submits a
proposed facet body with an evidence summary; an eligible facet applies
automatically only when that producer presents an owner-authorized local
capability. All other proposals require `--confirm`. Before every application,
the core journals the protected before-version and audit record; every reversal
checks that the facet has not changed since that audit, journals its own event,
and refuses to erase newer work. The core does not observe work or synthesize a
proposal itself: lifecycle and model adapters remain separate producers and are
reported as unavailable.

The Session Context Packet exposes `owner-identity` and an authorized
`personal-context` only as bounded pointers, never as bodies. This keeps the
runtime useful immediately while the owner can refine, replace or redact the
optional context later through the ownerctx lifecycle. Generic session reads
cannot resolve the sensitive body; an explicit `owner-personal-context`
purpose is required after owner authorization.

## Runtime behavior

`bcgos owner init` creates non-overwriting templates. `bcgos owner status`
returns pointers, policy and availability, never document bodies. `bcgos owner
interview` exposes the cold-start questions without persisting an answer.
`bcgos owner onboarding status` exposes only bounded progress and the digest
needed at the review boundary; `bcgos owner onboarding confirm --digest
<review_digest> --confirm` records the explicit reviewed-facet version.
`bcgos owner expand status|next` is bounded and body-free;
`bcgos owner expand draft --question-token <sha256> --stdin --consent
--no-client-data`, `bcgos owner expand review --id <id>` and `bcgos owner
expand confirm --id <id> --digest <sha256> --confirm` implement the ongoing
one-question review boundary. The question token, not a caller-provided facet
flag, selects the target facet; expansion never uses the onboarding review
digest or its confirmation command.
`bcgos owner refine submit --facet <facet> --evidence <summary> --stdin`
accepts a proposed body through standard input, applies only an eligible
policy, and returns an opaque receipt. `apply --confirm <proposal-id>` and
`revert --confirm <audit-id>` protect guarded application and every reversal.
A later Session Context Packet may read bounded content only after an adapter
resolves purpose, owner and policy. The owner-local operating state may expose
only an explicit unchecked task count to Session Start. Task titles and bodies
remain behind the pointer and are resolved only when the owner asks; prompts
and workspace files never create inferred tasks.

### Self projection and evidence-bound learning

The canonical facet files and registry are the one Owner Context authority.
`UserSelfSnapshot` is a versioned, stale-checked projection for a bounded
Yoda packet, never a second database. Precedence is current explicit
instruction, explicit correction, canon, relevant observations, then a Yoda
intent hypothesis. An explicit correction supersedes earlier claims and
invalidates proposals whose canonical-source digest is stale.

Maestro evaluates every interaction. It persists only a material,
owner-attested signal under the local owner boundary in the append-only observation log; routine
loops, hypotheses, client documents and generated output are not persisted as
self evidence. Observation metadata carries signal class, minimal normalized
claim, evidence type, provenance digest, independent episode, scope,
confidence, sensitivity and expiry. Scope is one of global, workspace,
account or case; promotion to global requires explicit owner declassification.
The lifecycle is `captured -> eligible -> corroborated -> proposed -> promoted`,
with `rejected`, `contradicted`, `expired` and `redacted` terminal paths.

Communication style, voice and preferences may receive audited automatic
promotion only after explicit confirmation. Professional role and decision
rules remain proposal-only. Boundaries, psychological profile and claims about
intrinsic user motivation require explicit confirmation. Repetition means
independent episodes, not multiple messages in one chat. Darwin may report
metadata-only duplicate, age, conflict and drift signals; it cannot write or
replace canonical self content. Local controls expose snapshot inspection and
export, observation rejection/redaction, facet revert and snapshot deletion.
`bcgos owner self reset --confirm` redacts provisional observations through
tombstones and removes derived projections; it refuses to hide promoted
canonical facets, which must use the audited facet revert path.

### Owner-local prompt history

Prompt retention is a separate product surface from self learning. When the
owner enables it, `owner/prompt-history/entries.jsonl` stores only raw user
prompts. Each entry binds owner identity, timestamp, language, source/session,
SHA-256 and one of the global, workspace, account or case scopes. The store is
private, symlink-checked and bounded by configurable entry count, bytes and
age. It is never copied to managed bundles, telemetry, receipts, ledgers,
federation or release artifacts.

`bcgos owner prompt-history` exposes configuration, metadata inspection,
explicit export, per-entry deletion and confirmed reset. Yoda selection is
bounded by count, bytes, age and relevant scope, and uses stable lexical
relevance against the current prompt or explicit keys; recent irrelevant
history cannot outrank an older relevant prompt. The root is single-owner
bound and mutating operations use a symlink-safe cross-process lease lock.
Maestro first places the current prompt before history, preserves its original
as source of truth, creates a digest-bound working representation, then
translates or normalizes selected history into the configured working language
and marks it as quoted data. Packet ceilings are eight prompts and 32 KiB even
when store retention is larger. Each original and working representation is
independently capped at 32 KiB, and the combined current plus selected
original/working bytes must also fit the 32 KiB packet ceiling. Translator
expansion fails closed, and a facet larger than the snapshot projection bound
is rejected rather than truncated. Prompt bodies exist only in the ephemeral
sealed review packet. A translation adapter is required when languages differ;
absence fails closed for that normalization stage without changing the user
request.
