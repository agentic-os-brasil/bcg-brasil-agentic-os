# Native Walter qualification recipe

The repository contains the runtime-neutral Walter wiring, but the capability
remains `unavailable` until a native Claude and Codex session proves the same
contract. Adapter-command receipts are diagnostic only.

The weekly self-review contract in this PR is intentionally limited to
Walter custody, canonical `ownerctx` observations/snapshots and
`PromptHistoryStore` selection, proposal validation, fenced metadata receipts
and the unavailable-by-default maintenance handler. It does not add scheduler
or launchd catalog entries. Its orchestration dependency is PR
[#148](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/pull/148);
after that PR lands, orchestration may rebase and trim this seam as needed.

The weekly occurrence is keyed by the command's stable occurrence digest plus
the bounded IntentHypothesis digest when a self-proxy hypothesis is present,
never by its retry command ID, and is reserved before any model call. A recoverable,
cross-platform OS advisory lock plus a renewable, deadline-bound occurrence
lease fences concurrent workers and allows an expired reservation to resume
after a crash. The current prompt is kept separate and
wins over history; selected history is translated through a caller-supplied
translator whose identity/version and receipt digest are bound into the
ephemeral input. Per-field and combined UTF-8 bounds plus translation
expansion checks run before the adapter and in request validation. Historical
entries are explicitly quoted non-instructional evidence. Walter receives only
an explicit minimal facet allowlist; sensitive facets require a declared
purpose and owner authorization. Walter may emit only a facet-bound proposal:
`communication-style`, `voice` and `preferences` use the declared low-risk
policy, `professional-role` and `decision-rules` remain proposal-only, and
boundaries/profile require explicit owner confirmation. Intrinsic-purpose
hypotheses remain task-local and cannot become self facets.

The facet sensitivity, readers, refinement mode and confirmation requirement
are derived from the canonical `ownerctx` snapshot and compared exactly with
the adapter result. After a model call, the actual ownerctx proposal ID,
proposal digest and canonical policy are bound into the terminal weekly
receipt. Re-running a crashed occurrence returns the same ownerctx proposal
and emits one terminal receipt without invoking the model again; it never
creates a second proposal. If a self-proxy hypothesis is present, both the
request and proposal must carry its exact digest; absent hypotheses cannot be
silently added by an adapter.

Walter is the user's alter ego/self proxy inside Maestro's loop. Its central
question is: “If the user saw this output now for the first time, would they
approve it as-is or request a specific adjustment?” It uses canonical
Owner Context plus bounded prompt history to test intrinsic purpose, while
remaining a calm, attention-saving Senior Advisor & Refiner: not a red team,
naysayer, mind-reader or second authority. Good output is approved quickly;
gaps return a concrete load-bearing refinement to Maestro, never a direct user
message. Current explicit prompt/correction dominates history. Self evolution
is a governed track—explicit signal, bounded candidate, independent episodes,
periodic Walter proposal, facet policy, owner-attested CAS promotion and
versioned rollback—and silence, generic OK, isolated inference and ordinary
loops do not rewrite canonical self. Darwin observes health, drift, conflict
and age only; it never authors or promotes self.

Run the qualification in an attended, fresh session for each runtime:

1. Install the exact adapter bindings and record runtime/platform identity.
2. Load the installation-scoped signer through OS-managed custody under
   `maestro/walter-review`. Never paste, print, persist or include the private
   key in a packet, receipt, log or evidence artifact. This signer is not a
   release-signing authority.
3. Exercise all four Maestro decisions: Account consultation with Walter,
   Account consultation without Walter, direct Case with Walter, and direct
   Case without Walter. Confirm the two decisions are independent and that
   direct Case never calls Account.
4. For the Account path, verify the packet contains Account framing and
   post-Case Account validation IDs/digests. For the direct path, verify the
   explicit execution-only/no-client-lens reason and absence of Account
   validation fields.
5. Observe a material recommendation, consequential trade-off and external
   artifact enter Walter. Confirm Walter receives only a bounded sealed packet,
   has no tools/delegation/user channel, and returns a typed verdict.
6. Confirm calm constructive behavior: approval may include non-blocking
   polish; refinement requires one to three load-bearing issues, a proposed
   fix and acceptance condition; cosmetic objections do not block.
7. Send an IntentReviewPacket with the literal prompt, Maestro route, bounded
   draft, canonical Owner Context snapshot version/digest and relevant
   observation references. Confirm Walter returns literal request,
   evidence-backed intrinsic-intent hypothesis, confidence, purpose
   satisfaction and a constructive refinement. A low-confidence,
   high-consequence case must return `clarify`; it must not invent intent.
8. After a loop with and without Walter, verify Maestro's interaction evaluator
   runs in both cases. Persist only authenticated, material owner signals;
   keep hypotheses provisional, keep raw prompt/client/artifact text out of the
   self log, allow only registered source-event codes plus source digests, and
   reject stale promotion CAS or Darwin-authored canonical edits.
9. Prove the negative cases: missing evidence is a finding, unavailable
   custody fails closed, and forged, stale, replayed, cross-item,
   cross-attempt, wrong-installation and wrong-scope signatures are rejected.
10. Rework one refinement and prove that the prior Account validation, packet
   digest and Walter receipt cannot satisfy the new attempt. Exhaust the three
   review rounds and verify the loop stops.
11. Capture metadata-only native evidence and have a reviewer compare it with
   the shared Claude/Codex conformance fixture. Only then may the capability
   manifest move from `unavailable`.

No qualification step authorizes Walter to browse, delegate, mutate policy,
approve its own output or speak directly to the user.
