# Spec 059 - Paired native runtime matrix

Status: accepted for implementation as development-only qualification evidence.

## Objective

One reproducible matrix must qualify the supported Maestro topology against the
same exact portable ZIP:

- Claude Code: native Maestro-folder Hub plus enrolled direct repositories and
  worktrees; and
- Codex: deterministic portable Hub activation plus enrolled direct
  repositories and worktrees.

Codex does not currently expose a Maestro-folder native Hub or managed native
specialists. The matrix must prove that limitation is declared by the canonical
capability manifest, never simulate those cells or silently count them as native.

## Paired report contract

Each run emits one bounded schema-versioned report containing only runtime,
runtime version, model where applicable, platform, release version, artifact
SHA-256, isolated runtime-config digests, boolean check results, receipt counts,
stream item counts and generic notes. It contains no prompts, response bodies,
tool inputs, paths, repository names, branch names, session IDs, auth material,
owner data or synthetic content sentinels.

The parity gate selects the newest Claude and Codex reports for the current
release and requires both reports passed; identical OS, architecture, release
version and artifact digest; and every shared direct-mode check passed on both
sides. A runtime-specific check can differ only when the canonical capability
manifest states the difference. Absence is not evidence of unavailability.

## Required matrix cells

### Hub and bootstrap

Claude runs the canonical Hub Doctor through the extracted ZIP and observes the
Hub SessionStart. Codex activates the same extracted portable core through its
transported bootstrapper and verifies the resulting private activation. Both
prove version and bootstrap integrity; only Claude may claim a native Hub
session.

### Direct skills, hooks and specialists

For both runtimes the matrix verifies the complete canonical skill projection,
native discovery of the selected Doctor/operator/memory methods, successful
Doctor execution and every hook binding supported by that host. Behavioral
hook evidence requires schema-valid metadata-only receipts and the expected
observable effect, not configuration or filenames alone.

Claude additionally discovers all five managed specialist definitions and
executes the governed sequential specialist flow. Codex verifies the same
five-role topology in its canonical orientation and verifies that native agent
orchestration remains explicitly unavailable in the capability manifest.

### Memory

The fixture creates one synthetic, owner-reviewed legacy Hub Markdown candidate.
The installed CLI performs inspect, bounded preview and dual-attested apply into
the exact enrolled workspace. A later native SessionStart must expose the
canonical imported sentinel for both runtimes, while the legacy source remains
byte-identical. No report retains the candidate, path or body.

### Native resume

Each host starts a persisted synthetic direct session with a random session ID,
records a synthetic continuity token in conversation and resumes that exact
session natively. The resumed response must recover the token without tools or
filesystem fallback. Claude persistence is redirected to an isolated temporary
configuration root; Codex already uses an isolated authentication/configuration
home. Session IDs and transcripts are deleted with the fixture and never enter
the report. `continue`, a new session or SessionStart context alone cannot
satisfy this gate.

### Distinct worktrees

The fixture creates a second real Git worktree from the same common repository
and enrolls it independently for the selected runtime. Reports require equal
repository IDs, distinct workspace IDs, clean projections and exact status for
both worktrees. Each worktree receives a different private continuity sentinel;
a native session in the second must observe only its own sentinel. Removing one
projection must preserve the other until its own explicit removal.

Codex may discover a project hook through the repository's common Git root.
Therefore a bounded native `cwd` may select another enrolled worktree only when
that worktree resolves to the same canonical repository ID as the hook's
configured worktree. An unenrolled path, relative path or separately enrolled
repository fails closed; the configured worktree remains the authority when the
native payload does not provide a worktree.

### Existing direct safety cells

The matrix retains transparent Maestro/host identity, reviewed SELF with
sensitive facet exclusion, workspace-native write, dangerous-Git denial with
unchanged dirty content, lifecycle receipts, independent second-session
continuity, isolated runtime configuration and clean reversible removal.

## Isolation and cleanup

Fixtures contain synthetic data only. Codex receives a bounded temporary copy
of local authentication and a minimal hook configuration. Claude may use local
authentication but persisted qualification state must live below the disposable
fixture. Cleanup scrubs authentication first and deletes both runtime session
stores by default. Cleanup failure changes the report to `failed`; `--keep`
retains only a sanitized fixture.

Native prompts receive only synthetic fixture content and distributed Maestro
instructions. The matrix keeps normal approval/sandbox boundaries and bypasses
only interactive trust review for the exact projected hooks already inspected
by repository tests.

## Required local tests

Implementation starts with failing tests for:

1. parity rejecting one-sided memory, resume, skill, topology or worktree cells;
2. reports rejecting mismatched artifacts or unsupported Hub/agent claims;
3. exact resume argument grammar without global approval bypass;
4. session IDs captured for execution but excluded from serialized reports;
5. distinct-worktree identity and private-context checks using real Git
   worktrees;
6. legacy source preservation and canonical-memory SessionStart evidence; and
7. cleanup invalidating an otherwise passing report.

The local gate is `go run ./dev/harness validate --full`. Native execution must
then run Claude and Codex against one freshly built ZIP, followed by the paired
parity test and two independent Claude Opus reviews.

## Evidence boundary

A green matrix qualifies only the named runtime versions, selected models,
macOS-arm64 host and artifact digest. It is not Windows-native evidence, a
signature, notarization, clean-device trial, release approval or pilot approval.
