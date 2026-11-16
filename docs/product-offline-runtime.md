# Authenticated offline document runtime

**Current source capability:** the pinned authenticated model-free converter has
Apple Silicon macOS execution and a Windows amd64 producer, native confinement
implementation and explicit placement qualification path. Windows native kernel
and real converter qualification remain **unattested** until the ordinary-account
job actually passes. Cross-compilation, downloading and assembly establish none
of those native results. Intel macOS and Linux conversion remain unavailable.
Runtime construction, converter execution, hosted CI, installed clients and
organization-signed distribution require separate evidence.

Read this guide when assembling a runtime, diagnosing its availability, or
qualifying conversion. Source policy and fallback rules remain in
[Spec010](../specs/010-local-ingestion-runtime.md).

## Prepare exact inputs and construct the pack

The maintainer supplies an already acquired directory containing the Python
archive and all 51 wheels named by `internal/ingest/docling/runtime/inputs.json`.
`assemble.go` binds the interpreter and lock digests. Assembly checks every raw
input before creating its destination, then constructs a deterministic archive,
complete file inventory and read-only installed tree. It downloads nothing and
never runs an ambient Python or package manager.

```sh
go run ./cmd/maestro-docling-pack --inputs "$MAESTRO_AUTH_INPUTS" --destination "$MAESTRO_AUTH_OUTPUT"
```

Both variables must name explicitly selected local directories, with a new output
destination. The result is unsigned. The existing release authority separately
signs the archive and release manifest; assembly does not grant trust or install
a production key.

## Admit and execute

`docling.New` receives an owner-bound DataRoot, CLI compatibility version and a
trusted authority-registry path/digest from the native constructor. Those values
are absent from model tool arguments. Existing scripts/server packet startup
resolves its fixed registry sidecar and verifies the linker-bound registry digest;
a binary without that seed leaves conversion unavailable and preserves unrelated
MCP behavior. This slice does not introduce modern installation or enrollment.

`Status` is read-only. It verifies active registry authority, signed manifest,
compatibility, archive signature/hash, the canonical interpreter/lock/input
closure and every installed regular single-link file. Missing, extra, linked,
altered or foreign inputs refuse; checking status does not install or repair them.

`Convert` requires a canonical workspace-relative source in the [closed format matrix](product-mcp-content.md) bound to the exact
registered workspace. It bounds compressed input and expanded ZIP contents,
revalidates the source and executes a fresh private snapshot. The pinned Python
runs isolated, without site packages or bytecode writes, under native network
denial. The bridge independently checks denied IPv4/IPv6 access and empty model
and cache directories. Remote services and external plugins are disabled.

Returned Markdown and structured document bytes stay private; metadata-only
provenance records their hashes and the authenticated pack. The caller decides
whether to retain reviewed knowledge later. Conversion itself changes no memory,
wiki or authored page. Its private execution directory is removed on return.
Output/diagnostic buffers, a 90-second process deadline, cancellation and the
bridge's output budget bound execution. Unsupported formats do not trigger a
silent network or MarkItDown fallback.

## Verify the capability

- Run focused assembly, authority/refusal, policy, release-manifest and scheduler
  no-follow contracts without converter assets.
- Supply an isolated official input directory through `MAESTRO_DOCLING_TEST_INPUTS`
  to run `TestAuthenticatedRealDocling`. It builds a real pack and ephemeral
  engineering authority, converts the retained DOCX marker and exact 2x2 table,
  exercises tampering/scope/budget/refusal cases, cancelled/expired requests and
  unchanged source/archive bytes. The authority is a test fixture, not production.
- Retained conversion and alias tests require their own explicit fixture and
  independent registry seed. Their default skip is missing evidence, not PASS.
- The focused `authenticated-offline-runtime.yml` CI job acquires exact official
  inputs, requires native Apple Silicon and runs the real converter acceptance
  with no skipped cases. It retains only synthetic test logs and metadata.
- Record exact source, native OS/architecture, official-input hashes, authority
  provenance, test names/outcomes and output evidence. A cancelled request before
  execution does not prove a mid-process kill or power-loss cleanup.

The full source cannot be called delivered merely because this guide, a catalog
entry or a pack manifest exists. Hosted native conversion and final signed
installation/client acceptance remain explicit gates under issue #485 and epic #460.


## Windows amd64 construction and explicit qualification

Select the Windows closure explicitly; the historical command default and the
Darwin interpreter, 51-wheel lock and bridge bytes are preserved:

```sh
go run ./cmd/maestro-docling-pack --target windows/amd64 --inputs "$MAESTRO_AUTH_INPUTS" --destination "$MAESTRO_AUTH_OUTPUT"
```

The Windows closure uses CPython 3.12.16+20261026 from the official
[python-build-standalone release](https://github.com/astral-sh/python-build-standalone/releases/tag/20261026),
52 pinned wheels from PyPI, and the same Docling 2.145.0 SimplePipeline contract.
Ten native wheels select `win_amd64`; the remaining existing package versions
are retained. Windows-only `colorama==0.4.6` satisfies the `tqdm` and `typer`
platform markers. Upstream package notices remain inventoried within each
wheel's `dist-info` license files; no dependency installer runs during assembly.
The Windows producer also makes one app-local `python/msvcp140.dll` alias from
the exact authenticated NumPy-supplied DLL bytes, SHA-256
`a4c2229bdc2a2a630acdc095b4d86008e5c3e3bc7773174354f3da4f5beb9cde`.
Rtree imports that unmangled basename. The bridge verifies those bytes and the
actual loaded module's snapshot location before any wheel imports, and excludes
PATH/current-directory DLL search. A missing installed DLL refuses before an
ambient VC++ redistributable could satisfy it. This changes no acquired bytes
or original wheel notices.

**Redistribution authority remains unresolved.** NumPy/Pandas notices do not
grant Microsoft runtime rights. A properly licensed distributor must qualify
[the applicable Microsoft redistribution terms](https://learn.microsoft.com/en-us/cpp/windows/redistributing-visual-cpp-files)
before any team or public runtime publication, including an experimental pilot.
Local reversible source construction accepts no license and grants no such
permission. The runtime inventories this exact provenance and outstanding gate
in `native-runtime.json`.

Exact raw acquisition is 127,966,637 bytes: the interpreter is 47,113,364 bytes
with SHA-256 `590dfcbe3a34c07301e6332e13e2188b98655cd072b08afa45a7998dc4c22bc5`,
and the wheels total 80,853,273 bytes. The target lock SHA-256 is
`728aab0b59dd5011070dfc4e9159e3ed6180b1668539f84275f1b88e81f5be58`.

`BuildModernCandidate` can carry this exact target pack through the existing
modern signed setup route. The unversioned desktop pilot retains its explicit
refusal of signing/registry/version inputs. Assembly, ephemeral engineering keys
and local qualification never replace organization-controlled release authority,
Windows Authenticode, corporate device acceptance or native client loading.

Only explicit authenticated `PlaceVerified` runs Windows qualification. It
creates a fresh private execution snapshot, never a hardlink to the installed
runtime or source documents. A unique ordinary-user AppContainer profile has
zero capabilities; only that profile SID receives snapshot read/execute and
execution-local scratch access. Scratch uses low integrity. No global ACL,
firewall rule, service, admin elevation, network capability or
`ALL APPLICATION PACKAGES` grant is installed. The profile is removed after
execution, with removal failure reported. This is a disclosed per-user Windows
profile effect of explicit setup/conversion, and is never run by a query.

The process is created suspended with a stdio-only handle allowlist, checked for
the expected actual AppContainer SID and zero capabilities, attached to a
kill-on-close single-process Job with a 1 GiB process-memory limit, then resumed.
Child process creation is forbidden. The bridge observes TCP/UDP IPv4/IPv6
network denial, outside synthetic-file read/write denial, snapshot write denial,
allowed scratch writes and native child-creation refusal before using real
Docling. System files deliberately available to ordinary AppContainers remain an
OS dependency; this is no claim to an independent virtual filesystem or disk
quota. The 90-second execution deadline and existing source/output/expansion
budgets remain enforced. A platform/corporate policy that cannot meet the native
controls refuses rather than running Python without confinement.

After a real synthetic DOCX marker and 2x2 table convert successfully, placement
re-verifies installed signed bytes and atomically seals a private DPAPI proof
with UI forbidden. It binds pack and signed-manifest digests, actual embedded
production runtime/helper sources, Go toolchain and resolved schema-validator/OS/Unicode dependency
versions/sums, and hashes of owner/machine/volume/install-root/kernel-build
identity. It attests the shared converter implementation; it does not attest that
the MCP server executable ran. Test binaries or vendor builds that cannot provide
the required dependency identity cannot issue that proof.

`Status` stays read-only: it verifies current bytes and the bound DPAPI proof.
Absent/stale/invalid proof returns `unqualified` with
`requalification_required`, without running a converter, creating a profile,
repairing state or writing a receipt. Repeating explicit placement can requalify
an unchanged authenticated pack only after actual successful conversion again.
Every explicit conversion independently repeats its kernel denial controls.

## Native Windows gate

The manual-only `windows-docling-runtime.yml` uses the existing exact-source
engine builder's `-DoclingRuntime` branch and the same
`windows-core-pilot.ps1 -DoclingManifest` credentialed ordinary-account launcher.
It stages exact public source, a copied SDK, checksum-verified read-only module
cache, all 53 raw public inputs and the exact test image under a closed inventory.
It grants the disposable ordinary account read/execute access to those inputs;
private caches, profile effects and synthetic documents stay in its owned proof
subtree. Dependencies are unavailable from the network during the ordinary-user
phase. No generic command/test selector is accepted.

The closed gate requires actual pass outcomes without skips for:

- `TestWindowsNativeConfinement`: actual AppContainer token, TCP/UDP IPv4/IPv6,
  outside/private reads and writes, immutable snapshot, writable scratch and
  native child denial, including repeated phases.
- `TestWindowsNativeConfinementDeadline`: actual in-flight native Job termination.
- `TestWindowsSharedImplementationIdentityAcrossEntryModules`: independent
  root-module and MCP-module production executables compute the same identity.
- `TestWindowsRealAuthenticatedDocling`: real native modern factory, ephemeral
  engineering signed release, explicit placement/smoke, eleven format journeys,
  real export classification/conversion, tamper refusal, query preservation and
  conscious repeat qualification.

Only bounded synthetic proof/log files are retained. The workflow has no push or
pull-request trigger: publishing source and dispatching it require separately
approved scope. Until its actual ordinary-user result is inspected, native
Windows converter, client loading, clean-device and production-release gates
remain unattested.
