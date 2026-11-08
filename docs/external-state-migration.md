# External-state migration engineering runbook

Use this with [Spec059](../specs/059-external-state-migration.md) and decision XMIG.
This is the controlled engineering procedure. Source tests do not qualify a real
owner migration, native clients or distribution. Windows external mutation is
still unavailable; the helper must refuse before writing.

## Select and review

Use a dedicated test profile and synthetic or approved archived-package fixtures.
Keep the previous installation intact. Select SOURCE, the managed PRODUCT and the
private HOME explicitly; HOME and all control writes must be outside SOURCE and
PRODUCT. The profile may be their parent, but its locator cannot be inside them.
Close participating clients before an admitted real operation; this does not
magically fence old ZIP scripts that never consulted external authority.

For 0.1.11/0.1.12, the source is legacy data. For 0.2, current live brain is the
source, including edits/additions/deletions after v1 migration. Historical data
must not overwrite it. A data-only rename also breaks the preserved v1 runtime.

The commands below use placeholder shell variables for operator-selected paths.
The helper is the verified managed executable, not a binary discovered from cwd.

```sh
(
  umask 077
  MAESTRO_HOME="$maestro_selected_home" "$maestro_helper" migration external plan \
    --source "$maestro_previous_zip" \
    --product-root "$maestro_managed_product" \
    --user-home "$maestro_test_profile" > "$maestro_private_plan"
)
```

Planning leaves SOURCE, HOME and locator untouched. Stdout contains the exact
canonical plan; stderr reports `plan_sha256`. Review roots, source layout,
destinations, inactive controls, retained namespaces and archive location. Keep
the plan file private (`0600`). Do not edit it and calculate a new digest as a
substitute for reviewing a new plan.

## Apply and inspect

Use the reviewed digest, attempt ID and HOME from that plan:

```sh
"$maestro_helper" migration external apply \
  --plan "$maestro_private_plan" --plan-sha256 "$maestro_reviewed_sha256"

"$maestro_helper" migration external status \
  --home "$maestro_selected_home" --attempt "$maestro_attempt_id" \
  --plan-sha256 "$maestro_reviewed_sha256"
```

Progress on stderr contains phase/count metadata. Pending origin/locator records
do not admit effects. Only the complete concordant commit does. After an
interruption, rerun the same reviewed operation; do not delete partial state,
reinitialize HOME, generate a new attempt over it or copy files manually. A
conflict preserves both trees and requires diagnosis.

An already committed attempt reports already migrated and preserves target
edits. It does not recopy the historical source. Retained namespaces are readable
history; copied native receipts, capture keys, scheduler authorization, dispatch
control and pending daily/dream work remain inactive outside the active brain.
Their owning workflow must create fresh authority. A native receipt is still
required to qualify the new client.

## Enroll the chosen workspace for MCP

After status confirms a current committed HOME, explicitly enroll each workspace
with the compiled product MCP helper using the same profile and product binding:

```sh
"$maestro_mcp" init --product-root "$maestro_managed_product" \
  --user-home "$maestro_test_profile" --workspace "$maestro_selected_workspace"
```

This preserves the migrated origin and authored state while creating fresh
workspace authority under HOME/state. Existing machine enrollments remain
inactive history. Build client packets only after the selected workspace is
ready; packet generation does not prove native execution.

## Rollback without discarding new work

```sh
"$maestro_helper" migration external rollback \
  --home "$maestro_selected_home" --attempt "$maestro_attempt_id" \
  --plan-sha256 "$maestro_reviewed_sha256"
```

Rollback first checks the preserved original, revokes new authority, retains HOME
in the planned private sibling archive and restores only its owned locator and
prior empty-home state. It never overwrites the original or an existing archive.
Resume the same rollback if interrupted. Keep the archive until newer edits are
reconciled; inactive does not mean deleted.

The helper checks the actual previous binding read-only. If `MAESTRO_HOME` still
selects the archived attempt, it reports `rolled_back_binding_pending` and exit 2
even after the file transaction completed. It does not change the parent shell or
claim native runtime restoration. Correct the caller's selected binding and
reopen the supported previous runtime deliberately. Verify that workflow before
calling rollback qualified; `restored_runtime: false` remains honest until such
execution is observed.

Record source/local-test, CI, native/model, signing, publication and pilot states
separately in the complete roadmap. These commands do not authorize cleanup of
either authored history or publication of an unsigned engineering candidate.
