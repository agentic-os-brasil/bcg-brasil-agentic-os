# ZIP update qualification

This gate is intentionally split into offline wiring and a live Agent canary.
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
report the SHA-256 of its exact platform ZIP, zero failures and no skipped
checks.

## Live Claude Agent receipt

This command contacts the configured Claude service and can consume up to
US$1.00. It requests the organization-permitted `opus` alias with `xhigh`
effort. Run it attended and only after approving that the fixed synthetic
prompt plus distributable workspace instructions may be sent:

```bash
bash acceptance/zip-update/live-agent-canary.sh \
  --zip dist/Maestro-v0.1.12-macos.zip \
  --trace macos-agent-trace.jsonl \
  --receipt macos-agent.json
```

On Windows, run the native equivalent first in Windows PowerShell 5.1 and then
in PowerShell 7 when available:

```powershell
.\acceptance\zip-update\live-agent-canary.ps1 `
  -Zip .\dist\Maestro-v0.1.12-windows-powershell.zip `
  -Trace .\windows-agent-trace.jsonl `
  -Receipt .\windows-agent.json
```

A PASS requires observed SessionStart and UserPromptSubmit hooks, one real
`Agent` call to Darwin followed by one real `Agent` call to Yoda, both Agent
PreToolUse hooks, both correlated synthetic returns and the hub's return after
Yoda. The canary deliberately uses the narrow `dontAsk` permission mode plus
an `Agent`-only allowlist; it does not enable Claude Auto mode or grant file,
shell or network tools. Preserve the raw trace privately; it is diagnostic
evidence and must not be shipped to users.

## Update rehearsal

On a disposable copy of one real 0.1.11 installation, follow the exact
`LEIA-ME-PRIMEIRO.md` from the update kit. Hash representative fixture files
inside `data/agents` and `data/workspaces` before and after. The hashes must be
identical and only the new core may report 0.1.12. Repeat on both platforms.

The release gate is closed only when offline and live receipts pass for each
platform ZIP digest and both update rehearsals preserve `data/`.
