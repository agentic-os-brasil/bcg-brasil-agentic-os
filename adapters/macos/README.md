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
03fe7a0bdcb12bf6fbab693fa8e5fca418b160b3` (PR #150; documentation follow-up
stacked) · live `launchctl` output: not captured in this snapshot ·
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

The implemented Canary contract uses one periodic presence wake; due
daily/weekly/monthly work is derived by the runtime worker. `status`
distinguishes the plist from
native loaded/enabled state, local IANA timezone, due work and unavailable jobs.
Pause, resume, status and uninstall are attended, idempotent and reversible.
The enrollment records the timezone and exact activated jobs; an unattended wake
uses only that persisted qualification and never obtains new authority inline.

The scheduler only wakes the process. The local Darwin worker contract has a
bounded deterministic success boundary; weekly deep review emits
proposal-only evidence. Walter and monthly structural work remain unavailable
until their owning runtime integrations are qualified. The owning subsystem
must establish its durable success boundary before a successful receipt can be
recorded.
