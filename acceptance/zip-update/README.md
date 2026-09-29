# ZIP update qualification

This release gate is intentionally split into offline wiring and a live Agent
qualification test. Both results apply to the exact ZIP digest under test.
Neither result proves the other.

## Offline native receipt

Run the platform-specific release ZIP on each target:

```bash
bash acceptance/zip-update/native-smoke.sh \
  --zip dist/Maestro-v0.1.12-macos.zip \
  --output macos-offline.json
```

On Windows, run natively in Windows PowerShell 5.1 and repeat in PowerShell 7
when available:

```powershell
.\acceptance\zip-update\native-smoke.ps1 `
  -Zip .\dist\Maestro-v0.1.12-windows-powershell.zip `
  -Output .\windows-powershell-offline.json
```

Git Bash and Python are not part of the Windows hook runtime. Each receipt must
report the SHA-256 of its exact platform ZIP and zero failures.

The macOS wrapper is macOS-only: its verdict is `PASS_MACOS_ONLY`, with
`qualification_scope: macos-only` and `release_qualified: false`. It accepts
exactly one deferred check, `windows-native-path-parity`, and only when the
log's SKIP reason is exactly
`native Windows drive-letter/junction parity requires dedicated PowerShell CI`.
The receipt retains `skipped: 1` and records that check in `deferred_checks`;
it does not convert the missing Windows evidence into a PASS.

The classifier rejects nonzero evaluator exit status, any failure, unknown or
additional skips, missing native Mac alias PASS assertions, malformed or
inconsistent counts, and incomplete logs. Both Mac alias checks and the
same-case positive control are mandatory. Native Windows must still produce
its own successful PowerShell evidence; its HOLD is not relaxed by this change.

Run the replay/mutation regression tests without repeating the product eval:

```bash
python3 acceptance/zip-update/test_classify_macos_eval.py -v
```

The committed fixture is a path-sanitized copy of a real 0.2.0 Mac evaluation.
Wrapper tests replay that transcript at the evaluator subprocess boundary in a
temporary repository; their receipts are test fixtures, not release evidence.

## Live Claude Agent receipt

This command contacts the configured Claude service without imposing a local
budget cap. Account or provider limits and charges still apply. It requests the
organization-permitted `opus` alias with `xhigh` effort. Run it attended and
only after approving that the fixed controlled prompt plus distributable
workspace instructions may be sent:

```bash
bash acceptance/zip-update/live-agent-qualification.sh \
  --zip dist/Maestro-v0.1.12-macos.zip \
  --trace macos-agent-trace.jsonl \
  --receipt macos-agent.json
```

On Windows, run the native equivalent first in Windows PowerShell 5.1 and then
in PowerShell 7 when available:

```powershell
.\acceptance\zip-update\live-agent-qualification.ps1 `
  -Zip .\dist\Maestro-v0.1.12-windows-powershell.zip `
  -Trace .\windows-agent-trace.jsonl `
  -Receipt .\windows-agent.json
```

A PASS requires the project `init` event with Yoda and the update contract,
both SessionStart handlers, UserPromptSubmit, exactly one real `Agent` call to
Yoda, its correlated Agent PreToolUse hook, its controlled return token, the
hub's return after Yoda and a successful Stop hook. This machine-readable
route is also the supported fallback when a Claude Code version does not expose
the interactive `/status` or `/hooks` commands. The qualification test
deliberately uses the narrow `dontAsk` permission mode plus an `Agent`-only
allowlist; it does not enable Claude Auto mode or grant file, shell or network
tools. Preserve the raw trace privately; it is diagnostic
evidence and must not be shipped to users.

## Update rehearsal

On a disposable copy of one real 0.1.11 installation, follow the exact
`LEIA-ME-PRIMEIRO.md` from the update kit. Hash representative fixture files
inside `data/agents` and `data/workspaces` before and after. The hashes must be
identical and only the new core may report 0.1.12. Repeat on both platforms.

The release gate is closed only when offline and live receipts pass for each
platform ZIP digest and both update rehearsals preserve `data/`.
