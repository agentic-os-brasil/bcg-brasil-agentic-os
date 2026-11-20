# R17: registryRead-only Windows LPAC experiment

Status: isolated synthetic CI candidate; no shipping or real-profile authority.
Decision: [RGCI](../decisions/decision-log.md#rgci---confine-one-registryread-experiment-to-synthetic-windows-proof).

## Run and interpret

1. Review the exact candidate commit against preserved zero-capability commit
   `c72250a53764719ded9cc16309c466eac387f513`. Complete independent source review
   before dispatch. This branch changes the actual converter launch, not a test
   substitute.
2. Use only the existing `Windows authenticated Docling runtime` workflow,
   `mode=proof`, its disposable GitHub-hosted Windows runner and fresh ordinary
   account. Use the original synthetic corpus. Real user machines/profiles,
   private hives, credentials and client data are outside this approval.
3. Require the actual suspended child to have its expected AppContainer SID,
   exactly one derived `registryRead` SID with attributes `SE_GROUP_ENABLED`, and
   the existing LPAC exact mask6: package and ARAP allowed, AAP excluded. Any
   missing, extra, foreign or differently attributed capability refuses before
   resume. Capability presence does not prove network denial or compatibility.
4. Require the same four native parents, all eight strict `WSAEACCES` probes,
   filesystem/snapshot/child/Job controls, thirteen positive formats, six PDF
   refusals and nine PPTX refusals. Keep failures and skips fatal. Preserve the
   same converted/exported runtime-pack identity checks; no new converter route.
5. Retain only bounded synthetic proof/logs under qualification profile
   `windows_docling_registry_read_r17`. The original factory accepts only the
   baseline `windows_docling_documents_v1`, so experimental proof cannot enter
   its export or assembly path. The workflow also rejects non-proof modes.
6. Report startup, confinement and conversion separately. A complete pass is
   evidence for this source/host experiment only. On failure, stop this
   hypothesis; further capabilities require a new decision. No new signing,
   packaging, real-client loading or distribution follows from success.

## Actual permission and privacy scope

`registryRead` is a named capability under the application-capability namespace.
It is distinct from the built-in package/ARAP identities. It broadens registry
read eligibility; Windows still intersects the ordinary user's rights with
package/capability resource ACLs. It does not make the process administrator or
by itself grant registry writes. No setup code writes registry values, and this
experiment adds no independent negative registry-write proof; the capability
name alone is not a measured statement about every resource ACL. The exact keys readable on a target host and
whether private user/application settings become readable are not established.
Do not describe this as private-registry exclusion or a proven minimal fix.

The experiment uses one fixed capability name, not caller-supplied names, SIDs,
flags or a capability framework. Profile registration stays capability-free;
only the owned child token receives this single capability. No resource ACL,
registry value, firewall/service policy or persisted system permission changes.
The child retains its closed synthetic environment; no parent credentials,
proxy configuration or secret-bearing environment is copied into it.

There is no registry-key enumeration or retained registry content. Normal
Windows/Python startup may perform reads allowed by the candidate token. Use
only the fresh synthetic account. Before any real-profile test or distribution,
prepare the concrete readable resource/privacy scope and obtain approval for
newly accessible private user/application settings. This experiment supplies
neither that scope nor that approval.

## Evidence and binding

The Windows production source is embedded in `runtimeidentity.Docling`; changing
its capability selection/attestation changes the existing implementation-bound
private qualification identity. Old zero-capability receipts cannot qualify
this candidate, and its receipts cannot match a baseline implementation. The
private receipt schema remains2; no imported proof or alternate issuer exists.
The driver also binds candidate profile, exact source revision, sealed public
inventory, actual test image, ordinary account, pack and complete test log.

The current failures motivating this hypothesis are WSAStartup10107 and Python
activation-context/SxS `0xc0000022`; their shared cause is unknown. Microsoft
states LPAC's registry restriction. Chromium's pinned broader LPAC setup uses
registryRead alongside other capabilities; that is precedent, not proof that
registryRead alone fixes either failure.

Primary sources:

- [Microsoft LPAC construction and resource model](https://learn.microsoft.com/en-us/windows/win32/secauthz/implementing-an-appcontainer)
- [Documented SID derivation and LocalFree ownership](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-derivecapabilitysidsfromname)
- [Pinned Chromium setup, commit20f043d5](https://github.com/chromium/chromium/blob/20f043d5ef7456dc6ef4a0652519b171f225f8d2/sandbox/policy/win/sandbox_win.cc)

## Pinned settings import compatibility candidate

The Windows `model-free-documents-v1` assembly applies the fixed
`pydantic-settings-lazy-asyncio-v1` transform to the authentic 2.16.2 source.
Its upstream wheel and raw lock remain unchanged. Only the genuine `asyncio`
import moves from module initialization into the existing coroutine CLI branch;
real `BaseSettings`, synchronous CLI and coroutine code remain present. This
avoids initializing unused Windows asynchronous socket machinery during settings
imports; it does not make asynchronous networking usable inside confinement.

Assembly requires the exact original wheel, source and RECORD identities and
unique reviewed anchors. Unexpected upstream bytes fail closed. It updates only
the installed RECORD row for `main.py` and adds explicit compatibility provenance
with original/installed source and RECORD hashes and the preserved license hash.
The normal post-transform `pack.json` inventories those final bytes. The pack
verifier requires that exact transformed contract, and the existing embedded
implementation identity invalidates older qualification receipts. Other platform
and legacy profiles remain unchanged.

This is a source compatibility candidate within the same synthetic-only R17
policy. An inert assembly or passing helper tests do not prove Windows conversion.
The original native confinement, filesystem/Job/deadline and 13/6/9 conversion
checks remain required, with all eight strict network-denial assertions intact.
No new capability, socket permission, mock module or distribution authority is
introduced by this transform.

## Failure-only native metadata observation

The original native fixture still compares every sorted full entry name,
UTC/RFC3339Nano modification time and mode string before and after read-only
status. Any inequality remains fatal. The existing walks additionally retain
those same observations in memory so a failure can report bounded counts of
added/removed entries, affected files/directories and modification-time/mode
changes. A deterministic first difference emits only an exact known-fixture role
or `unknown`, plus a closed field bitmask. Each count saturates at 1024 and a
boolean records saturation; no name, path, path hash, raw time/mode, snapshot,
content, ACL or arbitrary error is logged. Role classification performs no
additional filesystem reads and does not infer roles from basenames/prefixes.

Pure equality/privacy/count-bound checks run inside the existing original native
parent. Cross-compilation on Mac does not execute them. This observation changes
no product/status behavior or permission, and supplies no tolerance, warmup,
retry or successful outcome for a metadata mismatch. READY alone does not prove
read-only purity or completion of the subsequent 13/6/9 corpus gates.
