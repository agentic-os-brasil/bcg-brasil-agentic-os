# Spec 053 - Script-only portable distribution

Status: accepted for implementation as a distinct controlled beta. It is not
the native CLI, a signed installer, a policy bypass or a universal-host claim.

## Objective

Provide a non-technical macOS or Windows handoff that requires no Go, compiler,
Docker, administrator or Maestro native executable. The owner extracts one
target ZIP, opens `maestro-os` in Claude Code, speaks naturally and confirms
one bounded preparation. Claude invokes a text installer internally.

## Profiles

```text
Maestro-Portable-<version>-macos-shell-local-beta.zip
Maestro-Portable-<version>-windows-powershell-local-beta.zip
```

The profiles are `macos-shell-local-beta` and
`windows-powershell-local-beta`. They never reuse native-seed or source-build
names and cannot be selected through a runtime flag.

## Closure and trust

Each ZIP contains only:

- a minimized, distribution-allowlisted managed-content projection as plain
  files; redundant raw agent and skill sources are removed after the exact
  target projection is built;
- target text scripts and an optional double-click delegator;
- one seeded `maestro-os` workspace;
- a machine-readable capability matrix;
- strict path and SHA-256 inventories for the package and installed runtime; and
- bounded provenance, ZIP checksum and start-here guidance.

The factory rejects Mach-O, PE, ELF, archives, object files, bytecode,
symlinks, traversal, special files, Git history,
development tooling, credentials and user/client data. The endpoint package
contains no Go source or Go toolchain.

The delivered shell/PowerShell and managed skills/content remain readable to
the recipient. This profile avoids disclosure of the Go implementation and the
repository, not all product logic or intellectual property; a local script-only
product cannot honestly promise zero readable code.

The inventory is checked before product mutation. Because its verifier and
inventory arrive in the same ZIP, this detects extraction corruption and
post-delivery changes but is not publisher authentication. The ZIP SHA-256 is
delivered independently through the controlled channel. The profile must not
be described as signed, notarized, Authenticode-approved or production-trusted.

## Endpoint runtime

macOS uses `/bin/sh` and standard system utilities. Windows uses Windows
PowerShell and may use a CMD file solely to delegate to PowerShell without an
execution-policy override. Scripts must:

1. reject root/elevated execution before product mutation;
2. require the matching operating system;
3. validate the complete inventory and reject undeclared package files;
4. stage and inventory a version under the conventional user application root,
   and materialize a stable `~/Maestro/maestro-os` or
   `%USERPROFILE%\Maestro\maestro-os` workspace outside the extracted ZIP;
5. preserve the preceding active version for rollback;
6. replace the active-version pointer after the workspace projection succeeds;
7. project only Maestro-owned workspace files; and
8. write bounded state, project the versioned capability matrix and bind the
   seven Claude lifecycle events to target text handlers; and
9. commit a bounded projection receipt only after the full Maestro-owned
   workspace projection succeeds. The receipt binds profile, version,
   runtime-inventory digest, hook-settings digest, the exact managed
   `CLAUDE.md` block digest and
   `configured_on_disk`, but never an absolute path or work content.

Scripts never remove quarantine or MOTW, call `spctl`, change Gatekeeper,
invoke `Set-ExecutionPolicy`, pass `-ExecutionPolicy Bypass`, request elevation
or download a substitute. A script-policy rejection is an unsupported-host
result before Maestro starts.

On Windows, a pre-existing workspace `CLAUDE.md` must be valid UTF-8 without a
byte-order mark. The installer decodes it strictly before workspace mutation
and fails closed with the original bytes preserved when that contract is not
met; it never normalizes or silently re-encodes owner content.

Update installs a newer script package through the same inventory/staging
flow. Rollback switches to the exact retained prior managed-content version
and reprojects it; it never downloads or recompiles anything. Runtime version
staging and pointer replacement are transactional. Workspace projection uses a
user-local journal, a fully prepared target and a backup containing only the
prior Maestro-owned projection. A later install/update/rollback reconciles a
pending journal before new work: it restores the prior known projection and
pointer state idempotently, then retries the requested operation. Unknown live
bytes fail closed without discarding the journal. Multiple file replacement is
not physically atomic, but failure injection must prove deterministic recovery
without changing owner profile, continuity, event history, owner skills/agents
or content outside the managed `CLAUDE.md` block.
The `doctor` action proves installed-runtime integrity, the local completion
receipt, global/workspace active-version agreement, exact seven-hook
settings/handler identity, managed skill and agent projections, capabilities
and the managed `CLAUDE.md` block. Rerunning the active package repairs an
absent Maestro-owned projection. This remains configured-on-disk evidence, not
proof that a native Claude session invoked each hook and not publisher
authentication.

## Capability contract

The script profile retains only file-driven capabilities:

- Client Account Agent, Case Agent, Walter, Darwin and PA Expert as operational
  Claude project agents rendered from the same canonical definitions as the
  native adapter; Maestro remains the main-session identity;
- all file-driven skills, including account/case setup, execution continuity
  and reviewed presentation personalization; only skills that require native
  CLI authority remain excluded;
- script-specific onboarding, interaction-profile, checkup and content-lifecycle
  overlays replacing their native-only variants;
- orientation and conversational onboarding;
- atlas and managed knowledge navigation;
- managed memory/profile policies and templates;
- `continuity-lite-v1`: owner-reviewed Markdown tasks/checkpoints plus one
  bounded owner-local pointer whose validated state, logical path and
  checkpoint-presence flag may be injected at SessionStart without the task
  body;
- `session-profile-lite-v1`: an explicitly consented pointer to one
  owner-reviewed local professional profile. SessionStart may inject only the
  closed `standard|advanced|power` level, fixed relative pointer and positive
  revision after validating the profile digest; it never injects the profile
  body, digest or absolute path;
- `agent-route-lite-v1`: one bounded, session-scoped, metadata-only state
  machine over the five managed specialists. It permits only one recognized
  specialist at a time, completes the Client Account–Case–Client Account
  strategic round trip before ordinary Stop, keeps Darwin work isolated,
  permits PA Expert and Walter as direct leaf routes from an idle turn and
  stores only closed states, counters and digests of runtime identifiers;
- workspace-local managed projection; and
- Claude `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
  `Stop`, `SubagentStart` and `SubagentStop` bindings implemented by bounded
  shell/PowerShell handlers; and
- script-content status, doctor, update and rollback.

The following remain explicitly unavailable because their authority or state
machine belongs to the native CLI: signed release/provider verification,
operating-system credential storage, authenticated native hook receipts,
cryptographic external-mutation challenges, native signed specialist-route
enforcement and receipts, native schedulers, background maintenance, binary-backed ingestion
and CLI-owned execution/memory ledgers. This beta projects only the Claude
runtime surface; the canonical Codex runtime adapter is not installed by these
target scripts. Claude must not simulate unavailable receipts or claim them
active.

`continuity-lite-v1` is explicitly degraded rather than a replacement for the
native Execution Ledger. `.maestro-script/continuity-state.json` is at most
2 KiB and contains only an integer schema version, `active|paused`, a bounded
positive integer revision, one safe relative `brain/tasks/<name>.md` pointer
and boolean checkpoint presence. String coercion is rejected equally on both
platforms. The
pointed regular Markdown file remains owner-reviewed workspace data and is
never copied into hook output. Update and rollback preserve this owner-local
state. Invalid, symlinked or oversized continuity state injects no work body or
authority and is reported only as requiring repair. A missing state file means
that no reviewed continuity pointer has been registered yet.

`session-profile-lite-v1` is likewise degraded rather than native SELF or a
Session Context Packet. `.maestro-script/session-profile.json` is at most 2 KiB
and binds schema version, closed interaction-profile level, positive revision,
the exact `.maestro-script/local-profile.md` pointer, SHA-256 and explicit
session-use consent. Consent text states separately that the hook emits only
level, relative pointer and revision and that Claude may later open only a
relevant section of the reviewed Markdown to adapt the interaction. Revocation
removes only `session-profile.json` and preserves the reviewed Markdown unless
the owner separately requests deletion. The pointed regular non-link file is
at most 1 MiB. Missing state means an ordinary `standard` session; invalid, unconsented, stale or
digest-mismatched state injects a repair-required message and fails closed to
`standard`. Update, rollback and recovery never rewrite either owner-local
file. Schema and revision are bounded JSON integers and consent is a JSON
boolean on both platforms; string coercion is rejected. Facet allowlists, HMAC,
attested learning, Walter snapshots, dreaming and Codex Session Context remain
unavailable.

The script hook contract is deliberately narrower than the native hook
authority. Session and prompt hooks inject bounded capability-aware orientation.
Every event that consumes hook stdin reads at most 64 KiB plus one overflow
byte before decoding or materializing the payload; an oversized stream is
rejected without waiting for EOF or buffering the remaining sender input. The
POSIX reader preserves all input bytes, including NUL, in a bounded encoded
representation until the size check; shell-variable byte loss may never make
an oversized payload appear valid.
The pre-tool hook may deny only conservative protected-path/removal matches and
may return Claude's native `ask` decision for recognized external mutations; it
does not treat that prompt as the native identity-bound challenge. Post-tool,
stop and subagent hooks persist only bounded event metadata, never prompt,
command, tool-input or model-output bodies. `agent-route-lite-v1` may block Stop
only while a recognized bounded route is active or incomplete; this is
best-effort hook state, not authenticated native route authority. It does not
provide the native signed-packet, receipt or route-governance contract.

The installer may create `.claude/settings.local.json` when absent and may
replace it later only while its exact prior digest matches Maestro's ownership
receipt. An unrelated or owner-modified file is a conflict before replacement:
nothing is overwritten. Hook scripts and settings follow the selected runtime
version during update and rollback.

This is intentional reduced capability, not silent feature parity. A later
script implementation may promote an item only with a specific contract and
tests.

## User journey and evidence

The normal quick path is extract, double-click the target `Start Maestro`
launcher, confirm once, then open the stable `maestro-os` revealed in Finder or
Explorer so a fresh Claude session loads `SessionStart`, the remaining hooks
and project agents. Revealing the validated local directory is best-effort,
runs only after the projection receipt commits and never changes the install
result. The launcher does not try to discover or open Claude automatically.
The extracted ZIP is disposable after that handoff; owner profile and workspace
artifacts live in the stable workspace rather than Downloads.

When double-click is blocked, the explicit fallback is to open the seeded
`maestro-os` in Claude, speak and confirm once. That internal installer path
does not reveal a directory or add another prompt. Both launchers remain plain
scripts, may open a terminal window and may be blocked by endpoint policy; the
owner is never instructed to bypass the policy or type a command. Failures
explain that nothing was deleted and name one support action.

Factory tests cover deterministic ZIPs, complete allowlist projection, absence
of native/compiler content, inventory tamper, macOS install/update/rollback,
idempotence, runtime tamper detection and interrupted-projection recovery.
The Windows-native conditioned test covers the same lifecycle and recovery
contract when run on Windows. Native Windows execution, real device power-loss,
elevation rejection and paths containing spaces remain acceptance gates. Native
acceptance must use the real Slack/SharePoint/browser extraction path on one
managed macOS and Windows device without policy changes. No package can promise
every environment: shell lockdown, PowerShell execution policy, AppLocker and
EDR remain external enforcement boundaries.
