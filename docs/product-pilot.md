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

Before confirmation, each preparer discloses the settings for its new dedicated
Code profile. Exclusive creation writes `autoMemoryEnabled:false` and
`syncClaudeAiPlugins:false` to `client/code/settings.json` as UTF-8 without a BOM.
These choices disable the client's automatic memory and account-synced plugin
loading/downloads only in this profile. They do not disable Maestro's governed
memory. Existing roots preserve all settings/customizations/session data, even
an absent settings file; preparation never repairs eligibility by overwriting a
profile. Profiles remain outside managed inventory and the generic kit ZIP.
This bootstrap does not qualify hooks or promise ordinary active-plugin parity.

Code launches a session-local plugin using a separate CLAUDE_CONFIG_DIR and the
client's normal login flow. Generic portable packets retain external userConfig
selection; see the [official Code manifest reference](https://code.claude.com/docs/en/plugins-reference#user-configuration).
The prepared Desktop MCPB uses `mcpb --desktop-pilot`: an explicit engineering-only
profile with a full workspace-ID extension name, a short visible rehearsal label
and the exact prepared owner/workspace paths in its server argument vector. It
omits `user_config`: displayed directory defaults did not establish saved MCP
configuration in an observed Claude Desktop 4.23118.0 rehearsal. The explicit
preparation review and normal native installation remain required. It is limited
to ordinary unversioned `0.0.0` rehearsals and rejects signed/enrolled, stable,
native and lifecycle profiles. The normal product identity and generic packet
contract remain unchanged. The typed optional inventory binding regenerates the
exact canonical manifest during validation. `pilot-check` matches it against the
admitted preparation and includes the exact visible extension in its prompt.
Packet startup also requires the exact prepared owner/workspace paths and current
registered workspace ID/reference, so a changed caller selection cannot redirect
this extension to another admitted workspace.
Bound paths containing the reserved interpolation start `${` refuse before export
and during packet validation. Choose another ordinary local rehearsal path;
literal dollar signs, shell-substitution text, quotes and spaces are preserved.
Under DPMT, the Darwin DesktopPilot producer emits regular files and inventory
as 0600 and the executable as 0700 before computing the inventory and ZIP. Exact
role checks remain mandatory; this is not a predicate accepting any narrower
permissions. Generic, stable and Windows logical roles remain unchanged.

The public packet validator remains strict about ancestor, leaf and inner links.
Only the runtime's local engineering MCPB fallback may resolve the managed
root's parent, append the unchanged basename and strictly validate that physical
packet. It must be an ordinary DesktopPilot without stable, registry or native
authority, and its physical server path/digest must match the executing file pin
captured at process startup and rechecked after validation. Later managed reads
retain the physical root. Owner/workspace paths are never resolved this way;
no OS alias or installed permission is changed. The pin is bounded file identity,
not kernel image attestation or publisher authority.
Legacy generic pilot archives remain diagnosable.

Share the generic factory ZIP; the locally bound MCPB contains machine-local
paths and must remain on the preparing machine. Each rehearsal can coexist with
other Maestro extensions. An unexpected proposal to replace another extension is an explicit stop,
not an automatic replacement.

Preparation runs the existing executable's read-only `pilot-check` command and
retains a new JSON/Markdown report under `diagnostics/` for every check. Reports
live outside the hashed product, exports and launcher trees. The check validates
the emitted Code packet and Desktop archive, connects to the actual Code packet
server through the MCP SDK and compares the workspace to `preparation.json`.
Archive validation does not prove Desktop loading. A blocked initialization also
keeps the verified executable, test launcher and bounded diagnostic report;
neither the preparer nor the checker repairs owner ACLs or removes partial data.

The participant opens **Testar Maestro** and chooses a clearly named route.
Options 3 (Code) and 4 (Desktop) are independent read-only connection checks. Code opens with its exact report prompt as one literal initial argument, without
clipboard or manual paste. Desktop copies one prompt bound to the verified workspace
and exact client connector;
Desktop names the unique local extension and requires neither the Code CLI nor
a Code work item. Each prompt calls only `maestro_context` once and reports the
workspace and returned connection-health state. It neither queries work-item
state nor resumes work. The JSON fields `code_ready_prompt` and additive
`desktop_ready_prompt` are always strings and empty on blocked admission; emitted
files are `CODE-CONEXAO.txt` and `DESKTOP-CONEXAO.txt`.

Option 1 is the complete Code exercise; option 2 is the subsequent Desktop
continuity/resume exercise. These prompts bind a synthetic trial marker. The
Desktop continuity prompt discovers the single matching work item and uses its
current revision, so the participant does not transport execution IDs/revisions.
The launcher opens Code with the selected initial prompt, or copies the Desktop
prompt and opens that client, only after the explicit menu choice. Ordinary
**Abrir Code** without a prompt still opens an empty session. On Windows,
automatic prompt delivery requires a native `claude.exe` application on PATH;
`.cmd`, `.bat`, `.ps1`, functions and aliases never receive the automatic prompt.
Use the authorized native installation, or open **Abrir Code.cmd** without arguments
and manually paste the current diagnostic's CODE.txt or CODE-CONEXAO.txt. This kit
does not install or rewrite the client; ordinary opening retains its prior resolver.
For Desktop, the participant confirms installation of the named extension and
pastes the copied prompt into a new conversation. No directory fields need
copying or saving. Settings > Extensions > Configure > Enabled is conditional
recovery if that extension is disabled or unavailable. An enabled toggle alone
does not prove a connection; the prompt must obtain the expected workspace
through that extension. Basic Desktop connectivity does not require the Code
CLI; the cross-client checkpoint scenario requires the Code step first.
`--check-only` on Mac and `-CheckOnly` on Windows save reports without opening a
client or report viewer; preparation uses that mode. Missing `claude` is reported
without installing software or preventing the independent MCP/archive checks.

The short participant instructions and actual-client flow live in
[LEIA-ME.md](../dev/pilot/LEIA-ME.md) and [ROTEIRO.md](../dev/pilot/ROTEIRO.md).
They keep packaging, import, connection, cross-client work and accepted effects
separate. Automatic reports leave Code/Desktop client loading, login, real host
hooks and confirmation as `not_tested`; participants record those observations
after using the clients. Share only the reviewed Markdown report, not owner,
workspace, client profiles or participant-local generated packets/prompts.

## Desktop owner review channel

The explicit local preparation review discloses selection of `local-browser`
before PREPARAR or digest confirmation. Both platform preparers build the
ordinary Desktop pilot with `--owner-review-channel local-browser`; its typed
inventory and canonical manifest retain that exact startup choice. Native forms
remain first when advertised. Otherwise, a foreground protected request opens
its complete review and approval schema in an ephemeral local browser page,
requiring the initially unchecked checkbox and conscious exact-record approval.
Decline, cancellation, closing, expiry after at most 150 seconds or opener error
never approves. No URL or approval token is returned to the model. Generic,
stable, native and lifecycle profiles remain unchanged. Read-only tool permission
never substitutes for review. See [the channel contract](product-local-owner-review.md).
Actual Desktop presentation/effect/readback and native hooks remain separately
observed evidence; this local channel does not qualify MCP native forms.

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

The existing Windows native-engine workflow also has an explicit exact-artifact
route (#531). Its Mac job runs the same constructor once, retains the generic
ZIP/checksum plus producing run, source tree, KIT-ID and Windows PE hashes, then
transfers those immutable bytes into the existing Windows ordinary-account job.
In this route the sealed public server is copied from the verified ZIP; rebuilding
a substitute server is unavailable. The existing core parent executes that PE,
and the existing preparation parent extracts the same ZIP and runs its unchanged
PowerShell 5.1 review, digest confirmation, emitted checker and repeat preparation.
The engine qualifier stays on PowerShell 7 so its strict JSON case-pair and skip
classification remain intact; the helper separately observes the exact stock
PowerShell 5.1 executable used for emitted scripts and binds its edition/version.
Source-only jobs retain their original meaning and do not inherit artifact proof.

The bounded `native-kit-proof.json` and `native-kit-qualification.json` receipts
bind the ZIP, KIT-ID, extracted/prepared/packet PE, source/test images, native token,
PowerShell version and prepared workspace. Missing or changed input/evidence
refuses qualification. Repeated checks preserve private and managed bytes/modes.
The canonical catalog has 92 methods; the Code packet has 94 skill folders because
`operator` and `resume` are additional bridges. Cross-format projections are not
required to have identical Markdown bytes. Use the exact CI-produced candidate
and recorded hashes; this does not relabel an older local ZIP as executed.

This hosted evidence covers actual Windows artifact execution, not corporate
download policy, SmartScreen admission, native Code/Desktop login/import or
colleague feedback. For the remaining human test, download the recorded candidate,
extract it into an ordinary local folder, open `Preparar Maestro.cmd`, then use
`Testar Maestro.cmd` and return its Markdown report with the actual client result.
Keep prepared exports/private state local and preserve any policy refusal.

No new smoke/canary framework or CI band is introduced. Existing focused product
journeys and direct emitted-preparer verification cover source behavior; full
affected hosted checks and independent reviews remain promotion gates.

Native Desktop form qualification remains `unavailable` until an actual client advertises and renders the form. Local-browser review is a separate supported channel and does not close a native-form qualification gate.
