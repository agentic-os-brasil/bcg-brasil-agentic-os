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
Startup without an active grant, status and review do not bootstrap or repair
private state.

## Inspect and choose a job

Call `maestro_query` with `{"operation":"maintenance.status"}`. The response
lists job IDs, revision, state, schedule, write set and qualification metadata.
The closed catalog has eighteen jobs; this binding implements sixteen, with attended-only jobs separated from recurring enrollment:

| Job | Cadence after separate enrollment | Actual result or prerequisite |
| --- | --- | --- |
| `memory-l1-capture` | Daily after separate qualification and enrollment | Body-free Code Stop observations and witnessed canonical completed-result metadata admitted through the canonical capture/denial owner |
| `memory-light-dream` | Every three hours | Deterministic L1 for one oldest pending eligible ingestion day; no short historical cutoff |
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
capability. The remaining two entries, `self-observation-capture` and
`yoda-self-review-weekly`, retain explicit unavailable reasons until their
complete execution journeys exist. Supporting workspace capture does not enable
owner self-observation or Yoda.

### Capture eligible metadata, then generate L1

Explicit memory setup prepares the existing capture-provenance key. Code Stop
can then queue a sealed observation of its actual adapter receipt; it remains
`observed_only`, never proof of completed work or native Agent invocation. The
capture job separately reads the confined execution owner's completed records
and matching passing criterion receipts. These provide historical
`committed_effect` metadata, without proving the original host/caller or current
artifact bytes. Read/plan operations, failed or incomplete work and model
approval/sanitizer claims do not establish that evidence.

Use the reviewed qualify/enroll sequence below first with
`job_id: "memory-l1-capture"`, reading that job's current revision at each step.
Qualification performs one real capture pass; enrollment separately permits its
daily cadence. Its review names both source classes and the metadata-only scope.
Signals contain closed labels, opaque source hashes, revisions and timestamps;
they never copy prompts, transcripts, tool bodies, objectives, summaries,
document paths or document contents. Exact replay does not duplicate captures;
changed sources refuse, and corrections/deletions remain authoritative.

Lifecycle startup context and status/query operations never capture, create
signing authority or enroll jobs. Stop only records/queues diagnostic metadata:
it never captures, dreams or
enrolls. Capture provenance grants no tool permission or native execution proof.
Inspect `maintenance.report` for `memory-l1-capture` to see its actual bounded
capture/class counts; this report contains no captured text.

After eligible captures exist, qualify and enroll `memory-light-dream`
separately. Each occurrence selects at most one oldest pending eligible ingestion
day from the bounded canonical memory cohort. Its selected period/fingerprint
stays fixed through execution and qualification, so a capture committed before
midnight can still reach L1 after a long client shutdown. `GeneratedAt` records
actual synthesis time; this does not backdate an execution or remove the
scheduler's separate no-backfill boundary.

Current permanent correction/deletion denials apply to every run and public
readback. The existing privacy purge can physically remove generated versions
used as qualification evidence. When that happens, the job requires fresh
explicit qualification with current eligible inputs before it can run again.
New inputs alone do not silently requalify it. Inspect status, obtain a new
review, and approve only the intended requalification; never restore deleted
text or copy old evidence to keep a cadence active.

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
`maintenance.run`. These operation names are separate from the sixteen supported
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

## Keep enrolled work running

While a Code or Desktop MCP process is open, its owned background worker checks
the existing active enrollments independently of conversation requests. It
checks once when the server starts and waits thirty seconds after each finished
check. The job's reviewed schedule still decides when an occurrence is due;
the pulse does not create a new schedule or qualify a job. With no active
enrollment, starting and closing the server creates no maintenance state.

Automatic work requires a real native observation of five minutes idle. Active
or unknown activity leaves due work pending; it does not create a grant, lease
or domain output. The server keeps observing while work runs and cancels when
activity returns or observation becomes unavailable. Each publication also
rechecks eligibility and current authority. An explicit foreground
`maintenance.run` retains its separate owner-requested behavior.

macOS uses the existing HID observer. Windows requires the process's own active
interactive session and valid input/tick counters. A headless/disconnected
session, hidden window station or missing/ambiguous counter stays unavailable.
No key, input text or clipboard content is collected. Native platform/client
qualification remains separate from source and controlled constructor tests.

The worker uses the same current implementation, qualification, revision and
lease checks as foreground execution. Competing clients cannot claim the same
occurrence as separate successful work. Closing the MCP connection cancels and
joins that process's worker; completed domain results remain available to the
next session. If all clients are closed, work waits until a later server starts
and performs the existing bounded catch-up. This does not install a LaunchAgent,
Windows task or always-running service.

Interruption preserves the existing attempt fence. A cooperative handler can
finish cancellation cleanup while its process remains open; an abrupt process
exit or an unresponsive handler can leave a quarantined lease requiring the
existing explicit recovery. Reopening a client never clears that fence or
relabels partial work as success.

Private-memory wiki compilation needs its own qualification and enrollment.
An automatically committed memory rollup can trigger that job's actual source
event in the same worker; enabling memory upkeep alone does not enable wiki
compilation. Authored-page navigation remains a separate compiler and scope.

Read `maintenance.status`, the supported `maintenance.report`, or
`memory.context` to inspect actual state and committed output. A live client,
successful pulse or wake alone does not prove a domain commit. A changed build,
configuration, paused/revoked grant or unsafe binding refuses work; inspect and
review current qualification rather than copying an old control record.

## Execute an immediate check and inspect the result

Call `maestro_apply` with `{"operation":"maintenance.run"}` to execute active
due jobs in the foreground. It has no job selector and performs no implicit
qualification, enrollment, dependency installation or model/provider call.
`no_due_work` is a valid result; repeated calls do not invent new occurrences.
The response contains bounded due/receipt metadata. A wake-up or a process exit
alone is not proof of a committed domain result.

For capture, deep dream, diagnostic/index, Wiki, update-check and the three Darwin jobs,
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
