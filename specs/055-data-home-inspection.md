# Spec 055 — Read-only Maestro home inspection

Status: accepted for isolated preparation. This implements the diagnostic
boundary of #433 and checklist B01. Product freeze, migration, host
qualification and rollout remain governed by epic #428 and Proposal 004.

## Observable contract

`maestro-runtime home inspect --product-root ABS [--source-root ABS]`
resolves owner state independently of the managed code directory. It reads
`MAESTRO_HOME`, the OS user's `.maestro-home.json` locator, then the default
`.maestro` directory, in that order. An explicitly defined empty environment
value is invalid. A present invalid locator blocks resolution even when the
environment selects another path. It never searches the current case or cwd.

The command writes one JSON result to stdout on successful inspection and
returns exit 0. A discovered but unconfirmed home is a successful inspection
with `writable=false`, not permission to initialize. Invalid input, inaccessible
state or conflicting authority returns exit 2 and a bounded diagnostic on
stderr. Invalid resolution emits no successful JSON result.

| Field | Meaning |
|---|---|
| `home` | Absolute owner-state directory |
| `brain_root` | `home/brain` |
| `selected_by` | `env`, `marker` or `default` |
| `origin` | `fresh-unconfirmed`, `fresh-confirmed` or `migrated` |
| `writable` | Logical origin validity, independent of code trust and host permissions |
| `diagnostic` | Bounded explanation of readiness or the remaining authority check |

The resolver never creates directories, repairs permissions, publishes a
locator, activates a home, migrates a ZIP or changes a runtime profile.
Existing owner data and managed files remain byte-identical. The API takes
explicit user/product/source roots; the command obtains the user directory
through `os.UserHomeDir`, using the native Windows user-profile convention.

## Record and path authority

The locator and origin schemas and layout are defined once in
[Proposal 004 Spec A](../docs/proposals/004-plugin-mcp-distribution/specs/spec-A-data-home-decoupling.md).
Inspection enforces its 16 KiB, UTF-8, exact-field, duplicate-free single-object
JSON contract and schema version 1. All fields are mandatory, including empty
attempt fields for a fresh origin. Invalid, missing-target or contradictory
locators never fall back silently.

Existing roots and ancestors must be ordinary directories, with no symlink or
Windows reparse traversal. HOME cannot contain or be contained by product or
SOURCE. Private HOME and records use the existing platform-specific scheduler
path checks. USERHOME and system ancestors need not have mode 0700. Missing
HOME is reported without creation; a populated home without origin is blocked.

`fresh-confirmed` is logically writable only when its home matches the
selection, its attempt/digest fields are empty and a nondefault home has a
matching locator. A matching migrated origin is reported as
`migration-unverified`, `writable=false`: validating a committed migration
journal/plan/receipt is reserved for #434. These local records never establish
release signing, principal authentication or host tool permissions.

Unix inspection checks the current UID and rejects group/other-writable owner
authority. Windows inspection validates path/record structure but reports
`ownership-unverified`, `writable=false` for a fresh origin until the owner/ACL
verifier is implemented and qualified. Successful parsing never fills this gap.

## Compatibility and test boundary

This adds an explicit read-only helper command. Existing `--root` operations,
ZIP migration, shell/Python hooks and explicit workspace DataRoot APIs retain
their existing semantics until they are adapted together. Therefore this PR
does not close #433, B01, or the full parity checklist.

Tests exercise `datahome.Resolve` and the helper command as public seams:
selection order, malformed/conflicting records, missing targets, spaces,
overlap, links/reparse handling and before/after preservation. Native Windows
drive semantics are tested on Windows; cross-compilation alone is not native
qualification. A read-only result cannot attest that the nine hook pairs have
already migrated to the new home.
