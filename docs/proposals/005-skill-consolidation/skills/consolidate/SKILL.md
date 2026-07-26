---
name: consolidate
description: Memory-hygiene pass over the atlas — dedup, detect staleness, archive closed items, and reconcile drift — proposing a short ranked list of confirmed changes before applying any. Use periodically, when the atlas feels messy, or when offered after a Friday eod.
---

# Consolidate

An orchestration ritual that scans across every atlas domain at once (which is why it
belongs to no single keeper) and then routes the confirmed fixes to the keepers that own
each file.

## Workflow
1. Scan the atlas for the hygiene failures: duplicates, stale facts past their as-of window,
   closed items not archived, the same number drifting in two places, orphans (a file with
   no link in or out). A read-only exploration agent may do the broad scan.
2. Propose the ≤5 highest-value changes, ranked, for one-word approval — don't auto-apply.
3. On approval, route each change to the keeper that owns the file:
   - daily/backlog/project/decisions → **`work-logger`**;
   - client files → **`client-keeper`**; people files → **`people-keeper`**;
   - development/learnings → **`career-keeper`**.

## Relations
- **Orchestration**, held by the hub. Calls the keeper agents to apply confirmed changes;
  never edits atlas files directly itself.
