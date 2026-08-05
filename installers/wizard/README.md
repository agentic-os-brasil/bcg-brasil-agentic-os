# Maestro visual installer

This directory is the user-facing visual layer for the future signed Windows
and macOS installer. It is intentionally static and dependency-free:

- `index.html` is the four-step wizard shell;
- `theme.css` defines the Maestro visual identity;
- `assets/` contains the deterministic SVG mark, orbit background and status icons;
- `app.js` provides static navigation and, when served by `cmd/maestro-installer`,
  calls the confirmation-bound verification and install endpoints.

The visual layer does not establish trust, install unsigned bytes, choose a
managed root or replace the CLI. The executable installer must hand those
operations to the signed `bcgos-bootstrap` process and surface its result in
these same steps. Opening `index.html` directly remains a non-mutating preview;
the runtime bridge is detected through `/api/state`.

## Technical rehearsal

Run the packaged bridge with `--simulate` to exercise the full verify → install
→ open flow in an isolated user-space sandbox. The wizard labels this as
`ensaio técnico`, writes only deterministic rehearsal markers, and never claims
that unsigned bytes are a release. The generated DMG helper is
`dev/release/maestro-rehearsal-dmg.sh`.

## Static preview

Open `index.html` in a browser. The footer deliberately says `modo de
apresentação`; this is a non-mutating design inspection, not an installation
rehearsal.

## User promise

The copy is part of the installation contract:

- no administrator permission for the normal user-space path;
- no global `PATH` mutation;
- release verification happens before activation;
- owner data and workspaces are outside the managed update transaction;
- rollback remains available after a failed update.

After the signed core is installed, the wizard initializes the canonical local
workspace and installs the complete five-event **Claude** lifecycle projection
by default. It renders the explicit Codex projection only when the owner
selects Codex; a workspace never carries both managed projections at once. The
wizard checks the chosen runtime surface deterministically and enrolls the
explicit per-user macOS maintenance lifecycle against that workspace. The
native LaunchAgent must be loaded, enabled and identity-bound before the UI
declares readiness; failures remain visible with a reproducible remediation
command.

The finish screen keeps evidence classes separate: hooks may be configured
before a native Claude or Codex session observes them, while the maintenance
LaunchAgent is reported as native-observed only after launchctl verification.
Model-backed maintenance remains unavailable and the scheduled lifecycle never
invokes a model implicitly. In direct preview mode, no workspace or lifecycle
is created.

Claude and Codex keep separate, owner-controlled trust boundaries for project
hooks. The wizard makes that review explicit at handoff: it installs and
verifies the five definitions but never grants global trust automatically. The
owner reviews the exact commands in the selected runtime on first use; only
then can later sessions run them automatically.

The installed `--orchestration-state` argument is executable contract state,
not display metadata: hook processing rejects path escape/symlink drift, opens
the bounded durable snapshot and binds lifecycle metadata to its digest.
Session Start additionally starts the enrolled presence wake asynchronously;
the hook does not wait for maintenance or invoke a model.

The wizard must never turn a failed verification into an unsigned fallback.
