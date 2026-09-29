# ZIP workspace migration v1

The sole engine is internal/zipmigration, exposed by the packaged command:

    maestro-runtime migration --project ROOT

Windows and macOS execute the same Go implementation. The old Python filename
is only a version/hash-verified compatibility launcher, with no migration logic.

## Invocation and state

Optional mutually exclusive modes: --dry-run, --status, --rollback ATTEMPT,
and --resolve-legacy agents|workspaces|canary.

The JSON result contains schema_version (1), state, attempt_id,
source_fingerprint, target_fingerprint, plan_sha256, errors (array), legacy_map
(object), restored_runtime (false), resumable, current and verification.
Absent digests and attempt IDs are empty strings.

States: not_needed, planned, partial, blocked, committed, rolled_back.
Exit 0 means committed, not_needed, explicitly revoked rolled_back, or a dry-run
planned. Other states exit 2. Hooks check state as well as exit code: rolled_back
never permits initialization.

Status and legacy resolution are read-only. A committed receipt is immutable,
point-in-time evidence. Current verification is separate:

- committed + current=true + verification=current: the original and planned
  targets still match the receipt.
- committed + current=false + verification=target_evolved: the authenticated
  local receipt remains historical evidence, but target bytes have since changed.
  Normal initialized operation remains usable and never recopies or overwrites
  authored work. This result must not be reused as a fresh update PASS.
  Only ordinary regular-file edits, additions and deletions qualify as this
  evolution. The complete target tree is scanned before content comparison;
  symlinks, changed file/directory types (including implicit mapped ancestors),
  unreadable files/directories and IO failures remain blocked.
- verification=legacy_current: only the requested retained namespace was freshly
  validated; unrelated authored brain edits do not block this read adapter.
- blocked/unverified: source drift, malformed plan/receipt, symlinks, unsafe
  paths or a pending/partial destination conflict require inspection.

Here authenticated local receipt means bound by plan hash, exact installation
path, attempt ID and consistent state/receipt fields; it is not an organizational
signature or a defense against an owner rewriting every authority file.

## Durable transaction

State resides outside the original data tree:

    brain/.maestro/migration/state.json
    brain/.maestro/migration/attempts/ID/plan.json
    brain/.maestro/migration/attempts/ID/receipt.json

Each attempt has a cryptographically random, 32-character lowercase hexadecimal
ID. The immutable plan has schema_version, attempt_id, absolute project,
source_fingerprint, legacy_map and entries. Each entry has source, target, kind
(file|directory), action (copy|retain), sha256, target_sha256 and transform (empty
or legacy_case_marker). Empty target means retained. Every original file and
directory, including hidden entries and empty directories, is accounted for.
Duplicate files mapping to one destination block before copying; collapsing
obsolete directory containers remains explicit in the plan.

The plan digest is SHA256 over exact UTF-8 plan bytes. Inventory fingerprints
represent each entry as this ASCII line:

    BASE64(UTF8(relative POSIX path)) TAB kind TAB sha256-or-dash LF

Sort complete lines in ordinal ASCII order, concatenate and SHA256 the bytes.
Digests use lowercase hexadecimal. Source fingerprint covers the full original
inventory. Target fingerprint covers unique planned copy destinations. Retained
source entries remain represented in the original inventory.

Paths retain filesystem spelling; collision keys use Unicode NFC plus full case
folding through the same Go dependency on every platform. Windows reserved
names, trailing dots/spaces, colon and unsafe separators block portably.
Symlinks and special files block rather than being followed.

All effects use Go os.Root confinement. File copies stage under migration
control, fsync and rehash, then hard-link to atomically create a previously absent
destination. A file is never overwritten. Interrupted staging cannot produce a
half-written target. Filesystems without safe hard-link support return visible
partial failure; there is no unsafe overwrite fallback.

Only after full source and destination validation are the immutable receipt and
committed state written. The original data tree is never written, including its
historical completion marker. Partial copies resume under the same immutable
attempt only if their source and existing owned outputs match. Unexpected new
target content, source changes or tampering of pending targets block resumption.

## Retained legacy consumers

Agents, workspaces and canary retain their original namespaces. After commit:

    maestro-runtime migration --project ROOT --resolve-legacy workspaces

returns committed, verification=legacy_current, namespace=workspaces and a
validated absolute path to ROOT/data/workspaces. A consumer reads ordinary
relative context below that path. It must never guess brain/workspaces, presume
the namespace was empty, or use a path from an unvalidated old receipt.

The adapter is read-only. It verifies requested namespace hashes against the
immutable inventory before returning its root. Acceptance tests resolve and read
real retained content, preserve access after unrelated brain edits, and reject
changed retained content. SessionStart surfaces the live map and doctor uses
this contract. Retained README/control entries remain named in the plan.
Any retained content means the original data folder must remain available.

## Initialization, health and revocation

The Bash migration gate runs before any logging, recovery, backfill or
initialized fast path whenever original data or migration state exists.
Missing/tampered helper or nonterminal/revoked migration leaves initialization
pending and surfaces a diagnostic. Fresh workspaces without legacy data take
the normal scaffold path.

The shared Bash adapter strictly parses the manifest using native macOS
Foundation, verifies version, exact artifact selection/path and SHA256, and
rejects managed symlinks. The Go binary is likewise the native Windows
PowerShell adapter's migration authority. Native runtime qualification is a
separate gate, not implied by a direct helper invocation.

Doctor recomputes status instead of trusting initialized or old reports.
Target evolution after a terminal commit is informational for ordinary work;
qualification must obtain new evidence rather than reuse the historical PASS.
Such a new qualification may reference the historical committed attempt together
with fresh final workspace/runtime checks. It need not force authored targets
back to their old hashes or rewrite the historical receipt.

A temporary unsafe finding is returned as blocked but never overwrites the
committed state/receipt. Repeated SessionStart calls remain blocked while the
unsafe path exists and can resume after it is repaired. Partial, nonterminal
attempts retain their stricter destination-conflict rules.

UTF-8 BOM on legacy active/pending markers is removed only from the interpreted
case identifier. Original source bytes and their inventory hashes remain intact.

Rollback is revocation only. It writes rolled_back and restored_runtime=false,
preserves every original and copied file including newer edits, and directs the
owner to reopen the previous untouched installation and validate that runtime.
No runtime or file restoration is claimed. This command does not know the
previous installation's path and never invents it.

## Local verification

    go test ./internal/zipmigration ./cmd/maestro-runtime -count=1
    python3 -m unittest discover -s acceptance/migration -v

Python here is the development fixture runner, never a product prerequisite.
Native macOS checks run on macOS. Cross-directory Go tests exercise case-fold
collisions even when the host filesystem prevents same-directory case aliases.
