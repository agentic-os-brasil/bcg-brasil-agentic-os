# Spec 033 - Workspace first-value vertical

Status: accepted for initial implementation; runtime-first transition adopted by AGNT.

## Objective

Deliver the first useful professional result from a workspace through a guided
conversation, while deterministic CLI commands retain the governed state.

## Contract

The reviewed brief contains mandate, decision, horizon, objectives,
stakeholders, authorized materials, constraints, success signals, open
questions and balanced bullish/bearish hypotheses. Every thesis contains its
evidence, assumptions, counter-evidence and invalidation signal.

The plan has one to three actions. Each names an outcome, accountable owner and
observable completion criterion. The agent produces a classified Markdown
decision brief under `brain/deliverables/`, with case context and the working
plan under `brain/projects/`, decisions under `brain/decisions/` and explicitly
accepted open work under `brain/tasks/`. Private metadata and the legacy
deterministic first-value CLI remain compatibility/recovery surfaces; a new
conversation must not require run IDs, JSON envelopes or `value submit`.

## Conversation and completion

The managed guide asks the six minimum questions: decision and horizon;
audience/constraints; useful result; authorized material; balanced hypotheses;
and next owner/step. The agent shows the normalized brief and one-to-three
action plan, waits for owner approval, then writes the reviewed Markdown
artifacts. A later conversation reads those artifacts and continues from the
latest checkpoint rather than replaying the interview.

## Metrics and limits

`time_to_first_value_seconds` begins when the deterministic run starts and ends
when the artifact is available. `manual_interventions` counts only explicit
brief, plan or artifact correction kinds. Both remain workspace-local and are
not performance measures.

No document ingestion, wiki, dreaming, external research execution, economic
refresh or new agent role belongs to this vertical.
