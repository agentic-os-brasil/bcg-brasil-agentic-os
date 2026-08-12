# Proposal 028 — Owner people record

**Status:** deferred by design; specifies the acceptance bar. This document
requests no implementation and proposes no schema.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 003 (people and account scope), Proposal 006 (owner
scope contract) and decision `OATL`.

**Unblocks:** nothing. It exists so that a future proposal starts from a
requirement list rather than from the original idea.

## Reading the proposal this document cites

Proposal 003 was accepted and then removed from the working tree. It is still
readable from history:

```sh
git show 760abd8:docs/proposals/003-people-cross-project-scope/README.md
```

One caution. `docs/proposals/003-qualification-unlock.md` is in the tree and is
a different document; the 003 whose seven requirements this proposal works
through is the people cross-project scope proposal above.

## Why this document exists

Proposal 003 accepted the need — the same colleague appears across engagements
and the record is rebuilt each time — and deferred the feature behind seven
requirements. Proposal 006 then created an owner scope, and its admission rule
excludes this content by construction: what it admits is content the owner
authored **whose data subject is not a third party**, in two shapes — pages
about the owner, and pages about the work itself.

A people record fails that test in the most direct way it can be failed: its
data subject *is* a third party — someone who did not write the page, did not
consent to it and cannot see it. It is neither of the two admitted shapes.
Almost every difficulty in Proposal 003's list lives in that one fact.

Proposal 006 anticipates the near miss and closes it. Colleagues may be named
inside admitted content, as sources and as context, and the contract says what
that costs:

> Names of colleagues, feedback sources, clients and projects remain personal or
> confidential metadata even when they are not the primary subject. The system
> must label and minimize them appropriately; it must not claim that attribution
> eliminates third-party considerations.

A page whose subject is the colleague is the other side of that line, not a
larger helping of the same thing.

The risk this document guards against is not that someone builds the feature
badly. It is that the feature gets rediscovered as an obvious convenience, and
the seven requirements get re-derived from scratch, or not at all.

## The seven requirements, applied to a people record

Each requirement is Proposal 003's. What follows each is what it demands for
this content specifically, and honestly.

### 1. Independent owner scope

**Requirement.** Storage and authorization distinct from workspace, client
account and managed bundle roots; the account role must not be reused as a
global owner.

**What a people record demands.** Proposal 006 satisfies the storage half: an
owner root exists, is distinct, and PR #286 made it writable through named
operations. It does not satisfy the authorization half for this content, because
its admission rule excludes third-party records by construction. A people record
therefore needs an **additional admitted class** inside owner scope, with its own
authorization predicate — not a folder inside the existing one. Placing a
colleague record under the same rule as the owner's retrospectives would silently
widen Proposal 006's contract.

### 2. Minimal record

**Requirement.** A closed field schema. Client names, engagement facts,
third-party feedback, inferred traits, diagnoses and behavioural scoring denied
by default.

**What a people record demands.** A closed enumeration of fields, with anything
unenumerated rejected rather than stored as free text. This is the requirement
where the owner scope helps least, because Proposal 006 deliberately does the
opposite for its own content: free-form Markdown is allowed, and segment
templates are "optional orientation and interoperability aids, not an admission
gate". That permissiveness is correct for a corpus whose subject is its author
and cannot be inherited here. The denial list is not advisory and must be
enforced in the schema:

| Denied field class | Why |
| --- | --- |
| Inferred traits and personality characterization | Not a fact about the person; a model's opinion of them, unreviewable by them |
| Behavioural or relationship scoring | Converts a colleague into a ranked object; no professional purpose survives the harm |
| Health, diagnoses, personal life | No collaboration purpose justifies it; sensitive by default under any regime |
| Performance assessment | The owner is not the authority on it, and the person has no visibility into it |
| Third-party feedback about the person | A second non-consenting subject inside an already non-consenting record |
| Client names and engagement facts | Reintroduces the boundary crossing the whole scope exists to prevent |

A closed schema also has to survive free-text pressure. A single "notes" field
would reopen every row above, so the admitted fields must each be typed and
bounded.

### 3. Explicit promotion

**Requirement.** A user-confirmed, redacted operation may copy a minimal fact
into the owner record. No cross-bundle link, workspace enumeration or automatic
aggregation.

**What a people record demands.** Three properties, all of them absent today:

- **Redaction as part of the operation, not after it.** The unit promoted is a
  minimal fact stripped of its engagement context, not a copied page. A
  promotion that carries the sentence it came from carries the client with it.
- **Owner confirmation of the redacted text**, shown as it will be stored.
- **No discovery path.** The owner must already know the person; the operation
  must not be reachable by enumerating a workspace's people index. Nothing
  declared today opens that enumeration, and nothing may.

### 4. Purpose and readers

**Requirement.** Every field has a professional purpose, sensitivity class and
closed reader set. Runtime packets carry minimum fields, not the record body.

**What a people record demands.** A purpose statement per field, testable at
review: if a field cannot be tied to a specific collaboration decision, it fails
admission. The reader set must then be drawn **narrower than the owner scope's
own default**. Proposal 006 admits five reader tiers over owner content — the
owner session, Maestro, Walter as a stale-checked self proxy, an explicitly
authorized attenuated excerpt or pointer for Case, Client Account and PA Expert
agents, and a whole-root dump for nobody. The fourth tier is exactly the one a
people record cannot use: those are the roles doing work alongside the person
the record is about. A future proposal must close that tier for this class and
say so in the authorization core, not leave it to a per-call purpose
declaration. That prohibition is what stops the record from becoming an input to
work about the person.

### 5. Human rights over the record

**Requirement.** Source, consent or other legitimate basis, correction,
deletion, retention, revocation and audit behaviour defined before persistence.

**What a people record demands.** This is the requirement Proposal 006 could
answer trivially and this feature cannot, and it should be stated plainly rather
than engineered around.

| Right | Owner-authored content (Proposal 006) | A people record |
| --- | --- | --- |
| Legitimate basis | The owner is the subject; no second basis is needed | Must be established. Consent is the honest basis, and consent the subject never gave is not consent |
| Awareness | Trivial — the owner wrote it | The subject does not know the record exists |
| Correction | The owner edits their own page | The subject cannot correct what they cannot see. A correction channel has to exist and be reachable by them |
| Deletion | The owner deletes: directly, or through the accepted `delete` operation behind an explicit confirmation | Must be honoured on the subject's request, which presupposes awareness |
| Retention | Owner's discretion | Must be bounded and expire, because the professional purpose expires |
| Audit | Provenance on every write | Same, plus the ability to answer the subject's question "what do you hold about me" |

A design that cannot give the subject awareness, correction and deletion has not
met requirement 5. Retention limits and provenance are necessary but not
sufficient substitutes.

### 6. No reverse leakage

**Requirement.** Owner context is never copied into a workspace or account
merely because the same colleague appears there.

**What a people record demands.** A symmetric, enforced prohibition — the shape
Proposal 006 already adopted for its own content, where an owner reflection about
an engagement never becomes an authority for the client or the case — plus one
guarantee specific to this record: an owner-scope fact about a person must not
influence, colour or be restated in workspace or account output. The value of
the record is exactly what makes leakage likely, so the guarantee has to be
enforced in the authorization core rather than asserted in a prompt.

### 7. Runtime-neutral enforcement

**Requirement.** Claude and Codex adapters use the same authorization core or
report the feature unavailable.

**What a people record demands.** No new principle, and no available shortcut. A
runtime lacking the core reports the capability unavailable and stops. A prompt
instruction telling a model to behave as though the schema were closed is not an
implementation of requirement 2, and does not satisfy this one either.

## Verdict

The feature should remain unavailable until a dedicated proposal satisfies all
seven requirements — not most of them, and not the six that are tractable.
Requirements 2 and 5 are the ones that decide it, and both turn on the same
fact: the subject is not the author.

Nothing about that is a reason to build a weaker version. Proposal 003 already
recorded the alternative and chose against it:

> The future feature is deliberately reported as unavailable rather than
> simulated through prompt instructions.

That sentence is the operative conclusion of this document. When the record is
asked for, the correct answer is that it is unavailable, with the reason —
because a simulated version has the same collection behaviour as a real one and
none of the guarantees. Saying so costs the automation and nothing else: the
owner may still think out loud about a colleague, and the runtime may still help
them prepare for a conversation. What it may not do is file the result as a
durable record of that person.

## Consequences

- The cross-project colleague record stays unavailable, and this document is the
  reference for why, so the deferral does not have to be re-argued.
- A future proposal has a checklist rather than a blank page, and must answer all
  seven requirements as its own document, with fixtures.
- Proposal 006's admission rule is confirmed rather than stretched: this content
  is outside it, and would require a new admitted class, not a new folder.
- Workspace people pages stay workspace-local under `case_agent`, a role that
  cannot leave the workspace, which is what prevents an accidental aggregation
  path from appearing before requirement 3 is answered. Proposal 027 records why
  no account root exists to receive one either.
- Duplicate workspace records remain the accepted cost, as Proposal 003 held.

## Explicit non-decisions

- no owner people record, schema, field, storage location or operation is
  created or approved;
- no promotion, redaction or aggregation path is created or implied;
- no role is added, modified or granted access to any people content;
- no workspace or account people contract is changed;
- no reader tier is added to, or widened in, Proposal 006's set;
- no HR, staffing, directory or profiling capability is created;
- no runtime is reported available by merging this document.
