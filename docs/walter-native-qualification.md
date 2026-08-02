# Native Walter qualification recipe

The repository contains the runtime-neutral Walter wiring, but the capability
remains `unavailable` until a native Claude and Codex session proves the same
contract. Adapter-command receipts are diagnostic only.

The weekly self-review contract is intentionally a silent, bounded ingestion
pass: Walter reviews eligible owner-local interaction evidence to compact its
self projection, without a weekly message, user-facing proposal, or unbounded
journal. It remains limited to Walter custody, canonical `ownerctx`
observations/snapshots, bounded `PromptHistoryStore` selection and
metadata-only fenced receipts. The shared maintenance scheduler already emits
the weekly occurrence and the macOS LaunchAgent may wake the worker; neither
mechanism installs model authority or turns an unavailable handler into a
successful review. The remaining work is an approved runtime model adapter,
its installation-scoped authority, deterministic scheduled input assembly,
the bounded silent-compaction publication boundary and fresh native-session
qualification.

The weekly occurrence is keyed by a stable occurrence digest, never by its
retry command ID. A recoverable, cross-platform advisory lock and a renewable,
deadline-bound occurrence lease fence concurrent workers and permit safe crash
recovery. Its input is a weekly high-watermark window—not an interactive
prompt: selected entries are translated through a caller-supplied translator
whose identity/version and receipt digest are bound to the immutable input.
Historical entries are quoted, non-instructional evidence. Per-field and
combined UTF-8 limits apply before the adapter, and the input selector has
finite interaction count, byte and age limits.

Only corroborated, owner-confirmed explicit instruction, correction or specific
endorsement may affect the self projection. Observed patterns and inferred
hypotheses remain provisional; an explicit signal cannot skip the required
state transitions. The deterministic Owner Context policy derives facet
sensitivity, readers, refinement mode and confirmation requirement. A
qualified weekly implementation must replace a bounded per-facet working
projection atomically, retain only metadata-safe lifecycle evidence for a
finite recovery window, and fail closed rather than truncate, append another
weekly narrative or create a user-facing proposal queue. The current preliminary
proposal seam is not qualified for this behavior and may not be activated.

Walter is the user's alter ego/self proxy inside Maestro's loop. Its central
question is: “If the user saw this output now for the first time, would they
approve it as-is or request a specific adjustment?” It uses canonical
Owner Context plus bounded prompt history to test intrinsic purpose, while
remaining a calm, attention-saving Senior Advisor & Refiner: not a red team,
naysayer, mind-reader or second authority. Good output is approved quickly;
gaps return a concrete load-bearing refinement to Maestro, never a direct user
message. Current explicit prompt/correction dominates history. Self evolution
is a governed track—explicit signal, bounded candidate, independent episodes,
facet policy, owner-attested policy where required and versioned rollback—and
silence, generic OK, isolated inference and ordinary
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
