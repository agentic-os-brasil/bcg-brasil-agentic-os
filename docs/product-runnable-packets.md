# Portable managed Maestro packets

The product `plugin` and `mcpb` builders produce a new portable packet from an
admitted local build binding and a selected native Maestro executable. The output
contains no builder owner/workspace path, owner HOME, locator, registration,
credential, checkpoint or memory content. Runtime participants select their own
external paths. Building does not install anything or contact a model/provider.

## Managed payload and integrity

Both formats contain the selected executable; all 92 canonical methods and their
249 recipe/policy/provenance/license assets; the operator/resume aliases; and six
raw canonical runtime/memory files. Both also include fourteen immutable agent
assets under `managed/agents/`: three JSON policy/catalog files, seven canonical
definitions and four scaffolding templates. The emitted MCP server exposes the
same bytes through the fixed `maestro://agents/` resources after revalidating the
admitted binding on every read; it never exports an owner registry or case files.

Code additionally carries six native specialist definitions in `agents/`, generated
by the shared projector with byte-zero YAML frontmatter. Maestro remains the main
session hub. Explicit method Agent invocations use the exact `maestro-product:`
namespace; canonical IDs, policy and resource paths retain their original bytes.
Canonical and projected files have separate paths and exact distributed hashes in
the packet inventory. Code Markdown method projection retains its canonical
provenance and distributed-byte projection hashes. MCPB carries canonical agent
content without Code native agents or hooks; its recipes remain inert resources,
not a native Desktop Skills installation. Native scheduling, tool denial, scope
mediation and signed completion remain unqualified host capabilities. Reading or
installing instructions does not grant those capabilities or prove enforcement.
Only an independently pinned **public** release-authority registry is optional;
private signing keys or an authenticated offline runtime are not copied from the
builder's product directory. Content capability stays unavailable without its
separately admitted offline pack.

`packet-inventory.json` records the exact emitted file paths, SHA-256, byte counts
and normalized distribution modes (0644 data, 0755 server). It excludes itself.
Every runtime use with `--packet` verifies the complete canonical payload, exact
file set and actual native server before continuing. It refuses missing, changed,
extra or linked files and never repairs them. A self-consistent edited inventory
cannot omit a canonical file or add a private/development file. Inventory integrity
does not authenticate a publisher or authorize an owner operation. On Windows,
POSIX archive modes express roles rather than a filesystem permission guarantee.

## Prepare and run the actual emitted executable

Keep the packet separate from owner state and workspace. The Owner directory
must already exist; the workspace may be new. Ask the participant to approve
preparation explicitly. For a Code packet, run its `scripts/maestro-mcp` (Windows:
`scripts/maestro-mcp.exe`). An extracted Desktop extension uses `server/maestro-mcp`
or `server/maestro-mcp.exe`. Use argument arrays or quote every selected path;
spaces and shell metacharacters are literal path bytes, never shell commands.

For example, in a POSIX terminal with explicitly selected paths:

```sh
"/path/to/packet/scripts/maestro-mcp" init --packet --product-root "/path/to/packet" --user-home "/path/to/owner" --workspace "/path/to/workspace"
"/path/to/packet/scripts/maestro-mcp" inspect --packet --product-root "/path/to/packet" --user-home "/path/to/owner" --workspace "/path/to/workspace"
"/path/to/packet/scripts/maestro-mcp" serve --packet --product-root "/path/to/packet" --user-home "/path/to/owner" --workspace "/path/to/workspace"
```

`inspect` is the existing doctor/diagnostic command. It reports fresh-unconfirmed
before preparation and ready after admission. It and `serve` never initialize,
migrate or repair. Explicit repeated `init` preserves admitted owner memory,
authored files, credentials, client registrations and workspace data. Overlapping
or otherwise invalid selections are refused by current canonical admission.
Partial canonical bootstrap follows the existing support-assisted recovery policy;
it does not roll back or delete private state. Legacy commands without `--packet`
retain their previous behavior for separately installed products.

## Client configuration and update boundary

Code's manifest uses `userConfig`; its MCP configuration uses
`${CLAUDE_PLUGIN_ROOT}` and `${user_config.KEY}` in argv. MCPB uses `user_config`,
`${__dirname}` and its own documented `${user_config.KEY}` expansion. Both require
Owner directory and Workspace with no default private value. These are distinct
client contracts: see [Code plugin reference](https://code.claude.com/docs/en/plugins-reference)
and [MCPB manifest](https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md).
Tests that expand these finite placeholders and launch the real emitted binary
prove packet/CLI behavior; they do not prove an actual client loader or UI ran.

Build an update to a **new** destination. A fresh private stage is published with
exclusive no-overwrite semantics. A failed unpublished stage is cleaned up; an
existing/newer target remains unchanged. A late publication sync error can leave
the newly published target present: inspect it rather than deleting/replacing it.
The inventory does not replace release signing, a distribution manifest or a
transactional client updater. Real fresh-client installation and host mediation
are tracked separately; this source does not claim pilot delivery,
SharePoint publication, platform signing or real client acceptance.

## Source-bound Windows packet qualification

The landed Windows private engine has its own ordinary-user qualification. Packet
qualification additionally requires the existing compiled public workflow to run
both emitted formats at versions 0.1.0 and 0.1.1 under that actual ordinary account.
The same closed companion inventory also requires canonical agent resource discovery and the callback
journey to refuse a stale binding without repairing owner/workspace state. It
contains 53 MCP parents and 149 MCP outcomes, plus one command outcome; the
previous 52 parents, 148 outcomes and all four portable children remain required.
Missing, skipped, duplicate or failed outcomes cannot grant elevated-host
exclusions. These
cases run the real emitted executable through init, inspect and stdio context,
preserve the same external workspace identity across formats/versions, repeat
preparation, and refuse overlapping state without private or managed mutation.
This is native packet/CLI proof when that source-bound hosted run actually passes;
it does not assert a Code/Desktop loader or UI ran. POSIX-only umask/mode/link
controls remain separately qualified on Unix.
