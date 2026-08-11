# Proposal 007 — Owner atlas operations

**Status:** request for decision. Specifies the preferred transactional
operation set; ships no segment, skill or runtime capability.

**Original contribution:** Marcelo Petrof Sanches. Refined against the current
advisory-runtime and owner-sovereignty contracts.

**Depends on:** Proposal 006, Spec 013 and Spec 014.

## Objective

Give attended skills and previously authorized rituals a reliable way to read
and maintain `atlas/owner/` without turning the command layer into a gate on the
owner's own Markdown.

The owner may always edit the private source directly. Named operations are the
preferred path when the system needs idempotency, provenance, scheduling,
conflict detection or recovery.

## Operating model

The generative/native layer understands the request, selects relevant context
and proposes content. The mechanical layer validates the target and applies the
effect safely:

```text
owner session or authorized ritual
  -> relevance selection and synthesis
  -> named bounded operation
  -> atomic local effect plus provenance
```

An unavailable operation degrades automation and managed persistence. It does
not block conversation, read-only reasoning, direct owner edits or preparation
of a reviewable draft. The runtime must distinguish "proposed" from "written".

## Operation set

| Operation | Behaviour | Repeat behaviour |
| --- | --- | --- |
| `collect` | Return a bounded, purpose-declared projection of named pages or a segment index | Read-only |
| `create-page` | Create a free-form page or optionally start from a versioned template | Existing page is preserved |
| `append-entry` | Append a bounded entry under a stable section | Same idempotency key produces one entry |
| `set-field` | Set a managed field without rewriting hand-authored prose | Revision conflict returns a reviewable proposal |
| `link` | Add a typed pointer or reference | Duplicate link is a no-op |
| `archive` | Move a page out of active navigation without deleting its body | Repeated archive is a no-op |
| `restore` | Restore an archived page when the target is unoccupied | Existing target produces a conflict |
| `redact` | Remove a selected body or field and retain metadata-only provenance | Redacted content is not retained in the receipt |
| `delete` | Permanently remove an owner-selected page after explicit confirmation | Missing page is an idempotent no-op |
| `export` | Produce an owner-requested local export with declared scope | Does not publish or transmit externally |

Segments and templates improve navigation but are not required for the owner to
create private content. A future managed ritual may restrict itself to declared
segments; that restriction belongs to the ritual's grant, not to the owner's
root.

## Reader and purpose rules

Every `collect` call declares its purpose and intended reader. The result is the
smallest useful projection:

- the owner session may request a named page or bounded index;
- Maestro may receive task-relevant owner context;
- Walter may receive a stale-checked self-proxy projection;
- Case, Client Account and PA Expert agents receive only an explicitly
  authorized attenuated excerpt or pointer;
- no caller receives an implicit whole-root dump.

Reader selection follows `native_advisory`; it does not create a new role edge,
tool grant or effect authority.

## Manual and scheduled authority

An attended operation uses the owner's current request. A scheduled occurrence
uses a standing grant that binds:

- ritual and version;
- segment or page family;
- allowed operation set;
- cadence and catch-up policy;
- reader and retention policy;
- creation, expiry, pause and revocation metadata.

Scheduled and manual execution share the same idempotency rules. A scheduler may
wake the ritual, but it does not invent content authority. Every occurrence
returns evidence or remains honestly pending/failed.

## Transaction and conflict contract

Each managed write carries:

- stable owner-root, page and operation identifiers;
- an idempotency key scoped to ritual or attended request;
- expected source revision or content digest;
- bounded input size and declared target section/field when applicable;
- provenance identifying request or standing grant, session/occurrence and
  timestamp;
- an atomic journaled transition and terminal result.

Path resolution is canonical and descriptor-anchored/no-follow. Unknown roots,
path traversal, symlink escapes and cross-root writes are rejected. Concurrent
or hand-edited changes fail as conflicts rather than being overwritten.

Receipts retain metadata and digests, not deleted or redacted bodies. A permanent
delete requires an explicit owner confirmation and propagates to derived local
indexes or projections; it does not silently delete workspace sources.

## Failure posture

Hard failure is reserved for the requested effect when its target is ambiguous,
outside the owner root, destructive without confirmation, concurrently changed
or mechanically unsafe. Failure of telemetry, native qualification, optional
context or scheduling does not block the rest of the owner session.

If the adapter is unavailable:

- direct owner editing remains available;
- read-only conversation continues with the context already authorized and
  available to the host;
- a draft may be prepared for review;
- no write, schedule or receipt is fabricated.

## Consequences

- Owner-scoped rituals can converge manual and scheduled use on one reliable
  operation surface.
- Free-form owner knowledge and managed fields can coexist without one
  overwriting the other.
- Workspace/client writers require a separate proposal and do not inherit these
  grants.
- Implementation remains pending until the identifiers, registry, journal,
  path boundary and operation tests above exist.

## Explicit non-decisions

- no atlas segment, template, page, skill or ritual is installed;
- no schedule or standing grant is created;
- no workspace, client account, managed or credential root becomes writable;
- no memory eligibility or Owner Context promotion rule is changed;
- no runtime capability is reported available by merging this proposal.
