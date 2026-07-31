# Spec 035 - Agent identity, personalization and ownership

Status: proposed implementation; deterministic CLI interview, strict profile
validation and signed instance identity are implemented. Names and emojis are
presentation metadata and never become authority.

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
| `walter` | internal pressure-test and review gate | governance |
| `darwin` | drift, health and bounded operating-model maintenance | governance |
| `pa_expert` | versioned Functional/Industrial Practice advice from the PA Expert registry | PA Expert registry |

The interview may also personalize scoped practice and specialist agents when
they are created. Every agent has one emoji-avatar. The avatar is a display
contract only: it cannot grant tools, widen scope, change delegation, or
replace a signed role contract.

## Ownership versus authority

The profile owner owns the selected display name and emoji within an explicit
ownership scope. Every selection must repeat the profile `owner_id`; a mismatch
is rejected. Account and case selections must bind to an explicit `agent_id`,
so a name cannot silently become global across clients or projects. The owner
does not own the underlying role authority. Maestro
remains the system hub; Client Account remains the account relationship layer;
Case remains the project execution owner; Walter remains a review role; Darwin
is the governance surgeon but is limited to `health/maestro-system`; and PA
expert canon/version remain centrally curated by the PA Expert registry.

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
bcgos agent personalize --stdin
bcgos agent identity
```

`interview` is read-only. `personalize` accepts one strict profile and requires
confirmation. `identity` returns the confirmed profile or the interview
schema when no profile exists, together with the resolved managed identities
for Maestro, Walter and Darwin. Agent scaffolding resolves identity from the
profile, signs name/avatar/owner/scope into the immutable instance manifest,
and continues to enforce the catalog role and scope independently.

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
