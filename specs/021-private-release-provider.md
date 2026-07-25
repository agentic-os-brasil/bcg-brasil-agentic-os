# Spec 021 - Private release provider and update confirmation

Status: accepted contract; the macOS native backend source and conformance
tests are implemented but not connected to a current product artifact. Windows
integration, native candidate builds, provider registration and production
approval remain gates.

## Objective

Let a non-technical pilot user discover and download a private Maestro release
without Git, a personal access token, the GitHub CLI or a credential file.
Provider authentication transports artifacts; it never defines release trust.

## Authentication

The pilot provider is a GitHub App installed only on the selected private
release repository with read-only Contents access. The distributed CLI contains
the public app client ID, never a client secret, and uses GitHub's browser
device flow:

1. require an approved native operating-system credential store;
2. request a short-lived device and user code;
3. show the verification URI and user code;
4. poll using the documented pending and slow-down states;
5. persist access and refresh credentials only in the native store;
6. refresh before expiry and delete on logout.

If Keychain or Windows Credential Manager is not available through an approved
adapter, authentication is `unavailable`. Environment variables, plaintext
files, Git credential helpers and `gh auth` are not fallback stores.

No output, error or log may include access, refresh or device credentials.

### Native store adapters

The dormant macOS backend uses Security.framework `SecItem` operations against
the data-protection Keychain. The credential uses
`kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`, is bounded to 64 KiB and
never enters a process argument, environment variable or plaintext file. A
build without CGO reports the adapter as `unavailable`; a denied, locked or
otherwise inaccessible Keychain fails closed at the attempted operation.

The source has unit/conformance coverage and a read-only native probe. Current
deterministic candidates still use `CGO_ENABLED=0`, select the unavailable
fallback and do not expose the Keychain backend through
`defaultReleaseAuthService`. Therefore this PR is source-level engineering
evidence, not usable product behavior or corporate-device approval. Windows
Credential Manager must implement the same observable `SecureStore` contract;
a later wiring change must build native platform artifacts and connect both
stores under the provider/authority gate.

## Provider adapter

`releaseprovider` is the boundary for listing immutable Releases and fetching
assets. Provider API requests use the short-lived credential. An asset API URL
must stay on the configured API host. Redirects to a different object-storage
host strip authorization and API-version headers.

Download happens in two trust phases:

1. fetch only `release-manifest.json` and its detached signature;
2. authenticate the exact manifest against the approved local Maestro
   issuer/key registry;
3. fetch only release notes, artifacts and detached signatures named by that
   trusted manifest;
4. run complete release-directory verification and atomically expose the
   verified directory.

Provider-only extra assets are ignored. Missing, duplicate or renamed required
assets fail closed.

## Update planning and confirmation

An update plan is a deterministic digest over the installed release, newer
release, component versions, exact platform artifacts and target. Standard
update planning rejects same-version releases and downgrades; rollback remains
the separate bootstrapper operation.

The user-facing contract is:

```text
bcgos auth login|status|logout
bcgos update --check
bcgos update --confirm <plan-id>
```

Every command writes one schema-versioned JSON result to stdout. An available
update always requires one confirmation bound to its plan ID. Confirmation of
an unknown, stale or recomputed plan is rejected.

## Availability boundary

The core device-flow, refresh, provider-download and plan contracts are
implemented and tested without network listeners. The shipped CLI remains
`unavailable` until an approved native secure-store adapter, GitHub App
registration, repository installation and production release-key registry are
configured. It must not silently fall back to a weaker path.
