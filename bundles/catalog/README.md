# bundles/catalog — extension slot

This bundle is a reserved slot for user-populated skill extensions in future
releases of Maestro. It is intentionally close to empty today.

## Intent

- Third parties and BCG teams will drop their own skills into
  `bundles/catalog/skills/` without touching `bundles/base/` (the canonical
  user bundle) or `bundles/tech-core/` (engineering skills).
- The catalog respects the same OKF-1 conventions as `base` and `tech-core`.
- Skills placed here are discovered by future scaffold and session-inject
  hooks once the extension contract is finalized.

## Current status

Empty. Do not populate arbitrarily. Populating requires a release-note entry
declaring the extension contract version and update ritual — see
§3.4 of the 2026-08-12 Darwin diagnostic for the motivating context.
