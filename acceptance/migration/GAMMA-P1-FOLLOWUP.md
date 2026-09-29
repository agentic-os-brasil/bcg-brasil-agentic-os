# Gamma P1 follow-up — committed target safety

Scope: internal/zipmigration and acceptance/migration only. No commits, no
subagents and no changes to concurrent owners' files.

## Reproduced RED

    PATH=/opt/homebrew/bin:$PATH go test ./internal/zipmigration -run "TestCommittedTargetSafetyAndRecovery|TestBOMCaseMarkers" -count=1

Six target safety subtests failed because committed target evolution swallowed
file symlink, ancestor symlink, directory type replacement, unreadable file,
unreadable directory and a newly added symlink after an earlier regular edit.
The BOM marker test failed with missing_active_case:cases/.active.

A second targeted RED found an implicit mapped ancestor problem: replacing
brain/accounts with a regular file was incorrectly interpreted as ordinary
deletion of the expected descendants.

## Fix

Committed verification first inventories all target topology and readable file
content. Only then does it compare expected kinds and hashes. New regular
files/directories, regular content edits and deletions are explicit ordinary
evolution; topology/type/permission/IO errors propagate as blocked diagnostics.
Implicit target ancestors must remain directories when they exist.

Temporary blocked observations never replace committed state or the historical
receipt, so the next SessionStart can recover after the unsafe path is removed.
Source bytes remain untouched. Original, BOM-prefixed active/pending markers are
hashed as-is; only their interpreted case identifier drops the leading BOM.

## GREEN on final code

    PATH=/opt/homebrew/bin:$PATH go test -race ./internal/zipmigration ./cmd/maestro-runtime -count=1
    PATH=/opt/homebrew/bin:$PATH go vet ./internal/zipmigration ./cmd/maestro-runtime
    PATH=/opt/homebrew/bin:$PATH python3 -m unittest discover -s acceptance/migration -v

Go race tests and vet passed. Python acceptance: 15 tests, 14 passed, 1 skipped
because this macOS filesystem cannot create same-directory case aliases;
independent Go mapped-collision coverage remains active.

The actual macOS scaffold test performs migration, commits, makes an ordinary
earlier edit, adds an unsafe later symlink, invokes SessionStart twice, removes
the unsafe symlink and invokes SessionStart again. Both unsafe calls block
without backfills or authority-file writes. Recovery runs normally, preserving
the authored edit, original data, committed state and historical receipt bytes.

Each ordinary evolution (edit, add file, delete file, delete directory) is tested
independently. Permission scenarios execute real chmod failures on macOS;
native Windows ACL evidence remains outside this local run.

The first full acceptance invocation exceeded the tool's 30-second timeout.
It was rerun with an extended tool timeout and completed successfully in
34.217 seconds; the timed-out invocation is not counted as passing evidence.

Historical evidence plus fresh final workspace/runtime checks can support a new
qualification. The historical migration PASS alone cannot establish freshness,
and normal authored work must not be reset to reproduce old hashes.
