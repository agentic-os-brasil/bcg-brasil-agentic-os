# Isolated developer pilot kit

The pilot assembles the current shared MCP executable and prepares a fresh local
binding through its existing init, inspect, plugin and mcpb commands. It does not
reimplement signed installation, enrollment, update, client registration or
native qualification. Issue #499 owns the isolated rehearsal; #470 retains real
client and colleague acceptance, and #457 retains Windows kit qualification.

## Build from exact source

Use a clean committed source tree on the Mac publisher, with Go, Git and Python3
available. The receiver needs none of those development tools.

```sh
./dev/pilot/build-kit.sh /absolute/local/Developer/new-pilot-kit
```

The existing constructor produces Darwin arm64, Darwin amd64 and Windows amd64
executables. It records the exact source commit/tree, target and trust class in
ARTIFACT.txt, file hashes in SHA256SUMS, a checksum-bound KIT-ID, and the final ZIP
digest. Cross-compilation is not native execution or platform signing. Failed
assembly retains partial material and never replaces an existing artifact.
The output directory is claimed exclusively. ZIP and checksum publication use
no-replace hard links from private adjacent staging, which is retained for
inspection. These are separate final leaves: if checksum publication refuses,
the new ZIP may already exist. Preserve both results and choose a new output;
there is no pathname-based failure cleanup.

After the separately integrated passive-context implementation is available,
the publisher can explicitly select it:

```sh
./dev/pilot/build-kit.sh /absolute/local/Developer/another-pilot-kit --lifecycle-context
```

The constructor checks that the actual native server advertises that flag before
selecting it. Ordinary packages remain the default; the Desktop package receives
no Code hooks. Passive context is distinct from qualified native enforcement.

## Participant flow and preservation

The emitted preparer verifies the generic kit, shows its exact local effects and
accepts an explicit confirmation. Its local selection digest binds this rehearsal
only; it is not an installer review or release authority. The actual MCP performs
owner/workspace admission and packet construction.

The same kit reuses its own prepared root only after validating its managed
files. It preserves all private state. Foreign, changed and incomplete material
remains in place; recovery chooses a fresh destination, without deleting a
previous claim, resetting memory or altering another profile.

Code launches a session-local plugin using a separate CLAUDE_CONFIG_DIR. Its
supported userConfig prompts select the same external owner/workspace as
Desktop's user_config prompts. See the [official Code manifest reference](https://code.claude.com/docs/en/plugins-reference#user-configuration).
An existing Desktop extension with the same product name is an explicit stop,
not an automatic replacement.

The short participant instructions and actual-client prompts live in
[LEIA-ME.md](../dev/pilot/LEIA-ME.md) and [ROTEIRO.md](../dev/pilot/ROTEIRO.md).
They keep packaging, import, connection, cross-client work and accepted effects
separate. Actual native client loading remains unobserved until those actions
are run and their receipts retained.

## Trust and platform limits

This is an unsigned, unversioned engineering rehearsal. Server release identity,
production signing, source enrollment and signed update are unavailable; 0.0.0
is only the portable packet label. Checksums identify integrity, not publisher
authenticity. Admitted offline conversion and native enforcement keep their
own qualification requirements. Windows Darwin jobs remain unavailable.

Windows receivers download/extract into an ordinary local directory; direct
OneDrive/Files On-Demand and reparse paths remain unsupported. The launcher
honors existing PowerShell policy. It never changes execution policy, removes
quarantine, installs a client, authenticates, activates a provider or sends mail.
The real SharePoint destination and organization-owned signing remain external
configuration. Local assembly proves neither publication nor colleague delivery.

No new smoke/canary framework or CI band is introduced. Existing focused product
journeys and direct emitted-preparer verification cover source behavior; full
affected hosted checks and independent reviews remain promotion gates.
