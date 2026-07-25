# Pilot hook conformance run

This is an operator runbook, not an end-user onboarding step. Run it in a new,
non-client test workspace on each supported platform and runtime before
describing native lifecycle injection as available to pilot users.

## What this proves

It proves three separate things: Maestro generated a safe bounded payload; the
runtime accepted the workspace-local configuration; and a new native session
actually invoked it. Do not treat one of these as proof of the others.

## Run

1. Install a released `bcgos` binary and create an empty local test workspace.
   Do not use OneDrive, a client workspace or `go run` for this evidence.
2. Initialize the workspace and install one runtime adapter:

   ```text
   bcgos init <test-workspace>
   bcgos adapter install --runtime claude <test-workspace>
   # or
   bcgos adapter install --runtime codex <test-workspace>
   bcgos adapter status --runtime <runtime> <test-workspace>
   bcgos doctor <test-workspace>
   ```

3. Open the workspace-local runtime configuration and verify that it contains
   one Maestro command, an absolute executable path and `timeout: 2`. Existing
   user hooks must still be present. If this configuration was already tracked
   by Git, stop: Maestro correctly refuses to overwrite it. Remove it from the
   index or use an untracked workspace-local configuration before retrying.
4. Copy that exact command and run it once in the test workspace. Confirm it
   emits `hookSpecificOutput`, `SessionStart` and a pointer-only packet; no
   client, owner or memory body may appear.
5. Start a fresh Claude Code or Codex session in that same workspace. Use the
   runtime's own hook diagnostics or visible session context to confirm that
   the command ran. Record an explicit failure if it did not.
6. Run `bcgos adapter uninstall --runtime <runtime> <test-workspace>` and
   confirm Maestro's entry is gone while unrelated entries remain.

## Record

Create one short receipt per runtime/platform in the pilot evidence location:

```text
date · Maestro version · OS version · runtime/version
workspace config path · direct command result · native-session result
removal result · operator · issues or omissions
```

Never include the packet body, user identity, customer names, workspace paths
or screenshots of client work in the receipt.
