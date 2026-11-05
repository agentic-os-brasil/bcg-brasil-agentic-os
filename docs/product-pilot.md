# Participant-local MCP developer rehearsal

This kit packages the existing ordinary unversioned MCP-only route; it does not
create another installer, identity, trust root or update authority. Its first
qualified producer/target is Darwin arm64. The included Darwin amd64 executable
has cross-build/native-format evidence only; actual Intel execution and Windows
remain separate qualification gates. No Rosetta installation is performed.

The producer runs `dev/pilot/build-kit.sh OUTPUT` with offline Go dependencies
and Developer-local cache/temp settings. It builds without Version,
CompatibleBundle or registry linker injection. The server reports `unavailable`;
the schema1 packet label0.0.0 is not a qualified server identity. Modern identity
inspection must refuse this candidate. Use native byte SHA, schema1 inventory and
actual SDK/ordinary-stdio behavior as the engineering proof.

The shareable ZIP contains the binary, standard-shell bootstrap, Portuguese
participant instructions, license, source/mode metadata and SHA256 inventory.
It excludes private HOME, workspace contents and already-bound client packets.
The bootstrap runs the existing `init`, `inspect`, `plugin` and `mcpb` commands
in a fresh user-owned Developer subtree. Client packets are generated on each
participant's machine because the builders bind absolute local paths. An outer
local marketplace references the unchanged generated plugin. The setup prints
supported Code registration commands and the Desktop import path; it neither
launches clients nor changes existing client profiles or credentials.

The setup refuses foreign roots, aliases and changed/partial generated content.
Rerunning the same complete setup verifies its full managed file inventory and
uses read-only inspection; participant memory/workspace changes are not treated
as managed artifact drift. Different source kits need a new rehearsal root;
there is no automatic migration or signed-update claim. Cleanup is conscious,
participant-operated removal of the exact isolated root after disconnecting
the rehearsal clients.

The real-client acceptance recipe is co-located in `dev/pilot/ROTEIRO.md`.
Code/Desktop loading, normal OS/enterprise artifact admission and observed
participant results remain separate from setup/SDK proof. Use synthetic data;
unsupported confirmation, native agents/hooks, unseeded Docling, Yoda and signed
installation/update remain unavailable. The broader Mac-and-Windows pilot and
complete product acceptance in issue439 stay open until independently observed.
