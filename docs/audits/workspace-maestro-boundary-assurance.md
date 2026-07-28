# Workspace boundary assurance — Maestro

**Date:** 2026-07-27
**Scope:** workspace bootstrap, workspace agents, research approval, context
promotion, owner context, interaction profile and Atlas.

## Verdict

The implemented local core is **fail-closed for its supported surfaces**:
workspace identity, promotion authority, source-root containment, review state,
provenance, integrity verification and revocation. It does not claim native
runtime enforcement where that adapter does not exist.

## Ownership and context-promotion matrix

| Surface | Owner | What may be stored/read | Promotion rule | Reversal / denial |
|---|---|---|---|---|
| Managed core | product distribution | code, safe templates, contracts | never receives client or owner data | release allowlist; no workspace fallback |
| Owner context | local owner | SELF facets and operating pointers | never copied to workspace or shared context | policy-bound refinement, audit and revert |
| Interaction profile | local owner | communication preference only | never grants authority, provider access or memory persistence | invalid configuration falls back to standard |
| Workspace / case | owning workspace agent | raw artifacts, dossier, stakeholder/project context, research evidence | one reviewed `account_safe` curated fact with authority for source workspace **and** destination account | source-root checks, provenance receipt, expiry and revocation |
| Account context | account agent | opaque, approved promotion record only | cannot enumerate/browse workspace sources | known promotion ID plus `read_account`; revoked/expired records fail closed |
| Public economic rollup | independent public layer | attested public claims and sources | never accepts workspace-derived inputs | independent-public attestation required |

## Covered adversarial boundaries

- forged workspace identity cannot bootstrap an Atlas or write its files;
- a promotion with a forged workspace ID, source from another workspace, a
  traversal path or an escaping symlink is rejected;
- promotion fails without `approved` review status or with a multi-line or
  Markdown/code-shaped body. The deterministic core does not claim to infer
  semantic curation from an arbitrary single-line string; account context
  stores only the reviewed statement and opaque source receipt;
- an inactive workspace cannot save a brief; a research-plan ID from another
  workspace cannot be approved or create destination evidence;
- source hashes, signed receipts and the monotonic promotion anchor detect
  tampering and make revocation immediately effective.

## Safe bootstrap

`bcgos init` first registers a path-bound workspace identity, then creates the
workspace-agent control plane and only then exposes optional human Atlas
bootstrap. Atlas initialization independently re-checks the registered
identity, so callers of the internal package cannot substitute a workspace ID.
The compact operational state retains IDs, lifecycle, current objective,
approval and artifact pointers; briefs, evidence, stakeholder context and
decision material remain versioned dossier artifacts.

## Gaps separated by type

### Technical gaps

- Native Claude/Codex adapters do not yet enforce the runtime-neutral
  workspace authorization contract for files, search, ingestion, memory,
  indexes, logs and intermediate outputs. These capabilities must remain
  unavailable rather than imply OS-level isolation.
- Cross-workspace delegation, archive/revoke of workspace credentials and
  scoped work-packet delivery are specified but not implemented in the local
  core.
- Research approval records intent and scope, but identity authentication is
  intentionally delegated to a future runtime adapter; the core validates the
  approval record and its workspace-local plan only.

### Product / governance gaps

- Stakeholder and relational context is guided by the Atlas templates toward
  necessary, sourced professional facts, but free-form Markdown cannot by
  itself prevent a user from creating a context blob. Retention, correction,
  deletion and structured stakeholder policy need product decisions.
- Client/account and project labels, lifecycle retention and the user-facing
  active-workspace switch are required by Spec 016 but are not yet represented
  in the compact registration schema.
- **Q-011 remains open.** The first-value workspace vertical demonstrates a
  governed flow, but does not yet establish which single use case is valuable
  for both classic and technical consultants. This audit records the gap; it
  does not close or reinterpret Q-011.

## Evidence

- Specs: `002`, `013`, `015`, `016`, `017`, `024` and `033`.
- Tests: `internal/workspace`, `internal/workspaceagent`,
  `internal/contextpromotion`, `internal/ownerctx`, `internal/profile` and
  `internal/atlas`.
