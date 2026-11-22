# R17: registryRead-only Windows LPAC experiment

Status: isolated synthetic CI candidate; no shipping or real-profile authority.
Decision: [RGCI](../decisions/decision-log.md#rgci---confine-one-registryread-experiment-to-synthetic-windows-proof).

The original statements below that no setup code writes registry values or
changes resource ACLs describe the **pre-assessment experiment**. The later
[finite fixture assessment](#finite-registry-fixture-assessment--source-only)
adds an explicitly separate transient artificial-key scope. Read that section
before interpreting newer logs. Historical source and failed observations stay
preserved; the new source is not evidence that the fixtures have run.

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

## Synthetic conversion wire

The native test child uses an explicit fixture-only envelope for a returned
`Conversion`: unchanged production metadata plus exact Markdown and Document
byte slices. Both sides explicitly encode/decode this private pipe protocol;
production `Conversion` JSON still excludes bodies. No pages, content or fallback
results are generated by the wire. Failure logs omit conversion structs, parser
errors and raw child output so newly transported bodies cannot enter retained
logs. The original structured-document, two-page, bounding-box, image, text,
table, refusal, source-identity and provenance assertions remain intact.

The child fixture source is portable only so focused Mac unit tests can exercise
its real encoder through a private pipe. Those tests never call `main`, create a
runtime or invoke a converter/model. The `testdata` harness remains outside the
distributed product and normal recursive package discovery; the original Windows
parent still builds its child for Windows. Pure byte roundtrip success does not
attest actual Windows PDF/PPTX semantics. The earlier metadata inequality remains
unexplained and its original fatal guard is unchanged.

## Finite registry fixture assessment — source only

This addition implements a reviewed local-source proposal against experimental
commit `1161e6f3fe27ee04888c0ef52befce7f8d6cb97c`. Local construction and
cross-compilation authorize neither fixture creation nor hosted execution.
Root must separately approve an exact-source run in a disposable ordinary
Windows account. Production launch, sole enabled `registryRead` capability,
pre-resume token attester, LPAC mask6, environment and original proof inventory
remain unchanged. R17 export and assembly still refuse.

Only `TestWindowsNativeConfinement` prepares one fresh volatile root and five
fixed direct leaves under the actual process user's existing
`HKEY_USERS/<user>/Software` anchor, in the 64-bit view. Root and leaf names are
derived internally from the original cleaned work path; callers supply no SID,
registry path, mask or inherited registry handle. Each create must report a new
key. Existing, unsupported or ambiguous state is preserved without permission
repair, privilege enablement or elevation. Other parents, including
`TestWindowsRealAuthenticatedDocling`, do not prepare these fixtures.

Preparation additionally requires all three **existing** markers:
`MAESTRO_WINDOWS_DOCLING_QUALIFY=1`, `GITHUB_ACTIONS=true` and
`RUNNER_ENVIRONMENT=github-hosted`. The existing credentialed launcher inherits
its caller's environment and the existing Docling helper sets qualification=1;
no production child environment is changed. These markers protect against
accidental local-suite writes, rather than attest a host against forgery.
Ordinary local suites retain both original confinement calls and assertions,
with no assessment setup, rows, registry access or new parent skip.

After verified setup, the parent exclusively creates a fixed marker in its
known private work root. Its only bytes are the constant definition version and
hash. Each child checks that bounded file before any assessment registry access;
absence preserves its original behavior. The enabled parent requires all 30
observations per child, so a missing marker cannot silently omit the assessment.
The marker carries no registry path, user identity, capability or permission
selector. The driver requires a complete assessment receipt in hosted scope;
outside that scope it keeps the original proof and rejects unexpected R17 rows.
The environment-inheritance expectation follows Microsoft's
[Start-Process contract](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/start-process);
actual hosted propagation remains a native execution check.

The protected explicit DACL has ordered allow ACEs: OWNER RIGHTS gets
READ_CONTROL (`0x20000`), the setup user gets KEY_ALL_ACCESS (`0xf003f`), and the
optional fixed capability gets the exact measured subset below. OWNER RIGHTS
suppresses the owner's implicit WRITE_DAC; no deny-WRITE_DAC ACE masks the broad
control. Actual owner, ACE order/types/flags/masks/trustees, protection, label,
sentinel and fixed key metadata are checked in memory before and after both
original child executions.

| Fixed fixture | Optional capability grant | Mandatory label |
| --- | --- | --- |
| `owner_only` | None | Low, no-write-up |
| `capability_read` | `registryRead`, `0x00020009` | Low, no-write-up |
| `capability_broad` | `registryRead`, `0x0007000f` | Low, no-write-up |
| `foreign_capability` | Fixed unequal `internetClient`, `0x00020009` | Low, no-write-up |
| `integrity_pair` | `registryRead`, `0x0007000f` | Medium, no-write-up |

The foreign SID appears only in the artificial fixture ACL, never the child
token. Low/Medium labels use only handle-based LABEL_SECURITY_INFORMATION and
one NW ACE; setup requires an actual Medium parent and measurements an actual
Low child. The selected read subset is READ_CONTROL, QUERY_VALUE and
ENUMERATE_SUB_KEYS; the broad subset adds SET_VALUE, CREATE_SUB_KEY, DELETE and
WRITE_DAC. Capability grants include neither WRITE_OWNER nor CREATE_LINK.

Each actual confined child uses the working directory explicitly supplied by the
launcher, requires its absolute clean path and exact fixture marker, and requests
six fresh handles, one right at a time:
QUERY_VALUE, ENUMERATE_SUB_KEYS, SET_VALUE, CREATE_SUB_KEY, DELETE and WRITE_DAC.
Only a successful QUERY_VALUE open reads the fixed artificial 32-byte binary
sentinel; the child compares it in memory. The child performs no registry
mutation, subkey enumeration or real-value reads. Success measures **handle
eligibility**, not an executed registry write or exhaustive operational access.

Direct stdout carries exactly 30 finite rows even in the second non-verbose
child. The parent validates every row and the unchanged original passed marker,
then publishes 60 observations with its own first/second ordinals only after
both original calls succeed, readback is unchanged and cleanup completes. The
original shared 20-second child context stays unchanged. Parser controls run
inside that existing parent; no additional test parent or runner is added.

Cleanup rechecks cooperative custody after both children have stopped, closes
held leaf handles and deletes only the five fixed leaves, then the exact fresh
root relative to retained parent handles. It confirms root absence. Failed
setup, changed shape or unconfirmed child termination preserves names and emits
a bounded incomplete diagnostic. Volatility is a fallback lifecycle property,
not a cleanup-success claim. This contract provides **cooperative custody**;
it makes no physical identity/atomic-CAS guarantee against a malicious process
running as the same user.

The separate `docling-registry-assessment.json` binds definition version/hash,
source revision/tree, sealed inventory/test-image identity, original policy and
runtime profile/lock, actual runtime pack, original log and both phase ordinals.
Rows retain only fixed enums, unsigned numeric LSTATUS and sentinel equality.
Missing, duplicate, unknown, malformed, diagnostic or truncated protocol data
refuses a receipt. Completeness and discriminating contrasts are separate:
60 all-denied observations are complete and explicitly non-discriminating.
Neither outcome qualifies real profiles, global write exclusion, private-key
exclusion, export, publisher authority or participant distribution.

The original four-parent/13-format/6-PDF/9-PPTX gates, eight strict network
denials, filesystem/snapshot/descendant/Job/deadline assertions and baseline
proof receipt remain required. The failure-only loader replay still runs only
`TestWindowsRealAuthenticatedDocling`; its historical `no_registry_changes=true`
describes that replay's narrower scope and does not describe the instrumented
confinement parent. This addition does not rewrite earlier failed observations.

### Parent descriptor and metadata readback

The parent's relative query readback handle additionally requests READ_CONTROL
and repeats the original exact owner, protected DACL and mandatory-label
validation on that reopened handle. It then checks the original sentinel and
compares every metadata field, including security-descriptor length and last
write time, against the retained full-access handle. No field is omitted or
normalized. The child still requests each of the original six rights separately;
its masks, capability, labels, observations and cleanup contract are unchanged.

This is a stronger parent readback candidate, not proof that a missing access
right caused an earlier metadata mismatch. Microsoft's RegQueryInfoKey contract
requires KEY_QUERY_VALUE; it does not document access-dependent descriptor-size
results. READ_CONTROL has a concrete purpose here: the reopened handle also
reads the actual security descriptor through the existing validator. Native
acceptance of the complete candidate remains separate from source validation.

A metadata inequality remains fatal and projects only a fixed field enum:
subkeys, maximum subkey-name length, values, maximum value-name length, maximum
value bytes, security-descriptor bytes, last write, or multiple differing fields.
Values, deltas, names, paths, SIDs, sentinel contents and arbitrary errors remain
private. The full struct comparison controls acceptance; diagnostic
classification cannot make a mismatch pass. Any diagnostic still refuses a
complete assessment receipt. No earlier failed observation or preserved state
is replaced by this candidate.
