# Spec 050 - BCGOS operational method

Status: accepted

## Purpose

An installed Maestro must know how to operate, inspect, verify and recover the installed BCGOS control plane without discovering commands by trial and error or turning CLI mechanics into the user's experience. Ordinary professional work remains owned by the runtime agent.

## Installed method

The base bundle ships `bcgos-operator` as a compact control-plane method. It maps setup, health, version, update and exceptional recovery to the public CLI. Onboarding, agents, work continuity, memory, prior work and ingestion route to governed skills and canonical workspace artifacts instead of CLI-first flows.

The method grants no authority. Workspace isolation, explicit confirmation for external or destructive effects, managed-root protection and runtime-native tool permissions remain authoritative.

## SessionStart contract

Every installed Claude and Codex SessionStart packet selects the integrity-checked `bcgos-operator` pointer with reason `deterministic_operational_method`. The native directive tells Maestro to load that skill before choosing, interpreting or recovering a BCGOS operation.

The skill body is not embedded in the packet. SessionStart carries only the relative installed pointer and the instruction to load it, preserving bounded startup context and keeping the skill bundle as the canonical method.

At most three skill pointers may be present:

1. `bcgos-operator`;
2. `maestro-onboarding` when onboarding is pending;
3. up to the remaining capacity from the existing bounded task router.

The router itself continues to select no more than two task methods. Duplicate IDs are removed while preserving operational, onboarding and task order.

## Context budgets

The total native SessionStart output is capped at 16 KiB. Generated local memory remains independently capped at 8 KiB, so the added capacity is reserved for operating instructions and pointer metadata rather than broader memory exposure. Oversized pointer envelopes remain omitted whole instead of being serialized partially.

## Operating loop

The method requires Maestro to:

1. resolve the exact installed CLI and active workspace from SessionStart;
2. inspect current state before changing it;
3. route ordinary work to the runtime skill and control-plane work to the smallest public command;
4. execute routine mechanics silently;
5. verify the resulting state with the relevant status or doctor surface;
6. recover from typed errors without bypassing guards or guessing.

User-facing answers lead with outcome, next action and relevant evidence. Commands and internal capability state are exposed only when the owner requests a technical explanation or must make a consequential choice.

## Validation boundary

Contract and unit tests cover projection, pointer selection, ordering, budgets and installed orientation. Model-backed evals are intentionally outside this change and do not gate availability of the deterministic method.
