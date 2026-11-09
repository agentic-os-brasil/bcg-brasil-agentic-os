# Scoped workspace knowledge operations

The canonical `internal/atlasops` API provides named operations over authored
owner pages or the `brain/` of one registered workspace. It preserves the
separation between authored knowledge, private generated memory and managed
product assets. This source capability is the owning prerequisite for later
MCP document and attended-page mappings; it does not register new MCP operations
or install a native client capability by itself.

## Open the owning scope

`Open(dataRoot, now)` retains the existing owner-corpus interface.
`OpenWorkspace(atlas.Options)` requires the canonical data root, exact registered
workspace path and workspace ID. It opens an existing workspace and brain,
validates registration and the no-follow metadata boundary, and creates no page.
A caller cannot substitute another workspace or inherit an owner grant.

The engine revalidates the selected directories, registration and control scope
before reads and effects. Roots come from trusted local admission; a product
adapter must not expose these constructor arguments as arbitrary model-selected
filesystem roots. `Root()` is local API metadata, not a grant to read a corpus.

## Named operations and outcomes

| Public method | Observable contract |
| --- | --- |
| `Collect` | Read explicitly named pages for a declared purpose and reader. Missing pages become omissions; malformed or escaping paths fail. |
| `CreatePage` | Create an absent page with provenance. An occupied page remains intact. |
| `AppendEntry` | Add one bounded entry under a named existing section. The same operation key does not duplicate it. |
| `SetField` | Replace only the named unambiguous managed field; preserve surrounding authored prose. |
| `Link` | Add a workspace-relative reference under a named section. An existing matching link is unchanged. |
| Grant methods | Create, inspect, pause, resume and permanently revoke scoped operation grants; expiry is checked at use time. |

Writes return `written`, `unchanged` or a reviewable `proposed` result. When an
adapter is applying previously reviewed content, carry the returned revision in
`ExpectedRevision`; a changed body produces a proposal instead of replacement.
Every write carries a stable idempotency key and attended session provenance or
an existing standing-grant occurrence. Model assertions do not establish the
attended request or standing authority.

Collection is bounded to 24 named pages and 128 KiB total body content. A page
is bounded to 256 KiB and an appended entry to 8 KiB. Delegated collection needs
explicit authorization and returns attenuated pointers instead of page bodies.
A request without named pages is refused.

## Publication, grants and recovery

One corpus mutation lock orders grant changes with page effects. Grants bind
scope, page-family path components and operation sets; a copied owner/workspace
grant cannot authorize another scope. The registry records cadence metadata,
but `Scheduled=false`: this API does not wake rituals automatically.

The journal binds the scope, operation, request digest and before/after page
revisions. After an interrupted publication, reopen the same admitted scope and
retry the exact request/key. An already applied effect is recognized without
repeating it. A changed request or newer page remains a conflict; do not remove
control files or overwrite a newer page to force recovery. Historical owner
receipts remain readable through their supported compatibility path.

On admitted Unix systems, the publisher traverses no-follow directories and
keeps the publication directory descriptor through atomic replacement or
exclusive creation. Exclusive creation uses the shared no-replacement primitive
and leaves one readable link, including after the tested process-exit boundary.
Existing owner-authored page directories retain their supported ordinary modes;
owner roots, controls and workspace page directories retain private admission.

## Qualification boundary

The focused source contracts cover cross-workspace scope, alias/hardlink and
occupied-target refusal, stale revision proposals, concurrent appends and grant
revocation, exact replay and process-exit recovery. Executing those contracts,
hosted checks, native clients and actual power-loss durability are separate
forms of evidence; the existence of a test is not its observed result.

New workspace mutations on Windows explicitly return unavailable until native
ownership/confinement publication is qualified. Existing owner behavior is
preserved. No Windows mutation bypass, archive/restore/redact/delete/export
operation, generated-wiki publication, scheduler activation, external send or
client rollout is introduced by this capability. Strong hostile same-account
filesystem custody remains a separate qualification requirement.

See [Proposal 007](proposals/007-owner-atlas-operations/README.md),
[Spec 007](../specs/007-content-navigation.md),
[Spec 008](../specs/008-wiki-update-okf.md) and issue #484 for the governing scope.
