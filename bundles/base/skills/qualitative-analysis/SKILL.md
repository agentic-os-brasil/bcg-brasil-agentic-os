---
name: qualitative-analysis
description: Synthesize bounded qualitative evidence into themes, counter-evidence and decision implications. Use for "synthesize these interviews", "what are the themes", "pull the qualitative story", or with a bounded set of authorized sources. Does not authorize research, broad browsing or persistence.
---

# Qualitative Analysis

Analyze only the supplied evidence set. Identify recurring themes, meaningful
differences, supporting and counter-evidence, confidence and implications for
the stated decision. Keep observations distinct from interpretations and
hypotheses. Return a concise synthesis with evidence pointers and unresolved
questions.

This method cannot expand source scope, perform external research, promote
facts to account context or persist a conclusion. Those remain decisions and
authorities of the owning vertical agent.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the synthesis.
It changes explanation depth only; it never changes source or scope
authorization.

## Required input

Ask for the following, one at a time, if any item is missing before proceeding:

1. **Evidence set** — the raw material to analyze: interview transcripts,
   documents, field observations, or other authorized sources. Minimum two
   distinct sources are required to begin synthesis.
2. **Central question** — the specific question the analysis must answer.
   Restate it back once confirmed so the scope is unambiguous.
3. **Audience and purpose** — whether the output is for internal hypothesis
   testing or client-ready communication, and who the primary reader is. This
   governs framing and confidence language, not analytical rigor.

If the evidence set is absent, return the error contract below. If items 2
or 3 are absent, ask for the missing item before proceeding; do not begin
analysis with an assumed question or audience.

## Method

1. Read the full evidence set and restate the central question verbatim.
2. Conduct **thematic synthesis**: group recurring observations into named
   themes; each theme requires at least two independent evidence pointers.
3. Apply **evidence traceability**: every claim in a theme must reference a
   specific source segment (document title, interview speaker, observation
   note). No claim may float without a pointer.
4. Run a **competing hypothesis check**: for each theme, state at least one
   alternative interpretation the evidence could also support, and explain
   why the primary interpretation is preferred or why the two cannot yet be
   distinguished.
5. Assign a **confidence signal** per theme: `high` (multiple independent
   sources, no material counter-evidence), `medium` (partial support or one
   conflicting source), or `low` (single source, weak signal, or meaningful
   counter-evidence present).
6. Identify **decision implications**: what each theme means for the decision
   the analysis is meant to inform.

## Output contract

Return the following structure:

- **Themes**: for each theme — label, one-paragraph description, supporting
  evidence pointers with source references, confidence level, and the primary
  competing explanation.
- **Competing explanations**: a consolidated view of the most significant
  alternative interpretations that could not be ruled out.
- **Decision implications**: what the synthesis recommends or flags for the
  decision at hand, tied to specific themes.
- **Limitations**: what cannot be concluded given the current evidence,
  including gaps that would materially change the interpretation if filled.

## Invariants

- Does not fabricate evidence or introduce examples not present in the supplied
  sources.
- Does not declare a theme where fewer than two independent evidence pointers
  exist; labels such signals as `signal — insufficient evidence to form theme`.
- Explicitly flags every low-confidence zone in the output; never smooths over
  uncertainty to appear more conclusive.
- Does not promote a finding beyond what the evidence warrants, regardless of
  what conclusion may seem desirable for the case.

## Error handling

If the evidence set is empty or contains fewer than two distinct sources,
do not attempt a synthesis. Instead return:

- a scoped statement of what is available (source count, type, approximate
  coverage);
- the minimum evidence that would unlock a defensible analysis (source types,
  quantity, breadth required);
- a recommended next step for gathering that evidence.

Do not partially synthesize from a single source and present it as a theme.
