# Spec 027 — managed agent scaffolding

The base bundle projects data-free definitions for Maestro, Case Agent, Client
Account Agent, PA Expert, Yoda, Darwin and Gamma Guardian. Installed instances add immutable
scope, owner, authorization and state digests; they do not create new roles.

| Role | Owner | Scope | Direct user | Delegation |
| --- | --- | --- | --- | --- |
| `hub` | Maestro | control | yes | direct spokes only |
| `case_agent` | Maestro | case/workspace | no | none |
| `client_account_agent` | Maestro | account | no | none |
| `pa_expert` | PA Expert registry | practice | no | none |
| `reviewer` | Maestro | review | no | none |
| `governance_analyst` | Maestro | health | no | none |
| `quality_guardian` | Maestro | quality_longitudinal | no | none |

Scaffolding validates the canonical catalog, exact ID-to-scope relationship,
parent Maestro identity, role contract and data-free managed definition.
Legacy practice identities are rejected and must be re-registered as a
versioned FPA/IPA PA Expert.

Case Agent and Client Account Agent instances never receive child-delegation
rights. A Case may be linked to an account in its signed relation, but the
runtime still mediates every transition through Maestro.

Personalization changes display name and avatar only. It cannot change role,
scope, tools, delegation, runtime state or native qualification.
Maestro, Yoda and Darwin are interviewed one at a time. Narrative suggestions
may reflect only preferences the owner explicitly states. The local profile
changes only after a digest-bound draft is reviewed and confirmed against its
base revision; scaffolding continues to derive authority exclusively from the
canonical catalog and signed instance.
