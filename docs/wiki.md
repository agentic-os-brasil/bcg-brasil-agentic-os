# Wiki and atlas entrypoint

This page is the navigation and maintenance entrypoint for the managed Maestro
atlas. Canonical sources remain in the repository; the atlas is a deterministic,
derived OKF bundle and must never be edited by hand.

## Source-of-truth layers

1. **Canonical contracts** — specs, accepted decisions and product-facing docs.
2. **Reviewed source map** — [`managed-allowlist.json`](../dev/wiki/managed-allowlist.json)
   selects the sanitized sources that may enter the managed atlas.
3. **Generated bundle** — [`bundles/base/atlas/managed/index.md`](../bundles/base/atlas/managed/index.md)
   contains the compiled human- and agent-readable navigation view.

The generated bundle is distributed through the base bundle. It is not a second
source of truth, a private-memory store or a client/workspace content store.

## Canonical routes

- [Content navigation contract](../specs/007-content-navigation.md)
- [Wiki update lifecycle and OKF profile](../specs/008-wiki-update-okf.md)
- [Darwin lifecycle and cadence](../specs/037-darwin-lifecycle-cadence.md)
- [Model-backed maintenance activation](../specs/041-model-backed-maintenance-activation.md)
- [Release and publication contract](releasing.md)
- [Accepted wiki decision](decisions/decision-log.md#WIKI)
- [Accepted OKF decision](decisions/decision-log.md#OKFP)
- [Accepted Darwin maintenance decision](decisions/decision-log.md#DARN)
- [Accepted bounded weekly self/state decision](decisions/decision-log.md#SILE)

## Official maintenance flow

Run from the repository root:

```sh
go run ./dev/harness wiki reconcile
go run ./dev/harness wiki validate
go run ./dev/harness wiki verify
```

`reconcile` is the only generation path. `validate` checks the OKF/profile and
Markdown-link contract; `verify` recompiles in an isolated temporary directory
and compares every generated file without mutating the checked-in bundle.

The link lint resolves source-relative links against their canonical source.
Links to allowlisted concepts become atlas routes; valid repository sources that
are not distributed become opaque `repo://` pointers; missing targets fail the
generation and validation gate.

## Current evidence boundary

The managed atlas compiler is implemented and deterministic. Managed content is
locally reviewable and distributable as a bundle, while private atlas
compilation, runtime-native navigation and pilot distribution remain separate
qualification and release gates.
