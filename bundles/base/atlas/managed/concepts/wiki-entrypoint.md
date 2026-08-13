---
type: Navigation Contract
title: Wiki and atlas entrypoint
description: Canonical source map and maintenance entrypoint for the managed atlas.
resource: repo://docs/wiki.md
tags:
    - atlas
    - navigation
    - source-of-truth
sources:
    - id: wiki-entrypoint
      resource: repo://docs/wiki.md
      title: Wiki and atlas entrypoint
status: stable
x-bcgos-profile-version: "1"
x-bcgos-stable-id: managed/wiki-entrypoint
x-bcgos-scope: managed
x-bcgos-source-fingerprint: cd8ccde65dd74bde8cd482f44f0f1289df1bcf4856b70f6c909ce815a9210d8b
x-bcgos-freshness: fresh
x-bcgos-status: active
x-bcgos-generator-version: bcgos-managed-wiki/0.2
x-bcgos-policy-version: managed-product/1
---

# Source snapshot

This managed concept is generated from the reviewed repository source `docs/wiki.md`. The source remains authoritative.

## Related

- [Content navigation through a compiled LLM wiki](/concepts/content-navigation.md)
- [Wiki update lifecycle and OKF profile](/concepts/wiki-okf.md)
- [Darwin lifecycle and cadence](/concepts/darwin-lifecycle-cadence.md)
- [Model-backed maintenance activation](/concepts/model-backed-maintenance-activation.md)

## Source content

# Wiki and atlas entrypoint

This page is the navigation and maintenance entrypoint for the managed Maestro
atlas. Canonical sources remain in the repository; the atlas is a deterministic,
derived OKF bundle and must never be edited by hand.

## Source-of-truth layers

1. **Canonical contracts** — specs, accepted decisions and product-facing docs.
2. **Reviewed source map** — [`managed-allowlist.json`](repo://dev/wiki/managed-allowlist.json)
   selects the sanitized sources that may enter the managed atlas.
3. **Generated bundle** — [`bundles/base/atlas/managed/index.md`](repo://bundles/base/atlas/managed/index.md)
   contains the compiled human- and agent-readable navigation view.

The generated bundle is distributed through the base bundle. It is not a second
source of truth, a private-memory store or a client/workspace content store.

## Canonical routes

- [Content navigation contract](/concepts/content-navigation.md)
- [Wiki update lifecycle and OKF profile](/concepts/wiki-okf.md)
- [Darwin lifecycle and cadence](/concepts/darwin-lifecycle-cadence.md)
- [Model-backed maintenance activation](/concepts/model-backed-maintenance-activation.md)
- [Release and publication contract](/concepts/release-distribution.md)
- Accepted decision anchors: `WIKI`, `OKFP`, `DARN`, and `SILE` in the canonical decision register.

## Official maintenance flow

Use the repository-owned wiki harness from the repository root with these stable
subcommands:

```text
wiki reconcile
wiki validate
wiki verify
```

The development-only executable invocation is intentionally kept outside the
managed distribution surface; the subcommands above are its stable contract.

`reconcile` is the only generation path. `validate` checks the OKF/profile and
Markdown-link contract; `verify` recompiles in an isolated temporary directory
and compares every generated file without mutating the checked-in bundle.

The link lint resolves source-relative links against their canonical source.
Links to allowlisted concepts become atlas routes; valid repository sources that
are not distributed become opaque `repo://` pointers; missing targets fail the
generation and validation gate.

## Current evidence boundary

The managed atlas compiler is implemented and deterministic. For the current
source baseline (`as_of: 2026-08-06`, commit
`43e86494b2e32ca8eccece843514b75d2c98ffa7`), the local harness records
`wiki validate` and `wiki verify` as reproducible evidence. Managed content is
locally reviewable and distributable as a bundle; private-atlas compilation,
runtime-native navigation and pilot distribution remain separate qualification
and release gates. A generated atlas page or a local wiki pass does not prove
hosted CI, native qualification, signing/notarization, Windows acceptance or
pilot readiness.
