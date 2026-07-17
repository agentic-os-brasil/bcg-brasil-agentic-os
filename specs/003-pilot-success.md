# Spec 003 - Pilot success

Status: proposed baseline.

## Pilot shape

Approximately ten users spanning classic consulting and technical profiles.

## Experience targets

- Median time from installer to healthy workspace below ten minutes.
- No Git, Python, Node or Docker prerequisite for the user.
- No administrator permission required for the normal path.
- A second `init` is idempotent.
- Update preserves test fixtures representing local configuration and workspace data.
- Failed update rolls back without user data loss.
- `doctor` gives an actionable result for auth, proxy, permission and runtime failures.

## Validation sequence

1. Build release artifacts.
2. Install on a clean Windows pilot device.
3. Initialize a non-Git case folder and a code repository.
4. Run `init` twice.
5. Update from one pilot version to the next.
6. Simulate interruption and verify rollback.
7. Repeat on a representative BCG X device.
8. Onboard the remaining users only after the two-device smoke test passes.
