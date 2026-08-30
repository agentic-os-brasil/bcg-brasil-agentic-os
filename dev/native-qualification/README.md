# Native qualification matrix

This development-only command qualifies one exact macOS-arm64 Maestro ZIP with
the locally authenticated Claude Code or Codex runtime. It creates only
disposable Hub and synthetic Git fixtures.

```text
installers/zip/build-release.sh 0.1.13 macos-arm64
go run ./dev/native-qualification \
  --artifact dist/Maestro-Portable-0.1.13-macos-arm64-local-beta-unsigned.zip

go run ./dev/native-qualification \
  --runtime codex \
  --codex-model gpt-5.6-sol \
  --artifact dist/Maestro-Portable-0.1.13-macos-arm64-local-beta-unsigned.zip
```

The Codex path uses one persisted initialization session for the native-resume
cell plus bounded ephemeral qualification sessions with an explicitly named model,
`--approve-for-me` and its workspace-write sandbox. It ignores user/project
execpolicy rules and runs with a disposable `CODEX_HOME` containing only a
temporary copy of the existing authentication file plus a minimal config that
enables hooks. User AGENTS, skills, plugins, MCP servers and configuration are
not inherited. After the tool-free session performs local initialization, the resulting
config SHA-256 is recorded and checked again after all sessions. The matrix
bypasses only the interactive trust review for the exact
projected hooks already inspected by this synthetic matrix. Normal users must
review those hooks through `/hooks`; the product does not bypass trust.

The identity gate uses a tool-free session. Passing evidence must affirm both
the host runtime and the configured Maestro layer and contain no refusal or
prompt-injection language. Owner context, imported memory, Doctor, workspace
write, specialists, guard and linked-worktree isolation use independent native
cells so a small model cannot satisfy one check by silently skipping another.
Claude's persisted identity cell and Codex's initialization cell are resumed by
their exact native session IDs; those IDs never enter the report.

Use `--evidence <path>` to retain the bounded JSON report and `--keep` only
while diagnosing a failure. The temporary `auth.json` copy is always scrubbed,
including with `--keep`; only the sanitized synthetic fixture remains. The
default deletes the entire fixture. The report never
contains prompts, source bodies, tool payloads, local paths or native session
identifiers. It qualifies only the runtime version, operating system,
architecture and artifact digest printed in that report. Codex checks report
configured native events separately from behavior observed through context,
guard and metadata-only receipt evidence. The guard gate requires a new valid
`pre_action_guard` receipt whose metadata-only action digest matches the
requested Git command and an unchanged dirty sentinel. Continuity requires new valid
`session_start` and `context_inject` receipts, the injected sentinel, at least
one native file edit and no command, MCP or unknown tool item. Receipt files are
schema-validated, not merely counted by filename. Codex 0.149.1 surfaces
successful local hook delivery as bounded `error` stream items; these are
counted in the report as non-executable diagnostics and cannot satisfy any
behavioral gate.

The linked-worktree cell reuses the same isolated runtime home. If Codex
discovers hooks through the common Git repository, the hook adapter accepts the
native absolute `cwd` only after proving it is independently enrolled and has
the same repository ID as the configured binding. A separately enrolled
repository is rejected rather than receiving its private context.

The Codex Doctor check accepts only a successful canonical
`cat .codex/skills/maestro-doctor/SKILL.md` execution whose output identifies
the projected skill, plus the exact `bcgos <release>` version output. A failure
while qualifying, scrubbing credentials or removing the fixture invalidates
`result: pass` before the bounded report is written.

Repository tests pair the newest Claude and Codex evidence records for the
current release. The pair must cover the same platform artifact and pass every
shared qualification check. Updating only one runtime's artifact evidence is a
test failure until the other runtime is requalified against the same bytes.
