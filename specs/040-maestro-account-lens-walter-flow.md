# Maestro account lens and Walter review flow

This contract supersedes generic depth-based routing descriptions for the
Maestro delivery path. Maestro makes two independent, typed decisions:

1. `account_consultation_required` is resolved from client strategic-lens and
   stakeholder-pressure signals. It is not derived from task size or technical
   complexity. No signal is fail-safe and consults Client Account.
2. `walter_required` is resolved from high-leverage signals: consequential
   decisions, executive recommendations, important tradeoffs, external
   artifacts, reputational risk, or hard-to-reverse outcomes. Ordinary,
   reversible, low-leverage work may carry an auditable Walter skip.

The two axes are independent. The mediated paths are:

- Account-assisted: Maestro -> Client Account framing -> Case -> Client
  Account validation -> Maestro -> optional Walter review -> Maestro -> User.
- Direct Case: Maestro -> Case -> Maestro -> optional Walter review -> User.

If Client Account frames the work, its validation is mandatory. If it does not
frame the work, it does not validate the result. Walter is internal Senior
Advisor & Refiner: calm, proportional, constructive, and load-bearing. A
`REFINE` verdict requires a concrete actionable refinement; `HOLD` is reserved
for an exceptional material governance blocker. Walter has no tools, no
delegation, and no direct user channel.

Every runtime transition is mediated by Maestro with one active spoke and no
nested or agent-to-agent delegation. Darwin receives only metadata-only
receipts and may propose calibration for account selection, Walter invocation,
and useful-versus-nitpick refinement drift. Weekly scorecards distinguish
strategic-signal under-routing/over-routing, Walter invocation and skip reason
codes, useful load-bearing refinements and naysayer/nitpick drift. Darwin
cannot mutate live policy or self-approve a proposal.
