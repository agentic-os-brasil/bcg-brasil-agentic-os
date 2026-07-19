# Spec 003 - Pilot success

Status: proposed baseline.

## Pilot shape

Approximately ten users spanning classic consulting and technical profiles.

Windows and macOS are equal first-class pilot platforms. A capability is not pilot-ready if the normal user path works on only one of them. Platform-specific mechanics may differ, but the observable installation, initialization, diagnosis, update, rollback and data-preservation contracts must remain equivalent.

Linux remains a supported build and development target; Linux pilot parity is not an initial acceptance requirement.

## Experience targets

- Median time from installer to healthy workspace below ten minutes.
- No Git, Python, Node or Docker prerequisite for the user.
- No administrator permission required for the normal path.
- A second `init` is idempotent.
- Update preserves test fixtures representing local configuration and workspace data.
- Failed update rolls back without user data loss.
- `doctor` gives an actionable result for auth, proxy, permission and runtime failures.

## Validation sequence

1. Build release artifacts for Windows and macOS.
2. Install on one clean corporate Windows device and one clean corporate macOS device.
3. On each platform, initialize a non-Git case folder and a code repository.
4. On each platform, run `init` twice and verify idempotency.
5. On each platform, update from one pilot version to the next.
6. On each platform, simulate interruption and verify rollback without data loss.
7. Compare `doctor` results and normal-path behavior across both platforms.
8. Onboard the remaining users only after both platform gates pass.
