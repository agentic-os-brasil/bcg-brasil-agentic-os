# Windows canonical contracts under the admitted user profile

The Windows product-contracts job reuses the native standard-account driver for
six unchanged canonical test executables: execution, datahome, ownerctx, memory,
privatewiki and processidentity. The first profile run is intentionally diagnostic:
retain failures from obsolete oracles and public schema admission before changing
those owners. A private-owner refusal is never converted to success by altering
ownership or relaxing the guard.

The hosted job archives its exact committed source into a synthetic, public,
read-only fixture tree and stages the pinned SDK there. Test executables retain
that source directory for package-relative schema fixtures and runtime.Caller
policy fixtures. The ordinary account may read these inputs but cannot modify
them. Its temporary files and Go caches are created inside its own private root.
The SDK executable hash and its actual selected root are checked before command
evidence; caller-supplied GOROOT is rejected.

The driver verifies the six-package manifest, executable hashes and exact test
names against each copied binary. It runs every package even if an earlier package
fails and records bounded individual logs plus canonical-contract-results.json.
Each listed top-level test must have an observed outcome. Failures, incomplete
inventories and unclassified skips keep the canonical qualification incomplete.
The native core/atlas acceptance checks keep their existing no-skip contract.

Current canonical packages include Unix-specific counterparts, obsolete Windows
unsupported expectations and cross-binary tests requiring real compatible/older
producer executables. The diagnostic report preserves their actual names and
outcomes; an available environment variable or fabricated cohort record is not a
substitute for a real historical executable witness. Resolve each demonstrated
coverage gap before reporting full Windows canonical qualification.

Use the workflow artifact for the exact source, SDK, package hashes, expected and
observed test inventories, skips and failure logs. A later protocol or static-check
step that never executes remains unvalidated. This acceptance machinery is
repository development tooling and is excluded from the distributed product.
