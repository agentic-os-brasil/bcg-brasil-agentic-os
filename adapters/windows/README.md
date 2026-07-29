# Windows maintenance adapter

This directory contains a disabled reference template for a per-user Windows
Task Scheduler wake-up. It is not installed by the base bundle and is not
evidence that native maintenance is available.

Only a future installer, after workspace enrollment, local binary verification
and executor qualification, may render and explicitly enable the XML. Until
then the task must remain disabled. An enabled task must run under the logged-in
user, require no elevation, invoke the runtime-neutral `bcgos maintenance wake`
command, and keep all paths/diagnostics local. It must not carry credentials,
prompts, memory bodies or client content.

Exit code `3` / `state: unavailable` means the owning executor is not qualified;
the scheduler must not convert that into a successful maintenance receipt.
Installing this template as-is must not create a recurring failure loop.
