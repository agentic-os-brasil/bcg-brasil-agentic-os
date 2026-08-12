---
name: quantitative-analysis
description: Analyze bounded quantitative evidence with explicit assumptions, checks and uncertainty. Use only with authorized data and tools; it does not grant data access, execution or publication authority.
---

# Quantitative Analysis

Define the question, data scope, calculations, assumptions and validation
checks before reaching a conclusion. Return the result, evidence pointers,
material sensitivity, limitations and any decision implication. Do not treat a
calculation as a fact when its inputs, assumptions or validation remain open.

This method cannot acquire data, run an ungranted tool, persist outputs or
promote conclusions. The owning vertical agent remains accountable for those
actions.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the analysis. It
changes explanation depth only; it never changes data or tool authorization.

## Required input

Ask for the following, one at a time, if any item is missing before proceeding:

1. **Dataset or data description** — the actual data to analyze, or a precise
   description of it (source, structure, time range, unit of observation).
   Pasted tables, uploaded files, or inline figures are all acceptable.
2. **Metric or question** — the specific metric to calculate or hypothesis to
   test. Restate it back once confirmed so the scope is unambiguous.
3. **Data access** — whether the data is being supplied directly (pasted or
   uploaded) or whether it lives somewhere that requires a separate step to
   retrieve. If retrieval is needed, return the data-gathering specification
   below instead of proceeding.
4. **Output format preference** — whether a headline finding with key numbers
   suffices, or whether a full breakdown with all supporting calculations is
   required. Governs presentation depth; does not change the analysis.

If items 1 or 2 are absent, ask for the missing item before proceeding.
If item 3 reveals that the data is not accessible, return the error contract
below. If item 4 is absent, default to headline finding plus supporting
numbers.

## Method

1. Restate the question and confirm the data scope and unit of observation.
2. **Descriptive statistics**: compute the key summary measures relevant to the
   question (counts, totals, means, medians, distributions, or rates as
   appropriate). Show each calculation step.
3. **Pattern identification**: surface trends, concentrations, or structural
   differences that are material to the question. Label each pattern and link
   it to the underlying numbers.
4. **Outlier flagging**: identify observations that deviate materially from the
   central tendency; assess whether they are data quality issues or genuine
   signals; flag which interpretation is more likely and why.
5. **So-what framing**: translate numbers into a consulting-grade insight —
   what this means for the decision or recommendation, in language the audience
   can act on.

## Output contract

Return the following structure:

- **Headline finding**: one to two sentences stating the main result and its
  direction, magnitude, and significance for the decision.
- **Supporting numbers**: the key figures with calculation steps shown, so the
  reader can verify and recreate the result without assistance.
- **Sensitivity or confidence caveat**: how the conclusion changes under
  different assumptions, data cuts, or if identified data quality issues are
  resolved differently.
- **Decision implication**: what the analysis recommends or flags for the
  decision at hand, stated in terms of action or trade-off rather than
  numbers alone.

## Invariants

- Does not fabricate numbers or introduce figures not present in the supplied
  data.
- Explicitly flags when the available data is insufficient to answer the
  question with defensible confidence; does not smooth over gaps.
- Never presents correlation as causation without an explicit qualification
  stating the limitation in the output.
- Shows calculation steps so the consultant can recreate the result
  independently; conclusions without traceable arithmetic are not returned.

## Error handling

If the data is not accessible — because it lives in a system that requires
a separate retrieval step and no authorized access has been established — do
not attempt a partial analysis. Instead return a **data-gathering
specification**:

- what data is needed (fields, granularity, time range, unit of observation);
- the format in which it should be supplied (pasted table, uploaded file, or
  described summary);
- any minimum completeness threshold below which the analysis cannot be
  performed;
- a recommended next step for obtaining and supplying the data.

Do not invent placeholder numbers or illustrative figures to simulate the
analysis while waiting for real data.
