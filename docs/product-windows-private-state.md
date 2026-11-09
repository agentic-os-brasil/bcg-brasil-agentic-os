# Windows private-state engine

The shipping MCP server uses the same canonical HOME, owner, memory and execution engines on Windows. Platform adapters own filesystem handles; they do not grant domain approval, change workspace selection or replace trusted review.

## Scope and storage admission

Windows state requires a clean absolute local-drive path and an actual current-user owner. The finite native owner validates the held object type, local volume, effective DACL and private-file link count. It refuses reparse points, junction traversal, offline/recall objects, alternate streams, device/UNC names, unexpected principals and hardlinked private files. It never repairs an existing object's owner or ACL to make admission succeed.

Ordinary ancestors and read-only SDK/workspace evidence have a distinct read-only traversal. Reading a source file does not admit it as private state or grant publication authority. Private creation, replacement, removal and rename operate relative to held directories, with explicit private descriptors and real directory-flush errors. A successful namespace publication remains intact if a later flush reports an error; domain recovery must inspect the resulting complete state rather than erase it.

Persistent transition locks retain their native filename custody while held. Existing prompt-history locks, owner review revisions, memory visibility/revocation and execution state revisions remain domain-owned. The engine introduces no model approval flag, authority environment variable, installer bypass or repair mode.

## Expected public behavior

The explicit `maestro-mcp init` command admits the selected fresh HOME, publishes origin last, prepares the owner and initializes one workspace. `inspect` and ordinary startup do not bootstrap missing state. The owner interview still requires trusted review of exact supplied text. Owned memory can be prepared, captured, corrected and deleted; restart and consolidation must preserve revocation and never resurrect removed source text. Execution checkpoints and pause/resume continue from the same workspace and stored revision after restarting the server.

Installation and initialization must use a non-elevated Windows process. A hosted administrator token is useful for refusal tests but cannot qualify the ordinary-user public journey. Native directory/volume operations that the device cannot support fail with their actual error; there is no no-op durability success.

## Reproducible qualification

The **Maestro Windows native engine** workflow builds the shipping executable and public SDK witness from an archive of the exact committed source and a hash-verified copied Go SDK. The existing `acceptance/windows-core-pilot.ps1` creates an isolated temporary ordinary account, verifies its token and private temporary directory and runs four mandatory families:

- **Core:** real shipping initialization and inspect, trusted memory review/decline, memory capture/rebuild/revocation, execution checkpoint/pause and process restart/resume.
- **DirectoryDurability:** actual ordinary-user directory flush, held fresh traversal and exclusive bootstrap/publication/reopen.
- **PrivateState:** independently inspected owner/DACL fixtures, unsafe principal/owner, unowned read, junction/hardlink/offline/nonregular refusal, held publication, collision/compensation and native lock contention/custody.
- **OwnerReview:** quick and complete interview, exact trusted review, declined state unchanged, restart and concurrent intervention refusal.

Every selected family must actually pass. A compiler result, skipped test, missing family, synthetic unavailable result or previous branch's passing run cannot establish current-source qualification. Generic contract suites and client/distribution evidence retain their own gates.

The baseline at `f7e274729e9d1c468ea6e0baf820eaacf03fe03b` established the missing behavior: independent native token/private-temp attestation passed, then the shipping public init failed as unavailable. The subsequent engine candidate must replace that observed failure with a fresh exact-source native pass before merge. This source document alone is not a success receipt.

## Limits

This work does not activate a client profile, install an MCPB/plugin, prove Desktop or Code host loading, execute a model, enable private wiki, convert documents on Windows, sign a release or qualify a colleague's managed device. Actual host loading, corporate filesystem policy, supported storage and organization-controlled distribution require separate evidence. Failures preserve diagnostic state; deleting the HOME, locator, locks or visibility controls is not a supported recovery shortcut.
