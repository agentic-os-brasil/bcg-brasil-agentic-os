# Daily checkpoint in a positively identified ZIP runtime

Use this branch only when the already loaded installed ZIP bootstrap or its
configured native hook explicitly routes daily continuity to its verified
managed helper. Read the current
[admitted owner-state paths](../../interaction-profile/resources/owner-paths.md).
The helper's current `home paths --product-root <product_root>` response must be
writable and name the admitted product_root, brain_root and owner_scope_root.
Neither a missing/failed MCP call nor this recipe admits a ZIP installation,
selects a root, initializes state or grants a host file/shell capability.

1. Select one agent-authored summary, explicit decisions and next actions for
   material work or interruption. Use the actual session ID, or one stable
   session-local UUID when unavailable. This is authored identity, not native
   session attestation. Generate a stable ID per checkpoint and retain the exact
   payload and ID on retry. No transcript parsing, tool logs or raw documents.
2. Use the admitted brain_root for owner-only work, with scope `owner`. For
   client work read only `accounts/.active` beneath that admitted brain and
   require its confirmed `<account>/<case>` pair; use
   `<brain_root>/accounts/<account>/cases/<case>` and
   `scope: account/<account>/case/<case>`. A missing/changed case leaves that
   checkpoint pending; never redirect it to owner or another case.
3. Build the canonical input below. ZIP's `workspace` is the admitted
   owner_scope_root consumed by this helper; product_root still selects the
   installed helper. Do not substitute product cwd or a product MCP workspace.

```json
{"schema_version":1,"id":"checkpoint-uuid","session_id":"session-uuid","workspace":"<admitted owner_scope_root>","scope":"owner","captured_at":"2026-09-29T14:30:00-03:00","local_date":"2026-09-29","summary":"Selected work, evidence and limits.","decisions":["Explicit decision"],"next_actions":["Next safe action"],"provenance":"agent-authored"}
```

IDs/session IDs are 1–64 ASCII alphanumerics/underscore/hyphen, starting
alphanumeric. Entire input is at most 16384 UTF-8 bytes; summary and each list item
at most 4096 bytes, with at most 16 decisions and 16 next actions. Capture date and
explicit RFC3339 offset determine the local daily page, including across
midnight. No semantic-sanitization attestation is implied.

4. Through the host's already admitted file capability, stage
   `<scope-directory>/.maestro/daily-pending/<id>.tmp`, then publish in the same
   directory without overwrite: Bash `mv -n -- <id>.tmp <id>.json`, or
   PowerShell `Move-Item -LiteralPath <id>.tmp -Destination <id>.json` without
   `-Force`. Inspect the actual final input and temporary-file outcome. An
   occupied ID retains the exact earlier payload; never overwrite it or claim a
   new enqueue when its publication did not occur.
5. Invoke the installed verified helper with its current product binding. Bash:
   source `<product_root>/.claude/hooks/lib/maestro-runtime.sh`, then
   `maestro_runtime "<product_root>" daily-stop --root "<product_root>"`.
   PowerShell: dot-source the matching verified `maestro-runtime.ps1`, then
   `Invoke-MaestroRuntime -Root '<product_root>' -RuntimeArgs @('daily-stop','--root','<product_root>')`.
   Keep any separately installer-bound user-home selection unchanged; the helper
   resolves owner_scope_root itself. Stop/SessionStart can retry pending input,
   but their configuration alone is not proof of a successful native run.
6. Inspect the content-free receipt and exact checkpoint ID. A batch handles at
   most 32 inputs. Verify the scope's actual `daily/<local_date>.md` block and
   committed-ID state before describing that checkpoint as saved/replayed;
   receipt counts alone do not identify it. Busy/failed or interrupted inputs
   remain pending. A leftover daily.lock requires explicit writer diagnosis,
   not heuristic removal. Legacy dream timestamps are normalized only by the
   existing bounded helper path; no timestamp asserts synthesis success.

This route neither enables another collector/model nor schedules work, promotes
memory layers, publishes another case's knowledge or confirms human `eod`.
