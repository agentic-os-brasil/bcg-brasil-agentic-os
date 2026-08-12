---
name: client-delivery-gate
description: Quality gate before any client-facing output — deck, email, memo, analysis, recommendation. Applies three review lenses and returns a green/yellow/red verdict with specific concerns and suggested fixes. Use before sending anything to a Partner, client or external stakeholder.
---

# Client Delivery Gate

Trigger phrases: "revisar antes de enviar", "gate de qualidade",
"client delivery check", "review before sending", "check antes de enviar pro
cliente", "pode enviar?", "está pronto para o cliente?", "pre-send review".

Resolve the canonical `interaction-profile` skill before presenting the verdict
to any agent that surfaces output to the owner. It adjusts explanation depth
only; it never changes the gate verdict, the concerns found, or the invariants.

A quality gate for consulting outputs before they reach a Partner, client, or
external stakeholder. The skill applies three review lenses and returns a
verdict with specific concerns and suggested fixes. It does not rewrite the
output — identifying and suggesting is the boundary; the owner decides and edits.

## Input required

Collect the following before proceeding. If any item is missing, ask for it
before applying any lens.

1. **Output to review** — the full text, a pasted excerpt, or a faithful
   description of what will be sent. Do not proceed on a summary invented by
   this skill.
2. **Audience** — who will receive it: Partner BCG, C-Suite client, operational
   client team, regulator, or other. Audience tolerance for uncertainty and
   framing choices differs significantly across these groups.
3. **Delivery format** — email, deck, memo, analysis document, or verbal
   recommendation.

If the output has not been provided, respond with:

> "Para revisar, preciso do conteúdo que será enviado. Pode colar o texto ou
> descrever o que vai no deck?"

If the audience has not been specified, ask before applying any lens.

## The three review lenses

### Lens 1 — Analytical solidity (Kahneman)

- Is the conclusion anchored in explicit evidence or in intuition?
- Does the output assert causation where only correlation is present?
- Is the stated confidence level consistent with data quality?
- Does the reasoning survive a 20% error in the underlying data?

### Lens 2 — Risk management (Taleb)

- What is the worst case if the recommendation is wrong?
- Does the output ignore tail distributions or rare events?
- Is something presented as certain that is actually probabilistic?
- Is there an asymmetry between upside and downside that the client must know?

### Lens 3 — Framing integrity (Ariely)

- Does the presentation create artificial anchors that distort perception?
- Are comparisons built on a fair baseline, or one chosen to favor the thesis?
- Is any material information omitted that would change the client's decision?
- Does the framing direct toward a conclusion the data alone do not support?

## Evaluation procedure

1. Apply each lens to the provided output. For each lens, note every concern
   found with the specific passage or section where it appears.
2. Classify each concern as material (affects the client's decision or trust) or
   minor (presentation, clarity, or cosmetic).
3. Determine the overall verdict:
   - Any material concern in any lens → verdict is 🔴 SEGURAR or 🟡 LIBERADO
     COM RESSALVAS depending on severity.
   - No material concern in any lens → verdict is 🟢 LIBERADO.
   - Never assign 🟢 if a material concern was found.

## Verdicts

**🟢 LIBERADO** — No material concern found across all three lenses. Can be sent.

**🟡 LIBERADO COM RESSALVAS** — Concerns that do not block delivery but should
be addressed. List each concern as:

```
[Lente N] <specific concern with passage cited> → <suggested fix>
```

Owner decides whether to incorporate before sending.

**🔴 SEGURAR** — A concern that creates real risk with the client or Partner.
Describe the specific risk (not a generic warning). Propose the minimum fix
required to unblock. Do not send until resolved.

## Output format

```
## Gate de Entrega — <date>

**Output revisado:** <brief description>
**Audiência:** <who>
**Veredito:** 🟢 / 🟡 / 🔴

### Concerns
[omit section if none]
- [Lente 2] A projeção de R$X assume crescimento linear — tail scenario de contração não modelado. **Sugestão:** adicionar range pessimista.

### Próximo passo
[clear instruction: "pode enviar" / "incorporar concerns e re-submeter" / "resolver item crítico antes de enviar"]
```

## Invariants

- Never return 🟢 if any lens identified a material concern.
- Do not paraphrase or reconstruct the output — review only what was provided.
- Concerns must cite a specific passage or section, never be generic.
- Do not rewrite the output; identify and suggest only.
- If the output was not provided, do not proceed — ask for the material first.

## Error handling

- **Output not provided** → "Para revisar, preciso do conteúdo que será enviado.
  Pode colar o texto ou descrever o que vai no deck?"
- **Audience not specified** → ask before applying any lens; Partner BCG, C-Suite,
  and operational teams have meaningfully different tolerances.
