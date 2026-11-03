# Distribute signed Maestro generations through a synced folder

This guide covers the signed-source publisher, participant update and compatible
rollback interfaces. Local engineering evidence, independent source review,
production release, native-client installation and SharePoint publication are
separate gates, recorded by the delivery evidence. The
[delivery plan](plans/2026-11-03-synced-product-distribution.md) and
[release contract](../specs/020-release-distribution.md) define those boundaries.

The publisher places immutable signed releases and generation records in a
folder. SharePoint/OneDrive replicates those bytes. Each participant's admitted
local installer authenticates the source, copies a complete release into private
local staging, and runs the canonical reviewed installation transaction. The
MCP remains a local stdio process; this distribution path creates no network MCP
endpoint or new hosted service.

## Publish an immutable generation

Prepare a complete modern release using the
[first-install factory](product-install.md). Complete the independently admitted
native installer and exact-byte release signing with the approved organization
authority. A source-aware release must bind `maestro-synced-source-v1` in both
its authenticated server component and the actual verified executable identity.
A version label, a valid signature on an older server or an edited metadata
field cannot establish that execution capability.

The generation is a closed JSON object conforming to
[release-generation.schema.json](../schemas/release-generation.schema.json).
It names one stable source ID, channel and exact OS/architecture; a positive
monotonic sequence; publication and expiry times; the exact release, manifest
digest and client-input digest; an explicit recall array; and the minimum reader
contract. It contains no participant path, credential, bootstrap public key or
replacement authority registry.

Publication and expiry are explicit canonical UTC timestamps at second
precision. Lifetime must be positive, at most 30 days, and within the admitted
signing key's validity. That limit is an engineering contract, not an
organization availability SLA. Choose the operating publication cadence under
the channel's actual approved policy; there is no implicit seven-day promise.

The factory publisher accepts the following command from the repository root:

```sh
go run ./dev/release publish-generation \
  --source-directory /publisher/local/channel \
  --release-directory /publisher/local/signed-release \
  --authority-registry /approved/public-authority.json \
  --generation /publisher/local/generation.json
```

The command requires the approved signing seed on standard input. Connect that
input only through the organization's approved signing custody. Do not put a
seed in the generation JSON, shell arguments, repository, synced directory,
participant packets or review evidence. The source implementation's input seam
does not itself establish production signing custody. There is no command-line
clock or unsigned-trust override.

The resulting folder uses these exact names:

```text
channel/
  current.json
  current.json.sig
  generations/
    <sequence>-<generation-body-sha256>/
      generation.json
      generation.json.sig
  releases/
    <release>/
      <os>-<arch>/
        <complete authenticated release files>
```

The publisher closes the immutable release and generation before promoting the
current signature and current body last. Current and immutable generation pairs
must contain identical authenticated bytes. An interrupted or partially synced
pair refuses; participants do not combine one generation's body with another
signature or execute a file directly from the share. An identical complete retry
is idempotent. Reusing a sequence with another body or rewriting an immutable
release is refused.

A later generation may point to the same unchanged release and client inputs
to refresh signed freshness or distribute a recall. Release identity and source
metadata sequence serve different purposes. A metadata-only refresh does not
invalidate a carrier whose active release binding is unchanged.

Exact generation JSON and source-aware manifest bodies are limited to 64 KiB
each before admission. Whitespace is part of those signed bytes and counts
toward the limit; signatures are not transferred onto normalized JSON. These
finite supported envelopes leave room for current/previous bindings and recall
custody under the private install-state budget. An oversized signed envelope
is refused as unsupported before acceptance, rather than losing a later denial
because it could not be persisted. Historical release profiles retain their
own bounds.

## Keep participant authority local

The participant enrolls an exact source tuple and local synced-folder locator
through a trusted operator action. A URL, an authenticated OneDrive account or
the model's choice of a directory cannot enroll a source. No SharePoint library
or endpoint has been selected for the team yet. The local implementation must
not guess it or obtain cloud credentials.

Private installation custody stays outside the synced share, managed payload
and generated client packets. Source-aware install state retains the accepted
sequence, exact signed metadata, cumulative known recalls, reader floor and last
accepted observation time outside the rollback payload. Confirmation binds the
actual review and current state before those declared custody/cache effects.
Read-only startup and status do not silently refresh that authority.

The current owning implementation uses these fixed participant-local locations:

| Purpose | Location beneath the selected user home |
| --- | --- |
| Install state and enrolled source custody | `.maestro-install/config/install-state.json`, source-aware schema 4 |
| Complete verified candidate snapshots | `.maestro-install/updates/synced-<generation-sha256>/` |
| Pending reviewed update | `.maestro-install/config/pending-update.json`, source-aware schema 2 |
| Canonical activation plan | `.maestro-install/updates/tx-*/activation-plan.json`, source-aware schema 4 |

These paths belong to the existing installer owners. They are not a second
queue or an invitation to edit JSON by hand. The source-aware transaction
receipt uses schema 3; historical legacy variants retain their own contracts.

An authenticated higher generation retains its denial information even when
it has expired or its payload is unavailable. The installer then refuses any
freshness-dependent action. A known recall cannot disappear by selecting an
older fresh generation, rolling back payload, reconstructing a plan or
reconciling an interrupted transaction. Invalid authentication, scope or shape
does not gain custody authority.

The finite recall store also fails closed when full. A confirmed authentic
higher generation that would exceed its capacity retains the higher signed
record and a sticky `recall_capacity_exhausted` denial while preserving the
bounded previous list. Launch, candidate use and rollback then refuse. Restart,
an older fresh generation or another higher generation cannot clear that
denial. Keep the private state for admitted installer diagnosis; do not truncate
the list or edit the marker. Payload use can resume only through a separate
explicit supported repair/migration, which this source task does not provide.

Offline use is bounded by the last accepted authentic generation's signed expiry,
known recalls and reader floor. It cannot promise immediate knowledge of an
unreplicated SharePoint update. The contract uses the trusted local wall clock
and refuses a time earlier than the persisted accepted observation; it does not
claim tamper-proof time or remote revocation of arbitrary old executables.

## Route client execution through the admitted installation

Generated source-aware carriers select the independently admitted installed
launcher, `<managed_root>/maestro-install launch`, with fixed participant
bindings. The launcher checks private source custody, the current installation,
actual executable bytes and capability before starting the server. Packets still
contain their immutable executable/resource closure; carrying those bytes does
not authorize a stale packaged server to bypass the installed launch boundary.

New running servers repeat current source/release checks at request and existing
publication boundaries. A changed active release requires new carriers and a
restart. A blocked MCP must leave the independent installer available for
diagnosis, authenticated update, reconciliation or compatible rollback. These
source controls cannot retroactively make an arbitrary old copied executable
or an already running old binary enforce a new contract.

Code registration, Desktop loading and crash reconciliation remain the next
delivery task. Packet creation, a registration command's exit status or an open
request alone does not establish native activation. Actual client forms, device
policy, supported Windows confinement, organization signing, SharePoint
publication and the two-participant pilot require their own evidence.

A guarded MCP operation that cannot establish current source authority reports
`source_recovery_required` with a bounded `next_action` pointing to the
independent installer. The model-facing result carries no participant paths.
Use the fixed local installation request below to review/refresh source or
diagnose the blocked payload. Startup and status remain read-only; metadata
refresh still requires the actual attended review.

## Participant command runbook

Use the independently admitted installed `maestro-install` executable, with its
adjacent pinned public authority. Windows executable names include `.exe`.
The unsigned development executable cannot grant native admission.

For first source enrollment, create a private local request with an exact folder
and source tuple. Values below are illustrative and must match the actual signed
channel and installed target:

```json
{
  "managed_root": "/local/installed-maestro",
  "user_home": "/local/owner",
  "source_root": "/local/synced/maestro-canary",
  "source": {
    "source_id": "team-pilot",
    "channel": "canary",
    "os": "darwin",
    "arch": "arm64"
  }
}
```

`source-review` authenticates the metadata and reports the exact current state,
candidate and declared custody/snapshot/plan effects without making them:

```sh
/local/installed-maestro/maestro-install source-review < /local/private/source.json
```

Read the complete local review, retain its exact `digest`, and confirm the
identical request only if those effects are intended:

```sh
/local/installed-maestro/maestro-install source-check \
  --review-digest SOURCE_REVIEW_DIGEST --confirm < /local/private/source.json
```

`stage` is an alias for this confirmed source-check path. Do not execute both
as separate approvals. A newer eligible release is copied into private verified
staging and receives a pending `plan_id`; this does not activate it. The
confirmed observation can retain newer recall/sequence custody while refusing
candidate use, including for authentic expired metadata. Such a nonzero result
does not promise that every private custody byte remained unchanged. Inspect
the structured result and preserve the state for diagnosis.

After enrollment, later source review/check may use only the fixed managed root
and user home; the installer resolves the enrolled tuple and locator from
private state. It cannot silently select a different source. Save that smaller
request separately as `/local/private/installation.json`:

```json
{
  "managed_root": "/local/installed-maestro",
  "user_home": "/local/owner"
}
```

Review the actual pending plan, then activate that exact plan only after local
confirmation:

```sh
/local/installed-maestro/maestro-install update-review \
  --plan-id PLAN_ID < /local/private/installation.json
/local/installed-maestro/maestro-install update \
  --plan-id PLAN_ID --confirm < /local/private/installation.json
```

The plan binds current installation state, source generation and candidate
manifest/client inputs. Staging, confirmation and activation reauthenticate
those inputs; a stored approval boolean cannot substitute for current truth.
Activation does not prove Code/Desktop registration. Regenerating and activating
the current participant packets is tracked separately by the next delivery task.

After interruption, preserve the exact pending plan and use canonical
reconciliation rather than deleting transaction files or rerunning setup:

```sh
/local/installed-maestro/maestro-install reconcile \
  --plan-id PLAN_ID --confirm < /local/private/installation.json
```

Rollback is another reviewed operation. First inspect the compatible previous
release, then confirm its exact returned review digest:

```sh
/local/installed-maestro/maestro-install rollback-review < /local/private/installation.json
/local/installed-maestro/maestro-install rollback \
  --review-digest ROLLBACK_REVIEW_DIGEST --confirm < /local/private/installation.json
```

The previous payload must be authentic, source-aware, compatible and not
recalled. Rollback does not restore earlier source custody, remove known recalls,
lower the accepted sequence/floor or make expired metadata current. Historical
schema-3 installations without authenticated source-aware executable bindings
receive an explicit migration-unavailable refusal rather than an inferred
upgrade from version strings.

For model-assisted diagnosis, `maintenance.qualify` for job `update-check`
requires the existing trusted attended review and real confirmed source/plan
effect. It creates no recurring enrollment and activates no update. A model's
approval flag, a timestamp or a synthetic report cannot qualify that job. Use
the independent installer commands above when MCP launch or requests are blocked.

Repeating an unchanged candidate check returns `reviewed_no_change` and retains
the existing genuine qualification. A new observation time may change the exact
pending plan ID; that alone is not a fresh check qualification. The report keeps
its stored plan identity and marks it historical when stale. A separate bounded
`current_pending` field, when available, is verified against the actual current
pending plan. Review that current plan explicitly before activation; a historical
diagnosis or qualification receipt does not confirm it.

The independent installer's own admitted authority also permits source
diagnosis/refresh when the active MCP file is missing or corrupt. This does not
make that damaged payload a verified rollback backup. The standard update path
preserves its verified-backup requirement: damaged-payload replacement reports
an explicit repair-unavailable result until a separate reviewed repair variant
exists. Preserve those bytes and owner state for diagnosis; do not delete them,
quarantine automatically or supply a replacement trust registry to force update.
