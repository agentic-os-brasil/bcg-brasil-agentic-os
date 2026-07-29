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

The final action opens the installed Maestro data folder. It does not invent a
workspace: the person chooses and initializes a test workspace after the
installation. In direct preview mode, the same action explains that no
workspace exists yet.

The wizard must never turn a failed verification into an unsigned fallback.
