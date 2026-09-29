# v0.2.0 integration decisions

Integration: `218a81a` (0.1.12 corrections) plus `ce1fcf599f30e32343e1f7ba305f7ef907bbc490` (complete 0.2.0 chain). This ledger records content reconciliation, not release or native-runtime qualification.

| Conflicted file | Resolution | Validation / remaining owner |
| --- | --- | --- |
| `VERSION` | 0.2.0 | Mac artifact build |
| `bundles/base/agents/activation-policy.json` | Canonical 020 policy/lifecycle with union of 012 explicit invocation synonyms; remove obsolete no-consumer claim | Evaluator agent routing |
| `bundles/base/distribution.json` | Union keyed by destination path, reject conflicting source mapping | JSON parse, package coverage |
| `bundles/base/known-issues.md` | Preserve honest mutable-marker isolation limitation and brain paths; native Windows runtime wording | Product owner to reconcile final runtime readiness |
| `bundles/base/manifest.json` | 020 version and required isolation hook; retain PS paths and anchored matchers | JSON parse and evaluator wiring; Windows owner qualifies |
| `bundles/base/skills/maestro-doctor/SKILL.md` | Brain layout with 012 native hook diagnostic wording | Released to migration owner for failure semantics |
| `bundles/base/skills/maestro-environment-setup/SKILL.md` | Brain interpreter record and explicit degradation rather than silent-success claim | Evaluator isolated interpreter fixtures |
| `bundles/base/skills/maestro-setup-update/SKILL.md` | 012 exact-ZIP/core/baseline/root binding and terminal receipt, brain lifecycle markers | Evaluator phase 25; product owner aligns update kits |
| `bundles/base/tools/agent-route.py` | 020 router context and shared policy | Evaluator route positives, ordinary-work and dormant negatives |
| `docs/decisions/decision-log.md` | Both decision histories retained | Product owner audits final decision history |
| `installers/zip/eval-release.sh` | 020 phases 1–24 plus 012 long-running receipt phase 25; 012 isolated interpreter fixtures and alias negatives retained | Fresh integrated package evaluator |
| `installers/zip/user-template/.claude/agents/darwin.md` | 020 brain diagnostics and bounded authority | Projection/evaluator coverage |
| `installers/zip/user-template/.claude/agents/pa-expert.md` | 020 brain exclusion, dormant policy retained | Dormant routing negative |
| `installers/zip/user-template/.claude/agents/yoda.md` | 020 SELF authority and restricted reads | Projection/evaluator coverage |
| `installers/zip/user-template/.claude/hooks/block-cross-case-writes.sh` | 012 filesystem alias resolution with 020 account/case pair identity; no raw-string allow shortcut | Traversal, same-case-name different account, symlink, missing marker and missing-parser negatives |
| `installers/zip/user-template/.claude/hooks/context-inject-userprompt.sh` | 020 brain/skill/agent route paths and owner identity | End-to-end evaluator routing |
| `installers/zip/user-template/.claude/hooks/lib/python.sh` | Shared resolver with brain record path | Isolated interpreter fixtures, no host PATH leakage |
| `installers/zip/user-template/.claude/hooks/session-start-memory-inject.sh` | 020 brain/lifecycle with UTF-8 parsing | Emitter/cumulative cap coordinated with root |
| `installers/zip/user-template/.claude/settings.json` | 020 stop lifecycle additions plus already-merged anchored 012 matchers | JSON parse and settings evaluator |
| `installers/zip/user-template/CLAUDE.md` | 020 brain trees, retain platform-specific scaffold declaration | Product owner aligns native adapter |
| `installers/zip/user-template/README-INSTALL.md` | Copy-only brain or legacy data, retained backup and doctor plus terminal receipt before deletion | Evaluator deletion gate |

## Fixtures and evidence

Phase 6 now makes `brain/` read-only (the directory actually scaffolded). Phase 19 uses an allowlisted temporary PATH rather than assuming `/usr/bin` has no Python. Security coverage includes aliases outside the accounts tree and aliases inside the active case to another account with the same case ID. No skipped check constitutes native qualification.

The migration script and first-run scaffold merged cleanly; their implementation ownership transferred immediately to migration_v020. Doctor ownership transferred after its single conflict was resolved. Integration makes no claims about the new migration transaction, Windows native execution, attended host hooks, signing, hosted CI, merge or publication.

Focused host-local guard regression suite: `python3 installers/zip/test-account-guard.py` passed all 10 tests (54.782 seconds). These are post-resolution characterization/regression tests, not claimed as test-first evidence. Bash syntax validation and `git diff --check` passed.

The first Mac package evaluator run produced 196 pass / 4 fail / 0 skip. That run exposed two remaining fixture defects (Phase 6 created data while chmod targeted brain; Phase 17 omitted native /hooks and /status), now corrected. It also exposed the receipt phase appended after the earlier summary/exit; Phase 25 now precedes the single terminal summary. The remaining product contract gap is `brain/canary/` missing from the on-demand path contract; product owner must reconcile the receipt namespace rather than suppress the check. A rerun log is `/tmp/maestro-v020-integration-eval-final.log`.

The package tested in these runs predates the final cumulative-cap hook wiring and subsequent parallel task edits. Final qualification must rebuild the artifact from the coordinated source revision. Security tests used the current source guard; release evaluation is not attributed to later revisions.
