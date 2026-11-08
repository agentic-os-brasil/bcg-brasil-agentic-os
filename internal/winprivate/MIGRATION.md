# Windows external-migration kernel

These operations supply filesystem custody for Spec059. They do not enable
external migration, authorize a plan, or implement its journal/recovery protocol.
The migration gate remains unavailable until the finite adapter and native
qualification cover the complete transaction.

## Source reads

`InspectPinnedTree` admits a clean local path without following reparse points
and retains all ancestor handles. Its `SourceTree` grants only read operations;
it does not accept the private owner/DACL authority required by `Directory`.
Public/read-only product assets can therefore be inspected without being adopted
as private state. Every opened file must be ordinary and single-linked.

Keep the source tree open until its returned files close. `OpenFile` withholds
write and delete sharing for the file's lifetime; the caller still supplies the
domain byte bound, reviewed hashes and final source-stability checks. A child
retains its own ancestor references even after its parent wrapper closes.

This is not a same-user sandbox or a frozen filesystem snapshot. Admission does
not prevent an authorized owner from changing ACLs or creating other entries.
The domain must keep its immutable inventory and validate changes before commit.

## Deterministic private stages

Start with `OpenPinnedTree` and use `PinnedChild` for private descendants.
`OpenStage(name)` creates or resumes that exact private, single-linked file
without truncating it. The caller validates the existing prefix and planned
length before appending; kernel admission does not authorize arbitrary old bytes.
The stage retains parent custody independently of the caller's directory handle.

`Stage.Publish(target, name)` flushes and exclusively renames the same held file
into a held pinned target. An occupied destination is never replaced. An
unpublished stage remains on disk after `Close`, so the domain can resume its
deterministic plan rather than start a new random temporary file.

Once native rename succeeds, `moved` remains true even if a subsequent flush or
diagnostic-label transfer fails. Preserve that complete destination and let the
domain journal identify its actual phase; do not delete it or recopy the source.

## Rollback root transfer

Use `OpenMovableDirectory` as the initial root admission for a rollback, before
reading/revoking its proof. It retains a DELETE-capable private root and its
pinned private parent. It is not an upgrade of a previously admitted directory:
independent root handles that withhold DELETE sharing can make admission fail.
Never close a proof-bearing handle and reopen its basename as a substitute.

`MoveTo(name)` is same-parent and exclusive. It preserves the admitted root
identity, updates `Path`, and leaves `Directory()` bound to the archived object
for receipt operations. Diagnostic `os.File` wrappers are retargeted by
duplicating the held handle before closing the previous wrapper, without a
pathname reopen or a custody gap. Keep the owning wrapper across this call;
do not cache its raw file pointer/handle for later phases.

Existing children are not a second active HOME. The domain must bind its archive
name, attempt/digest, revocation, locator and retirement records and must refuse
an unrelated occupied archive. All post-rename errors retain `moved=true`.

## Qualification limit

The dedicated Windows tests require the actual ordinary-account proof driver.
They cover source alias refusal and pins, read-only files, deterministic stage
resume, occupied destinations, same-object publication and archived receipt
writes. A PE cross-build proves compilation only. Native local-volume behavior,
durability and domain interruption/rollback still require the corresponding
hosted proof before the adapter can enable mutation.
