# ZIP update qualification

This gate is intentionally split into offline wiring and a live Agent canary.
Neither result proves the other.

## Offline native receipt

Run the same release ZIP on both targets:

```bash
bash acceptance/zip-update/native-smoke.sh \
  --zip dist/Maestro-v0.1.12.zip \
  --output macos-offline.json
```

On Windows, run the identical command in **Git Bash** and use a different
receipt name. PowerShell-only is not a supported profile because the shipped
Claude hooks invoke Bash. Both receipts must report the same release SHA-256,
zero failures and zero skipped checks.

## Live Claude Agent receipt

This command contacts the configured Claude service and can consume up to
US$0.30. Run it attended and only after approving that the fixed synthetic
prompt plus distributable workspace instructions may be sent:

```bash
bash acceptance/zip-update/live-agent-canary.sh \
  --zip dist/Maestro-v0.1.12.zip \
  --trace macos-agent-trace.jsonl \
  --receipt macos-agent.json
```

Repeat in Windows Git Bash. A PASS requires observed SessionStart and
UserPromptSubmit hooks, a real `Agent` call with `subagent_type: yoda`, the
Agent PreToolUse hook, Yoda's synthetic return and the hub's return. Preserve
the raw trace privately; it is diagnostic evidence and must not be shipped to
users.

## Update rehearsal

On a disposable copy of one real 0.1.11 installation, follow the exact
`LEIA-ME-PRIMEIRO.md` from the update kit. Hash representative fixture files
inside `data/agents` and `data/workspaces` before and after. The hashes must be
identical and only the new core may report 0.1.12. Repeat on both platforms.

The release gate is closed only when the offline and live receipts pass on the
same ZIP digest on both platforms and both update rehearsals preserve `data/`.
