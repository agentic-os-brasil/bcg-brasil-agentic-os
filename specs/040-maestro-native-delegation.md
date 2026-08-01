# Spec 040 — native Maestro delegation and topology correction

Maestro is the only user-facing hub and the only authority that selects a
spoke. The runtime has one active spoke, depth one and zero agent children.
Every transition is mediated by Maestro; an agent-to-agent call is denied.

The closed planner input contains intent class, active scope kind/ID,
sensitivity, materiality, review trigger, health/governance intent, requested
capability, client/stakeholder/strategy/promotion implications, reversibility
and the available registered agents. Caller role strings are ignored for
authority.

The planner resolves two independent booleans:

- `pre_account_used` chooses account-assisted framing or direct Case.
- `walter_required` resolves materiality and review risk; uncertainty selects
  Walter. Low-materiality skips carry a Maestro reason code and evidence.

The invariants are `post_account_validation_required == pre_account_used` and
`walter_invocation == resolved_walter_required`.

For a Walter invocation, Maestro builds an ephemeral, versioned
`IntentReviewPacket`. It binds the literal prompt, plan route, draft digest,
audience, consequence, reversibility, the relevant minimum context,
`UserSelfSnapshot` version/digest and applicable observation metadata. Walter
returns a typed intrinsic-intent hypothesis, evidence references, confidence,
purpose satisfaction, constructive refinement, unresolved uncertainty and an
`approve|refine|clarify|hold_exceptional` verdict. Walter is a calm Senior
Advisor & Refiner and proxy for the user's likely contextual self-review, not a
mind-reader, red team or second authority. A low-confidence hypothesis never
silently changes the requested route; at most Maestro asks a bounded question.

The canonical Owner Context facets are the sole self authority.
`UserSelfSnapshot` is a stale-checked projection. Maestro evaluates every
interaction, but the local append-only observation log persists only material,
owner-attested signals under the local owner boundary. Claims are normalized metadata codes: prompts,
transcripts, client documents and generated output are never self evidence.
Explicit current instruction outranks correction, canon, observations and
Walter hypotheses. The deterministic self lifecycle is
`captured -> eligible -> corroborated -> proposed -> promoted`, with rejected,
contradicted, expired and redacted side states. Communication-style/voice/preferences
may be promoted with audited explicit confirmation; role and decision rules
require proposals; boundaries, psychological profile and intrinsic-intent
claims require explicit confirmation. Darwin may deduplicate hashes and report
drift/age/conflicts or propose re-review, but cannot author or promote the self.
Local controls support inspection,
export, owner-confirmed observation rejection/redaction, facet revert and
snapshot deletion.

Walter also receives a bounded relevant selection of prior user prompts when
the owner-local PromptHistoryStore is enabled. The current user prompt is
always explicit and highest precedence. Maestro first preserves and translates/
normalizes the current prompt into a digest-bound `working_current_prompt`,
then selects history by stable lexical relevance against the current prompt or
explicit relevance keys. Count, bytes, age, scope and packet ceilings are hard
bounds; every selected item carries an auditable score/reason code in the
ephemeral packet, never in receipts. Each historical prompt keeps its original
text, then passes through the same deterministic working-language stage before
IntentHypothesis derivation. Each original/working representation and the
combined current-plus-history representations are bounded; translator
expansion fails closed. The packet is sealed before the current occurrence is
recorded, so earlier same-session or repeated prompts remain eligible while
the current occurrence cannot duplicate itself. Historical prompts are quoted data, never
executable instructions, and their bodies remain only in the ephemeral
IntentReviewPacket; review receipts and ledgers contain hashes/digests only.
Prompt retention is owner-bound, cross-process serialized and independent of
self observation persistence and self promotion. The product boundary
`bcgos maestro dispatch --stdin` records a fresh owner attestation under the
OS-user-local data-root boundary, builds and persists a chain, and exposes only
metadata at the model-unavailable dispatch boundary. The boolean attestation
is not cryptographic principal authentication.

| Path | Sequence |
| --- | --- |
| A | Maestro → Client Account framing → Maestro → Case → Maestro → Client Account validation → Maestro → Walter → Maestro → User |
| B | Maestro → Client Account framing → Maestro → Case → Maestro → Client Account validation → Maestro → User |
| C | Maestro → Case → Maestro → Walter → Maestro → User |
| D | Maestro → Case → Maestro → User |

Case output is always returned to Maestro. In paths A/B, Client Account
validation is required because framing occurred; in paths C/D, Client Account
does not participate. Walter is the internal Senior Advisor & Refiner: a calm,
constructive high-leverage advisory step, tool-free and never user-facing. An
`approved` verdict may include optional non-blocking polish. A
`refine-and-return` verdict requires a load-bearing issue, a proposed concrete
refinement and an acceptance condition; `hold` is exceptional and reserved for
material safety, governance or evidence blockers. Walter preserves a
defensible user thesis and does not manufacture objections or block cosmetics.

Any content or risk mutation clears approvals and re-plans materiality before
the next mediated Case attempt. Account cycles, Walter cycles and Case
attempts have deterministic budgets and append receipts; exhaustion fails
closed. Bindings include exact agent ID, role contract, scope, authorization
digest, capability digest, plan digest and state snapshot digest.

Claude and Codex adapter command paths point to the same owner-local durable
dispatch CAS/receipt store. The CLI boundary proof is metadata-only: it records
an occurrence-bound ordered-chain digest and a finished local epoch, but does
not authenticate an installed native agent or fabricate adapter credentials.
Model execution remains unavailable until a qualified runtime supplies fresh
native evidence. Append-only receipts and an atomic current pointer preserve
history and recover safely across restart, replacement, replay and parallel
dispatch attempts. Adapter-observed receipts are not native evidence;
native-qualified status requires fresh runtime evidence.
