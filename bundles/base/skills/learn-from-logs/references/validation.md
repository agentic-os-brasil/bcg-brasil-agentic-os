# Local selector qualification

The privacy selector was developed test-first in the v0.2.0 consolidation.
Behavioral stub RED: eight tests failed for missing scope checks/read behavior.
After implementation, all passed. An additional unknown-export-container test
failed against permissive object handling, then passed after fail-closed support.

Command: /usr/bin/python3 -m unittest acceptance/test_log_selection.py

Coverage: explicitly selected session only, foreign workspace before body open,
missing consent, source/session/total-byte limits, oversize before read, symlink
and traversal rejection, changed selection/source rejection, digest-bound batch
resume, same export limits, worktree mismatch and unknown export container.

The reader opens approved files through no-follow directory descriptors and
checks file identity before/after its bounded read. It has no network or durable
write path. Its output includes ephemeral bodies separately from metadata-only
audit/cursor. The caller must never persist the full result as an audit log.

This is optional Python tooling. Hosts without Python or secure no-follow
descriptor support report unavailable and keep manual onboarding usable.
Native Windows secure-reader qualification is not claimed. Critical hooks do
not depend on Python. Product registration/distribution belongs to integration.

Skill consumer pressure scenarios are documented in extraction-and-mapping.md.
They have not been executed in a fresh native agent session. Deterministic tests
are not evidence of agent behavior, host qualification or a live user import.
