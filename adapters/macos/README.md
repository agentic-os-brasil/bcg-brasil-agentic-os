# macOS maintenance adapter

This directory contains a disabled reference template for a per-user `launchd`
wake-up. The filesystem adapter in `internal/macosadapter` can install an
equivalent plist only after explicit `bcgos maintenance canary --install-macos`
opt-in; installation is adapter evidence, not native qualification.

The installer never uses sudo or shell interpolation and writes atomically into
the selected user's `Library/LaunchAgents` path. Native `launchctl` probing is
an attended qualification step and is not performed by unit tests. An enabled
adapter must:

- run as the logged-in user, without administrator privileges;
- invoke `bcgos maintenance wake --trigger presence` (or the equivalent future
  runtime-neutral entry point);
- remain bounded and asynchronous;
- never place credentials, prompts, memory bodies or client content in the
  plist;
- treat exit code `3` / `state: unavailable` as a capability gap, not success.

The current Canary path uses one periodic presence wake; due daily/weekly/monthly
work is derived by the runtime worker. Pause, resume, status and uninstall are
filesystem-only and reversible. Installing this template as-is must not create
a recurring failure loop.

The scheduler only wakes the process. The owning memory, wiki or runtime
subsystem must establish its own durable success boundary before a receipt can
be recorded.
