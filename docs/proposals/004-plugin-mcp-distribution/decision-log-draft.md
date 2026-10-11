# Decision Log Draft — Proposal 004

**Formato:** `pm-decision-log-entry`. Entry abaixo é DRAFT — só entra no decision log oficial após decisão do owner.

---

## D-NNN — Distribuição do Maestro migra de ZIP-por-email para Plugin + MCP + Connector (v0.3.0)

- **Decider:** Daniel Scardini (owner) — PENDENTE
- **Date:** PENDENTE
- **Status:** PROPOSED

### Context
Distribuição atual (ZIP assinado por email + ritual manual de update) gera dor estrutural: 25 PRs de fix nas releases 0.1.10–11 concentrados em path/hook/onboarding, causados por produto e estado do usuário coabitarem pasta movida manualmente. Maintainer sem canal de revogação nem telemetria de versão. Estudo validado hands-on (2026-10-09, Walter refine-and-return absorvido) confirma viabilidade: marketplace git + plugin install/update funcionam em Claude Code e Codex; `${CLAUDE_PLUGIN_ROOT}` oficial; SessionStart injection via plugin comprovada.

### Decision (proposta)
1. Separar estado do produto: `data/` → `~/.maestro/` (Spec A), com migração automática via doctor shipada no ÚLTIMO release ZIP (Spec E).
2. Três plugins mapeando os bundles existentes: `maestro`, `maestro-tech-core`, `maestro-pm-core` (Spec B).
3. Runtime Go empacotado como binário `maestro-mcp` declarado no `.mcp.json` do plugin, bootstrap verificado sha256+Ed25519 (Spec C).
4. Connector = mesmo binário registrado no Claude Desktop local (Spec D).
5. Trust chain reusa manifest Ed25519 da Spec 020; doctor como conformance check (Specs E/F).
6. Paridade Codex por degradação graciosa (projeção `.codex/hooks.json` via doctor).
7. Scheduler permanece fora do plugin (enrollment consentido).

### Rationale
- Elimina a classe inteira de bugs de path por construção (plugin imutável + `${CLAUDE_PLUGIN_ROOT}` + home fixo).
- Update de ~10 min com medo → 1 comando (`claude plugin update`) com estado intocado.
- Usa o trilho nativo da plataforma em vez de lutar contra ela (installers/wizards).
- Mantém (não rebaixa) a cadeia de proveniência Ed25519 existente, somando revogação e telemetria de versão.

### Gates condicionantes (não são decisão, são pré-condições)
- G0: merge da v0.2.0 em main (ação do owner) + teste T3 (hooks-via-plugin no Codex, ~30min).
- G2: piloto com 1 colega valida auth GitHub EMU/SAML antes de rollout — se proibitiva, decisão Q2 reabre o canal.

### Linked
- `docs/proposals/004-plugin-mcp-distribution/` (PRD, migration-plan, user-stories, specs A–F)
- Spec 020 (release manifest Ed25519), Spec 040/049 (native agents — conteúdo do plugin base)
- Estudo de origem: kowalski-os `wiki/investigations/2026-10-09-agentic-os-brasil-plugin-mcp-study.md`

### Review
- Revisit se: T3 falhar sem fallback viável; piloto G2 revelar fricção de auth proibitiva; feature "mods" amadurecer (TTL watch 2026-11-09) a ponto de mudar o desenho do onboarding.
