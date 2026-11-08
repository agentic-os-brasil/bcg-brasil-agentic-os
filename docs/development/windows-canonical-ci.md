# Windows canonical contracts under the admitted user profile

The Windows product-contracts job reuses the native standard-account driver and
retains exact source, SDK, executable and test-inventory evidence. Its current
`windows_public_external_migration_v1` profile runs eight owning package binaries:
execution, datahome, ownerctx, memory, privatewiki, processidentity, winprivate and
zipmigration. The ordinary account may read the sealed public source/SDK fixtures;
its temporary files and Go caches stay in its own private root. Native core and
atlas acceptance retain their existing no-skip contract.

## Closed profiles and history

| Profile | Scope and admission |
| --- | --- |
| No qualification_profile | Original six canonical packages; no migration public qualification claim. |
| quarantined_windows_migration_primitives_v1 | Rejected historical profile; never silently upgraded. |
| quarantined_windows_migration_primitives_v2 | Eight packages; exactly 12 kernel and 13 adapter prerequisite cases. Reporting stays primitive/quarantined, with public transaction and resolver qualification false. |
| windows_public_external_migration_v1 | The same 25 prerequisites plus the three exported transaction tests, reviewed-digest negative and sealed-source canonical resolver listed below. |

Unknown profile names, package substitutions, missing/extra/duplicate native
prerequisite cases, an altered public-family inventory or a missing public/resolver
required case refuse before creating the ordinary account. Each copied binary's
hash and complete test list must match its closed manifest before execution. A
manifest field or environment flag alone never qualifies an operation.

## Required public observations

The public profile requires exactly these exported-API cases in zipmigration:

- `TestWindowsExternalPublicReviewedPlanRefusesBeforeEffects`
- `TestWindowsExternalPublicCommitPreservesOriginalAndOfflineProof`
- `TestWindowsExternalPublicInterruptionResumesAndRollbackRetainsWork`
- `TestExternalWindowsUnreviewedDigestRefusesBeforeEffects`

It also requires datahome's
`TestResolveCommittedMigrationUsesSealedSourceIdentity`. This test runs inside the
explicit ordinary-account Windows fixture cohort and must observe canonical
writable migrated HOME, exact sealed source binding, copied-locator/unrelated-source
refusal and continued resolution after the synthetic historical source goes offline.
The fixture flag selects test coverage; production migration never reads it as an
admission override.

For every required case, the owning package must exit zero with a complete
inventory, exactly one observed PASS and no top-level or nested skip. Absence,
duplicate outcomes, failure, skip and partial inventories remain unqualified.
`public_external_migration.public_transaction_qualified` requires the observed
public cases and both prerequisite proofs. `canonical_resolution_qualified` also
requires the actual resolver case. Per-package records retain their exact required
names, observations, exit codes and log hashes.

## Subset proof and full coverage

All eight packages run even if an earlier package fails. Bounded individual logs
and canonical-contract-results.json preserve every outcome. The public/resolver
subset flags may be true while overall `qualification_complete` remains false.
Every legacy skip, including a nested skip beneath an otherwise passing parent,
remains missing coverage. The final incomplete-coverage failure is preserved; a
passing public subset does not turn the full Windows job green.

The compatible producer and unchanged historical-library witness retain independent
SDK/source/archive/executable custody and actual owning-memory outcomes. No
available environment variable, fabricated cohort record or skipped historical
case can substitute for those witnesses. Windows local private-state qualification
also does not establish native Code/Desktop client installation, enterprise device
policy, signing, distribution or same-user filesystem isolation.

Use the workflow artifact for exact source revision, source/SDK and executable
hashes, expected and observed inventories, public/resolver proof, remaining skips
and failure logs. This acceptance machinery is repository development tooling and
is excluded from the distributed product.
