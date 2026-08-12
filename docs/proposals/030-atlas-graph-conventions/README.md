# Proposal 030 — Atlas graph conventions

**Status:** request for decision. A hygiene layer over segments that already
exist; lower priority than the proposals that create the authorities it depends
on, and than the operations it would be checked with.

**Original contribution:** Marcelo Petrof Sanches.

**Depends on:** Proposal 007 (owner atlas operations), PR #286 (the three
implemented operations) and decision `OATL`.

**Unblocks:** an orphan and filing check in a future maintenance pass — not
today, and not until `link` is built.

## The problem

The atlas works well at twenty pages and degrades at two hundred. The failure is
not corruption; it is that content becomes unfindable. Three specific things go
wrong, and each has a cheap rule that prevents it.

| Failure mode | What it looks like | Rule below |
| --- | --- | --- |
| Filed where it was produced | A decision recorded in the daily page it happened on, and nowhere the reader would look for a decision | (a) |
| Disconnected page | A page reachable only by knowing its exact path already | (b) |
| Reference without a path | "See the project page" — resolvable by the person who wrote it, by nobody else | (c) |

These are conventions, not a new authority. They constrain how content is filed
and linked; they do not change what may be stored, who may read it or who may
write it.

## Rule (a) — retrieval-first filing

Content is filed where it will be looked for, not where it was produced. Work
backwards from the question the content answers, and file it under the page that
answers that question.

The test is concrete and can be applied at write time: **could a reader, or the
command layer, find this again from a cold start by searching the obvious
term?** If finding it requires remembering the meeting it came from or the day
it was written, it is filed wrong.

Practically: a load-bearing number belongs in the current-truth table, with its
source and as-of date, not in the prose of the analysis that produced it. A
durable decision belongs in the decision log, not in the daily page. A daily
page may record that the decision was made and link to it — the event and the
record are different things, and only the record needs to be findable.

The payoff is that the second read is as cheap as the first. Content filed by
origin has to be re-found by reconstructing its history; content filed by
question is found by asking the question again.

## Rule (b) — no orphans

Every page declares at least one neighbour: a parent index, or an explicit
`## Related` reference. A page with no declared neighbour is a defect, not a
style preference.

This is what makes the atlas a connected graph rather than a folder of notes.
The practical payoff is that navigation works without search: from any index a
reader can reach every page that matters, and a page that nothing points to is
a page that will not be read again — which usually means it will be rewritten
from scratch instead.

**This rule is a standard, not a check, and cannot be enforced today.** Proposal
007's `link` operation is what would eventually make it mechanical: because
references would be added through a named, idempotent operation rather than by
hand-editing Markdown, the edge set would be known, and the same mechanism that
creates a reference could be asked whether one exists. `link` is accepted and not
implemented — PR #286 shipped `collect`, `create-page` and `append-entry`, and
nothing else. Until it is built, every reference is written into the page body,
the edge set is whatever a text search over the atlas reports, and an orphan
check would be a heuristic over Markdown rather than a query over a known graph.
The rule is worth adopting anyway, because a convention followed at write time is
what makes the later check cheap; it should simply not be described as enforced.

An orphan check is therefore a natural future addition to a maintenance pass
rather than an available one. Such a pass, when it is proposed, could attach an
orphan to a reachable page as a confirmed repair; this rule is the standard it
would check against. The base runtime separately declares a
`wiki-integrity-check` job whose success boundary is that "broken-link, orphan,
freshness and revocation diagnostics are persisted". That job is declared
unavailable, its stated reason being that the private atlas integrity reader is
not installed, and this proposal does not activate it.

## Rule (c) — path-preserving links

Every reference to another page is a live Markdown link that keeps the path
visible — ``[`clients/acme.md`](clients/acme.md)`` — never a bare path, a code
span with no link, or a wiki-style bracket form.

Both halves earn their place. The **link** draws the edge, so rule (b) has
something to verify and a reader can follow the reference in one action. The
**visible path** makes the reference resolvable from a cold start: a projection,
a diff or a plain-text read still shows where the target lives, without a
resolver and without the index that produced the link.

The payoff is that references survive being moved out of context. A link whose
target is implied works only in the renderer that resolves it; a path-preserving
link works in a terminal, a diff, a search result and a bounded projection.

## Scope of these rules

| Applies to | Does not apply to |
| --- | --- |
| Owner atlas pages under Proposal 007's operation set | Managed bundle content, which has its own compiler and index |
| Pages created by templates in the accepted segments | Canonical memory, which is derived and not navigated by hand |
| Workspace atlas pages, if and when an operation set is accepted for that root | Repository documentation, specs and proposals |

The rules bind the operations, not the owner. A person editing their own page by
hand is not in violation of anything; the conventions govern what the command
layer creates and what a review would expect to find.

## Consequences

- Segment templates gain a stated filing intent, so a template author has a test
  to apply rather than a preference to assert.
- `link`, once built, acquires a reason to be called routinely rather than
  occasionally, and the edge set of the atlas becomes a known quantity. Until
  then the rules are followed by the author of each write.
- An orphan and broken-link diagnostic becomes specifiable now and implementable
  in owner scope once `link` exists — as a later addition to a maintenance pass,
  which is itself not proposed here.
- Content already filed by origin is not migrated. The rules apply to new writes;
  a retro-fix pass, if wanted, is its own proposal.
- Nothing here changes admission. A page that these rules would file neatly is
  still rejected if its scope contract denies it.

## Explicit non-decisions

- no atlas segment, template, page or operation is created or modified;
- no operation in Proposal 007's accepted set is reported implemented, and
  `link` in particular is not;
- no maintenance job is activated, scheduled or reported available;
- no link checker, index generator or search capability is implemented;
- no role, grant, packet type or catalogue entry is changed;
- no scope, admission or promotion rule is widened by a filing convention;
- no runtime is reported available by merging this document.
