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

## Explicit engineering runtime kit

An independently reviewed modern canary release and its public registry may be
selected through the existing constructor:

```sh
./dev/pilot/build-kit.sh /absolute/local/Developer/full-pilot-kit \
  --engineering-runtime /absolute/verified-modern-release /absolute/public-registry.json
```

This closed profile accepts one active `maestro-engineering-pilot-YYYYMMDD`
issuer and key, with a lifetime of at most 14 days. It verifies the full signed
release before and after copying it. `RUNTIME.json` discloses actual issuance,
expiry, version, target, public registry, manifest and runtime archive digests.
The server comes from that exact signed release; `ARTIFACT.txt` distinguishes its
provenance from the clean builder source. The initial complete carrier targets
Darwin ARM64. A runtime-bearing Windows carrier and Intel runtime are unavailable
until their own platform and distribution gates are satisfied.

The Mac preparer first presents the usual isolated-directory review, then the
complete native `pilot-runtime-review`. The participant must consciously type
`ATIVAR CONVERSOR` before `pilot-runtime-apply` rechecks the exact review and calls
the canonical `PlaceVerified`. A generic preparation digest alone never grants
this second approval. Signed metadata, actual server identity, target, owner,
workspace ID and existing runtime/placement digests are bound to the review.
Decline/stale input leaves placement untouched; differing/partial private data
is preserved. Successful replay checks the existing runtime without replacing
it. Startup, connection checks and `content.status` never place or repair it.

The new prepared layout uses only `product/server/maestro-mcp` and its fixed
public registry under `product/managed/trust`. Both Code and the explicitly
selected `--engineering-runtime-pilot` Desktop export retain the same version,
registry, server, manifest and archive identities. Full inventory and exact
physical executing-image validation precede authority seeding, including the
controlled Desktop ancestor-alias path. Native selection/configuration
fingerprints and normal source/enrolled/stable production admission are
unchanged. This experimental key does not establish organizational publisher
authority, native platform signing, production enrollment or production updates.

The kit copies a synthetic DOCX into the fresh workspace. Its existing guided
Code and Desktop journeys request actual conversion and paged derived-artifact
readback, checking the independent marker/table. The read-only doctor reports
runtime availability separately from conversion and actual client observation.
`content.status` owns the actual admitted format list; package presence is not
format support. Expiry refuses with a newly reviewed kit as the recovery route,
without changing dates, deleting data or silently renewing trust.

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
client's normal login flow. Ordinary preparation remains MCP-only or passive
lifecycle context. **Testar Maestro → 5** is the separate, optional seven-hook
qualification route. It calls the existing `native-review`, displays its full
plan and requires an explicit per-session estimate and `QUALIFICAR` confirmation
before `native-qualify-and-build`. Preparation, ordinary open, connection checks
and check-only never start model qualification. The current canonical plan allows
at most three CLI sessions, eight parent turns and 120 seconds per session, with
an explicitly selected estimated per-session limit in `(0, 1]` USD. Provider
estimates are not hard billing ceilings; child agents may make further requests.
There is no canonical price recommendation or default. The participant chooses
the estimate required to bind the reviewed plan, rather than guessing a quoted
model price; the complete plan displays its aggregate estimate before consent.
Admission, normal login and supported-host checks can still refuse before any
model session. Existing results and private profiles are preserved rather than
deleted or automatically retried.

A successful result is written to `exports/code-native-qualified`. The checker
and opener select it only after canonical packet validation, matching host target
and server SHA, and exact admitted owner/workspace binding. A valid `mcp_only`
fallback keeps using the ordinary `exports/code`; engineering fixtures, malformed
exports and changed bindings refuse. The baseline managed inventory excludes only
this named optional export, which retains its own closed canonical inventory;
unknown files elsewhere still refuse. Repeating preparation preserves and checks
both the original managed material and this optional result.

For the qualified packet, the opener supplies exactly `user_home` and `workspace`
through a new private session settings file under `client/code`, passed as
`--settings` with `pluginConfigs["maestro-product@inline"].options`. It does not
rewrite the sealed export, existing settings, credentials, permissions or hook
configuration. Code supports this session selection through its
[settings input](https://code.claude.com/docs/en/settings#change-a-setting-for-one-session)
and [plugin options](https://code.claude.com/docs/en/settings-reference#pluginconfigs).
Generic portable packets retain external userConfig selection; see the
[official Code manifest reference](https://code.claude.com/docs/en/plugins-reference#user-configuration).
The full plan and result are retained as private `native-*.json` diagnostics;
share only the reviewed `RELATORIO.md`. This delivery path does not continuously
recompute the qualification configuration fingerprint or establish active
third-party-plugin parity. Packet qualification and actual client observation
remain distinct gates.

The prepared Desktop MCPB uses `mcpb --desktop-pilot`: an explicit engineering-only
profile with a full workspace-ID extension name, a short visible rehearsal label
and the exact prepared owner/workspace paths in its server argument vector. It
omits `user_config`: displayed directory defaults did not establish saved MCP
configuration in an observed Claude Desktop 4.23118.0 rehearsal. The explicit
preparation review and normal native installation remain required. The default profile remains limited
to ordinary unversioned `0.0.0` rehearsals; the explicit runtime profile above is
the only versioned exception. Both reject enrolled, stable and native carrier
combinations; Code lifecycle context is separate. The normal product identity and generic packet
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
packet. It must be either an ordinary unversioned DesktopPilot without registry
authority or the explicit runtime profile with the same pinned public authority.
Stable and native carriers still refuse. Its physical server path/digest must
match the executing file pin
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

Installed-profile diagnostics allow a separate finite budget for complete
runtime authentication. Earlier cancellation still stops the request, and
cleanup is observed separately. The [installed-checker contract](../specs/063-product-native-plugin.md#isolated-pilot-self-service)
defines those budgets and preserves all signature, Current and inventory checks.
A diagnostic pass remains separate from actual Desktop or Code acceptance.

The participant opens **Testar Maestro** and chooses a clearly named route.
For the complete native Code trial, first use option 3 to open the dedicated Code
profile and finish its normal login, then return to option 5, review the actual
plan and consciously authorize its model checks. After qualification, the same
ordinary Code routes select the validated seven-hook export automatically.
Desktop continues using the separate MCP extension; it does not install Code's
seven native hooks or claim Code subagent/skill execution parity.
Options 3 (Code) and 4 (Desktop) are independent read-only connection checks. Code opens with its exact report prompt as one literal initial argument, without
clipboard or manual paste. Desktop copies one prompt bound to the verified workspace
and exact client connector;
Desktop names the unique local extension and requires neither the Code CLI nor
a Code work item. Each prompt calls only `maestro_context` once and reports the
workspace and returned connection-health state. It neither queries work-item
state nor resumes work. The JSON fields `code_ready_prompt` and additive
`desktop_ready_prompt` are always strings and empty on blocked admission; emitted
files are `CODE-CONEXAO.txt` and `DESKTOP-CONEXAO.txt`.

Options 1 (Code) and 2 (Desktop) open the current-step selector. It starts with
read-only reconciliation of the existing owner, page, memory and execution
stores. Only the selected step is sent to the client; later mutation instructions
remain outside that request. The participant chooses next, previous or
reconciliation after interruption. Sending a step never advances it automatically.
Each prompt binds the workspace and selected client, resolves actual IDs,
revisions and pagination, and ends the whole turn on unresolved review. Missing
records do not establish human decline or authorize replay. The memory refusal
control requires an observed explicit decline in its separately selected step.
The existing string fields are retained as read-only entries; additive
`code_steps` and `desktop_steps` carry the separately deliverable prompts.
Conversion and document preparation occur only in the selected document step.
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

The installed launcher also supports a Desktop-first phase without changing a
saved **Code and Desktop** selection (issue #638). Before any Code login/status
probe, the participant chooses whether to prepare/resume Code now. Enter keeps
Code pending and asks the original manager to prepare the complete Desktop
extension and live-method library. It does not publish a Code packet, register
Code, invoke a model, or substitute a resources-only profile. The installed
bootstrapper and backend must both support the preparation contract; older
readers refuse the new phase before publication. Existing complete and explicit
Desktop-only records retain their original semantics.

An admitted partial record keeps aggregate preparation partial. Its Desktop
journey begins with its own reconciliation and can create the synthetic work in
Desktop; it does not resume a nonexistent Code checkpoint. Completing Code later
uses the original same-call native qualification and preserves the prepared
Desktop bytes and matching installation observations when unchanged. A failed or
canceled Code attempt does not become Code readiness.

For an exact-current complete record, explicitly choosing a Desktop-first update
may be a no-op. The checker then uses the closed read-only
`--observation-client desktop` scope: the selected clients and prior validated
qualification remain unchanged, while Code prompts/probes and all-client-ready
claims are omitted from this attempt. Unknown scopes refuse. This flag supplies
no admission or native authority. Management refreshes the original checker into
a fresh private diagnostic child, preserves earlier reports, and prints the
latest Markdown path. Only the current valid client actions are offered. Actual
Desktop installation, connection, loaded methods and functional behavior remain
separate participant observations.

The short participant instructions and actual-client flow live in
[LEIA-ME.md](../dev/pilot/LEIA-ME.md) and [ROTEIRO.md](../dev/pilot/ROTEIRO.md).
They keep packaging, import, connection, cross-client work and accepted effects
separate. Automatic reports leave Code/Desktop client loading, login, real host
hooks and confirmation as `not_tested`; participants record those observations
after using the clients. Share only the reviewed Markdown report, not owner,
workspace, client profiles or participant-local generated packets/prompts.

## Automatic installed MCP functional check

In the installed participant kit, open **Maestro.command** on Mac or
**Maestro.cmd** on Windows and choose **10 — Testar MCP automaticamente**.
Selecting the action explicitly permits one synthetic document and one test
item in the work history, completed on success. It requires no development
toolchain, protocol requests or copied prompts. Ordinary startup and check-only
refreshes remain read-only.

The existing installed checker uses a dedicated authenticated diagnostic purpose
through the same admitted stable carrier. Its child does not start automatic
maintenance; ordinary client sessions and other scheduled/wake workers retain
their existing behavior. The checker purpose is not a normal-client attestation.
It checks the workspace and canonical methods/resources, converts the canonical
synthetic DOCX once, reads both derived outputs, and exercises a synthetic
create/checkpoint/pause/reconnect/resume/evidence/complete journey. It also checks
that a stale checkpoint is refused. Existing files are preserved; a conflicting
fixture or unavailable runtime leaves the relevant stage incomplete. The action
does not install or repair a converter or retry an ambiguous mutation.

The action prints a fresh **RELATORIO.md** path and preserves previous reports.
Complete resource verification and conversion can take several minutes; no
additional participant action is required while the checker is running.
Blocked, unavailable and failed checks return to the menu. Read the first pending
stage and any retained item identity before another attempt; use option **5** for
normal preparation or share only the reviewed Markdown with the team. JSONs,
raw checker output and logs remain private.

A passing result proves the installed MCP/SDK journey only. Native Code/Desktop
loading, normal login, Desktop extension and skill import, hook/agent execution,
memory and scheduler qualification retain their separate status. Continue through
the existing client options to test those experiences. This feature requires a
new compatible kit and its own installed acceptance; it does not change earlier
sealed packages or their evidence.

## Desktop owner review channel

The explicit local preparation review discloses selection of `local-browser`
before PREPARAR or digest confirmation. Both platform preparers build the
ordinary Desktop pilot with `--owner-review-channel local-browser`; its typed
inventory and canonical manifest retain that exact startup choice. This explicit channel takes precedence even when native forms are advertised.
A foreground protected request opens
its complete review and approval schema in an ephemeral local browser page,
requiring the initially unchecked checkbox and conscious exact-record approval.
Decline, cancellation, closing, expiry after at most 150 seconds or opener error
never approves. No URL or approval token is returned to the model. Generic,
stable and source profiles remain unchanged. The separately selected experimental
Code pilot may carry the same channel with `--code-pilot`; its exact local
binding/channel is retained in immutable inventory, and its native fingerprints
remain unchanged. Read-only tool permission
never substitutes for review. See [the channel contract](product-local-owner-review.md).
Actual Desktop presentation/effect/readback and native hooks remain separately
observed evidence; this local channel does not qualify MCP native forms.

## Trust and platform limits

The ordinary profile is an unsigned, unversioned engineering rehearsal; `0.0.0`
is only its portable packet label. The explicit runtime profile has a pinned
version and short-lived experimental signature. Production signing, source
enrollment and signed update remain unavailable in both profiles. Checksums identify integrity, not publisher
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

## Windows participant acquisition adapter

The clean Mac builder accepts `--engineering-acquisition COMPLETE_SIGNED_RELEASE
PUBLIC_REGISTRY`. Its local complete release is verified normally, but only the
signed non-runtime basebundle/server, notes, manifest, signatures and the two
signed sidecars from sibling `runtime-recipe/` enter the Windows ZIP. No runtime
archive, CPython, wheel or third-party DLL is copied. The development-only
`dev/pilot/verify-carrier` calls canonical acquisition.Review against the emitted
closed carrier, including every declared hash and signature, without downloading,
executing, placing or claiming whole-directory release admission.

The native preparer initializes only the isolated layout, then presents
`pilot-acquisition-review`: admitted owner/workspace ID, fixed private cache,
all public inputs, aggregate bytes, worst additional retention, attempt and
expiry. BAIXAR CONVERSOR approves that complete record. The native apply command
requires its exact digest/file, rejects altered destination/workspace, and hands
the original reviewed Plan to acquisition.Acquire. Only Windows/amd64 may acquire.
The core preserves its finite lock/cache/attempt/expiry rules and returns a new
complete private snapshot, leaving the incomplete carrier unchanged. The caller
performs whole VerifyDirectory again. A separate pilot-runtime-review and
ATIVAR CONVERSOR consent then own canonical PlaceVerified and Windows confinement
qualification; no download approval grants placement or model execution.

The same preparer exports Code with explicit CodePilot/local-browser selection
and Desktop with the existing engineering runtime profile. This adapter depends
on the independently reviewed CodePilot owner-review implementation. The test
menu preserves optional native-hook qualification as another conscious action.
Read-only doctor/startup never acquires, places, retries or repairs. Interrupted
preparations/cache remain private and preserved; choosing a fresh rehearsal is
the current guided recovery, without claiming automatic resume. Organization
signing, native Windows acceptance and actual client presentation remain distinct.

## Full Windows experimental kit construction

For the native unsigned-export, root-only-signing and public-metadata assembly
sequence, use [the complete Windows engineering kit recipe](development/windows-complete-experimental-kit.md).
Native construction and actual colleague Code/Desktop/OneDrive acceptance remain separate.

The complete experimental Windows kit selects the authenticated
`model-free-documents-v1` runtime: the original eleven formats plus native encoded
PDF text and modern PPTX, with explicit finite refusals and no OCR/model download.
Actual native/client acceptance is recorded against that exact pack and final ZIP.
