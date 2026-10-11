# Migration Plan — ZIP → Plugin + MCP + Connector (v0.3.0)

**Método:** `spec-driven-delivery` — fases com gates observáveis; separação explícita de fatos vs assunções vs questões abertas.
**Task graph:** ver [user-stories.md §5](user-stories.md). **Decisões D1–D7:** [PRD.md §4](PRD.md).

---

## 0. Fatos vs Assunções vs Questões Abertas

### Fatos (verificados hands-on)
| # | Fato | Evidência |
|---|---|---|
| F1 | v0.2.0 NÃO está merged; main = 0.1.11 | `origin/main` = 81022cd; branch `codex/release-0.2.0-consolidation` = 6a3ee889; tags param em v0.1.10 |
| F2 | `${CLAUDE_PLUGIN_ROOT}` é oficial e usado por plugins Anthropic | `learning-output-style/hooks/hooks.json` local |
| F3 | SessionStart hook via plugin funciona (context injection always-on) | `learning-output-style` instalado e observado |
| F4 | Marketplace git + install/update funciona em Claude Code E Codex | mattpocock-skills instalado nos dois (2026-10-06); `~/.codex/config.toml` |
| F5 | Go não foi removido — 70+ pacotes vivos sob slash-commands | mapeamento v0.2.0; CLI `bcgos` removido no PR #349 |
| F6 | Maestro já tem manifest de release assinado Ed25519 | Spec 020 |
| F7 | Maestro já tem projeção `.codex/hooks.json` trust-gated testada | repo |
| F8 | 25 PRs de fix 0.1.10–11 concentrados em path/hook/onboarding | CHANGELOG |

### Assunções (explícitas, cada uma com teste que a derruba)
| # | Assunção | Como falsificar |
|---|---|---|
| A1 | Hooks entregues POR plugin rodam no Codex | T3: plugin com SessionStart trivial no Codex (~30min) |
| A2 | Colegas conseguem auth GitHub no repo marketplace (EMU/SAML) | Gate G2: piloto com 1 colega |
| A3 | Token cost de 44 skills base always-on é aceitável | Medição no piloto (context size antes/depois) |
| A4 | MCP declaration via plugin funciona no Codex | Teste na fase C; fallback: `mcp_servers` config-level |
| A5 | Migração doctor cobre 100% das instalações 0.1.x/0.2.0 reais | Teste 4 (fixtures sintéticas) + piloto |

### Questões abertas (donas de decisão)
| # | Questão | Decide | Quando |
|---|---|---|---|
| Q1 | Marketplace no próprio repo ou repo `maestro-releases` dedicado? | Daniel | Antes da Spec F freeze (recomendação: dedicado — separa conteúdo sensível do canal de distribuição) |
| Q2 | Se A2 falhar: marketplace público com conteúdo auditado, ou outro canal? | Daniel + security review | Pós-G2 |
| Q3 | Naming final dos comandos (`/maestro:doctor` vs `/maestro-doctor`) | Daniel | Spec B |

## 1. Fases e Gates

### P0 — Pré-requisitos (bloqueante)
- **P0.1** Merge `codex/release-0.2.0-consolidation` → `main`. **Ação de Daniel** (no-direct-merges). Specs 040/049 mudam o conteúdo do plugin base — nada downstream começa antes.
- **P0.2** T3: teste Codex hooks-via-plugin (~30min). Resultado versionado em `acceptance/parity/`.

**Gate G0 (observável):** main contém v0.2.0 taggeada + resultado do T3 commitado (✅ ou ❌ com fallback D5 ativado no design).

### P1 — Estado fora do produto (Specs A + E)
- Implementar `~/.maestro/` como data home; hooks resolvem via marker/env.
- Doctor ganha migração automática idempotente + rollback.
- **Ship como último release ZIP (v0.2.x)** — a base instalada migra ANTES do plugin existir.

**Gate G1 (observável):** teste 4 do PRD passa (fixture 0.1.11 e 0.2.0 → migra → equivalência + marker + rollback); release ZIP v0.2.x publicada; ≥1 instalação real migrada sem perda (verificada por doctor PASS).

**Condição de entrada P1→P2 (anti-SPOF de migração):** P2 só inicia com **cobertura de migração MEDIDA contra a lista conhecida de destinatários de ZIP** (canary Spec 054 já existe na base ZIP e reporta o marker de migração) — não basta "ZIP v0.2.x publicada". Colega que nunca rodar o último ZIP não pode perder memória silenciosamente: o onboarding do plugin, ao detectar `~/.maestro/` com origem `fresh`, pergunta "usava Maestro por ZIP nesta máquina?" e roteia para migração assistida ANTES de escrever qualquer estado (Spec E §3.5).

### P2 — Plugin + Pipeline (Specs B + F)
- 3 plugins (`maestro`, `maestro-tech-core`, `maestro-pm-core`) no marketplace.
- Pipeline de release: tag → build → manifest Ed25519 → marketplace.json.
- Doctor vira conformance check pós-install/update.

**Gate G2 (observável):** **2 pilotos** — 1 técnico (Rafael) E 1 colega NÃO-técnico sem setup prévio de GitHub (persona Carla, que carrega o risco A2/A3; um piloto só-técnico é survivorship no gate). Cada um completa install limpo + 1 ciclo de update sem escalação; doctor PASS nos dois momentos; fricção de auth EMU/SAML documentada do zero real; medição A3 coletada.

### P3 — MCP + Connector (Specs C + D)
- `maestro-mcp` cross-compilado, declarado no `.mcp.json` do plugin, bootstrap verificado (sha256+Ed25519 pinados no plugin), instalado em `~/.maestro/bin/`.
- Mesmo binário registrado no Claude Desktop (connector local).

**Gate G3 (observável):** tool MCP respondendo em Claude Code e Claude Desktop na máquina do piloto; versão runtime = versão plugin (doctor verifica); update de plugin atualiza binário atomicamente.

### P4 — Rollout
- Comunicação aos colegas: instrução de migração (1 email final — o último).
- Canary reporta `plugin_version` + `runtime_version`.
- Janela de convivência ZIP/plugin: 1 release; depois ZIP descontinuado.

**Gate G4 (observável):** ≥80% da **lista explícita de destinatários de ZIP** (denominador conhecido pelo maintainer — o canal era email) reportando versão via canary em plugin ≥ v0.3.0; destinatários sem canary contam como não-migrados (nunca saem do denominador); zero escalações de migração abertas há 2 semanas.

## 2. Riscos e circuit breakers

| Risco | Gate que o detecta | Circuit breaker |
|---|---|---|
| Migração corrompe estado de colega | G1 | Rollback documentado; ZIP v0.2.x anterior permanece disponível |
| Hooks não rodam no Codex via plugin | G0 (T3) | D5: projeção `.codex/hooks.json` via doctor |
| Auth GitHub proibitiva (EMU/SAML) | G2 | Q2: decisão explícita de canal alternativo — rollout pausa, não prossegue com workaround improvisado |
| Token cost inaceitável | G2 (A3) | Re-split de bundles / lazy loading antes do rollout |
| Binário MCP flaggeado por endpoint security BCG | G3 | Default = binários embutidos no plugin + codesign/notarytool (macOS) + Authenticode (Windows) no pipeline (Spec C §2, Spec F); download-on-first-run só como fallback com sign-off do security review |

## 3. O que este plano NÃO faz
- Não muda comportamento de skills/agents (distribuição, não produto).
- Não remove o caminho ZIP antes do G4.
- Não automatiza update sem consent.
- Não hospeda server remoto (v0.4+).
