# Spec 054 - Anonymized telemetry export and receiver-side verification

Status: proposal. Contract shape, storage layout, anonymization allowlist and
receiver verifier described; the daily hook trigger surface, the darwin skill
body that produces the artifact, the CLI to request/share the bundle and any
transport are deferred to later slices.

## Objective

Give every workspace a bounded, structurally validated telemetry artifact that
is produced by a daily darwin skill, kept on the owner's disk, and that a
receiver can later verify — without trusting the sender — before ingesting it
for cross-workspace analysis. The artifact carries only metadata about agent,
skill and hook activity: identifiers of adapters and skills, coarsened
timings, exit outcomes and opaque window ids. It never carries prose, tool
inputs, tool outputs, prompts, file bodies, absolute paths, owner-identifying
strings or client data.

The pipeline has two ends. The sender end is a daily anonymization pass that
compiles the last window of local telemetry into an append-only artifact under
the workspace boundary. The receiver end is a structural verifier that
re-runs the allowlist before any record is accepted; a bundle that fails the
verifier is refused, never repaired.

## Distinction from existing layers

- Spec 006 (memory persistence) owns workspace-scoped continuity across L1,
  L2, L3 and lifetime. Telemetry export is a projection of activity metadata,
  not a memory layer, and is never injected into any prompt.
- Spec 047 (agent breadcrumbs) tracks a metadata-only, hash-linked tail per
  agent invocation for local recovery. Telemetry export reshapes a bounded
  window of that space plus adjacent hook and skill signals into an
  anonymized artifact designed to leave the workspace when the owner chooses
  to share it. Breadcrumbs are the input space; the export is not a mirror.
- Spec 052 (agent context snapshot) is agent-scoped prose that is injectable
  on next invocation. Telemetry export is workspace-scoped, non-prose, and
  is never injected.
- Spec 053 (knowledge folder manifest) tracks per-path fingerprints per
  analyzer for delta scans. Telemetry export tracks per-invocation metadata
  aggregates and shares no key space with the manifest.
- `internal/darwinobservability` already defines the metadata-only evidence
  contract, `OpaqueWindowID`, `BindEvidenceID`, `DecodeStrict` and
  `InputDigest`. This spec reuses those primitives; it does not reinvent
  them and does not widen `EvidenceAuthority` beyond
  `AuthorityCallerAssertedShadow`.
- `internal/ingest.Fingerprint` and the durable rename pattern from
  `internal/memory` are the atomic-write and hashing primitives; the export
  reuses both.

## Storage

Each workspace carries a bounded telemetry tree at its own root:

```text
<workspace>/telemetry/
  <yyyy-mm-dd>/
    export.jsonl
    export.manifest.json
```

`export.jsonl` is an append-only line-delimited stream of anonymized records
for that UTC day. `export.manifest.json` is the sidecar that carries the
bundle's schema version, canonical digest, record count, window bounds and
the allowlist policy version under which it was produced. Both files are
written through the durable rename pattern already used by `internal/memory`:
the daily pass writes siblings `.tmp`, fsyncs them and renames them into
place. A crash between write and rename leaves the previous day's artifact
untouched.

Retention is a bounded rolling window of the most recent 30 UTC days per
workspace. Older days are removed without ceremony. There is no cross-
workspace aggregation on disk. Telemetry files never leave the user-local
workspace boundary except through an explicit owner-initiated share; there
is no automatic network transport in this slice and none is planned in the
contract.

## Record shape

Every record in `export.jsonl` is a closed union with the same posture as
`darwinobservability.Record`: strict JSON, unknown fields rejected, duplicate
keys rejected. The exportable fields are exactly:

```json
{
  "schema_version": 1,
  "kind": "invocation | hook_fire | skill_fire",
  "opaque_window_id": "win-<sha256_32>",
  "day_utc": "2026-08-14",
  "hour_bucket_utc": 18,
  "adapter_id": "claude | codex | …",
  "runtime": "cli | app | headless",
  "skill_id_hash": "sha256:<hex>",
  "agent_id_hash": "sha256:<hex>",
  "hook_kind": "session_start | pre_tool | post_tool | stop | …",
  "outcome": "ok | error | timeout | refused",
  "exit_code_bucket": "0 | 1-63 | 64-127 | 128+ | signal",
  "duration_bucket_ms": "0-100 | 100-500 | 500-2000 | 2000-10000 | 10000+",
  "input_bytes_bucket": "0 | 1-1k | 1k-10k | 10k-100k | 100k+",
  "output_bytes_bucket": "0 | 1-1k | 1k-10k | 10k-100k | 100k+",
  "policy_version": "telemetry-allowlist@1"
}
```

`opaque_window_id` is produced by `darwinobservability.OpaqueWindowID`
applied to the local invocation identifier. `skill_id_hash` and
`agent_id_hash` are `sha256:` of the canonicalized skill and agent names;
raw names are never emitted. Durations and byte counts are coarsened to the
enumerated buckets before write, never emitted as absolute numbers.

Absent from the record shape, by construction and enforced at the write
path: prompt text, tool inputs, tool outputs, file paths, workspace names,
workspace ids, owner name or email, model responses, error messages,
stack traces, environment variables, hostnames, IPs, ports, urls, and any
free-form string field. There is no `notes`, no `body`, no `context`, no
`extra`, no map-typed field. The record shape is closed.

## Anonymization contract

The write path is the enforcement boundary. The daily pass consumes local
breadcrumbs, hook logs and skill logs, maps each source event to at most one
allowlisted record, and refuses to emit a record whose shape or content
deviates from the allowlist. Structural rules:

- every field is one of the enumerated keys above;
- every string field either matches its documented regex (e.g. `hour_bucket_utc`
  is an integer 0..23; `sha256:` fields match `^sha256:[0-9a-f]{64}$`;
  `opaque_window_id` matches the `win-` prefix pattern from
  `darwinobservability`) or the record is refused;
- every bucket field is one of the enumerated literals; a value outside the
  enumeration refuses the record;
- unknown fields on read refuse the entire bundle via `DecodeStrict`;
- duplicate keys refuse the entire bundle.

Refusal is silent locally in the sense that the day's export excludes the
offending record; a counter in `export.manifest.json` records the refusal
count per policy rule so the receiver can see whether a bundle was
structurally lossy. Refusals are never repaired by best-effort re-shaping.

`policy_version` is a fixed identifier of the allowlist under which the
bundle was produced. A receiver that does not know the policy version
refuses the bundle rather than assuming compatibility.

## Bundle manifest

`export.manifest.json` accompanies each day's `export.jsonl`:

```json
{
  "schema_version": 1,
  "policy_version": "telemetry-allowlist@1",
  "day_utc": "2026-08-14",
  "record_count": 1234,
  "refused_count_by_rule": {
    "unknown_field": 0,
    "non_enumerated_bucket": 3,
    "non_matching_regex": 0
  },
  "input_digest": "sha256:…",
  "content_sha256": "sha256:…",
  "produced_by": {
    "adapter_id": "darwin-telemetry-export",
    "policy_version": "telemetry-allowlist@1"
  }
}
```

`input_digest` reuses `darwinobservability.InputDigest` over the canonicalized
records. `content_sha256` reuses `internal/ingest.Fingerprint` over
`export.jsonl`. The manifest is the primary handle the receiver reads first;
a bundle whose manifest fails validation is refused before any record is
parsed.

## Daily hook

A daily hook fires the darwin telemetry-export skill once per UTC day per
workspace. The hook is idempotent: firing twice on the same UTC day for the
same workspace produces the same `content_sha256` for the day's artifact,
because the record set is bounded to a closed UTC window and the write path
is deterministic. The hook trigger surface (launchd, cron, session start,
sessionhook, or dedicated scheduler) is deferred to a later slice; this
spec fixes only the contract that whatever fires it must satisfy: a UTC
day boundary, a single artifact per day per workspace, no network access,
no cross-workspace read.

## Receiver-side verifier

The receiver is a darwin skill that a maintainer runs against a bundle a
user voluntarily shared. It never trusts the sender. Its checks:

1. `DecodeStrict` on `export.manifest.json`; refuse on unknown field,
   duplicate key or trailing data.
2. Confirm `schema_version` and `policy_version` match a known allowlist.
3. Recompute `content_sha256` over `export.jsonl` and compare with the
   manifest; mismatch refuses the bundle.
4. Stream `export.jsonl` through `DecodeStrict` per line; each record is
   validated against the closed union above and against the regex/bucket
   rules; the first structural failure refuses the entire bundle.
5. Recompute `input_digest` and compare with the manifest.
6. Sweep every string field against a forbidden-content regex set —
   absolute paths, urls, email patterns, credential patterns — and refuse
   on any hit. This sweep is redundant with the write-path enforcement by
   design; the receiver assumes the sender may have been modified.

A bundle that passes all six checks is accepted. A bundle that fails any
check is refused, logged with the failing rule id, and never partially
ingested.

## Non-goals

- No network transport in this slice. Sharing is manual and owner-driven.
- No content export. Prose, tool inputs, tool outputs and file bodies are
  never in scope, and no future adapter may widen this without a new
  spec.
- No cross-workspace aggregation on the sender side.
- No injection into any prompt on either side.
- No delegated write. Only the daily darwin skill running under the owner's
  workspace may write the artifact.
- No widening of `EvidenceAuthority`. The bundle is caller-asserted like
  every other observability record; native provenance would be a separate
  spec.

## Runtime portability

The record shape, manifest shape, allowlist, bucket enumerations, daily hook
contract and receiver checks are runtime-neutral. A Claude adapter and a
Codex adapter must produce byte-identical bundles for byte-identical input
sequences.

## Test expectations for this slice

- structural validation of `export.manifest.json` and every record kind,
  including refusal on unknown field, duplicate key and trailing data;
- refusal of a record with a non-enumerated bucket value;
- refusal of a record with a raw skill name or a raw agent name in place of
  the `sha256:` hash form;
- refusal of a record carrying any field outside the closed union
  (e.g. `notes`, `body`, `context`);
- idempotency of the daily pass: two runs on the same UTC day produce the
  same `content_sha256`;
- receiver refusal on `content_sha256` mismatch;
- receiver refusal on `input_digest` mismatch;
- receiver refusal on a bundle whose `policy_version` is unknown;
- receiver forbidden-content sweep catches an absolute path, an email
  pattern and a url pattern smuggled into a string field, even when the
  sender's write path failed to;
- retention: the 31st daily artifact removes the oldest, leaving 30 on
  disk.

## Open questions deferred to a later slice

- The hook trigger surface (launchd, cron, sessionhook, dedicated
  scheduler) and its failure semantics.
- The darwin skill body that consumes local breadcrumbs, hook logs and
  skill logs and emits the daily artifact.
- The CLI surface for inspecting, exporting and clearing the local
  telemetry tree.
- The receiver skill's UX for reporting refusal rules to a maintainer.
- Any future transport (bundle upload endpoint, signed handoff, etc.) —
  each requires its own spec and does not change this contract.
