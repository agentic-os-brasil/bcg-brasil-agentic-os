# Spec 021 - Private release provider and update confirmation

Status: accepted contract; the macOS and Windows native backend sources,
conformance tests, native candidate build path and fail-closed provider/auth
wiring are implemented. Provider registration and production approval remain
gates, so the current managed configuration stays explicitly `unavailable`.

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

The managed provider configuration is embedded in the CLI and distributed as
inspectable bundle content. It contains only the public GitHub App client ID,
fixed GitHub API/auth endpoints and selected repository coordinates. Strict
parsing rejects duplicate or unknown fields and partial registration. The
native credential store is constructed only when the complete managed
configuration state is `approved`; the checked-in production-neutral
configuration is `unavailable` and contains no client ID or repository.

### Native store adapters

The dormant macOS backend uses Security.framework `SecItem` operations against
the data-protection Keychain. The credential uses
`kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`, is bounded to 2.5 KiB for
parity with Windows Credential Manager and
never enters a process argument, environment variable or plaintext file. A
build without CGO reports the adapter as `unavailable`; a denied, locked or
otherwise inaccessible Keychain fails closed at the attempted operation.

On Windows, the dormant backend calls `CredReadW`, `CredWriteW`,
`CredDeleteW` and `CredFree` directly. It stores a `CRED_TYPE_GENERIC`
credential with `CRED_PERSIST_LOCAL_MACHINE`, so the encrypted credential is
local to the device and never enters PowerShell, a process argument or a
plaintext file. The common 2.5 KiB payload limit matches the Windows
Credential Manager maximum required by this pilot contract.

The sources have unit/conformance coverage, native error mapping and
platform-specific read-only probes. The release-candidate workflow builds each
CLI binary on its native Windows or macOS runner, with CGO enabled for macOS,
then assembles the closed candidate on Linux from those exact prebuilt files.
Every native build emits separate provenance with the source commit, workflow
run, exact runner image, Go/compiler identity, CGO mode, binary size and
SHA-256. Rolling runner images mean this records traceability but does not claim
byte-identical rebuilds across toolchain changes.
The local all-in-one candidate command retains its CGO-free cross-build fallback
for development only. `defaultReleaseAuthService` connects the native backend
only after the embedded provider registration is complete and approved.
Because the current configuration is intentionally unavailable, candidate
presence remains engineering evidence rather than usable provider behavior or
corporate-device approval.

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

The core device-flow, refresh, provider-download, managed registration and plan
contracts are implemented and tested without network listeners. The shipped
CLI remains `unavailable` until GitHub App registration, selected-repository
installation, native-store approval and a production release-key registry are
configured. It must not silently fall back to a weaker path.
