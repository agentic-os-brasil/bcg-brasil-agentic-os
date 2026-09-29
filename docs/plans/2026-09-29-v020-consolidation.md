# Maestro v0.2.0 consolidation

Authority: Daniel authorizes implementation, PR consolidation and merge in this session; Walter is his decision proxy. Walter approved this design before implementation. Publication/distribution remains a separate qualification step.

## Specification / global constraints

- Integrate the complete Marcelo chain through `ce1fcf5` into `218a81a` (0.1.12 fixes), preserving provenance. Main and its unrelated WIP are untouched.
- Preserve user data and customization. Migration is copy-only, inventoried and hash-verified, resumable with immutable plans, fail-closed on ambiguity or drift. No initialization marker before successful commit. Legacy agents/workspaces remain readable; never assume empty directories.
- Windows uses native PowerShell 5.1/7, no Git Bash or Python requirement for critical hooks or migration. Mac retains Bash 3.2 and an explicitly resolved Python runtime. Optional Python utilities report unavailable when absent.
- Native Codex uses supported instruction, skill, agent, MCP and hook surfaces. Configured, observed and native-qualified are distinct states; no unsupported enforcement claims.
- caseOS is consented, workspace/case-allowlisted, healthy-before-use and deny-by-default for export. No credentials or personal case IDs in distributed files. Active routing is useful when connected, honest when unavailable.
- Update kit contains recipes and verification contracts, not a replacement for personal state. Persistent checkpoints, subagent review and terminal receipts support long-running execution without claiming prompts can override host limits or approval policy.
- Cover 0.1.11 and 0.1.12 upgrade, clean install, idempotence, rollback, Unicode/spaces, case collisions, permissions, partial attempts, stale receipts, symlinks and scope boundaries.
- Marcelo PRs close only after consolidation is merged and the full concept matrix is verified. Gamma reviews after migration/boundaries and exact-SHA premerge. Walter decides readiness; no hidden P0/P1 or fabricated native/live receipts.

## Task 1: Reconcile the integration

Resolve the 21 merge conflicts with a per-file decision/test ledger. Preserve 0.1.12 alias-safe guards, native PowerShell support and update qualification; absorb new brain/accounts architecture, routes and lifecycle concepts. Keep both decision-log histories and release evaluator coverage. Correct evaluator fixtures that assume `/usr/bin` lacks Python or chmod the wrong directory. Record unresolved downstream gaps rather than marking them delivered.

## Task 2: Transactional migration

Owner: migration_v020. Implement inventory, immutable plan, copy, verify, commit, status/resume/rollback and retained legacy access contract. Add executable edge-case tests first. Update first-run migration sequencing and doctor failure semantics. Coordinate shared JSON contract with Windows implementation. Do not modify PowerShell or build scripts.

## Task 3: Native Windows runtime

Port the verified shared migration contract and brain/accounts hooks to PowerShell 5.1/7. Add new lifecycle handlers, alias-safe account/case guards and route behavior. Test fixture parity without Bash/Python. Wire Windows packaging/settings and hosted native test matrix.

## Task 4: Native Codex and caseOS

Use canonical caseOS onboarding references and current official Codex documentation. Implement supported native adapter and explicit trust/consent flow, MCP install guidance and active routing. Test configuration and deny-by-default data boundaries with offline fake connectors. Live authentication remains explicit.

## Task 5: Product contracts and update delivery

Align identity schema/scaffold/onboarding, log ingestion consent/allowlists, frontmatter generation/backfill, pre-emission total context budget and Yoda verdict semantics. Build Mac/Windows recipe update kits and durable continuation/qualification receipts. Refresh README, release notes, decision log, PR concept matrix and source maps. Tests verify actual behavior, not only text presence.

## Task 6: Qualification and merge

Run local focused/full suites, both upgrade baselines and package verification; label reconstructed baselines. Obtain Gamma cold review and Walter verdict, fix findings, then exact-SHA CI including Windows 5.1/7. Open/attach PR, merge when approved, verify main and close superseded PRs with evidence links. Report code/test/CI/merge/distribution states separately.

## Interface review and progress

| Tasks | Shared interface | Decision |
|---|---|---|
| 1 → 2–5 | Reconciled worktree | No worker edits a conflicted file until integration owner releases it. |
| 2 ↔ 3 | Migration plan/state/receipt JSON | One schema, cross-engine fixtures; no Python dependency on Windows. |
| 2 ↔ 5 | First-run identity and doctor | Migration owner owns ordering; product task changes schema only after migration handoff. |
| 3 ↔ 4 | Hooks and host event payloads | Native adapters normalize real host contracts; unsupported events remain unqualified. |
| 3–5 ↔ 6 | Build/distribution maps and tests | Final integration owns package map, qualification and exact-head receipts. |
| 1–6 | Internal consistency | Test expectations implement the global constraints; blocked evidence never means pass. |

Progress: isolated checkout created from 218a81a; merge of ce1fcf5 started with 21 expected conflicts. No release is qualified yet.
