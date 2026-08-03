# macOS maintenance adapter

This directory documents the per-user `launchd` wake-up. The adapter in
`internal/macosadapter` renders and installs an equivalent plist only after
explicit `bcgos maintenance canary install-macos --confirm` opt-in.
Filesystem installation and native `launchctl` loading are reported separately.

The evidence classes are separate as well: a rendered plist is **configured**;
the deterministic worker and repository fixtures are **local contract**
evidence; a live `launchctl` result qualifies only the macOS scheduler. It does
not qualify Claude/Codex lifecycle invocation or the owning Darwin runtime.
Evidence snapshot: `as_of: 2026-08-02` · `base_commit:
61c66dfecbdc21bff6137398243236232fd14988` (current PR #150 parent; documentation
follow-up stacked) · live `launchctl` output: not captured in this snapshot ·
release/pilot evidence: not present.

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
successful receipt with `MaxCatchUp=1`. `memory-checkpoint` is the only
locally qualified memory handler: it emits a workspace-bound metadata-only
receipt and reads or synthesizes no memory body. `memory-light-dream` shares
the three-hour due contract and `memory-deep-dream` is weekly, but both remain
unavailable until a synthesis adapter is qualified.

The checked-in template passes `--idle-state unknown`, which is intentionally
fail-closed: unknown is not idle. A qualified activity observer may provide an
explicit `idle` or `active` value at the same CLI boundary. Suppressed due work
remains due, suppression receipts do not advance the success anchor, and a
15-minute cooldown prevents receipt loops. `status`
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
