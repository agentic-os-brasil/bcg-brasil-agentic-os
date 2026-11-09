# Complete deterministic maintenance through the product

Authority: Spec062, Specs006/009/036 and the canonical owner/memory review
contract in Spec060. Construction follows the reviewed E4 local-content source
in the existing isolated Maestro worktree. Root owns Git/index/refs and the
canonical spec/decision/plan; no main-checkout WIP is touched.

## Global Constraints

One production implementer per isolated worktree. Independent fronts may run in parallel under root coordination; maintenance tasks remain sequential behind their review gates. Reuse Worker, canonical memory, scheduler
and private filesystem helpers. Read-only operations never initialize stores.
No model-supplied approval, qualification, path or runtime observation is trusted.
Preserve exact lifetime review and current capture visibility. All writes stay
under Developer. Focused local checks only; hosted CI and normal writer auth,
native clients/tasks, providers/models, signing, distribution and pilot remain
separate evidence. This delivery does not complete the wider product mission.

## Task 1 — Own qualification, enrollment and guarded memory execution

Own internal/maintenance/** for the new service and directly affected tests;
narrow internal/scheduler/** and internal/memory/** publication/configuration
seams only where required. Do not edit MCP, recipes, root docs/specs or Git.
Read the E5 service-bindings and construction-order artifacts first, then inspect
actual signatures. Implement read-only status and revision-CAS per-job lifecycle,
separate actual one-shot qualification and recurring activation, no-backfill
planning and closed checkpoint/light-dream bindings. Preserve legacy Canary
compatibility without using its constant digest as product qualification.

Qualification must execute/read back real domain output and bind executable,
configuration, workspace/job/platform and evidence. Empty/no-op/source-free
execution never qualifies. No arbitrary public callback can declare an output
verified. A reviewed one-shot authority avoids requiring existing qualification
to bootstrap qualification. Successful one-shot work still uses canonical
occurrence leases/receipts and is not recurring activation.

Add live revision checks to grants and short guarded publication seams, using
one documented lock order for enrollment, lease and domain writes. Revoke may
commit while synthesis is blocked; subsequent visible commit and terminal
success must fail. Preserve timeout quarantine, source visibility and private
staging semantics. Make checkpoint readback genuinely read-only and use existing
confined primitives for new product control/checkpoint paths.

Observe behavioral RED before the shared fix. Prove trusted capture → real L1
and receipt → separately qualified checkpoint/readback, exact-one enrollment,
decline/no state at the service's trusted authority boundary, stale revisions,
copied/cross-workspace evidence, changed configuration/implementation, restart
replay, pause/resume and concurrent revoke before active publication. Include
an actual second-process control mutation where needed to prove cross-process
fencing. No full suites or host installs. Freeze with file hashes and report
the finalized service/MCP caller API; root then stages and obtains independent
Spec+Quality review before Task2 starts.

## Task 2 — Expose the complete memory-maintenance journey

Own adapters/mcp maintenance operations, directly affected registration/types,
cmd/maestro-mcp operator paths and focused public tests. Use Task1's service;
derive identity/configuration from the admitted Bound and actual executable.
Reuse the memory factory or move the existing shared construction narrowly
rather than maintaining divergent budgets/attestors. Closed query/apply schema,
exact trusted modern/legacy elicitation and stdin fallback cover qualification,
enrollment, due-run and lifecycle transitions. Expose bounded status, revision,
qualification and domain references, never private bodies or grant material.

Actual SDK and rebuilt stdio perform a reviewed capture, real qualification,
exact-one enrollment, separately qualified checkpoint and restarted readback.
Exercise decline, no form support/local fallback, stale and changed-source
review, cross-workspace replay and ordinary status non-mutation. Preserve all
owner/memory/content behavior. Freeze and obtain independent Spec+Quality review.

## Task 3 — Complete the remaining deterministic bindings

Own the Task1 service's closed handler construction and directly owning memory,
skills/runtime/capability diagnosis interfaces, catalog/validator fixtures and
focused MCP tests. Add deep-dream, retention-check, skills-index-refresh,
runtime-health-check, capability-recheck and runtime-drift-check end to end.
Inspect existing implementations before adding code. Derive durable metadata
from actual canonical state; missing evidence stays explicit. Deep dreaming
uses deterministic weekly configuration and exact lifetime candidate review,
never a replacement model or enrollment-wide lifetime approval. Correct stale
catalog/validator wording together while retaining unavailable global catalog
and disabled defaults. Qualification must prove each owning durable boundary,
not merely a callable handler. Preserve all eighteen catalog contracts.

Prove each named output through the public service/MCP path and check one
meaningful refusal for its actual trust boundary. No broad suites, synthetic
healthy builders or no-op qualification. Freeze; independent task review.

Extract declarative maintenancecatalog below runtime embedding with compatible
maintenance aliases/parser, allowing direct closed projection/readiness readers
without arbitrary observation callbacks. Use the Spec062 exact external index
root and atomic generation pointer; keep factory Build/Write unchanged. Runtime
selection comes from actual projection metadata, and trusted CLIVersion forwards
the existing native option without guessing missing values. A genuinely observed
diagnosis may report missing/unavailable and qualify detector execution, never
native capability. Reuse existing exact lifetime review; ordinary job intent
cannot approve future text. Root owns Spec012/062/RCIX updates; this worker may
touch only the needed narrow cadence constructor/confirmation seams and tests.

## Task 4 — Close method, documentation and qualification wiring

Own installed maintenance/memory recipe changes and exact method/catalog/embed
tests. Root integrates the operator guide, canonical decisions/spec status and
affected CI filters/jobs. Document exact commands, status meanings, foreground
versus native evidence and recovery limits. Rebuilt Code/Desktop packets must
contain current recipes and shared executable behavior. Use existing tests for
projection/packet integrity; do not add tests that only restate instruction text.
Independent task review, then four final lenses against the exact final tree.

Affected installed recipes are dream-memory, maestro-runtime-checkup,
maestro-environment-setup and maestro-doctor. Put the admitted MCP product path
before positively identified legacy ZIP instructions. A missing/failed MCP tool
is not legacy selection. Product memory uses reviewed captures/consolidation,
never ad-hoc Read/Write/Edit, raw daily journals, auto-dream startup or marker
deletion. Map actual maintenance.status/report/qualify/enroll/run/pause/resume/
revoke and exact local operator fallbacks after Task3 schema freeze. Distinguish
detector execution from native capability and foreground due work from OS wake.
No silent repair/enrollment/interpreter installation follows a diagnostic. Keep
healthy ordinary work available when a separate capability is missing. Preserve
legacy recipes only inside their explicit legacy scope and update affected
catalog/metadata hashes through the existing compiler/integrity tests.

## Preflight interface scan

| Tasks | Producer/consumer or internal check | Resolution |
| --- | --- | --- |
| 1/2 | Service identity and confirmation are different authorities | Bound supplies admitted roots; trusted reviewed intent supplies exact transitions |
| 1/2 | Qualification cannot require prior enrollment | Separate bounded attended one-shot with real domain verification |
| 1/3 | Daily memory factory lacks weekly eligibility | Shared configuration plus separately reviewed weekly candidate; no silent lifetime grant |
| 1/3 | Existing lease guard covers receipt only | Fence domain publication too, with live enrollment revision and tested lock order |
| 1 | Workspace enrollment can backfill a later job | Per-job no-backfill boundary and exact planner filter |
| 1 | Existing Load creates directories | New product status/readback uses confined read-only lookup |
| 2/4 | Recipes must consume implemented closed API | Freeze schema before method change; packet proof after final bytes |
| 3 | Catalog model wording conflicts with deterministic engine | Reconcile declaration/validator/spec together; no availability promotion |
| 4 | Native qualification versus local compiled proof | Report separately; neither catalog nor packet implies native execution |

After this functional delivery, continue private wiki, real Darwin/model/Yoda
and delegation custody, provider lifecycle, complete-format/platform packaging,
verified SharePoint distribution and native/pilot qualification. Do not end the
construction mission at a source milestone.

## Construction checkpoint

Tasks 1–3 passed independent task gates and were saved into the maintenance
integration front. Latest feature: 53cce58a3c77c8361ff6c281f55b8c7a7df3d068;
integration: b60276d9274aa8ef70b1d3d15cd64f3b2421217b. The reviewed source tree
is d3a683a692262d92a07425505b3fad50a936cc8f. These are local integration refs.
Task4 method/packet closure and root documentation/CI remain in progress.

The [operator workflow](../product-mcp-maintenance.md) describes the actual
closed interfaces, prerequisites and recovery. Relative guide links and all
eight documented catalog IDs were checked against source. The affected workflow
parses as YAML; that syntax check is not a hosted CI result. Full local suites
were not run. Final task review and four front-wide review lenses remain due.
