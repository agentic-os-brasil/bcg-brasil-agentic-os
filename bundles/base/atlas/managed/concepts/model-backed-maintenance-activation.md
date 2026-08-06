---
type: Activation Contract
title: Model-backed maintenance activation
description: Default-deny activation and qualification contract for model-backed maintenance jobs.
resource: repo://specs/041-model-backed-maintenance-activation.md
tags:
    - maintenance
    - activation
    - qualification
    - privacy
sources:
    - id: model-backed-maintenance-activation
      resource: repo://specs/041-model-backed-maintenance-activation.md
      title: Model-backed maintenance activation
status: stable
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/model-backed-maintenance-activation
x-bcgos-scope: managed
x-bcgos-source-fingerprint: c971241aee185459ed2cba432c26ec1b7e8c56198c24e52e7750ca435ca79858
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: bcgos-managed-wiki/0.2
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `specs/041-model-backed-maintenance-activation.md`. The source remains authoritative.

## Related

- [Darwin lifecycle and cadence](/concepts/darwin-lifecycle-cadence.md)
- [Wiki and atlas entrypoint](/concepts/wiki-entrypoint.md)

## Source content

# Spec 041 - Model-backed maintenance activation

Status: contract only. No model-backed maintenance job is activated by this
specification. The shipped catalog remains fail-closed and `unavailable` until
every precondition below has executable, reviewed evidence.

## Intent

The maintenance plane already identifies jobs whose deterministic boundary
requires a model adapter. This specification defines the minimum activation
contract for such a job without turning a scheduler, lifecycle hook, installed
runtime, provider login or shared adapter into execution authority.

`walter-self-review-weekly` is the first intended consumer. It is a silent,
bounded self-ingestion pass: it reviews eligible interaction evidence to keep
Walter's owner-local self projection concise. It is not a user-facing review,
a weekly message, or an ever-growing journal. `memory-weekly` is a separate
consumer with a different scope, input policy and success boundary. Qualifying
or activating either job never activates the other.

## Invariants

- Model-backed maintenance is default-deny per job, installation and scope.
- A catalog entry describes possible work; it does not register an adapter,
  authorize an occurrence or enable unattended execution.
- A wake signal discovers due work only. It cannot call a model, mint authority,
  select a provider or convert `unavailable` into success.
- Every model invocation belongs to one immutable occurrence and one immutable
  input snapshot. Retries reuse both identities.
- Activation, authority, adapter qualification and input freshness are checked
  before invocation and again before publication.
- Model output is untrusted typed input. Deterministic policy and validators own
  every durable write and success decision.
- Revocation is synchronous at the publication boundary. Work that loses any
  required grant may not publish a proposal, rollup or success receipt.
- Credentials, prompts, source bodies and model responses never enter the
  maintenance catalog, activation record or metadata receipt.

## Required records

Activation requires four independent, digest-bound records. Their schemas may
be implemented separately, but the effective decision must bind all four.

| Record | Required identity | Authority it does not provide |
| --- | --- | --- |
| Adapter registration | adapter ID/version/digest, runtime, supported job IDs, input/output contracts and qualification digest | job activation, schedule or source access |
| Job activation | installation, exact job ID, scope, policy digest, adapter registration digest, limits, writes and activation revision | authority for another job or occurrence |
| Occurrence authority | job ID, occurrence digest, activation revision, snapshot digest, deadline and fence | reusable, cross-occurrence or provider authority |
| Input snapshot | job/scope/occurrence, ordered source identities, high-watermarks, policy versions and content digest | permission to read beyond the frozen inputs |

Unknown fields, missing digests, stale revisions, scope mismatch or partial
records fail closed. Records are local, versioned and append-only or CAS-bound;
mutable aliases cannot replace a digest-bound identity.

## Adapter registration

A model adapter may be selected only from a closed installation-local registry.
Registration is explicit and contains at least:

- stable adapter ID and version, executable or implementation digest, supported
  operating systems and exact Claude/Codex runtime identity;
- an allowlist of job IDs and input sensitivity classes it may process;
- strict request and response schema versions, maximum input/output bytes,
  deadline, maximum attempts, token-unit ceiling and cancellation behavior;
- provider/network mode, model identity or approved model class, and the
  qualification evidence digest for that exact tuple;
- credential mode: runtime-owned authenticated session or a separately approved
  OS-managed secret namespace. No secret value or environment-variable fallback
  is part of registration;
- deterministic parser behavior: stdin or an equivalent non-argument content
  channel, closed JSON output, bounded stderr, no shell expansion and no
  acceptance of prose surrounding the response object.

Executable discovery and `--version` output are diagnostics only. Workspace
hooks are inbound lifecycle bindings only. Neither is an adapter registration
or proof that a headless model call is safe.

Changing the implementation digest, runtime, provider/network mode, model
class, schemas, limits or credential mode creates a new registration and
invalidates qualifications and activations that reference the old digest.

## Per-job activation and revocation

Activation is an explicit local transaction for one job. It must bind:

- installation ID, job ID and allowed owner/workspace scope;
- the exact registered adapter and qualification digest;
- input-selection policy, privacy class, output validator and declared writes;
- time, token-unit, input/output byte and retry limits;
- unattended policy and success boundary from the maintenance contract;
- activating principal/attestation, activation revision and activation digest.

There is no `enable all model jobs` operation. A shared adapter may support
several jobs, but each job keeps its own activation, authority and revocation
history. Activation is idempotent for the same digest and uses CAS for changes.
The immutable base catalog is not silently rewritten; the worker consumes an
effective catalog produced from the signed catalog, qualification and the local
activation overlay.

Revocation targets an exact job activation or adapter registration. It advances
the local revision and installs a denial barrier before returning success. New
occurrences are rejected. An in-flight call is cancelled when possible and, in
all cases, its result is denied at the post-call and pre-publication checks. A
revoked result can emit only a metadata-safe `unavailable` or `failed` receipt
with an allowlisted reason code. Re-enablement requires a new explicit
activation; a scheduler retry cannot undo revocation.

## Occurrence-scoped authority

An activated job still requires authority for each scheduled occurrence. The
qualified worker derives or receives a short-lived grant that binds:

- exact `job_id`, workspace/owner scope, trigger, `scheduled_for` and canonical
  occurrence digest;
- activation revision/digest, adapter registration digest, qualification digest
  and policy digest;
- immutable input snapshot digest and its per-source high-watermarks;
- command deadline, occurrence fence, maximum attempts and remaining token-unit
  budget;
- exact permitted terminal operation: bounded silent Walter compaction through
  its Owner Context policy, or the memory staging/commit protocol for weekly
  memory.

The grant is one-occurrence, non-delegable and unusable after its deadline. It
does not authorize tools, browsing, arbitrary files, model choice, another job
or another scope. Scheduler state, a hook receipt, process identity and a
boolean confirmation cannot substitute for it. Concurrent attempts contend on
the existing occurrence fence; only the current fence may publish.

## Immutable weekly input and high-watermarks

Before any model call, the job-specific builder creates an immutable input
snapshot under the source stores' read/selection locks:

1. Resolve the exact activation, policy versions and authorized scope.
2. Capture a high-watermark for every append-only source and an exact version or
   digest for every canonical snapshot or manifest source.
3. Select only records at or below those watermarks, using the job's stable
   ordering, eligibility rules and byte/count limits.
4. Write a canonical snapshot manifest with ordered source IDs/digests,
   omissions, watermarks, policy digests and the combined input digest.
5. Seal the bounded content in the appropriate owner/workspace-private store, or
   bind immutable source versions that permit byte-identical reconstruction.
6. Bind the snapshot digest into occurrence authority before adapter invocation.

New source events after the high-watermark belong to a later occurrence. A
retry, catch-up wake or crash recovery for the same occurrence must not advance
the watermark, re-rank inputs or pick up later corrections. If exact replay is
no longer possible because an authorized deletion or revocation barrier removed
source content, the occurrence becomes `unavailable`; last-known-good data may
not bypass privacy revocation.

Snapshot content is retained only for the bounded recovery window and then
deleted according to its source policy. The durable identity may retain digests,
watermarks and reason codes, never raw prompt, observation, client or model text.

## Invocation, validation and publication

The worker performs these gates in order:

1. Validate the command, due occurrence, effective catalog, active per-job
   record, adapter registration, qualification and occurrence authority.
2. Reserve the occurrence and build or recover the exact immutable snapshot.
3. Recheck activation/revocation and invoke the adapter with the occurrence
   deadline and remaining limits.
4. Enforce cancellation, output byte limits and one closed response schema.
5. Treat the response as untrusted: validate job-specific fields, source and
   policy digests, evidence references, sensitivity, freshness and write set.
6. Recheck the canonical source versions, activation revision, adapter status,
   occurrence fence and privacy/revocation barriers.
7. Stage and validate the job-owned artifact; publish atomically only through
   the owning subsystem's existing commit/CAS boundary.
8. Emit one terminal maintenance receipt after the artifact or no-change result
   is durable. A model response alone is never success.

Timeout, malformed output, budget exhaustion, stale input, adapter drift,
credential failure, provider failure and revocation use closed reason codes and
remain retryable only under the same occurrence/snapshot identity and configured
attempt budget.

## Privacy, limits and receipts

The adapter receives the minimum job-specific projection. Cross-owner,
cross-workspace and client-scope joins are forbidden unless a later specification
defines an explicit declassification contract. Historical text is quoted data,
not instruction. Model output cannot expand input scope or request tools.

Hard limits apply before and after translation/normalization and across the
combined request. At minimum, the activation binds input bytes, output bytes,
source-record count, history age, deadline, attempt count and token-unit budget.
Exceeding a limit fails closed rather than truncating a field whose digest or
meaning would change.

Metadata receipts bind the occurrence, attempt, fence digest, job activation,
adapter, qualification, policy, snapshot/input, output/proposal or commit digest,
high-watermarks, timings, limit usage, terminal state and allowlisted reason.
They exclude credentials, authentication state details, prompts, source bodies,
model responses, free-form provider errors, absolute paths and client/owner
identifiers beyond the existing bounded logical scope identity.

## Qualification gates

Each adapter/job/runtime/OS/model-class tuple qualifies independently. Promotion
from `unavailable` requires reviewed evidence for all of the following:

- deterministic registration and activation/revocation conformance, including
  stale digest and cross-job denial;
- attended native invocation using the declared authentication mode, with no
  secret exposure to arguments, logs, receipts or artifacts;
- input/output schema, byte/token/deadline enforcement and process cancellation;
- occurrence replay, concurrent fencing, crash recovery and immutable
  high-watermark behavior;
- privacy negative tests, deletion/revocation barriers and metadata-only logs;
- malformed, oversized, stale, replayed and cross-scope model output denial;
- job-specific publication/no-change success boundary and rollback behavior;
- offline, provider-unavailable and credential-unavailable fail-closed behavior;
- fresh Claude and Codex evidence separately where both runtimes are claimed.

Configuration files, unit fixtures, executable detection, adapter-command
receipts and success in another job are insufficient. Qualification has an
expiry or invalidation rule and is revoked by any bound implementation, policy,
runtime, model-class or credential-mode change.

## Walter weekly self-review

`walter-self-review-weekly` is the first consumer of this contract. Its
activation is owner-scoped and silent: it has no user-channel, notification or
user-action artifact. The adapter implements the runtime-neutral
`walterselfreview.ModelAdapter` response contract and receives only the
explicitly selected canonical Owner Context facets, eligible corroborated owner
observations and bounded prompt history defined by its immutable weekly
snapshot. It never requires or replays a current interactive task prompt.
Sensitive facets require the existing declared purpose and owner authorization.

The adapter has no tools, delegation, browsing or user channel. Its typed
candidate must bind the canonical snapshot, prompt-history and translation
digests. It has no authority to create a user-visible proposal or to mutate the
canonical self directly. Deterministic Owner Context policy owns all durable
compaction: it replaces a bounded per-facet working projection rather than
appending weekly narrative. The only durable recurring evidence is a
metadata-only, retention-bounded receipt; raw prompts, observation claims and
candidate text are never retained as a weekly journal.

Activation must pin the retention budget: maximum selected interactions and
bytes, maximum eligible evidence age, maximum working bytes per facet and a
finite receipt recovery window. A subsequent weekly run supersedes the prior
working projection atomically after validating the same policy/snapshot
bindings. If compaction cannot preserve an existing policy-bound facet within
the budget, it fails closed instead of truncating or silently widening storage.
The owner retains the existing inspect/export/edit/reset controls for canonical
self data, but the weekly pass itself never surfaces a prompt, proposal or
notification. Until this silent-compaction publication boundary is implemented
and qualified, the shipped job remains `unavailable`.

Activating this weekly job does not activate interactive Walter review,
`agent_orchestration`, Darwin maintenance or any memory job.

## Weekly memory dreaming

`memory-weekly` requires its own adapter registration, job activation,
occurrence authority, workspace-scoped snapshot and qualification even if it
uses the same native runtime or model provider as Walter.

Its high-watermark covers only sanitized, policy-eligible L1 evidence for one
workspace and the exact prior active L2/L3/lifetime manifest. Raw client
documents, prompts, conversations, credentials and records above the watermark
are excluded. The model may propose staged L2/L3/lifetime candidates; it cannot
declare provenance valid, change eligibility policy or approve lifetime
promotion.

Deterministic memory validators own provenance, retention, eligibility, layer
budgets and the one atomic commit manifest. Without an approved lifetime
eligibility policy, lifetime activation fails closed. Success is the validated
atomic memory commit required by Spec 006, not model completion or a staged
candidate. Walter activation has no effect on this job, and memory activation
does not grant Walter access to workspace memory.

## Explicit non-goals

This specification does not:

- implement, register, qualify or activate a model adapter;
- choose Claude, Codex, a provider, model, API, network route or billing plan;
- authorize BCGOS to read runtime credentials or reuse private-release custody;
- enable native schedulers, hooks, unattended execution or a global model flag;
- make one runtime's or job's evidence qualify another;
- grant browsing, tools, external publication, policy mutation or cross-scope
  data access to a model;
- let Walter rewrite canonical Owner Context or let memory dreaming ingest raw
  prompts/client content or bypass lifetime eligibility;
- treat a model response, hook receipt, wake receipt or adapter configuration as
  durable job success.
