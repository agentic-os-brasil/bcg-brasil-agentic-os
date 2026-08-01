# Native Walter qualification recipe

The repository contains the runtime-neutral Walter wiring, but the capability
remains `unavailable` until a native Claude and Codex session proves the same
contract. Adapter-command receipts are diagnostic only.

The weekly self-review contract in this PR is intentionally limited to
Walter custody, bounded prompt/self inputs, proposal validation and the
unavailable-by-default handler. It does not add scheduler or launchd catalog
entries. Its orchestration dependency is PR
[#148](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/pull/148);
after that PR lands, orchestration may rebase and trim this seam as needed.

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
   self log, and reject stale promotion CAS or Darwin-authored canonical edits.
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
