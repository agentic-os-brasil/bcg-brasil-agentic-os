# macOS maintenance adapter

This directory contains a disabled reference template for a per-user `launchd`
wake-up. It is not installed by the base bundle and is not evidence that native
maintenance is available.

Only a future installer, after the owning executor is qualified, may render and
explicitly enable the template. Until then it must remain disabled. An enabled
adapter must:

- run as the logged-in user, without administrator privileges;
- invoke `bcgos maintenance wake --trigger presence` (or the equivalent future
  runtime-neutral entry point);
- remain bounded and asynchronous;
- never place credentials, prompts, memory bodies or client content in the
  plist;
- treat exit code `3` / `state: unavailable` as a capability gap, not success.

Installing this template as-is must not create a recurring failure loop.

The scheduler only wakes the process. The owning memory, wiki or runtime
subsystem must establish its own durable success boundary before a receipt can
be recorded.
