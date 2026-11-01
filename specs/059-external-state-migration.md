# Spec 059 — Explicit external-state migration

Status: accepted for construction under the complete Maestro instruction.
Scope: E2b of the complete product MCP roadmap, following Spec058. Source
construction does not authorize a migration of real owner data or qualify a
native client, Windows ownership, signing or distribution.

## Observable result

An operator reviews an immutable plan for a supported existing ZIP, applies it
explicitly, restarts the canonical resolver and uses authored state in private
HOME with the managed product read-only. Repetition never recopies old state over
new work. Interrupted operations resume the same plan or refuse an actual
conflict without overwriting either tree. Rollback preserves new HOME work and
provides a verified handoff to the preserved previous installation.

Existing `zipmigration.Run` and its v1 journal retain their contracts. There is
no default-mode switch, implicit startup migration or arbitrary transform RPC.

## Source and disposition

Seal source layout, roots and inventories in the plan before effects:

- Supported 0.1.11/0.1.12 data layouts use the existing legacy namespace mapping.
- Fresh 0.2 brain and v1-committed 0.2 brain use the current live brain, including
  edits, additions and deletions after the v1 receipt (`target_evolved`). Never
  repopulate it from historical data.
- Original data, brain, v1 control state and retained namespace dependencies stay
  intact. A blind `data → data.migrated` rename breaks the preserved committed
  runtime and is not the external migration protocol.

Live authored state goes to HOME/brain. Retained legacy namespaces and historical
machine controls go to HOME/lineage/ATTEMPT outside the active brain. Inspection
reports their disposition and readable retained paths; it does not enroll their
old agents, workspace roots or permissions automatically. Unknown author files
are preserved. Unsupported aliases, special files, ambiguous layouts or portable
name collisions fail before importing content; do not silently drop them.

Archive old `.maestro` authority, lifecycle/capture evidence and keys, dispatch
control, scheduler enrollment/preauthorization, prepared updates, host bindings
and pending daily/dream controls outside active reader paths. Root-free JSON or
a still-valid HMAC does not authorize relocation. Pending work is preserved with
an explicit inactive/reconfirmation status; automatic daily-group rebase is not
delivered by copying or rewriting a single root field. Existing owning workflows
must create fresh authority. Do not erase pending work or call it replayed.

Owner controls include the exact `owner/registry.json` and
`owner/observations/observations.jsonl` files and the `owner/interview/` and
`owner/refinement/` trees, including portable spelling variants. Their root-free
producer grants, pending approvals, interview confirmations and corroboration
are historical evidence after relocation. Preserve SELF bodies, authored
observation notes, prompt history and work state as authored content; do not
archive the entire owner tree. Plans from older engineering candidates that
classified these controls as live fail validation and must be rebuilt before
application. No compatibility exception may reactivate their old authority.

## Four explicit operations

Keep `PlanExternal`, `ApplyExternal`, `InspectExternal` and `RollbackExternal` in
`internal/zipmigration`, using small typed values. Preserve the one-way dependency
`datahome → zipmigration`; migration code must not import datahome or zipruntime.
Use existing confined filesystem and namespace helpers rather than a generic
bound-session framework or executable transform callbacks.

The plan binds clean physical SourceRoot, ProductRoot, UserHome and selected HOME,
source layout and fingerprints, every destination/disposition, prior locator and
empty-home state, and a random 32-lowercase-hex attempt ID. HOME must not overlap
SOURCE/PRODUCT or contain UserHome. SourceRoot and ProductRoot may be the same
installation. Validate the complete write set: locator and atomic temporary
records under UserHome must also stay outside SOURCE/PRODUCT, even for a custom
HOME elsewhere. The normal UserHome ancestor of an installation is allowed.
The fixed UserHome profile-control names `.maestro-home.json` and
`.maestro-external-migration` are reserved under portable NFC/casefold spelling
for HOME and archive selection, including when neither path exists. This bounded
reservation also applies on case-sensitive volumes; other root names retain
actual filesystem identity semantics.
Plan discovery is read-only; an explicit CLI output artifact is
separate from changing SOURCE, HOME or locator authority.

The immutable plan lives at
`HOME/brain/.maestro/external-migration/ATTEMPT/plan.json`; its exact bytes are
bound by the 64-hex digest in the existing closed origin record. Derive SOURCE
from this fixed, validated location during ordinary resolution. Do not add a
caller-selected source root, tree search or extra fields to origin/locator v1.
Journal/commit records share this attempt namespace and bind the same plan.
Private, bounded, exact-schema, duplicate-key/alias-refusing reads are required.

`ApplyExternal` requires the reviewed plan digest. It validates identities and
conflicts again before effects, then publishes durable pending evidence and the
concordant locator/origin before copying. `origin=migrated` alone is never active:
only a complete validated commit can admit writes. Copy/verify every planned
entry without following aliases or overwriting unrelated content; recheck source
stability before activation. Publish owned records atomically. Never mutate the
original or use chmod/repair to make an ambiguous installation pass.

Progress reports closed phase/count metadata, never content or arbitrary paths.
A caller can stop after a durable phase; rerunning must converge on the same
attempt. Keep interruption observation proportional and useful to installer UI;
do not add a production bypass or hidden environment switch for tests.

After commit, original source and authored target may evolve independently.
Repetition reports already migrated and never recopies. Admission validates the
self-contained immutable proof, current private ownership and selected binding;
it must not require an old ZIP directory to remain online or rehash mutable
authored state as though it were immutable evidence.

## Canonical admission and fencing

Extend datahome's existing strict origin/locator checks with the read-only
external inspector. Pending, revoked, incomplete or tampered proof is nonwritable.
Fresh initialization remains fresh-only and cannot adopt a pending migration.
Windows mutation remains unavailable until native ownership/confinement is
implemented and qualified; preserve refusal before the first write.

Expose only validated migration source identity to the existing runtime-path
decision. A verified migrated HOME may coexist with its deliberately preserved
legacy source at the same ProductRoot. That retained source is not an unrelated
dual-home conflict and must not be re-imported or revalidated as the live v1 tree.
An unrelated populated product/legacy tree still blocks. Explicit DataRoot and
case/workspace APIs retain their meanings.

Final helpers and MCP callers must check authority before effects. Explicit MCP
workspace initialization resolves admitted migrated state, bootstraps only fresh
unconfirmed state, and then enrolls the chosen workspace without replacing its
migrated origin. Invalid or incomplete proof never creates a workspace. Already issued
ZIP scripts that never inspect external authority cannot acquire this fence by
a marker they do not read. State exact supported versions; retain the native
overlap/old-client gate. This is not a same-UID filesystem sandbox.

## Recoverable rollback

Validate the original installation's supported readability before revoking a
committed HOME. Preserve post-migration edits by atomically archiving HOME into
the same parent's `.maestro-rollback-ATTEMPT` directory, then restore only the
locator/empty-home state owned by this attempt. The archive destination and its
private writable parent must be validated in the plan; never replace an existing
archive or another attempt's locator. Resume a crash between revocation, rename
and locator restoration using the same sealed attempt in HOME or that exact
archive. Reads/status never repair it. After a move, status may inspect only the exact
derived archive with matching sealed attempt/digest and revoked proof, remains
noncurrent, and returns retained paths rooted at their actual archived location.

The archive is inactive, not a newly admitted HOME. Current clients lose writing
authority; the preserved legacy installation becomes selectable again without
discarding either authored history. If the original is missing or conflicts,
refuse before revocation and retain the active HOME. Report the handoff and
retained work honestly: a file transaction alone does not prove a native runtime
was restored. An inherited `MAESTRO_HOME` override is not changed by restoring a
locator: report the binding as pending until a read-only check of the actual
selection succeeds. Do not change the parent environment or label filesystem
handoff as restored selection/runtime. Real owner reconciliation and native
acceptance remain explicit.

Publish a deny-only retirement record at the fixed UserHome control path
`.maestro-external-migration/retired/SHA256-CLEAN-HOME.json` before removing the
owned locator. It binds selected HOME, attempt/digest and the exact revoked
archive. `RetiredExternalHome(userHome, home)` is a bounded read-only predicate:
absent means false, valid retired archive means true, malformed matching evidence
fails closed. It never returns source authority, admits writes, scans attempts
or repairs state. This distinguishes a known revoked explicit environment
selection from a genuinely fresh custom HOME; the latter retains explicit fresh
setup. Default legacy fallback and a separately admitted fresh origin retain
their own contracts. Include this record and its staging path in the write set.

## Public acceptance

1. Supported data/brain layouts preserve every authored byte, hidden/empty entry
   and original tree; edited/new/deleted live brain fixtures stay edited/new/deleted.
2. Dry plan/status preserve both roots. Populated/foreign HOME, marker mismatch,
   overlap, aliases, special files, collisions and stale plans refuse before effects.
3. Stop at durable phases/copy progress, restart the public operation, converge
   without loss/duplication; conflicting partial content remains untouched.
4. Origin/locator/plan/journal/commit tampering or revocation never admits effects.
5. Restarted datahome/helper admission works with readonly product, same-source
   retained legacy state and a new product path; old controls remain inactive.
6. Target edits survive repeat. Rollback and interrupted rollback retain them,
   restore owned locator state and permit the previous supported local workflow
   after the actual binding is verified. Explicit environment override and
   UserHome/locator write-overlap cases have independent public oracles.
7. Original v1 migration, namespace-local readability, Daily typed refusal/recovery
   and workspace/case invariants stay green. Mutation refusal remains on Windows.

Use synthetic fixtures/public APIs and focused local tests. Archived-package and
full CI evidence are distinct from real installation, native/model qualification,
signing, publication and pilot rollout. Issues #433/#434 stay open until their
complete source and external acceptance criteria are evidenced.

Operator reference: [external-state migration runbook](../docs/external-state-migration.md).
