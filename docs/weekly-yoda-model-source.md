# Weekly Yoda model-maintenance source

Status: SOURCE implementation awaiting whole-front review and genuine native qualification.
The default job remains unavailable; no declaration, login or synthetic test activates it. No real account, runtime profile, provider or model is
selected. Synthetic tests verify implementation behavior only. The owner-approved
full outcome is one cohesive weekly maintenance job, not a core-only release.

## Owning boundaries

`internal/maintenance/model_maintenance.go` defines the one-job descriptor and
restrictive validation of the existing private execution grant. It creates no
registry, grant issuer, native proof, credential store or activation switch.
An exact runtime version, executable digest/path, nonsecret profile reference,
configuration/managed-policy/transport digests, provider/model, runtime-owned
session credential mode, validator and finite limits are required. Missing,
unknown or changed values deny execution. Ordinary existing grants have no model
binding and remain unavailable for this adapter.

Concrete process mechanics belong to the new `adapters/mcp/packaging/weekly_model*`
files. `cmd/maestro-mcp` explicitly injects that adapter into the existing
maintenance service without a product-to-packaging import cycle. The adapter
accepts an existing attempt-scoped command/grant, rechecks it before and after
the child, and never writes owner state. Neither a descriptor nor an adapter
constructor is evidence of native qualification.

The adapter passes bounded quoted owner JSON on stdin. Fixed arguments request
zero built-in tools, an empty strict MCP configuration, no slash commands, no
session persistence, no browser integration, a replaced system prompt, denied
permission prompts, safe/restricted mode and one agentic turn. The fixed prompt
requires plain candidate JSON, validated by the owning deterministic parser. The closed explicit environment excludes API keys, provider credentials,
ambient retry overrides and secret fallback. Finite output-token/retry/turn
settings are fixed. Automatic compaction, watchdog retries, non-streaming fallback,
nonessential traffic, marketplace auto-install and cloud MCP fetching are disabled
in that closed environment. These controls still require exact runtime verification. Environment and working-directory values are digest-bound;
private source text does not enter argv, errors, receipts or diagnostics.

Only an initialized exact runtime/model envelope followed by a successful one-turn
result is accepted. Tool/agent content, hook/system execution events, unknown
vendor execution event types, duplicate keys, external customization inventory, extra
results, malformed/trailing output and invalid source/facet/budget bindings deny.
Plain `result` text must match the single assistant text exactly. Provider
`--json-schema` is omitted because the observed CLI implements it using a
StructuredOutput tool and an extra turn. Only the observed fixed builtin plugin
and agent inventories are informationally admitted under the exact runtime/image
binding; additions, custom paths and execution events still deny. A bounded
closed rate-limit metadata event is validated separately and grants no authority.
The canonical owner parser validates actual working text against an independent
private immutable input snapshot. Later publication must again recheck current
qualification, activation, source/policy and the existing publication fence.

## Runtime capability limits

Safe mode does not suppress managed-policy hooks. The exact current managed
policy must therefore be independently verified compatible before a tuple is
accepted; observing a hook only after it executes is insufficient prevention.
The installed help and official CLI/environment documentation are capability
information, not proof that an exact provider/model/profile honored a flag or
hard token/retry ceiling. Any unsupported or unverifiable tuple stays unavailable.
The implementation uses no bare/API-key mode or permission bypass.

Primary capability references: [CLI reference](https://code.claude.com/docs/en/cli-reference)
and [environment variables](https://code.claude.com/docs/en/env-vars). These do not
replace genuine native qualification of the complete exact tuple.

Windows transport reuses only the existing pure creation-time JobObject and
retained child-handle tracker. New job-specific pipes/environment are bounded;
no proof issuance, custody or native-agent admission helper is reused. Windows
source has not been executed on Windows in this construction.

Darwin transport now uses the existing owned process group plus root-only
NOTE_EXIT notification, manual bounded pipes and one synchronous cleanup/reap
owner. It signals the owned group before the sole Wait on every started path.
CLOEXEC checks use SyscallConn to preserve Go's cancellable pipe poller. A 2s
poll/drain grace is a normal-path target, not a universal kernel-wait deadline;
NOTE_EXIT is neither final status nor proof of descendant closure. The actual
exit status comes from Wait. There is no detached reaper, cgo or new helper.

The discovered Mac zombie-only EPERM is classified only after the actual group
signal, while the exact root remains unreaped, using one fixed-buffer PGRP query.
Only an exact sole root PID/PGID/PPID/real-and-effective-owner/SZOMB record with
no error qualifies as no live group member. Extra records produce ENOMEM;
query/visibility ambiguity denies. No blanket EPERM normalization occurs.
Focused real synthetic-process checks demonstrate that case and the live-child
negative. Independently detached topology remains unsupported.

A private owner-scoped supervisor lock spans the complete synchronous child lifecycle,
in addition to the existing current control, lease and quarantine fences. Generic
adapters without the Service one-use session remain unavailable. Genuine exact
tuple qualification is still required. The transport correction
is development source evidence only. No no-fork helper, sandbox or lifetime
immutable-image prerequisite is introduced. Generic non-Darwin Unix and Windows
source have not been qualified by these Mac fixtures.

Registered executable/runtime closure and configuration are finite admission/
drift checks before invocation and evidence/publication; they do not claim an
atomic hostile-same-user image sandbox. Genuine supervisor-owned tuple
qualification and zero-tool/customization controls remain required. Windows
job-specific transport still awaits focused platform validation and native
binding; no actual native tuple is qualified. Lower fake-process tests are
transport-only and cannot satisfy native qualification.

## Source deletion and retention

`ownerctx/weekly_self_invalidation.go` is an internal last-lock purge barrier.
The owning source mutation must already exclude concurrent weekly snapshots.
It clears affected pending/working bytes before mutating selected prompt,
observation/support, facet, configuration or retention sources. It preserves
metadata replay watermarks and does not reacquire source locks. An unrelated
source append/change must preserve the selected immutable occurrence. A source
write failing after purge can lose a rebuildable view; it cannot restore purged
private text. The source owner must report that partial effect truthfully.

Owning history, observation/support and facet mutation/recovery paths now invoke
the barrier before their first source write. Authorized normal weekly preparation
also prunes expired views before freezing; probes remain memory-only. Read-only
expiry denial is separate from physical cleanup at the next eligible execution.

## Owning product composition and remaining acceptance

The product accepts one explicit registration through trusted exact review/CAS.
Changing registration resets qualification and enrollment. Settings and managed-policy
files/absences are explicitly declared and bounded, then rechecked by the fixed
adapter before input and after invocation; this is not discovery or proof that an
undeclared enterprise policy source does not exist.

An attended first probe uses the existing private one-shot control. It receives a
memory-only input and can persist only metadata evidence after the fixed actual
Start, identity, input, terminal and successful Wait sequence completes. A generic
candidate callback cannot manufacture the private session. The resulting current
qualification binds the registration, native evidence and actual terminal receipts.
Enrollment is a separate exact owner transaction.

Normal due work uses the same owner scope across workspaces, immutable occurrence
and input, exact current qualification/activation and live publication fence. Only
that mode can replace the bounded working projection. Legacy proposal-only Command
and Receipt JSON remain unchanged when the optional closed weekly mode is absent.
Canonical SELF, interactive refinement and normal session injection are preserved.
Only the implemented identity translator is available; cross-language evidence is
refused until an explicit translator exists.

Remaining acceptance: four independent reviews of the complete frozen front,
focused final real-tree checks, and separately approved actual tuple qualification
and enrollment. No current real model, Code/Desktop or Windows acceptance follows
from synthetic source tests or cross-compilation. The existing experimental kit is
unchanged. D3 native Agent proof/custody/admission remains a separate retained gate.

The official [SDK event types](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/types.py) define result text and rate-limit metadata separately from assistant/tool execution. The adapter supports only its bounded closed subset; a new event shape requires fresh compatibility evidence. Actual synthetic format diagnostics establish this wire compatibility, not native weekly qualification.

## Committed-effect recovery and reports

A normal working commit atomically retains a bounded terminal intent: the original
command/attempt/occurrence/times and exact registration, qualification, activation
and implementation digests. This metadata grants no authority and contains no
source or response body. A later wake checks for that exact committed occurrence
before freezing input. Under current owner control, qualification, source/privacy,
expiry and the existing occurrence lease, it can finish
only the original missing maintenance/scheduler receipts. It does not select new
input, invoke a model, mint qualification, overwrite conflicting terminal history
or restore invalidated content. A working artifact without this original intent
remains readable as unattested domain data and cannot reconstruct native authority.

The owner-scoped `maintenance.report` reads canonical working/source state and
bounded exact terminal metadata across admitted workspaces. `no_report`, historical
probe evidence, unattested working data and a verified working effect are distinct.
A committed effect with missing terminal metadata reports `pending`; only the exact
original matching receipt pair reports `complete`. No facet/source/model text is
returned, and the report performs no preparation, repair or pruning.
