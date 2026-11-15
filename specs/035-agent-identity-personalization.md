# Spec 035 - Agent identity, personalization and ownership

Status: deterministic one-question interview, review-bound strict profile
validation and signed instance identity are implemented. Names and emojis are
presentation metadata and never become authority. **CLI surface superseded** —
the `bcgos agent *` commands listed in the "Deterministic interface" section no
longer exist; the interview flow is now delivered through the
`/maestro-onboarding` skill. The contract (one-question cadence, consent
boundaries, profile schema) remains canonical. Gamma Guardian is a
system-known managed identity with optional presentation customization; its
runtime/native qualification remains a separate evidence gate.

## Purpose

The first setup conversation must let the user choose how the main agents are
presented while preserving a stable, governed architecture. The user sees a
short explanation of each role, receives suggested names and avatars, may
customize them, and explicitly confirms the result before it is persisted.

## Canonical roles shown in the interview

| Role | Responsibility | Ownership boundary |
| --- | --- | --- |
| `maestro` | user-facing hub and route planner | system |
| `client_account_agent` | partner-like account relationship and curated context | client account |
| `case_agent` | execution, analysis, code and deliverables for one case | case/workspace |
| `yoda` | internal pressure-test and review gate | governance |
| `darwin` | drift, health and bounded operating-model maintenance | governance |
| `pa_expert` | versioned Functional/Industrial Practice advice from the PA Expert registry | PA Expert registry |
| `quality_guardian` | longitudinal code-quality and architecture evaluation | quality longitudinal |

The interview may also personalize explicitly scoped specialist agents when
they are created. Gamma is not a practice-chain identity. Every agent has one
emoji-avatar. The avatar is a display contract only: it cannot grant tools,
widen scope, change delegation, or replace a signed role contract.

## Ownership versus authority

The profile owner owns the selected display name and emoji within an explicit
ownership scope. Every selection must repeat the profile `owner_id`; a mismatch
is rejected. Account and case selections must bind to an explicit `agent_id`,
so a name cannot silently become global across clients or projects. The owner
does not own the underlying role authority. Maestro
remains the system hub; Client Account remains the account relationship layer;
Case remains the project execution owner; Yoda remains a review role; Darwin
is the governance surgeon but is limited to `health/maestro-system`; Gamma is a
read-only quality/QA spoke owned by Maestro; and PA expert canon/version remain
centrally curated by the PA Expert registry.

Profiles are stored locally at `agents/personalization.json`, are strict JSON,
must be explicitly confirmed, and are written atomically with restrictive
permissions. Role aliases (`account_agent`, `workspace_agent`) are accepted
only as input for migration and are normalized to canonical roles on save.
Malformed, unconfirmed or out-of-scope profiles fail closed; the system may
fall back to a deterministic default identity without changing authority.

This is an explicit identity-contract migration boundary. Existing signed
manifests created before the identity fields existed require re-scaffolding;
old CLI projections and old activation-policy versions require consumers to
migrate to the canonical `case_agent`, `pae-v1-experimental` and PA Expert
vocabulary. Retired practice roles and ID prefixes are rejected rather than
silently reinterpreted. No
legacy signed artifact is silently reinterpreted.

## Deterministic interface

```text
bcgos agent interview
bcgos agent personalize draft --stdin --consent --no-client-data
bcgos agent personalize review --id <draft-id>
bcgos agent personalize confirm --id <draft-id> --digest <sha256> --confirm
bcgos agent identity
bcgos agent identity set --agent quality_guardian --display-name "Gamma Guardian" --emoji "🧪" --confirm --owner-id <id>
```

The installed catalog defaults are active even before a local personalization
profile exists. `bcgos agent identity` reports this as `state=defaults_active`
and resolves the managed roster, including Gamma's concrete ID
`gamma-guardian`, from the managed catalog. A local profile is needed only
when the owner chooses presentation customization. The first explicit
customization may create that profile with `identity set --owner-id ...
--confirm`; subsequent customizations must preserve the existing owner ID.
Aliases `gamma_guardian` and `gamma-guardian` normalize to the canonical role
`quality_guardian`. Names, emojis and this first-profile convenience remain
presentation-only and never grant authority.

The installer handoff exposes `agent_identity_state` separately from workspace
readiness. A clean install reports `defaults_active` and includes the bounded
managed roster; a confirmed local profile reports `profile_confirmed`. The
installer must not mark the workspace unavailable merely because the optional
presentation profile is absent.

`bcgos agent interview` also returns `profile_input`, the machine-readable
schema for the strict draft envelope. The envelope uses `selections[]`; the
human-facing interview labels `agent_names`, `agent_emojis` and
`ownership_scope` are not top-level profile fields. Account and case selections
must include a concrete `agent_id`, and the canonical scopes are `system`,
`account`, `case`, `governance`, `quality_longitudinal` and
`pa_expert_registry`. A fresh guided interview submits one main-agent answer
per draft in the order Maestro → Yoda → Darwin; previously confirmed answers
are carried forward rather than batching future questions.
If an identity draft is already `drafted` or `prepared`, the interview returns
`state=review_required`, no new question and the bounded `open_draft_id` needed
by `bcgos agent personalize review --id`. Invalid or multiple open drafts
return a blocked interview state instead of inviting another answer.

`interview` is read-only and returns exactly one next question for Maestro,
Yoda or Darwin while retaining the richer transparent catalog. A profile is
only a private draft until review and explicit confirmation. Its digest binds
the complete proposed profile, question-contract version, current profile
revision, consent and no-client-data attestation. Stale revisions and altered envelopes fail without overwriting the confirmed
profile. An already-applied exact draft returns its existing receipt only while
the canonical profile still matches that recorded result.
The attestation is owner-asserted; it is not a client-data classifier.
Identity drafts use the closed `drafted -> prepared -> applied` lifecycle.
`prepared` is persisted before the canonical profile commit, so a retry can
distinguish an unchanged base from an already-applied exact profile and close
idempotently. State, ID, digest and draft path are validated as one envelope.
Only one identity draft may be open, and confirmation compacts older applied
identity drafts to the latest receipt. Profiles are bounded to 128 scoped
selections and 16 capability tracks in addition to the strict input byte bound.
The scan/create and confirm/compaction boundaries share one cross-process local
transition lock; concurrent runtimes cannot create two open drafts.
`identity` returns the confirmed profile or the interview
schema when no profile exists, together with the resolved managed identities
for Maestro, Yoda, Darwin and Gamma Guardian. Agent scaffolding resolves
identity from the
profile, signs name/avatar/owner/scope into the immutable instance manifest,
and continues to enforce the catalog role and scope independently.

The three main-agent questions are exact and ordered:

1. Maestro: “Como você quer chamar o agente que fala com você e rege o trabalho profissional — e qual emoji deve representá-lo?”
2. Yoda: “Como você quer chamar o revisor interno que faz o pressure-test antes de algo importante chegar a você — e qual emoji combina com esse papel?”
3. Darwin: “Como você quer chamar o agente que observa saúde, drift e evolução do sistema — e qual emoji deve representá-lo?”

## Product MCP identity onboarding

The product owns the canonical identity state below its admitted data root,
`<Maestro home>/state/agents/`. It reuses `personalization.json` and the
existing `interview/drafts` lifecycle; it does not copy names into a second
profile or the managed product. Compiled server identity remains unrelated.

`agent.identity.interview` is a noncreating query. It returns the canonical
next question, open draft when present, confirmed profile when available and
`profile_revision` (`absent` or its exact SHA256). Invalid, unreadable or
aliased state blocks; it never becomes a fresh interview. The existing
three-question cadence remains Maestro, Yoda, Darwin. Scope/role authority
comes from the canonical catalog, never from presentation customization.

`agent.identity.answer` proposes one exact answer and, after ordinary native
full review, creates and confirms it through the canonical draft engine in
one short transition. Model fields are `owner_id`, `role`, `display_name`,
`emoji`, `question_id` and `expected_profile_revision`, with `agent_id` only
for an admitted concrete instance. The server carries previous selections
forward, derives scope and binds current profile, question and proposed
profile. Model consent, confirmation and no-client-data flags are forbidden.
The native review explicitly attests owner-only presentation data and the
exact requested effect. Decline before draft creation leaves no proposal or
profile effect. A later I/O failure may leave an inactive canonical draft or
an already-committed profile; report that recovery state instead of claiming
that every filesystem byte stayed unchanged.

`agent.identity.propose` offers the explicit draft-only variant of the same
reviewed answer. It preserves an open canonical draft without changing the
confirmed profile. `agent.identity.review` returns bounded draft metadata;
`agent.identity.apply` presents the full current/proposed profile for exact
native confirmation and resumes the canonical draft. They accept only the
returned `proposal_id`; callers cannot supply a review digest or approval.
A repeated applied confirmation returns its existing exact receipt when the
current profile still matches; it never replays over a later change.

`maestro_context` consumes confirmed preferences through the canonical managed
identity projection. Its bounded presentation roster keeps the fixed agent IDs
and roles while exposing the reviewed names/emoji after restart. No preference
profile reports `defaults_active`. A corrupt profile reports `profile_invalid`
and deterministic default presentation, preserving it and blocking identity
onboarding while unrelated admitted workspace context remains available. This
does not modify immutable compiled agent resources or activate a runtime.

The native confirmation envelope binds the operation as well as the complete
answer/base/profile. Approval of a draft-only effect cannot be replayed as an
immediate profile publication, including modern multi-round input responses.

Human interaction occurs outside all canonical store/control locks. The
mutation reacquires its transition lock, revalidates source/question/base and
reconstructs the exact review before publication. Canonical identity reads,
locks, writes, enumeration and compaction use bounded private no-follow I/O.
Queries do not create locks, directories or permission repairs. Accepted
writes preserve existing valid profiles and canonical paths. Registered
instance selections require their actual role/owner/scope binding; a missing
account/case instance remains an explicit dependency and never becomes a
global selection. Missing native form support authorizes no alternate write.

## Acceptance criteria

1. Initial setup exposes role purpose, name suggestions, emoji suggestions and
   ownership boundaries.
2. No profile is persisted without explicit confirmation.
3. Every scaffolded instance has a valid emoji-avatar and ownership scope.
4. Presentation customization cannot alter tool access, delegation or data
   boundaries.
5. Legacy role strings never appear in newly persisted profiles or signed
   role fields.
6. PA Expert version and canon remain centrally owned despite local display
   customization.
7. Gamma Guardian is recognizable by its managed default (`Gamma Guardian`,
   `🧪`, `quality_longitudinal`) even when runtime/native qualification is
   unavailable; customization changes presentation only.
8. The main-agent interview asks one question at a time and every write crosses
   a review-digest plus base-revision confirmation gate.
9. Confirmation recovers idempotently from every write failure after the
   canonical profile commit; malformed lifecycle metadata fails closed.
