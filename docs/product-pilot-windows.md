# Participant-local Windows MCP rehearsal

The Windows kit uses the same ordinary unversioned, fresh synthetic profile as
the Mac rehearsal. It adds no installer, trust root, update authority or native
profile qualification. The server reports `unavailable`; packet label0.0.0 does
not supply a qualified server identity. Windows x64 is the only packaged target.

The producer is `dev/pilot/build-windows-kit.ps1`, run under stock Windows
PowerShell5.1 from the exact composed source after native core qualification.
Its output is a generic ZIP, portable checksum, ordinary PE server, local
bootstrap, Portuguese guide/recipe and byte inventory. It builds without release
linker injection and excludes private state and participant-bound packets.
The producer itself requires Go/Git; participants require neither.

`Preparar Maestro.cmd` invokes stock PowerShell with `-NoProfile -File` and the
existing execution policy. The preparer rejects an elevated process before
state writes and creates private directories with a current-user owner and
protected DACL at creation. Existing directories are inspected and never
reowned or ACL-repaired. Fixed local drive, ordinary path and owner checks
precede creation; unknown/partial roots and changed managed exports refuse.
A private sibling stage is renamed without replacement to reserve the fresh
root, keeping a concurrent existing destination intact.

The existing public `init`, `inspect`, `plugin` and `mcpb` commands own actual
HOME/workspace admission and packet materialization. Exports are generated on
each participant's machine because their absolute binding is local. Preparation
prints normal Code registration and Desktop import instructions, without client,
profile, credential or trust modifications. Repeat setup verifies the complete
managed product/export inventory and reads public inspection; canonical private
state and authored workspace contents remain outside the managed inventory.

The native acceptance script is `dev/pilot/test-windows-kit.ps1`. It must run as
an actual standard user against the exact produced ordinary binary. It checks
fresh preparation, exact local packet binding/native bytes, repeated whole-tree
preservation and refusal controls. Source parsing, a cross-build or an elevated
runner cannot establish native preparation. Native Code/Desktop loading, normal
artifact admission, human review and colleague outcomes remain separate gates.

No execution-policy override, software installation, signing substitution,
source-custody downgrade or conversion/model acquisition belongs to this kit.
Private Windows wiki, unseeded Docling, native agents/hooks, Yoda and signed
installation/update remain explicitly unavailable. SharePoint can transport the
unchanged generic ZIP after acceptance; transport grants no update authority.
