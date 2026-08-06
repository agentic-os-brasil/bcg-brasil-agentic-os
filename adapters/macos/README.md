# macOS maintenance adapter

This directory documents the per-user `launchd` wake-up. The adapter in
`internal/macosadapter` renders and installs an equivalent plist only after
explicit `bcgos maintenance canary install-macos --confirm` opt-in.
Filesystem installation and native `launchctl` loading are reported separately.

The evidence classes are separate as well: a rendered plist is **configured**;
the deterministic worker and repository fixtures are **local contract**
evidence; a live `launchctl` result qualifies only the macOS scheduler. It does
not qualify Claude/Codex lifecycle invocation or the owning Darwin runtime.
Evidence snapshot: `as_of: 2026-08-06` · source baseline:
`b3d85edeac16816ccca8b69cf887a7d674786710` (`origin/main` at the snapshot;
the documentation refresh is pending integration) · live `launchctl` output:
not captured in this snapshot · release/pilot evidence: not present.

The installer never uses sudo or shell interpolation and writes atomically into
the current user's `Library/LaunchAgents` path. On macOS, the attended lifecycle
uses structured `launchctl bootstrap`, `enable`, `kickstart`, `print`,
`disable` and `bootout` calls with bounded timeouts. Fixture homes are always
filesystem-only and cannot load the real user's domain. An enabled adapter must:

- run as the logged-in user, without administrator privileges;
- invoke `bcgos maintenance wake --trigger presence` (or the equivalent future
  runtime-neutral entry point);
- remain bounded and asynchronous;
- never place credentials, prompts, memory bodies or client content in the
  plist;
- treat exit code `3` / `state: unavailable` as a capability gap, not success.

The implemented Canary contract uses one 15-minute presence pulse. The pulse
only asks the scheduler for due work and hands eligible occurrences to the
depth-one worker; it is not a 15-minute job cadence and does not run handlers
inline. Three-hour continuity is anchored to enrollment and then the last
successful receipt with `MaxCatchUp=1`. `memory-checkpoint` atomically advances a versioned,
workspace-bound watermark over allowlisted durable scheduler metadata, then
emits its own receipt. It reads or synthesizes no memory body and preserves the
last-known-good pointer if publication is interrupted. `memory-light-dream`
shares the three-hour due contract and is locally qualified through the bounded
deterministic L1 synthesizer over already-sanitized captures. It cannot write
L2, L3 or lifetime. `memory-deep-dream` remains weekly and unavailable until
deep synthesis plus lifetime eligibility are qualified.

The checked-in template passes `--idle-state auto`. On macOS this invokes the
bounded native HID idle probe with a two-second timeout and a five-minute idle
threshold. Missing, malformed, timed-out or unsupported observation becomes
`unknown` and fails closed; it is never assumed idle. Suppressed due work
remains due, suppression receipts do not advance the success anchor, and a
15-minute cooldown prevents idle-suppression loops. Recent failed or
unavailable attempts have a separate per-occurrence cooldown; their suppression
also remains due and retry resumes after expiry. `status`
distinguishes the plist from
native loaded/enabled state, local IANA timezone, due work and unavailable jobs.
Pause, resume, status and uninstall are attended, idempotent and reversible.
The enrollment records the timezone and exact activated jobs; an unattended wake
uses only that persisted qualification and never obtains new authority inline.

The scheduler only plans occurrences and the adapter only wakes the process.
The local Darwin worker contract has a
bounded deterministic success boundary; weekly deep review emits
proposal-only evidence. Walter and monthly structural work remain unavailable
until their owning runtime integrations are qualified. The owning subsystem
must establish its durable success boundary before a successful receipt can be
recorded.
