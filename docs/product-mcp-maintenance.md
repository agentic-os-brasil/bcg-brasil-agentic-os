# Product MCP: qualified maintenance and local operation

An admitted workspace uses one maintenance service through Code, Desktop or the
local executable. Inspect status first, review a named job, qualify its actual
execution, and enroll it separately. Qualification can write a real domain
result once; enrollment authorizes later due work for that job and workspace.
Neither operation installs an operating-system task.

Use the same explicit `--product-root`, `--user-home` and `--workspace` binding
for every local command. These select the installed product, existing owner
directory and registered workspace; they are not model-selected paths. Follow
the [module setup guide](../adapters/mcp/README.md) for explicit initialization.
Ordinary startup, status and review do not bootstrap or repair private state.

## Inspect and choose a job

Call `maestro_query` with `{"operation":"maintenance.status"}`. The response
lists job IDs, revision, state, schedule, write set and qualification metadata.
The closed catalog has eighteen jobs; this binding implements fifteen, with attended-only jobs separated from recurring enrollment:

| Job | Cadence after separate enrollment | Actual result or prerequisite |
| --- | --- | --- |
| `memory-light-dream` | Every three hours | Deterministic L1 from eligible reviewed captures |
| `memory-checkpoint` | Every three hours | Checkpoint of real durable scheduler metadata; bootstrap with a successful light dream |
| `memory-deep-dream` | Weekly, Sunday | Deterministic L2/L3 from eligible canonical ancestry; lifetime text retains its separate review |
| `memory-retention-check` | Daily | Durable diagnosis of actual retention/visibility state; no source deletion |
| `skills-index-refresh` | Daily | Immutable external index from verified installed routing inputs |
| `runtime-health-check` | Daily | Durable report of actual available and missing runtime observations |
| `capability-recheck` | Daily | Durable capability diagnosis; missing native evidence remains unavailable |
| `runtime-drift-check` | Daily | Durable comparison with actual admitted projection and installed state |
| `wiki-incremental-sync` | Durable source event only | Compile eligible committed memory and close its exact event once |
| `wiki-reconcile` | Weekly/presence | Reconcile actual source/view and recover missing historical events |
| `wiki-integrity-check` | Daily | Persist body-free current graph diagnosis; no repair |
| `update-check` | Attended only; never recurring | Authenticated source-custody observation, bounded candidate plan and diagnosis; never activation |
| `darwin-housekeeping-daily` | Daily after separate opt-in | Finite actual local diagnosis and exclusive first health-cache creation; stale replacement unavailable |
| `darwin-deep-weekly` | Weekly after separate opt-in | Bounded operational assessment/proposal; existing cache preserved; no authored-document rewrite |
| `darwin-structural-evolution-proposal` | Attended monthly only; never recurring | Immutable bounded structural proposal/assessment; no application of structural changes |

All schedules have bounded catch-up of at most one occurrence. Enrollment sets
a per-job no-backfill boundary. Memory consolidation needs eligible reviewed
captures and ancestry; an empty source or a successful no-op cannot qualify it.
Skills indexing needs verified installed assets, not an arbitrary product
directory. A diagnostic can qualify its real execution while its finding is
missing or unavailable. That qualification does not qualify the reported native
capability. The remaining three entries (two capture/observation jobs and Yoda)
retain explicit unavailable reasons until their complete execution journeys exist.

### Darwin diagnosis, first cache and proposals

Read status to distinguish absent/current/stale/corrupt/foreign/unsafe/unavailable
cache state. Status and reports never initialize, repair or enroll anything.
An attended first qualification may publish one cache exclusively after exact
review and confirmation. Its inputs are actual workspace/configuration/process/
projection/capability observations and bounded external scheduler receipts.
Unavailable native/document coverage remains explicit.

A stale daily cache exposes `manual_refresh_required`. Automatic replacement and
compensation remain unavailable while physical preservation under #462/#488 is
unresolved. Apply refuses that replacement before changing private controls or
effects. Preserve the cache and any recovery evidence for explicit reconciliation;
this operation supplies no delete/archive/command workaround. A failed readback
after exclusive first creation retains its actual partial bytes.

Weekly/monthly work can publish a bounded immutable assessment/proposal while
retaining a stale cache. Unchanged replay cannot mint new qualification merely
because time advances. Daily/weekly qualification remains separate from exact
opt-in enrollment; monthly work is attended-only and never applies a structural
change. Per-attempt publication rechecks current inputs and the existing signed
installation, process, revision and lease guards.

The three Darwin jobs currently require admitted Unix private publication.
Windows reports `production_binding_unavailable`; its current native private
kernel remains intact. Automatic repair, Windows Darwin, real client loading,
production trust and colleague acceptance are separate open gates.

Attended `update-check` may preserve authentic newer source denial even when its
release is expired or payload-incomplete. This is a custody observation, not
update eligibility or activation. A blocked server uses the independently
admitted installer's [signed-source recovery route](product-synced-source.md).

The public surface has eight operation variants: `maintenance.status`,
`maintenance.report`, `maintenance.qualify`, `maintenance.enroll`,
`maintenance.pause`, `maintenance.resume`, `maintenance.revoke` and
`maintenance.run`. These operation names are separate from the fifteen supported
job IDs listed above and the full eighteen-entry catalog.

## Review, qualify and enroll

Read the current revision from status. To request a one-shot qualification, call
`maestro_apply` with the exact job and revision:

```json
{"operation":"maintenance.qualify","job_id":"memory-light-dream","expected_revision":0}
```

The zero above is an example for a fresh job. Never reuse it after a transition.
Trusted client review presents the operation, workspace, schedule, write set,
implementation, configuration and current source state. Accept only the intended
operation. Decline leaves enrollment unchanged. Changed state, executable,
configuration or review requires fresh inspection and review.

After successful qualification, read status again and request enrollment with
its current revision and the owner's explicit IANA timezone:

```json
{"operation":"maintenance.enroll","job_id":"memory-light-dream","expected_revision":1,"timezone":"America/Sao_Paulo"}
```

The revision is illustrative. Only enrollment accepts `timezone`; no transition
accepts executable paths, qualification digests, replacement handlers, approval
flags or fabricated observations. Enrollment authorizes exactly the reviewed
job's schedule and write set. It grants no standing approval of future lifetime
memory candidates, providers or native scheduling.

## Execute and inspect the result

Call `maestro_apply` with `{"operation":"maintenance.run"}` to execute active
due jobs in the foreground. It has no job selector and performs no implicit
qualification, enrollment, dependency installation or model/provider call.
`no_due_work` is a valid result; repeated calls do not invent new occurrences.
The response contains bounded due/receipt metadata. A wake-up or a process exit
alone is not proof of a committed domain result.

For deep dream, diagnostic/index, Wiki, update-check and the three Darwin jobs,
call `maestro_query` with
`{"operation":"maintenance.report","job_id":"skills-index-refresh"}`,
substituting the supported job ID. Reports expose bounded findings, coverage,
counts, capability IDs and relative references. They do not return private
capture bodies, control grants or complete receipt proofs. Light-dream output
is available through `memory.context`; checkpoint qualification/status includes
its relative verified artifact reference. There is no `maintenance-report`
local CLI command.

The index lives under the resolved canonical home at
`brain/runtime/workspaces/<workspace-id>/skills-index`. Its verified immutable
generation contains `catalog.json`, `INDEX.md` and source metadata. The current
pointer selects one generation; ordinary operation must not edit those files.

## Pause, resume and revoke

Use `maestro_apply` with `maintenance.pause`, `maintenance.resume` or
`maintenance.revoke`, always supplying `job_id` and its current
`expected_revision`. Each operation requires trusted exact review. Pause keeps
pending work; resume preserves bounded catch-up. Revoke retains a denial
tombstone and prevents a running attempt from publishing new active output or
terminal success after revocation commits. It does not delete prior valid
domain results. A later enrollment needs current qualification and fresh review.

State and qualification answer different questions: `inactive`, `active`,
`paused` and `revoked` describe enrollment; `unqualified`, `qualified` and
`stale` describe execution evidence. A changed build or configuration makes
old qualification stale. Never repair that state by copying old control files.

After a build/configuration update, explicitly requalify index and retention
against their unchanged genuine inputs, then inspect the new revision before
enrollment or resume. Their durable evidence records the executing implementation/configuration;
there is no need to alter installed assets, inject captures or delete valid
generations to recover qualification. Repeated same-build no-ops still refuse.

## Local operator when client confirmation is unavailable

Use the existing local executable with the same three binding flags:

```sh
maestro-mcp maintenance-status --product-root /installed/product --user-home /owner --workspace /work/project
maestro-mcp maintenance-review --product-root /installed/product --user-home /owner --workspace /work/project < request.json
maestro-mcp maintenance-apply --product-root /installed/product --user-home /owner --workspace /work/project --review-digest REVIEW_SHA256 --confirm < request.json
maestro-mcp maintenance-run --product-root /installed/product --user-home /owner --workspace /work/project
```

`request.json` is an operator-selected private local file containing one closed
qualify/enroll/pause/resume/revoke request. Read the full local review, then use
the identical request, exact returned digest and explicit confirmation. Review
does not itself authorize the effect. A missing confirmation, altered request,
stale revision or changed source refuses; obtain a new review instead of
retrying an old digest. Do not paste private local review output into chat.
The local fallback reviews a maintenance intent, not future lifetime text.

The [memory guide](product-mcp-memory.md) documents reviewed capture and the
separate attended consolidation path. Weekly consolidation can keep L2/L3 when
lifetime confirmation is unavailable or declined. Generic `memory-review` and
`memory-apply` cover capture, correction, deletion and retention; they do not
provide a consolidation or lifetime-candidate fallback.

## Recovery and evidence limits

`confirmation_unavailable` selects local review; `confirmation_declined`
requires a new owner decision. `revision_conflict` requires current status and
fresh review. `qualification_required` requires actual requalification of that
job. `maintenance_unavailable` retains the explicit unavailable capability.
Other refusals require inspection of the binding and prerequisites. A missing
MCP tool is not evidence that legacy ZIP mode applies. Unrelated ordinary work
can continue while a separate capability is unavailable.

Preserve corrupt state, denial records, occurrence leases and timeout fences
for operator diagnosis. Do not delete markers, change clocks, clear locks,
copy older pointers back, silently repair dependencies or install OS tasks.
Publication preserves the last valid result when admission changes; a domain
commit followed by receipt failure remains partial evidence.

Focused local tests cover service execution, actual SDK and rebuilt stdio,
restart/replay and revocation. The foreground adapter reports `local`, even
when reached from a native client. These results establish neither idle OS
wake behavior nor native Code/Desktop capability. Hosted CI, native forms,
ordinary-user Windows admission, organizational signing, distribution and team
pilot acceptance remain separate gates.

Private wiki source/readback follows [the private wiki contract](product-mcp-wiki.md). Windows wiki requires actual owner and retained-namespace admission; a registered operation does not establish native or host qualification.
