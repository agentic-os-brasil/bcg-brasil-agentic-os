# Local installation trial

This is an engineering trial for the BCGOS install path. It proves that a
prebuilt binary can be copied, checksum-verified, activated and used without
Go being present at runtime. It is **not** a signed release, a GitHub Release
installer or a pilot distribution mechanism.

## What the trial proves

1. A supplied `bcgos` binary runs after installation.
2. The installer preserves an existing target instead of replacing it.
3. The checksum catches accidental or altered artifact bytes before activation.
4. `bcgos init` and `bcgos doctor` work from the installed binary in an
   isolated user home.

## What it does not prove

- private-release authentication;
- code signing, SmartScreen or Gatekeeper approval;
- automatic update or rollback;
- PATH configuration;
- install support for a non-technical pilot user.

## Running the automated trial

Contributors run the appropriate development smoke test from a clean source
checkout:

```text
macOS/Linux: ./dev/trial-install-smoke.sh
Windows:     ./dev/trial-install-smoke.ps1
```

Each test builds a disposable binary, creates a checksum, uses the relevant
installer and removes its temporary workspace afterward. The same checks run
in CI on Windows, macOS and Linux.

## Next distribution milestone

The next implementation must produce versioned, signed private-release
artifacts and replace the manually supplied artifact with an authenticated,
verified download. A Claude-led onboarding prompt may call that installer only
after those conditions exist.
