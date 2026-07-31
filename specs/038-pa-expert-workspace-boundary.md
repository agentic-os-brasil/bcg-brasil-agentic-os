# Spec 038 — PA Expert workspace boundary

## Status

Accepted for the runtime-neutral contract. Native PA Expert activation remains
unavailable until a Claude or Codex adapter proves the same boundary.

## Ownership model

```text
Client Account Agent  -> curated stakeholder and relationship intelligence
        |
        +-- Case Agent -> case-local raw context and deliverables
                         |
                         +-- Maestro declassification -> PA Expert (FPA/IPA)
```

- The Client Account Agent owns one account and never browses a case.
- The Case Agent owns one case/workspace and keeps raw project material local.
- Case-to-account promotion is explicit, reviewed, provenance-bound and
  reversible through the existing context-promotion service.
- PA Expert is a centrally maintained, scope-free advisory role. It is not a
  practice workspace, does not own client execution and cannot delegate.

## Declassified packet

`internal/paexpert` is the only contract for advice crossing from account/case
scope. It contains a packet ID, source and route digests, exact published PA
Expert identity/kind/version/canon digest, a question code, closed public or
internal fact codes and allowlisted output sections. It contains no account,
client, case, workspace, stakeholder or person identifiers; no raw excerpt,
prompt, attachment or scoped URI; and no filesystem path.

The exporter must attest to the absence of those classes and the validator
also rejects reserved scope tokens, path separators and pointer schemes. The
packet digest is computed after canonical validation, so changing an expert,
canon, source digest or fact changes the packet identity.

## Advisory receipt

The expert response is bounded to findings, assumptions, challenges and
application cautions. It must repeat the packet digest and exact expert/canon
binding. Validation returns a content-free receipt with `may_export: false`;
the receipt is evidence of a shadow advisory only and cannot authorize a
runtime, route change or client-context promotion.

Missing, stale, substituted or export-authorizing receipts fail closed. An
empty managed PA Expert registry remains empty and unavailable; scaffolding a
draft instance never implies published knowledge.

## Legacy practice identity

`practice_agent` is retained only as a migration input for existing signed
identities. It is not an active catalog edge, runtime authorization or child
delegation. The managed catalog carries an explicit expiry and replacement
role (`pa_expert`); after expiry, resolution fails closed and re-registration
under the exact PA Expert registry contract is required.
