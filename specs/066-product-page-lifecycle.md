# Spec 066 — Reviewed authored-page lifecycle

Status: accepted for construction in epic #460, child #462. Extends BRPG and
Spec064; local/source/native/client evidence remain distinct.

## Result and interface

Keep the three product tools. `brain.move` and `brain.retire` are closed,
attended `maestro_apply` variants requiring a selected page and exact current
content revision and `expected_lifecycle_revision` from read/status. The latter
is an intention/ABA fence included in the request digest, never consent. Move also names an absent destination. Retire always archives;
there is no delete mode or caller-selected archive root. Exact host review shows
the complete current/proposed page, stable identity, current/destination
locations, destination absence, scope/classification changes and affected
references within the currently admitted corpus. Refuse an incomplete bounded
reference analysis inside that corpus; excluded cases are not read or claimed.

The initial authored `id` remains immutable when its location changes. A bound
canonical lifecycle record authorizes current location and reserved former-name
aliases. An alias must pass admission of the current location before disclosure.
Alias keys normalize Unicode NFC and reserve names without differentiating
casing; preserve the original
spelling and ID. A pre-existing distinct-file case collision in the admitted
corpus refuses lifecycle and preserves existing reads/work. New identities
cannot take a reserved name. The registered README anchor and equivalent
aliases cannot move or retire. New-page validation stays strict;
moved-page validation uses only its registered stable identity. Scope follows
the admitted location: exact reviewed metadata may narrow to the active case,
preserving or raising sensitivity. Widening or cross-case/account relocation is
refused. Preserve optional fields, prose and LF/CRLF bytes outside reviewed edits.

Archive retains the selected file under the owning private machine namespace
`brain/.atlasops/retired/`, with a server-derived leaf. That namespace remains
unaddressable through page, collect and converted-source routes. Physical
cross-filesystem moves are unavailable; occupied targets are never overwritten.

## Transaction and visibility

The existing atlasops engine owns binding, corpus lock, review, journal,
revision checks, native publication and recovery. Lifecycle state is bounded
private metadata, not a second transaction runner or a store of draft bodies.
Before the first page effect, durably publish the lifecycle barrier and its
invalidation revision. Pending affected identities/locations and retired pages
are unavailable to every canonical reader; queries never complete a transaction.
Publish a terminal receipt only after source, destination/archive and lifecycle
effects are durable and mutually consistent.

An explicit reviewed retry binds the same request and checks the exact
before/intermediate/after states and current scope. It resumes/finalizes only
its own effect, refuses a different request under the key and preserves newer
source/destination bytes. No rollback, permission repair or authority flag is
inferred from a receipt. Public incomplete/conflict results provide metadata and
the next attended action without page bodies or private filesystem errors.

Existing brain mutations, named knowledge collection, link targets and replay
must honor current lifecycle admission. Converted-document sources remain
physical locations: an alias or retained artifact cannot renew source authority
after movement/retirement, including after external recreation of an old name.
Authored derived views must bind their source cohort to the lifecycle revision
and recheck visibility before disclosure, including old generations. Private
memory retains its separate source authority; authored archive does not claim
to revoke independently reviewed captures.

## Native publication and evidence

Reuse held no-follow parent directories, canonical exclusive native rename and
the existing Windows private-file owner. Preserve existing primitive callers.
The source leaf and both parents remain admitted through the namespace effect;
validate the selected source revision while held, never clobber a destination,
and sync both parents. Aliases, hardlinks, replaced ancestors, sharing conflicts
or unsupported filesystems preserve existing/outside bytes.

Public SDK and actual shipping-server tests cover create/move/restart/read by
stable identity, exact review, stale/occupied/declined/scope refusal and archive
visibility through reads/collect/converted sources/retries. Canonical engine
tests interrupt each durable phase and demonstrate exact-once recovery with
newer-file preservation. Actual ordinary-user Windows and Mac witnesses cover
the same publication/recovery seam. Source, cross-build and SDK acceptance do
not establish native client loading or final participant-kit qualification.

Converted artifacts from authored Markdown capture `source_lifecycle_revision`
before conversion and retain it through publication, read, review and replay.
A missing field is a legacy artifact admitted only while the workspace lifecycle
revision is zero. Ordinary non-authored document sources keep physical source
admission; this fence does not revoke independently reviewed knowledge/memory.
