# Install the local MCP product from a signed release

The modern `mcp-local-v1` delivery builds the shared MCP executable and an
independently admitted native installer. One reviewed setup activates the managed
payload, admits the selected private home and workspace, and creates local Code
and Desktop packets. Participants do not need a Git checkout. This first-install
milestone passed focused engineering tests and independent review, and is
locally integrated. Signed production execution and actual client loading
retain separate gates from the source-aware lifecycle described below.

## What a publisher prepares

The development factory builds both real entrypoints on the selected actual
Darwin or Windows runner. Run it from the repository root; the following paths
and versions are examples that the publisher must replace with approved values:

```sh
go run ./dev/release modern-candidate --version 0.1.42 --installer-version 0.3.7 --channel canary --os darwin --arch arm64 --authority-registry /approved/public-authority.json --output /new/local/factory-output
```

An optional `--docling-pack /approved/offline-runtime.tar.gz` supplies already
qualified runtime bytes. The factory never downloads a converter or model. A
release without that pack can still provide execution, memory and methods while
content conversion reports unavailable.

The output contains separate `candidate/` and `seed/` directories. The candidate
contains the MCP executable, complete immutable managed inputs, release notes
and manifest-v2 metadata. The seed contains the independently versioned installer
and its pinned public authority registry. Factory output is unsigned engineering
material. Follow [the release trust contract](../specs/020-release-distribution.md)
for organization-owned native signing and exact-byte release signing before any
production distribution. Historical beta or personal keys cannot establish that
authority.

SharePoint/OneDrive can transport replicated bytes after that admission. A synced
file, successful download or checksum alone is not release authority. Signed
generations and guarded update are being constructed under the
[signed-source guide](product-synced-source.md); registration follows in the
[complete delivery plan](plans/2026-11-03-synced-product-distribution.md). The
first-install milestone alone does not establish those complete journeys.

## Review the exact participant setup

Use the independently admitted installer and its adjacent `trust/` directory.
Select the local signed release directory, new managed installation directory,
owner directory, workspace and two new packet destinations. Keep private home
and installation custody outside the synced share and managed payload.

The closed operator request has these six fields:

```json
{
  "release_directory": "/local/signed-release",
  "managed_root": "/local/installed-maestro",
  "user_home": "/local/owner",
  "workspace": "/local/work/project",
  "code_packet": "/local/packets/maestro-code",
  "desktop_packet": "/local/packets/maestro-desktop.mcpb"
}
```

Save the request in a private local file. Run `maestro-install_VERSION_OS_ARCH`
(with `.exe` on Windows), preserving the publisher's exact versioned filename:

```sh
/local/seed/maestro-install_0.3.7_darwin_arm64 review < /local/private/setup.json
```

Read the complete returned local review. It binds release and server identity,
native seed and public authority, managed/private destinations, selected home,
workspace and both packets. Home preview is read-only and creates no authority.
Private installation state is fixed at `user_home/.maestro-install`, separate
from admitted owner state. Do not paste participant review output into chat.

If the exact effects are intended, use the identical request and returned
`review_digest` with explicit confirmation:

```sh
/local/seed/maestro-install_0.3.7_darwin_arm64 apply --review-digest REVIEW_SHA256 --confirm < /local/private/setup.json
```

Apply recomputes the review and verifies the installed seed again before the
canonical activation transaction. An unsigned seed refuses even with a valid
signed release and confirmed digest. There is no command-line engineering trust
override. Use an ordinary unelevated user process.

## Inspect completion and recover a partial setup

`ready` means this setup completed activation, admitted owner/workspace and
materialized both participant-local packets. `code_registration` and
`desktop_loading` remain `pending`: packet creation does not load a native
client. Both packets bind the same private home/workspace, contain the actual
server and complete immutable methods/agents/policy resources, and remain local
because they contain participant paths.

For a source-aware release, complete the explicit source review and confirmed
source-check in the [signed-source runbook](product-synced-source.md) before
opening the client carriers. First-install packet materialization is separate
from source enrollment. Qualified launch reads that private custody and can
refuse with source recovery required until enrollment is complete.

`activated_setup_pending` means signed activation completed but a later admission,
converter or packet step needs recovery. Preserve the managed payload, private
installation state, home locator and existing packets. Inspect the structured
result even when the command exits nonzero. The command retains the structured
phase and recovery detail alongside its neutral failure summary. Choose fresh packet destinations
and obtain a new exact review before retrying. Do not erase owner state, clear
transaction markers or reuse a stale digest. Ambiguous unadmitted populated
homes and overlapping paths refuse.

An existing modern installation with a failed authenticated server check is
preserved for diagnosis. Its expected size and digest are checked before its
identity command may execute; a failed check refuses automatic quarantine or
reinstallation. Preserve the existing bytes and transaction state for operator
inspection rather than replacing the server to force setup to continue.

The same root binding serves Code and Desktop through local stdio; there is no
hosted MCP endpoint. Actual registration/loading, trusted forms, organizational
policy and native wake require their own evidence. The source tracer built real
executables and packets and continued synthetic work after a fresh stdio restart.
Its successful inner native admission was a controlled in-process engineering
composition. Production-default unsigned installer probes refused. Neither result
qualifies organization signing or a participant device.

The source-aware signed update path has explicit review, stage, update,
reconcile and rollback commands in the
[synced-source runbook](product-synced-source.md). Its complete task review is
separate from the first-install milestone above. Historical modern state without
an authentic source-aware current binding receives a migration-unavailable
refusal. Preserve the existing state for diagnosis instead of using the legacy
updater, copying over an active server or clearing recall/custody state. Client
registration/recovery, ordinary-user Windows and actual Code/Desktop acceptance
retain their separate delivery gates.

After admitted installation, use the [participant client-management console](product-clients.md)
for explicit source enrollment, current packet materialization, supported Code
registration, Desktop handoff and exact interrupted-work reconciliation. It
collects ordinary local choices and generates the carrier ID; participants do
not need to hand-write request JSON. Native loading remains separately verified.
