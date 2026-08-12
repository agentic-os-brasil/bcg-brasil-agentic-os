# Windows platform report

Findings from running the suite on a real Windows machine, with proposed fixes.

**Environment.** Windows 11 Enterprise 10.0.26100 build 26100, go1.26.5, domain
account, `main` at `8e8ad91`.

## Read this first: elevation changes the results

Every measurement below was taken from a **non-elevated** shell. That matters
more than it sounds, and an earlier report of mine was wrong because of it.

Running the same packages from an elevated shell produces different failures,
because Windows derives a new object's owner from the token's default owner,
and for a member of Administrators with a full token that owner is
`BUILTIN\Administrators` rather than the account. Ownership checks then reject
files the current user just created.

| Package | Elevated | Non-elevated |
| --- | --- | --- |
| `internal/userlevel` | fail | **ok** |
| `cmd/bcgos-bootstrap` | fail | **ok** |
| `internal/agentorchestration` | 6 failures | 1 |
| `internal/scheduler` | ok | 1 failure |

Note the inversion in the last row: one test passes only *because* the shell is
elevated. Both directions are misleading, so any Windows result should state
which context produced it.

The installation path itself is sound: `cmd/bcgos-bootstrap` passes non-elevated,
and the refusal to install from an elevated process is deliberate and correct.

---

## Finding 1 — Unix mode bits are not an authority on Windows

**One root cause behind three unrelated-looking symptoms.** Go on Windows
synthesises `FileMode` from the read-only attribute rather than from an ACL. It
reports the same permissions no matter what was requested, and `Chmod` cannot
change them.

Measured directly:

```
MkdirAll(0700)  -> Perm() = 0777   Perm()&0077 = 0077
WriteFile(0600) -> Perm() = 0666
WriteFile(0666) -> Perm() = 0666   indistinguishable from the line above
Chmod(0600)     -> Perm() = 0666   unchanged
```

Any check treating `Mode().Perm()` as an authority therefore fails on Windows —
and fails in **both directions**, which is why the symptoms look unrelated.

### 1a — Fails closed: valid state rejected

`internal/actionconfirmation/store.go:211`

```go
if info, err := os.Lstat(store.Root); err != nil || !info.IsDir() ||
    info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
    return errors.New("confirmation store root is not a private directory")
}
```

The directory two lines above was created by `os.MkdirAll(store.Root, 0o700)`
and reports `0777`, so `Perm()&0o077` is `0o077` and the store refuses itself.
Every confirmation is denied. **Seven tests fail**, and action confirmation is
unusable on Windows.

### 1b — Fails open: invalid state accepted

`internal/agentorchestration`, `TestDurableStateRejectsUnknownFieldsNull
PermissiveModesAndOversizedJSON/permissive_mode` — "invalid durable state was
accepted".

The mirror image. A deliberately permissive file is indistinguishable from a
restrictive one, so the rejection never fires. This is the more serious
direction: a check that silently stops enforcing is worse than one that visibly
over-enforces.

### 1c — A test asserting the same thing

`internal/workspace/workspace_test.go:79` requires `Perm() == 0o600` on the
binding file, which is unreachable on Windows.

### Proposed fix

The repository already contains both halves of the answer.

**Guard the portable check**, as `internal/scheduler/private_path_api.go` does
at lines 40 and 64:

```go
if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
```

**Then assert the real thing on Windows.** `internal/agentorchestration/
file_privacy_windows.go`, added in #296, already inspects the security
descriptor and validates owner and DACL against an allowlist. Extending that
approach to the confirmation store gives Windows an equivalent guarantee rather
than an absent one.

For 1b specifically, guarding alone is not enough: the rejection has to move to
the security descriptor, or the invariant is simply unenforced on Windows and
should say so.

Where 1c is concerned the test is asserting a property the platform cannot
express, and should assert the platform-appropriate one.

---

## Finding 2 — Three tests need a privilege they do not declare

Creating a symbolic link on Windows requires elevation or Developer Mode. Three
tests create one as a fixture and fail hard when they cannot:

- `internal/scheduler/private_path_windows_test.go:44` — `TestWindowsSecureSchedulerRejectsReparseAncestor`
- `internal/workspace/workspace_test.go:150` — `TestResolveReferenceRejectsSymlinkedBinding`
- `internal/workspace/workspace_test.go:165` — `TestResolveReferenceRejectsSymlinkedBindingAncestor`

The first fails with its own message: "Windows CI must permit the reparse-point
test fixture".

**This is what produced the inversion in the table above.** These pass under
elevation because the fixture can be created, and fail without it — so a
contributor's result depends on how they opened their terminal.

### Proposed fix

`internal/atlas/atlas_test.go:15` already has the pattern:

```go
t.Skip("symlink creation requires privileges on some Windows runners")
```

Attempt the symlink, and skip rather than fail when the platform refuses. The
security property still gets covered wherever the fixture is available, and the
result stops depending on the shell.

---

## Finding 3 — External import cannot work on Windows

`cmd/maestro-installer`, `TestWorkspaceFlowRealExternalImportExecuteRollback
AndReplayGuard`:

```json
"blockers": [{"code": "plan_invalid",
  "message": "digest source note.md: secure workspace import source access is unavailable on this platform"}]
```

`internal/workspaceimport/secure_source_fallback.go` fails closed off Unix: the
descriptor-anchored source reader is implemented only for Unix, and the Windows
build has no counterpart. Failing closed is the right call for a security
primitive — but the installer wizard offers to import an existing folder, so
that path is unreachable on Windows and the user meets a blocker rather than an
explanation.

`TestWorkspaceFlowRealImportRejectsSourceChangeAndTamperedApproval` fails with
`400 "a confirmação precisa conter flow_id, plan_digest e action=IMPORT"`, which
looks downstream: no plan digest is produced when the plan is already blocked.

### Proposed fix

Two options, and the choice is a product decision rather than a technical one.

**Either** implement the Windows source reader. `internal/scheduler/
private_path_windows.go` already does descriptor-anchored, no-follow traversal
with `NtCreateFile` and `OBJ_DONT_REPARSE`; the same approach applies.

**Or** have the wizard detect the unavailability up front and not offer import
on Windows, with a sentence saying why. Today the offer is made and fails at
the end, which is the worst of both.

---

## Finding 4 — The installed executable is not found for hook configuration

Four `configureWorkspaceRuntime` tests stop at the same diagnostic:

> "O executável instalado do Maestro não foi encontrado para concluir os hooks;
> o workspace continua disponível."

State stays `configuration_pending` with `Schedule: not_configured`. The
workspace is created and usable, but no lifecycle hooks are wired, so nothing
that depends on them runs.

A fifth test, `TestConfigureWorkspaceRuntimeOnWindowsCompletesWithout
MacOSMaintenance`, records `calls = [][]string(nil)` where it expects adapter
verification followed by a stop — nothing was invoked at all.

I have not identified the root cause. The message is a symptom of resolution
failing, and knowing whether the executable is absent, at an unexpected path, or
not yet installed at that point in the flow needs someone with the installer's
intended layout in mind. Naming it here so it is not mistaken for a consequence
of the findings above — it is independent of all three.

---

## Finding 5 — `validate --full` cannot complete on Windows

`checkFormatting` in `internal/dev/harness/harness.go:210` walks the tree,
collects **absolute** paths for every Go file and passes them in one argv:

```go
command := exec.Command("gofmt", append([]string{"-l"}, files...)...)
```

408 files, 79,089 characters, against the 32,767 character `CreateProcess`
limit. `gofmt` never runs, and since it precedes `vet` and `test` in the
sequence, neither is reachable either.

Not a formatting problem — running `gofmt -l` per directory works normally.

### Proposed fix

Batch the invocation, or pass the root and let `gofmt` walk it. Until then no
Windows contributor can run the repository's own gate.

---

## Summary

| # | Finding | Severity | Fix has in-repo precedent |
| --- | --- | --- | --- |
| 1b | Permission check silently stops enforcing | **High** — an invariant is unenforced | Partly (#296's security descriptor) |
| 1a | Confirmation store rejects itself | High — feature unusable | Yes |
| 3 | External import unreachable | Medium — wizard path dead-ends | Partly |
| 5 | Repository gate cannot run | Medium — blocks contribution | No, but trivial |
| 2 | Symlink fixtures fail without privilege | Low — results depend on the shell | Yes |
| 1c | Test asserts an unreachable property | Low | Yes |
| 4 | Executable not found for hooks | Unknown — not diagnosed | — |

Findings 1, 2 and 5 have straightforward fixes and I can send them as separate
changes if that is useful. Finding 3 needs a product decision first, and finding
4 needs someone who knows the installer's intended layout.
