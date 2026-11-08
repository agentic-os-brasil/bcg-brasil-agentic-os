# Historical-library cohort witness

This is a **new harness linked to unmodified historical source**, not a historical
release executable. The pinned library is
`f93e9ce21edcd19246e7e5985d97489297dae18b`, immediately before the private-wiki
checkpoint extension. Direct builds remain unbound and refuse to run; the sealed
builder supplies the verified library revision.

## Build

From a Git checkout already containing that object, run
`python3 dev/pilot/private-wiki-old-cohort/build.py ABSOLUTE_NEW_OUTPUT --go GO_BINARY`.
The builder performs no fetch. It generates and retains the exact Git archive,
checks its commit header, safely extracts ordinary confined paths, and adds only
this harness's two Go files. It clears inherited Go flags/workspaces, uses offline
readonly module resolution and introduces no overlay or historical source patch.
Every original file's bytes and extracted mode are checked after compilation.
The retained manifest binds archive/library, added harness, builder, SDK and
actual executable SHA256. Existing output is refused.

The builder needs the original Git checkout/object. A public staged tree without
`.git` cannot generate this archive; build from the checkout before staging the
sealed executable. A cross-built Windows PE is not native Windows proof.

## Protocol

The development test supplies unchanged policy/budgets/freshness plus its fixture
clock in a strictly decoded JSON configuration. Paths must be absolute.

- `positive ROOT CONFIG` requires a separate empty root. Real old Bootstrap,
  plain-journal Capture, attestation, attested Capture, deterministic dreaming,
  Status and AssembleContext must succeed and recover a synthetic source marker.
  The attested envelope is an explicit synthetic skill-route fixture, not a
  native hook/client execution claim.
  A separate owned subtree proves actual old seal/capture/read/delete on supported
  platforms. Windows records its genuine unsupported owned preflight separately.
- `OP ROOT CONFIG CAPTURE_ID REVISION` calls exactly one named protected operation:
  `legacy_capture`, `legacy_status`, `legacy_context`, `owned_read`, `owned_capture`
  or `owned_delete`. Owned capture reports input sealing and the actual capture
  call separately; unsupported sealing is never presented as valid sealed input.

Receipts name actual APIs, library revision, platform, actual error and empty
return status, without returning private text. A real marker-schema refusal is
`visibility_requirement_refused`; `ErrConfinementUnavailable` remains
`confinement_unavailable`. Generic errors never become a cohort success.
The wrapper performs no protected-root checkpoint precheck.

The current test first proves the positive, reads its real artifacts with the
current reader, and checks final protected-root entries, bytes, file metadata and
Windows owner/DACL around each of six operations. Directory mtimes are excluded:
the historical library legitimately creates/removes activation locks before
refusal. This is an API compatibility test, not a same-user filesystem sandbox.
