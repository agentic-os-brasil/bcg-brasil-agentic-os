# Proposal 004 — Plugin/MCP/Connector Distribution

**Status:** PROPOSAL — pending owner approval · **Target release:** v0.3.0 · **Date:** 2026-10-09

Transforma a distribuição do Maestro de ZIP-por-email (ritual manual de update) para:
**Claude Code plugin** (conteúdo) + **MCP server** (runtime Go empacotado) + **connector** (mesma superfície MCP no Claude Desktop/web), com paridade Codex.

## Artefatos

| Arquivo | Conteúdo | Método |
|---|---|---|
| [PRD.md](PRD.md) | Problema, solução, decisões de implementação e teste | Template Pocock `to-spec` + lentes `ux-critique` |
| [user-stories.md](user-stories.md) | Stories por persona + task graph | Pocock `to-tickets` |
| [migration-plan.md](migration-plan.md) | Fases, gates, fatos vs assunções, critérios observáveis | `spec-driven-delivery` |
| [specs/](specs/) | 6 specs por componente (A–F) | Template spec do repo + Pocock |
| [decision-log-draft.md](decision-log-draft.md) | Entry D-NNN pendente de decisão do owner | `pm-decision-log-entry` |

## Pré-requisitos duros (ordem)

1. **Merge da v0.2.0** (`codex/release-0.2.0-consolidation` → `main`). Hoje NÃO está merged; main = 0.1.11. Esta proposta assume a 0.2.0 como baseline.
2. **Spec A (data home decoupling)** shipado como último release ZIP — migra a base instalada antes do plugin existir.
3. Teste de paridade: plugin com SessionStart hook no Codex (~30min) — decide design Spec B.

## Leitura em 60 segundos

Hoje: colega recebe ZIP por email → extrai → ritual de update multi-step (renomear pasta, extrair, copiar `data/` de volta, doctor). 25 PRs de fix nas v0.1.10–11 concentrados em path/hook/onboarding — a dor é estrutural.

Depois: `claude plugin marketplace add` + `claude plugin install maestro` (uma vez) → `claude plugin update` (sempre). `data/` vive em `~/.maestro/`, intocado por updates. Runtime Go vira binário MCP versionado junto com o plugin, verificado por Ed25519 (reusa Spec 020). Mesmo comportamento por construção: plugin imutável + scaffold determinístico + doctor como conformance check.
