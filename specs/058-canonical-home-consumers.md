# Spec 058 — Canonical HOME consumers

Status: accepted for construction under the complete Maestro instruction.
Scope: E2a of docs/plans/2026-10-31-maestro-product-mcp.md. E2b supplies the
external migration transaction; migrated origins remain unavailable until proven.

## Observable result

A new participant explicitly initializes a private HOME, then the distributed
ZIP hooks and tools read/write owner state there across product versions and
nested working directories. The managed product remains unchanged. The same
canonical resolver determines paths for Go, Bash, PowerShell and Python.

Existing supported legacy state remains usable through its existing local
binding when no external authority conflicts. New/ambiguous state does not
become a legacy installation merely because a lookup failed. Pending, revoked,
malformed or unverified HOME authority blocks ordinary effects before logging,
scaffold, recovery, indexing or interpreter-record writes. Windows external
mutation remains unavailable; diagnostics and existing legacy contracts retain
their explicit limits.

## Shared interface

Use a small canonical runtime-path value consumed by the existing zipruntime,
not an opaque new session framework. Keep product root, owner brain, owner
metadata, canonical DataRoot and owner-scope identity distinct. External owner
brain is HOME/brain; metadata is HOME/brain/.maestro; E1 DataRoot is HOME/state.
Explicit DataRoot APIs and workspace-local brain surfaces retain their meaning.
External owner identity is stable across managed product directory/version changes.

Expose local helper commands:

- maestro-runtime home paths --product-root ROOT [--user-home USER_HOME]
  returns bounded JSON metadata, mode, availability and canonical paths; it is
  read-only and performs no initialization, migration, repair or chmod.
- Optional --field selects home|brain|metadata|state|owner-scope|mode, returning
  one safe scalar only when admitted. No evaluation of shell output is needed.
- maestro-runtime home init --product-root ROOT [--user-home USER_HOME] is an
  explicit local operation using datahome.InitializeFresh. It never overwrites
  state, performs migration or merges legacy with a populated HOME.

Keep home inspect backward compatible. The canonical runtime resolver uses
existing datahome inspection and explicit legacy status inspection. No ordinary
startup calls mutating zipmigration.Run; explicit migration remains the local
migration command. No source layout is selected by mtime, absence after an IO
error or a copied initialized marker. Conflicting external/legacy state is
reported before effects. Migrated origin is still migration-unverified in E2a.

Before returning writable paths, reject static aliases in the closed set of
directories and output leaves used directly by the shipped hooks. This
check must precede scaffold, logs, interpreter records, markers and envelopes.
Canonical daily/lifecycle engines retain their existing no-follow checks,
typed refusals and recovery behavior for their own inputs and outputs; path
admission must not replace those operation-level contracts with a blanket scan.
Case/workspace aliases retain their existing guard semantics; do not scan or
redirect those authored trees as machine metadata. A blocked hook emits exactly
one `<!-- maestro:state-unavailable -->` marker on stdout, keeps diagnostic prose
on stderr and retains the guard-deny/non-guard-no-effect exit contracts.

All callers must consume this decision. Do not implement shell/Python parsers of
origin/locator authority. A local helper path/binding is selected through existing
verified runtime discovery; untrusted model tool arguments never select roots.
Python tools may share a small helper-invocation adapter, not a second resolver.

## Complete caller surface

- Go zipruntime: context cap, daily context/stop/ack/recovery, lifecycle and guards.
- All nine distributed shell/PowerShell hook pairs and their shared libraries in
  installers/zip/user-template/.claude/hooks. The repository-root dev hooks are
  outside this change.
- Interpreter selection records belong to the admitted owner brain.
- Bundled brain-index, brain-route, skill-route and paths-check tools, including
  their standalone public entry points and subprocess callers.
- Distributed owner instructions and skills with executable owner-brain paths.
  Logical workspace/case paths are not globally replaced.

Relative paths in guard input resolve against actual event cwd. Account/case
classification uses the admitted owner brain and existing external aliases;
product-relative guesses cannot grant a cross-case write. Keep denial encoding
and lifecycle invariants. Only Maestro-generated effects are claimed here;
this is not a general same-UID or shell sandbox.

## Public acceptance

Use canonical API, helper CLI and packaged hooks/tools. Fixtures/cache/temp live
under Developer locally; full suites remain hosted CI. Establish red before green.

1. Fresh explicit init then read/write workflow on an admitted Unix HOME with
   spaces; product root read-only and unchanged.
2. Two product paths/versions use the same owner state and pending continuation.
3. Source cwd nested or outside product resolves actual case-relative writes;
   cross-case/alias violations denied by the existing guard policy.
4. Every hook/library obtains the same root before any effect; pending/revoked/
   unverified/conflicting origins preserve both trees and create no logs/scaffold.
5. Python public tools index/route/check paths using owner state plus product
   assets, independent of cwd; interpreter records do not appear in product.
6. Ordinary path/context inspection never initializes or performs v1 migration.
7. Legacy supported fixtures retain their binding semantics without silently
   importing data into a fresh HOME. Unsafe ancestors/records stay fail-closed.
8. Windows external state writes/init refused; native ownership qualification is
   not inferred from CI. Existing legacy compatibility is characterized.

Native Code/Desktop invocation, external migration/rollback, model calls, source
publication, signing and pilot rollout remain their own gates. No real owner data
or host profiles are changed by this source implementation.

Source CI exercises path refusals, public isolated workflows, package closure and
qualification classifiers. Exact native installation/update qualification stays
an explicit workflow dispatch with its original preflight and assertions. A
Windows runner that is elevated remains unqualified; skipping its qualification
job during source CI does not supply a native receipt or close that gate.
