# Spec 061 — Local document to workspace knowledge

Status: accepted for construction under the complete-product instruction.
Scope: the complete local-content journey in E4, after Spec060. Connected sources,
native qualification and distribution retain their separate acceptance gates.

## Observable result

An admitted participant selects a workspace-relative professional document,
converts it locally with the verified Docling runtime, reads bounded derived
content, reviews an exact knowledge change and retrieves the written page after
restart. The original document and managed product are unchanged. A failure,
declined review or stale page cannot silently write knowledge. Source and pack
provenance remain inspectable without disclosing local paths or credentials.

## Canonical workspace knowledge

Extend the existing atlasops.Engine with OpenWorkspace(atlas.Options); do not
create another knowledge database or put client content into owner-global atlas.
Require DataRoot, the exact registered WorkspacePath and WorkspaceID; resolve and
revalidate their agreement through workspace. The corpus is WorkspacePath/brain,
with its own confined operation/grant journal. Ordinary open/read does not
bootstrap pages or substitute an owner atlas when workspace state is missing.

Every public workspace operation rechecks the bound identity before reading or
writing. Workspace journal/grant records and returned pointers bind that identity;
copied owner or other-workspace controls cannot acquire authority. Preserve the
existing owner constructor, owner pointers and owner record compatibility. Use
existing no-follow/private filesystem and cross-process locking primitives.
Serialize expected-revision comparison and page effect in the canonical engine;
trusted adapter review cannot close a race in an unlocked canonical writer.

Persist metadata-only prepared v2 operation evidence before page effects, binding
scope, exact request and before/after digests. Explicit retry under the corpus
lock finalizes a matching after-state without duplication, applies only a
matching before-state or preserves a newer state as conflict. Completed receipts
retain compatible v1 representation. Ordinary reads never recover state; no raw
draft body, second journal or content marker supplies operation authority.

Preserve named-page collection, reader attenuation, 24-page/128KiB collection
limits, 256KiB pages and 8KiB entries. The adapter derives the reader from its
admitted principal. No model argument can assert delegated authorization.
Standing-grant path coverage is component-aware; a segment never authorizes a
similarly prefixed sibling. Paused/revoked/expired grants refuse at effect.

## Verified local conversion

Docling remains the default under Spec010/DOCL. Reuse the ingestion contract and
bounded process protocol, with local paths only, no plugins, no implicit remote
service and no implicit model download. The original source stays in its selected
workspace. Reject absolute/traversal/aliased/special/oversize selections and
recheck identity at use; conversion cannot follow a replaced path outside scope.
Use the canonical default input/output budget. Add a format to the default
allowlist only with explicit policy and real converter fixtures; format support
in a third-party library alone is insufficient admission evidence.

A managed pack binds platform, compatibility, pinned interpreter/dependencies,
adapter and any required local model artifacts. Verify its complete executable
input inventory against an admitted release authority before use. Checking only
the adapter/interpreter or the presence of a provisioning receipt is insufficient.
No caller-provided command, verifier-that-always-succeeds or personal/beta key
may stand in for production authority. A synthetic engineering signing authority
is test evidence only. Missing or modified runtime/model files report unavailable.

The native executable derives its fixed registry from packet-root/managed/trust/
release-authority-registry.json, where packet-root is the parent of the executable
directory. ProductRoot binds product assets and does not choose release authority;
the expected registry digest is build-bound, with no executable flag/environment
override. Missing trust affects conversion availability without blocking unrelated
owner, memory or execution operations.

Installed trees are addressed by authenticated archive digest. Verify the signed
archive and complete installed tree on each use, then execute a fresh private
snapshot derived from those authenticated bytes; do not run an installed pathname
that an update can replace after verification. Snapshot/cache output remains
separate from immutable runtime inputs. This adds no second updater or authority.

Runtime provisioning is a distinct, explicitly confirmed operation with bounded
progress, verified inputs and retry. It must not happen on query/startup or upload
the document. Respect existing PYUV/UVIN decisions; an on-demand environment
receipt is not signed-pack qualification. Keep pack building and actual verified
conversion testable locally with synthetic documents. Platform/device signing,
network/proxy acceptance and real model/runtime installation need their own proof.

MarkItDown remains an explicitly selected permitted fallback under Spec031/MIDO;
never silently replace Docling or call a remote service. Report selected route,
fallback reason, fidelity and partial/unavailable state truthfully. A failed or
invalid conversion changes no knowledge or memory.

## Product operations and artifacts

Expose closed status, convert, artifact-read, reviewed knowledge-write and
named-page collection operations through the existing three tools. Keep roots,
workspace, commands and release authority locally selected. Resolve the immutable
artifact by an opaque ID bound to workspace, source hash, output hashes and pack
identity. Stage privately, validate bounded structured output/Markdown and publish
atomically without overwriting an earlier artifact. Retain artifacts only in
scoped private state; never in managed bundles, logs or telemetry.

Artifact readback is UTF-8-safe and at most 16KiB per slice. An artifact/provenance
cannot be selected across workspace boundaries. Knowledge writes bind the exact
artifact, named page/section, draft text, page revision and admitted scope through
the trusted Spec060 host review or explicit local operator fallback. Hidden proof
fields bind MRTR/CAS while human forms show all relevant domain content. Changed
source, artifact, scope or revision requires fresh review. A model-supplied consent
boolean is never authority. Use existing CreatePage/AppendEntry/Collect effects;
no raw arbitrary-file write or owner promotion is implied.

## Acceptance

1. A real local Docling fixture converts offline through a verified engineering
   pack, is reviewed/written and appears with correct revision/provenance after
   restarted SDK and compiled-stdio sessions with read-only managed product.
2. Distinct workspaces cannot read artifacts, pages, grants, journals or approvals
   from each other or owner scope. Rebinding, aliases and unsafe controls refuse.
3. Tampered manifest, dependency, adapter, interpreter or model cannot execute.
   Unsupported format, invalid/partial output and explicit fallback report honestly.
4. Decline/no-form/stale/retargeted approval and concurrent page edits preserve
   newer content. Retry after interrupted persistence does not duplicate entries.
5. Named readback and applicable grant revocation observe canonical scope/budgets.
   Existing owner atlas and approved ingestion behavior retain focused regressions.

Use synthetic data, task reviews and four final independent review lenses. Local
tests, actual converter evidence, hosted CI, native client confirmation, runtime
pack qualification, corporate signing, publication and pilot are separate claims.

Primary converter references: [Docling API](https://docling-project.github.io/docling/reference/document_converter/),
[offline/runtime options](https://docling-project.github.io/docling/reference/pipeline_options/),
[official offline FAQ](https://github.com/docling-project/docling/blob/main/docs/faq/index.md).
