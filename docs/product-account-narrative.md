# Attended shared account narrative

`account.narrative.status`, `account.narrative.read` and
`account.narrative.save` connect an already active client first-case binding to
its sole shared account narrative:

```text
admitted HOME/brain/accounts/<account-id>/<account-id>.md
```

The server derives HOME, workspace and exact account/case identities from the
existing registered workspace setup and HMAC-authenticated agent enrollment.
An internal, pending, changed or reconciliation binding refuses this scope.
Callers cannot choose a root, account, path, role or parent. Local HMAC integrity
is not publisher trust, model identity or a runtime/tool grant.

Queries neither create nor repair state. Explicit full-page Save uses the
existing authored metadata validator and atlas operations: native full current
and proposed review, digest revalidation, serialized publication, expected-page
revision, provenance, prepared journal recovery and exact retry receipts. The
existing publication guard applies to installed-source admission. The account
root is a fixed constructor, not a generalized HOME filesystem route. The
owner atlas and each workspace brain remain separate roots.

The narrative is bounded to 64 KiB UTF-8; the operation alone admits at most
512 KiB of encoded request bytes for JSON expansion. Trusted review keeps its
existing 1 MiB limit. Proposed `## Cases` has at most 32 independently checked
registered case IDs, including the current case, with exact account/case tuples.
Existing canonical references are identified separately up to 32 unique IDs;
all remaining legacy/duplicate lines are explicitly unverified, preserved in
full current text for conscious replacement. No foreign case body is read.
A second separately admitted workspace with the same exact immutable account
registration sees this one narrative. First-case finish never appends it.

These plain code references identify case registrations; they are not links to
foreign saved briefs. Current admitted brief metadata is returned separately.
Shared account search/index generation, foreign brief linking, relationship
history/recall, cross-case promotion, arbitrary hiring and actual model/client
qualification remain separate outcomes. Therefore the broader
`domain.account_case_scaffold` stays unavailable; the narrow registered
`domain.account_narrative` dependency is available without inventing authority.

The [shipping workflow](../bundles/base/skills/account-case-setup/resources/account-narrative-mcp.md)
contains the exact owner interaction and argument contract. Focused SDK and
compiled-stdio tests exercise durable save/readback/restart, second-workspace
sharing, replay, full review, rejection and concurrent-review changes. These are
local adapter/core evidence, not Desktop/Windows acceptance or hosted CI.
