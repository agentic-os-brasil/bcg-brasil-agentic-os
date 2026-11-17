# Connect the installed Maestro to Code and Desktop

The stable-local profile connects both clients to the same admitted local Maestro
installation and private workspace state. Server updates come from the reviewed
signed-source workflow; client materialization and native loading have their own
evidence. This guide describes the source workflow. Organization-owned trust,
native client compatibility and participant acceptance must be qualified before
a production rollout.

## Start with the participant console

Complete the [signed first installation](product-install.md), then open the
participant-local management launcher or invoke the admitted installer:

~~~sh
/installed/maestro/maestro-install manage --product-root /installed/maestro --user-home /participant --workspace /work/project
~~~

Use the actual installed paths. The console inspects the admitted installation,
collects any missing source/client choices, generates an opaque carrier ID and
shows the exact review before an effect. Choose local packet destinations and
an explicit Code scope; Code registration is optional and separately reviewed.
You do not need a Git checkout, hand-written request JSON or a manually
generated identifier.

Declining leaves that operation pending. A source observation, activation,
client regeneration and native registration are separate effects, so the console
shows the exact review at each required transition. It never grants blanket
approval, rewrites a raw client profile or installs a recurring updater.

## Versioned complete-client source workflow

New console setups select private manifest-bound packet generations and a fixed
local Code command-source provider. The selected workspace and canonical brain
binding are retained. Each release gets new packet slots; previous generations,
legacy fixed-slot records and occupied foreign paths remain unchanged. The
private current client record selects the exact verified current plugin. The
provider only reads this record through the admitted installed launcher. It
cannot select a source, repair a tree, update code or grant approval.

In Code or Desktop, an explicit "Atualizar Maestro" request uses the closed
`maestro_apply` operation `installation.update_handoff`, with no other arguments.
The stable launcher supplies the validated carrier as trusted startup context.
The backend repeats independent installer/current-record admission and verifies
the fixed update launcher before the normal host opener receives it. The console
opens directly into its existing update action. The handoff accepts no model
path, source, command, plan or approval; normal host permission governs opening.
Its `handoff_requested` result leaves activation pending native installer review.
Missing, ambiguous or unqualified context refuses and retains the independent
manager recovery route.

Full native preparation uses a reviewed bounded packet-generation identifier.
Carrier, manifest and generation determine finite sibling-slot names. A failed
qualifier retains its candidate; an explicit retry receives a fresh reviewed
generation, preserving failed candidates and their evidence.

Code uses the documented [command plugin source](https://code.claude.com/docs/en/plugins/marketplace-reference#command-plugin-source)
in COPY mode. Its native install/update commands own the host cache. Current
registration requires supported scoped readback and exact copied payload
identity against the recorded source, allowing host permission normalization.
Command-source policy and actual native compatibility remain host acceptance
checks; a rejected command source does not enable another updater.

The complete Code choice requires the existing real native qualifier for the
selected current executable and profile before current-record publication.
Qualification runs outside installer publication serialization. Only its
same-call `native_observed` result and unchanged canonically verified candidate
enter the existing guarded publisher. The qualification fingerprint binds the
stable carrier. An unavailable login, policy, input, proof or native capability
keeps the complete profile pending, preserving previous packets. The separate
resources-only choice reports native enforcement unavailable; an existing
complete profile cannot silently downgrade to that choice.

The same current server prepares a complete Desktop method bootstrap and verifies
its archive against its own canonical methods. Install this bootstrap once using
the normal host flow. Each invocation reads the live catalog, policy, selected
plan, current complete method instructions and referenced resources through the
existing admitted `methods.list`, `methods.plan` and `methods.read` operations.
Read every page with consistent resource/hash/length identities. Packaged
snapshots are reference content. New methods remain discoverable through the
operator entry even without a new installed slash alias. Ordinary instruction
changes need no reimport while `maestro-live-method-bootstrap-v1` stays compatible.
The bootstrap package version, live backend method identities and actual host
installation/loading are separate observations. Changed bootstrap contracts or
native permissions retain normal host review.

The console's update action composes the existing source review/update, full
native preparation, current packet publication, supported Code registration and
maintenance requalification owners. Each original exact review still requires
conscious confirmation. A deliberate reload/reconnect/new session follows;
the update does not terminate a running client to imply activation. A compatible
Desktop native-facing contract retains the exact previously admitted connector
bytes in a new private slot, preserving installation confirmation while requiring
a new connection/functional check.

Maintenance proof binds each job to the whole current executable and CLI
configuration. Preserved enrollment alone therefore does not preserve
qualification after an executable update. The console selects only previously
qualified jobs now reported stale, obtains each real existing job review and
runs that original qualifier after confirmation. It leaves enrollment, timezone,
opt-in and pause/revocation state with their owning service. Missing real inputs
remain waiting/pending. `revalidate-maintenance` retries this phase without
replacing the updater or relabeling old proof.

This is source construction, not acceptance of the distributed experimental kit.
The existing 0.1.50 experimental kit has no enrolled installer/source update
route. Testing this complete N-to-N+1 flow needs a separately reviewed
experimental installation/source admission under the existing short-lived
engineering authority, with its exact supported purpose and scope. Production
organization signing is a later distribution gate. Neither this source nor
the Desktop bootstrap changes that authority or clears the retained native
Agent proof/custody boundary. Current materialization, registration, reload,
native qualification, maintenance qualification and functional acceptance remain
separate status phases.

The resulting participant-local launcher reopens management for this exact
installation/workspace/carrier. Preserve its selected paths. Moving a packet,
changing the binding or editing the current record is not an update procedure.

## Register Code through its supported interface

The console uses the selected absolute Code executable and checks the supported
version/help/JSON interface before plugin effects. The scoped local marketplace
and qualified plugin ID are specific to this carrier. The adapter reads actual
marketplace/plugin state, uses supported add/install/update commands, then reads
both inventories again.

If you initially leave Code unselected, return through the same launcher and
choose Code registration. Select the first executable through its exact review;
the same carrier, workspace and explicit scope are preserved. This does not
silently replace an executable that was already selected.

The local in-place interface requires Code 2.1.289 or later. Native specialist
guard qualification retains its separate 2.1.295 minimum and host admission;
ordinary registration does not grant that qualification. Follow the official
[plugin CLI reference](https://code.claude.com/docs/en/plugins/cli-reference) and
[loading contract](https://code.claude.com/docs/en/plugins/loading) for native
session behavior.

An existing disabled plugin stays disabled. A marketplace with the same name but
a different source, duplicate identity or ambiguous scope is a conflict to
inspect, not permission to overwrite another installation. Updating does not
uninstall/reinstall the plugin or replace authentication and permission settings.

A successful supported CLI readback establishes registration. A current session
must separately load/reload the intended plugin, connect to the installed MCP
server and complete a harmless functional readback. Process exit zero alone
establishes none of those later states.

## Install the Desktop connector explicitly

Open the generated local MCPB through Desktop's normal private-extension flow
and review its actual permissions/configuration. Opening the file establishes a
handoff only. Confirm installation only after completing that native flow.

The status distinguishes:

| Evidence | Meaning |
| --- | --- |
| Bundle ready | Exact local package bytes passed materialization checks |
| Open requested / awaiting install | A handoff was requested; installation remains pending |
| User-confirmed install | The operator attested installing the exact package/configuration |
| Connection observed | A separately verified connection reached the intended current server |
| Functional check passed | A separately verified harmless operation/readback succeeded |

Operator confirmation never manufactures the last two observations or native
host qualification. Private MCPB installation/update remains a manual native
flow; no undocumented headless Desktop registration API is assumed. See the
official [local MCP guide](https://support.claude.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop).

The stable package is a candidate using an external absolute installed launcher.
Actual Desktop compatibility must be tested on the supported client. If it
rejects that configuration, the stable source-managed carrier is unavailable
under that client policy. `manual-desktop-update` regenerates the same stable
carrier and cannot bypass that restriction. An ordinary unmanaged MCPB is a
separate installation profile; it is not a source-managed fallback. Preserve
this limitation in participant status until the actual client is qualified.

## Legacy fixed-slot updates and shared source recovery

Use the same console and [signed-source update/recovery workflow](product-synced-source.md).
SharePoint/OneDrive supplies distribution bytes. The current source can materialize
the finite supported Windows Cloud Files input into ordinary private staging;
native OneDrive qualification remains pending. It refuses unsafe or incomplete
input and preserves the configured private brain location and owner data.
Signatures, enrolled source
custody and the independently admitted installer supply release authority.
There is no new hosted MCP endpoint in this local stdio delivery.

After N-to-N+1 activation, the old client materialization refuses until checked
regeneration authenticates the current server and complete client inputs.
Packets are generated by that current server, avoiding assets linked into an
older stable installer. The current record is published only after package
verification under the existing installation serialization.

Automatic replacement of a differing existing client packet is unavailable on
profiles without strong physical custody of the entire shared tree or package.
The operation preserves current packets, generated candidates and unknown
backups, then reports the required manual client replacement step. Initial
publication is exclusive and byte-identical reconciliation does not replace
files. No hash-then-rename or cleanup sequence is advertised as preserving an
uncooperative writer.

After explicit operator-managed replacement, Code registration uses the same
scoped marketplace/plugin and preserves disabled state. For Desktop, one exact previously installed stable
MCPB can be reused only if its carrier protocol/version and full native-facing
configuration/permission contract remain unchanged. Regeneration independently
checks current server/client inputs and the old exact package/provenance.
The bundled old server remains explicitly historical and inert; live tools and
resources come from the current external installed server.

This reuse is a construction behavior requiring native qualification, not a
promise that OneDrive updates an installed MCPB. A changed carrier/native
contract requires manual installation of the new package. Authenticated
metadata-only refresh with identical active immutable inputs needs no packet
regeneration, but expiry, known recall and minimum reader checks still apply on
every qualified launch.

Compatible rollback also requires checked current regeneration and preserves
advanced source history. Neither a packet backup nor an old client receipt can
restore source custody or bypass recall.

## Diagnose and recover exact interrupted work

Read-only status does not refresh source, migrate state, repair packages,
register a client or open Desktop. It distinguishes current materialization
from historical/pending host and session evidence. A source-blocked server uses
the independent admitted installer for diagnosis.

If activation succeeded but regeneration failed, keep the actual
activation-applied/client-materialization-pending state. Explicit reconciliation
reauthenticates Current and actual packet/host state before publication. It
preserves unknown ownership/aliases and refuses a live owner's lock; it does
not reset source history or remove another participant's work.
If an explicitly requested Desktop replacement already completed before the
record write was interrupted, reconciliation verifies the exact current package
and its exact recorded backup, then finishes that binding without repeating the
swap. Older backups remain preserved; no fixed-slot garbage collection occurs.

The advanced operator surface exposes client-capabilities, clients-review,
clients-apply, clients-status, clients-reconcile, code-review, code-apply,
desktop-confirm and stable-launch. These use exact reviewed typed requests and
binding/digest checks. The participant console composes them; stable-launch
accepts only its fixed installation/home/workspace/carrier identity and returns
the exact current pinned server launch.

Inspect the actual failure and obtain a fresh exact review when current inputs
change. Preserve private recovery evidence and the last valid source; do not
clear locks/markers, change clocks or hand-edit client records to force success.

## Evidence required for team testing

Construction proof uses actual factory/installer/server binaries, real SDK and
restarted stdio, controlled supported-client process witnesses and explicitly
labelled engineering native-admission composition. Shipping unsigned-native
refusal remains independent. These tests do not certify real Code/Desktop
loading, Windows private-state admission, organization signing or delivery.

Before inviting participants, qualify the exact native artifacts and clients,
publish through the approved organization trust/channel, and prove clean
installation plus update/readback with two participants. Use the selected shared
distribution destination and choose each participant's local synchronized folder
at first setup. Keep actual client/device/CI/signing/publication
and participant results separate from source review and local integration.
