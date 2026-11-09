# Build and qualify a native Code plugin

The shared Maestro MCP server works independently of native Code agents and
hooks. Use this guide when building a participant-bound Code packet or running
the attended qualification that can enable its native adapter. For product
initialization and ordinary MCP operation, use the
[MCP module guide](../adapters/mcp/README.md). The authority contract is
[Spec063](../specs/063-product-native-plugin.md).

| Packet | Native behavior |
| --- | --- |
| `plugin` | Builds the complete portable MCP Code packet, including six inert canonical specialist projections, with an explicit native-unavailable reason and no active hooks. Packaging starts no qualification or model session. |
| `plugin --lifecycle-context` | Builds the explicit `mcp_lifecycle` Code packet with three non-enforcing context/observation hooks. It does not qualify native enforcement or project specialists. See the [lifecycle context guide](product-lifecycle-context.md). |
| `native-qualify-and-build` | Reviews and exercises the exact local target, then immediately builds a native packet only after successful qualification. |
| `mcpb` | Carries the same shared server and immutable resources for Desktop. It installs neither Code agents/hooks nor Desktop Skills. |

The implemented native profile is bounded sequential `maestro_planner`.
`native_advisory` remains unavailable. This construction supplies an executable
qualification path; controlled source tests have not established actual native
CLI, Code-tab or Desktop acceptance.

## 1. Select the actual target

Use an initialized product/home/workspace binding that the shared product can
open. Select a synthetic, non-client workspace for the experiment: the actual
Code process uses this workspace as its current directory and may load and
send its effective instructions and context. A qualification from another
workspace cannot be transferred to this one.

Prepare the following operator-selected inputs:

- The actual installed native Code executable and compiled Maestro server must
  match the running OS/architecture. A script or invented version string cannot
  supply executable evidence. The dedicated enforcement minimum is Code
  `2.1.295`; Windows private native admission is currently unavailable.
- The public Maestro server file must be outside the selected workspace. It is
  the exact harmless outside-read target used to test canonical denial.
- The scratch root must already exist, be private and owner-checked, and lie
  inside `--user-home`. It must be disjoint from the product, selected workspace
  and resolved canonical state directory. Use a sibling of canonical state,
  rather than placing experiment files inside it.
- The challenge path must be a new, absent directory strictly inside the
  selected workspace, with an existing valid parent. The confirmed operation
  creates it for synthetic control files. The packet destination must also be
  new; existing contents are preserved on refusal.
- The selected installation must report a supported version and an authenticated
  normal configuration. Use its ordinary login flow if authentication is
  unavailable, then produce a fresh review.

The experimental native profile requires an existing explicit
`autoMemoryEnabled: false` in its selected local settings. It binds a bounded
superset of supported local instruction files, rules, imports and startup-mode
controls, including missing-source membership. Unbounded additional-directory
or third-party-plugin sources, unsupported managed-settings directories,
ambiguous import/alias layouts and unsupported Git worktree settings return
unavailable. The qualifier never changes the owner's profile automatically.
Implicit/default auto-memory is outside this native profile; ordinary MCP
operation remains available. Host identity is streamed under a separate
512-MiB bound, while the packet server retains its 128-MiB payload bound.

| Supported local-closure bound | Limit |
| --- | ---: |
| Transitive instruction import hops | 4 |
| Regular source files | 256 |
| One source file | 256 KiB |
| Total source content | 2 MiB |
| Instruction ancestry | 64 levels |
| Recursive rules | 32 directory levels |

Overflow, missing imports, cycles and unsupported aliases return unavailable
before effects. Normal Git-root local settings are included; worktree .git
files, bare/incomplete repositories and unsupported Git configuration refuse.
Inactive ancestry files remain fingerprinted without satisfying the required
active auto-memory setting. The qualifier hashes only named instruction/memory
environment controls, never credential namespace values.

Keep development and experiment writes physically under the approved Developer
directory. Existing credentials remain with the selected Code installation;
this path creates no alternate login profile or client registration.

## 2. Read the complete local review

The following is a Bash template. Replace every placeholder with an explicit
absolute path and the intended packet version. `--user-home` is the admitted
owner root; select the same value used to initialize this product binding.

```bash
MAESTRO_MCP="/absolute/path/to/Developer/maestro/bin/maestro-mcp"
MAESTRO_PRODUCT="/absolute/path/to/Developer/maestro/product"
MAESTRO_OWNER="/absolute/path/to/Developer/maestro/owner"
MAESTRO_WORKSPACE="/absolute/path/to/Developer/maestro/synthetic-workspace"
MAESTRO_CODE="/absolute/path/to/installed/native/claude"
MAESTRO_PACKET="/absolute/path/to/Developer/maestro/packets/new-code-packet"
MAESTRO_VERSION="0.2.0"

native_args=(
  --product-root "$MAESTRO_PRODUCT"
  --user-home "$MAESTRO_OWNER"
  --workspace "$MAESTRO_WORKSPACE"
  --server "$MAESTRO_MCP"
  --destination "$MAESTRO_PACKET"
  --version "$MAESTRO_VERSION"
  --native-executable "$MAESTRO_CODE"
  --native-scratch-root "$MAESTRO_OWNER/native-scratch"
  --native-challenge-directory "$MAESTRO_WORKSPACE/native-challenge-new"
  --max-budget-usd 0.25
)

"$MAESTRO_MCP" native-review "${native_args[@]}"
```

`native-review` performs bounded inspection of actual executable bytes, target,
binding and effective settings/instruction sources. It invokes the selected
executable's `--version` and `auth status`, each with a five-second context and
bounded output. Authentication inspection requires `loggedIn`, an authenticated
`authMethod` and a usable `configDirectory`; the settings/instruction files in
that configuration are fingerprinted without reading credential files.

Maestro creates no scratch, packet, registration or model session during review.
Running the installed CLI for inspection does not attest that the external CLI
never writes its own diagnostics.

Read the full `plan`, including host/server hashes, exact target and paths,
configuration/profile/binding fingerprints, context exposure, metadata writes
and limits. The operator selects and trusts the local installation; its vendor
provenance is not independently attested by this process. A `reviewable` result
means that the attended experiment is ready
for confirmation; it does not mean native qualification succeeded. Complete
this step only when all values match the intended target and the operator
accepts the disclosed effects.

The review explicitly marks remote or OS-delivered managed policy
**UNATTESTED**. Such policy may affect startup and loaded context, and the
current host protocol does not expose a complete source closure for it. Known
local managed files remain bound or explicitly unsupported. Confirmation does
not prove that remote policy is absent, and the local auto-memory setting is
not evidence of complete managed-policy behavior. A successful native
observation establishes the supported local source binding and observed guard
mechanics; enterprise device-policy admission and production rollout remain
separate acceptance gates.

The reviewed bounds are **at most three native CLI sessions, eight parent
turns per session and 120 seconds per session**. Agent children can make
additional provider requests. `--max-budget-usd` is mandatory and must be
greater than zero and at most one USD. At `0.25`, the displayed aggregate
estimated limit is `0.75` USD. Provider billing and CLI limits are estimates,
not hard spending ceilings. An observed per-session estimate above the reviewed
amount stops remaining sessions.

## 3. Confirm that exact review and build

Use the same arguments and the exact `review_digest` from the complete review:

```bash
MAESTRO_REVIEW_DIGEST="paste-the-exact-review_digest-here"
"$MAESTRO_MCP" native-qualify-and-build "${native_args[@]}" \
  --review-digest "$MAESTRO_REVIEW_DIGEST" \
  --confirm
```

The command reconstructs the review before effects. Changes to the selected
target, host, server, authentication or profile require a new review. It then
creates private candidate packets, synthetic workspace files, exact dispatch
session metadata, canonical flow metadata and lifecycle receipts for the
experiment. Scratch and observations are retained for diagnosis.

Successful genuine evidence creates an opaque `NativeHost` inside that process
and passes it immediately to the packet builder. A saved result, serialized
approval flag or earlier packet cannot mint that authority. A later ordinary
`plugin` command starts without admission and therefore builds MCP-only again.
Treat later host/server/settings/target changes as a new qualification; the
construction does not promise automatic invalidation of previously copied
packet files.

| Result | Operator action |
| --- | --- |
| `state: qualified`, `evidence_class: native_observed` | Inspect the generated packet inventory and retained evidence. Registration, native loading and release acceptance remain separate steps. |
| `state: unavailable` with a concrete reason | Keep ordinary MCP use available. Supported unavailable paths produce an MCP-only packet; inspect the returned reason and actual files before proceeding. |
| Changed review, unsafe input or filesystem error | Preserve existing state, correct the selected input and obtain a fresh full review. An error does not promise a packet was produced. |
| Incomplete challenge evidence | Retain the reported private scratch/session metadata and investigate the exact failed control. Assistant claims cannot promote the result. |
| Timeout | Qualification is unavailable. Cancellation targets the owned Unix process group; independently detached descendants and cleanup remain unattested. Use the exact retained session metadata for diagnosis. |

The command does not install/register Code, suppress native permission prompts,
clear a live fence or perform global process cleanup. A partial result may be
printed before a nonzero exit when scratch already exists; assess both the
result and exit status. Use a fresh absent challenge and destination for a new
attempt, retaining the earlier evidence.

## What the experiment observes

The fixed protocol uses the selected workspace and its normal project/local
hook settings. An empty MCP configuration and disallowed MCP tools narrow the
experiment; managed policy may still affect startup connections. Those
connections are neither used as qualification evidence nor qualified by this
result.

| Control | Required observation and boundary |
| --- | --- |
| Startup | Canonical read-only startup leaves the bounded selected state/workspace metadata unchanged. |
| Positive read | The native host successfully reads the exact synthetic allowed file. |
| Canonical denial | The attempted read of the public server outside the workspace produces the unchanged canonical guard's nonce-tagged denial and the host's blocking tool result. |
| Missing helper | The native host reports the failed enforcement hook and blocks the synthetic Edit. |
| Nonzero helper | A separate session reports helper failure and blocks the synthetic Edit. |
| Timed-out helper | A separate session reports the timeout and blocks the synthetic Edit. |
| Case → Gamma | The bounded sequential dispatch, native-ID correlation and stops agree with canonical flow metadata. These observations establish neither artifact custody/delivery nor a Yoda verdict or completed work item. |

Candidate-only instrumentation checks startup, tags the unchanged canonical
pre-tool decision using supported `systemMessage`, and adds a separate all-tool
scope fence. The scope fence permits only the reviewed controls and bounded
Case/Gamma dispatch; it cannot supply PASS evidence for normal denial or helper
failure. Fault injection targets synthetic Edit operations. The negative Read
is nondestructive even if enforcement fails open.

The verifier correlates actual tool attempts, hook/session IDs, native
failed-hook envelopes and host-generated blocking tool results. Timeout also
needs a timeout notice. Helper stderr, assistant prose and absence of a changed
sentinel file are insufficient. Native CLI events are emitted through
`--print --output-format stream-json --verbose --include-hook-events`, following
the [official CLI reference](https://code.claude.com/docs/en/cli-reference),
[hook message envelopes](https://code.claude.com/docs/en/agent-sdk/typescript#sdkhookresponsemessage)
and [hook output/failure contract](https://code.claude.com/docs/en/hooks).

Active product hooks use the selected server's direct executable argument
vectors and event-aware failure behavior. Their normal output contains no
qualification nonce markers. Each invocation revalidates the product binding.
Pre-tool policy admission retains native permission prompts and does not prove
filesystem race confinement. Nonblocking native events cannot inherit a
prevention guarantee; stop observations remain separate from verified results.

## Inspect the packet before loading

`packet-inventory.json` remains the closed file inventory with exact hashes,
sizes and modes. `maestro-packet.json` adds the selected OS/architecture,
method/resource identities, all seven canonical identities and
fourteen immutable agent assets, policy, provenance, licenses and any admitted
native files. Executable Agent calls use exact `maestro-product:<canonical-id>`
names. Maestro is the main-session hub; the six specialist projections preserve
their current tool contracts. Gamma currently performs tool-free bounded packet
analysis, rather than source inspection or full review custody.

Reference closure distinguishes embedded assets, external-source references and
individually unavailable legacy helpers. An inert resource is not an executing
capability. MCPB exposes the same server/resources and a typed inventory, while
keeping native Code mechanics out of the Desktop carrier.

Both carriers preserve the existing external Owner directory and Workspace
selectors and relocation tokens. Redistributable packets contain no builder's
private selection. A participant-local native build adds only the digest of its
reviewed owner/workspace identity to the existing hook command. At runtime that
optional selection check runs after full packet validation and before the hook
adapter. Moving the managed packet preserves the selection; selecting another
owner, workspace or workspace identity refuses. The digest checks equality and
cannot mint native admission or authorize another build. Private qualification
candidates retain exact synthetic local hook arguments only under the selected
scratch directory and are labelled `qualification_candidate`.

Engineering tests can create explicitly labelled `native_engineering_fixture`
packets from controlled processes. Those packets and compiled hook regressions
remain engineering evidence. Actual CLI qualification, Code-tab behavior,
Desktop loading, Windows private admission, organization signing, trusted
distribution, genuine delegation delivery and the team pilot require their own
evidence. Native success on one target does not close those gates.
