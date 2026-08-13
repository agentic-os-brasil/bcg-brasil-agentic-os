# Local installation trial

This is an engineering trial for the Maestro ZIP install path. It proves that
a ZIP release can be extracted, verified and opened in Claude Code without
Go, Python or any other runtime being present. It is **not** a signed release,
a GitHub Release installer or a full pilot distribution mechanism.

## What the trial proves

1. The ZIP extracts correctly to a local folder.
2. Opening the folder in Claude Code activates the scaffold automatically.
3. `/maestro-doctor` runs and reports capacities accurately.
4. `/maestro-onboarding` completes the first-session calibration.

## What it does not prove

- private-release authentication;
- code signing, SmartScreen or Gatekeeper approval;
- automatic update or rollback;
- install support for a non-technical pilot user end-to-end.

## Running the automated trial

Contributors run the appropriate development smoke test from a clean source
checkout:

```text
macOS/Linux: ./dev/trial-install-smoke.sh
Windows:     ./dev/trial-install-smoke.ps1
```

Each test builds a disposable ZIP, verifies checksum, extracts to a clean
workspace and removes its temporary artifacts afterward. The same checks run
in CI on Windows, macOS and Linux.

## Next distribution milestone

The next implementation must produce versioned, signed private-release
ZIP artifacts and replace the manually assembled ZIP with an authenticated,
verified download. A Claude-led onboarding prompt may call that installer only
after those conditions exist.
