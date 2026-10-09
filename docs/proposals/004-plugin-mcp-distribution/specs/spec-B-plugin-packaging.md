# Spec B — Plugin Packaging (3 plugins + paridade Codex)

**Status:** DRAFT — proposal 004 · **Decisões implementadas:** D2 (três plugins), D5 (paridade Codex por degradação graciosa) · **Depende de:** Spec A (data home `~/.maestro/`), T3 (teste hooks-via-plugin no Codex) · **Referências:** [PRD.md §4](../PRD.md), [migration-plan.md](../migration-plan.md), [user-stories.md](../user-stories.md) (US-C1, US-C2, US-C3, US-R1, US-R2)

---

## Problem

O conteúdo do Maestro (skills, agents, commands, hooks) é distribuído hoje como árvore de arquivos dentro de um ZIP, com paths resolvidos relativo à pasta extraída. Consequências estruturais (evidência: 25 PRs de fix nas 0.1.10–11, CHANGELOG):

1. **Paths quebram** quando a pasta é movida/renomeada no ritual de update (Windows drive letters, symlinks, espaços).
2. **Hooks apontam para arquivos inexistentes** quando produto e estado dessincronizam.
3. **Todos os usuários pagam o custo de contexto de todos os bundles**, mesmo quem nunca usa tech-core ou pm-core.
4. **Release é monolítica**: uma correção em 1 skill do pm-core exige re-shipar os 3 bundles.

## Solution

Três plugins Claude Code, mapeando 1:1 os bundles existentes do repo:

| Plugin | Bundle de origem (repo) | Carrega |
|---|---|---|
| `maestro` | `bundles/base/` | 44 skills base*, 7 agents, commands de controle, 4 eventos de hook (5 scripts), declaração `.mcp.json` (Spec C) |
| `maestro-tech-core` | `bundles/tech-core/` | 12 skills (verificado em `bundles/tech-core/skills/INDEX.md`) |
| `maestro-pm-core` | `bundles/pm-core/` | 11 skills (verificado em `bundles/pm-core/skills/INDEX.md`) |

\* O `bundles/base/skills/INDEX.md` da working tree 0.1.11 lista 43 entradas; o número 44 vem do mapeamento da baseline v0.2.0 (branch `codex/release-0.2.0-consolidation`, não merged). Reconciliar a contagem no freeze desta spec (ver Questões Abertas QB-2).

Motivos da decomposição (D2): (a) mapeamento 1:1 com bundles que já existem — zero re-arquitetura de conteúdo; (b) controle de custo de contexto/token — colega que não usa tech-core não paga os tokens das 12 skills técnicas; (c) cadência de release independente por plugin. A seleção de bundle, que hoje é decisão de empacotamento do maintainer, vira escolha de install do usuário.

## Design

### 1. Layout de manifest

Cada plugin é um diretório com `.claude-plugin/plugin.json` na raiz:

```
maestro/
├── .claude-plugin/plugin.json      # name, version, description
├── skills/<id>/SKILL.md            # ← bundles/base/skills/ (44)
├── agents/<id>.md                  # ← bundles/base/agents/ (7)
├── commands/<id>.md                # commands de controle (doctor, onboarding, setup-update…)
├── hooks/hooks.json                # 4 eventos, paths via ${CLAUDE_PLUGIN_ROOT}
├── hooks/*.sh                      # scripts (hoje em installers/ como .claude/hooks/*.sh)
├── .mcp.json                       # declaração do maestro-mcp (Spec C)
└── pinned/                         # chave Ed25519 + sha256 do binário (Spec C §bootstrap)
```

`maestro-tech-core/` e `maestro-pm-core/` carregam apenas `.claude-plugin/plugin.json` + `skills/` — sem hooks, sem agents, sem `.mcp.json`. Isso mantém o plugin base como único ponto de execução de código (superfície de security review menor).

Os 7 agents vêm de `bundles/base/manifest.json` (campo `agents`): `case-agent`, `client-account-agent`, `darwin`, `gamma-guardian`, `maestro`, `pa-expert`, `yoda`. Os 8 `agent_templates` do manifest acompanham o plugin base como recursos referenciados pelas skills de setup (não são agents instalados).

### 2. Hooks — paths portáveis e injeção de contexto

Os 4 eventos de hook do produto (fonte: `runtime_requirements.hooks` em `bundles/base/manifest.json`) migram para `hooks/hooks.json` do plugin base, com todo path resolvido por `${CLAUDE_PLUGIN_ROOT}` (F2 — suporte oficial, verificado no plugin Anthropic `learning-output-style`):

| Evento | Script (hoje) | Criticidade (manifest) | No plugin |
|---|---|---|---|
| `SessionStart` | `first-run-scaffold.sh` | required | `${CLAUDE_PLUGIN_ROOT}/hooks/first-run-scaffold.sh` |
| `SessionStart` | `session-start-memory-inject.sh` | required | `${CLAUDE_PLUGIN_ROOT}/hooks/session-start-memory-inject.sh` |
| `UserPromptSubmit` | `context-inject-userprompt.sh` | recommended | idem (two-phase inject: ~400 tokens no primeiro fire, stub ~40 tokens depois — contrato em `bundles/base/runtime/capabilities.json`, `context_injection`) |
| `PreToolUse` (matcher `Edit\|Write`) | `block-cross-case-writes.sh` | recommended | idem |
| `SessionEnd` | `session-stop-dream.sh` | recommended | idem |

**Regra de fronteira produto/estado (invariante herdada do manifest + Spec A):**
- Scripts de hook vivem no plugin (imutável, substituído a cada update).
- Todo estado lido/escrito pelos hooks vive em `~/.maestro/` — memória, perfil, workspaces, markers. O `session-start-memory-inject.sh` injeta contexto lido de `~/.maestro/`, **nunca** do diretório do plugin. Nenhum hook escreve dentro de `${CLAUDE_PLUGIN_ROOT}` (o manifest atual já declara "bundle never writes outside data/"; a regra transfere-se para o novo data home).
- O update notice da US-C3 (comparar versão local vs manifest remoto, ≤1 check/dia, cache em `~/.maestro/`, falha silenciosa offline) entra no `session-start-memory-inject.sh` ou num script dedicado — decisão de implementação, não de spec.

Nota: o `.claude/` na raiz do repo (`run-dev-hook.sh`, `settings.json`) é o harness de DESENVOLVIMENTO do repo, não o produto — fica fora dos plugins. Os scripts de produto são os referenciados pelo `installer_path` do manifest do bundle.

### 3. Namespace de commands

Plugins namespacam commands pelo nome do plugin: `/maestro:doctor`, `/maestro:onboarding`, `/maestro:setup-update`, `/maestro:operator`. Fonte dos commands: `control_plane_skills` do manifest base (operator, doctor, setup_update, onboarding, dream_memory).

**Questão aberta Q3 (migration-plan, dona: Daniel):** naming final `/maestro:doctor` vs `/maestro-doctor`. Os ids atuais shipados são da forma `/maestro-doctor`; o namespace de plugin produz `/maestro:doctor` naturalmente. A spec recomenda adotar o namespace nativo e documentar o alias na migração (onboarding/doctor mencionam o nome novo), mas a decisão é de Daniel antes do freeze.

### 4. Marketplace

Repo marketplace (Q1 — próprio repo vs `maestro-releases` dedicado; recomendação do plano: dedicado) com `.claude-plugin/marketplace.json`:

```json
{
  "name": "maestro-releases",
  "owner": { "name": "BCG Brasil Agentic OS" },
  "plugins": [
    { "name": "maestro",           "source": "./maestro",           "description": "Maestro base — OS profissional (obrigatório)", "version": "0.3.0" },
    { "name": "maestro-tech-core", "source": "./maestro-tech-core", "description": "Skills técnicas (opcional)",                    "version": "0.3.0" },
    { "name": "maestro-pm-core",   "source": "./maestro-pm-core",   "description": "Skills de project management (opcional)",       "version": "0.3.0" }
  ]
}
```

Os plugins opcionais declaram dependência semântica do base (se o mecanismo de plugin não suportar dependência declarativa, o doctor verifica: tech-core/pm-core instalado sem base → FAIL com instrução). Versionamento em lockstep nas 3 entradas por release (pipeline Spec F); cadência independente permanece possível para patches de bundle único.

### 5. Codex — matriz de paridade e degradação graciosa (D5)

Promise do produto: "Claude-first · Codex-compatible". Estado por feature (base: estudo 2026-10-09 + hands-on 2026-10-06):

| Feature | Claude Code | Codex |
|---|---|---|
| Marketplace add (repo git) | ✅ verificado | ✅ verificado (mattpocock-skills, `~/.codex/config.toml`) |
| Plugin install/enable/update | ✅ verificado | ✅ verificado |
| Skills via plugin | ✅ verificado | ✅ verificado |
| Agents via plugin | ✅ verificado | ❓ não verificado |
| Hooks via plugin | ✅ verificado (`${CLAUDE_PLUGIN_ROOT}` oficial) | ❓ **pendente T3** (~30min, antes do freeze) |
| SessionStart injection via plugin | ✅ verificado (`learning-output-style`) | ❓ pendente T3 |
| Declaração MCP via plugin | ✅ documentado oficial | ❓ não verificado (A4; fallback `mcp_servers` config-level) |

**Design de degradação (vale mesmo se T3 passar — é o contrato de robustez):**

1. **Camada 0 — conteúdo puro:** skills (e agents, se verificado) funcionam sem nenhum hook. O plugin não pode depender de hook para ser útil: nenhuma skill assume que a injeção de contexto ocorreu (as skills já leem estado de `~/.maestro/` sob demanda).
2. **Camada 1 — projeção de hooks:** `/maestro:doctor` detecta runtime Codex + hooks-via-plugin inativos (heurística: marker de execução de SessionStart ausente em `~/.maestro/` após sessão iniciada) → oferece instalar a projeção `.codex/hooks.json` project-level, trust-gated por hash — mecanismo que o Maestro já tem (F7). A projeção referencia os scripts dentro do diretório do plugin instalado (path absoluto resolvido pelo doctor no momento da instalação da projeção, re-resolvido a cada update).
3. **Camada 2 — MCP:** se declaração MCP via plugin não funcionar no Codex (A4), doctor projeta a entrada equivalente em `mcp_servers` do `config.toml`, com consent.

A matriz acima é versionada em `acceptance/parity/` com status ✅/❌/degraded por harness e re-executada por release (US-R1).

## Acceptance Criteria

Observáveis, incluindo failure modes:

1. **Install limpo (US-C1):** em máquina sem Maestro, `claude plugin marketplace add` + `claude plugin install maestro@maestro-releases` + trust prompt + `/maestro:onboarding` resulta em `/maestro:doctor` PASS. Zero passos de edição de arquivo ou path. macOS arm64/x64 e Windows x64.
2. **Teste de update com estado (teste 3 do PRD §5 — obrigatório):** instalar vN → popular `~/.maestro/` com fixture determinística (memória, perfil, workspace, markers) → `claude plugin update` para vN+1 → asserts: (a) `~/.maestro/` byte-idêntico; (b) skills novas da vN+1 ativas e listadas; (c) doctor PASS reportando vN+1 + assinatura verificada. Automatizado em CI.
3. **Isolamento de bundles:** instalação só do `maestro` base → nenhuma skill de tech-core/pm-core aparece no contexto nem no catálogo de skills da sessão (verificação da premissa de custo de tokens, A3 — medição de context size no piloto, gate G2).
4. **Hooks portáveis:** nenhum path absoluto ou project-relative em `hooks/hooks.json`; todos via `${CLAUDE_PLUGIN_ROOT}`. Lint de release (Spec F) falha a build se encontrar path fora do padrão.
5. **Injeção lê só do data home:** com `~/.maestro/` ausente, SessionStart degrada para scaffold/no-op explicável (fail-open, invariante do manifest atual) — nunca erro bloqueante de sessão; com `~/.maestro/` presente, o contexto injetado contém exclusivamente conteúdo originado de `~/.maestro/`.
6. **Degradação Codex:** com hooks-via-plugin inativos (simulado ou real), skills continuam funcionais; `/maestro:doctor` detecta e instala a projeção `.codex/hooks.json` com consent; sessão seguinte executa os hooks via projeção. Matriz de paridade commitada em `acceptance/parity/`.
7. **Failure mode — plugin opcional órfão:** `maestro-tech-core` instalado sem `maestro` → doctor FAIL com mensagem e fix de 1 comando.
8. **Failure mode — update interrompido:** update parcial (conteúdo novo, runtime antigo) → doctor detecta dessincronia e instrui re-run; nunca estado corrompido silencioso (US-C2).

## Out of Scope

- Pipeline de release, assinatura do marketplace e revogação — Spec F.
- Migração da base instalada ZIP→plugin (`data/` → `~/.maestro/`) — Specs A e E.
- Conteúdo das skills/agents (distribuição, não produto).
- Mods JS/TS (watch TTL 2026-11-09, PRD §7).

## Open Questions

Herdadas do migration-plan: Q1 (repo marketplace dedicado), Q3 (naming `/maestro:doctor` vs `/maestro-doctor`), A1/T3 (hooks via plugin no Codex), A3 (token cost), A4 (MCP decl no Codex).

Novas desta spec:
- **QB-1 — `bundles/catalog/`:** o repo tem um 4º diretório de bundle (`bundles/catalog/`, verificado em `bundles/`) não contemplado pela decomposição D2 (base/tech/pm). Entra no plugin base, vira recurso do runtime MCP (prior-work catalog, Spec C), ou fica fora do plugin? Dona: Daniel, no freeze da Spec B.
- **QB-2 — contagem de skills base:** INDEX.md da working tree 0.1.11 lista 43; PRD assume 44 na baseline 0.2.0. Reconciliar após merge do T0 (specs 040/049 mudam o conteúdo do plugin base).
- **QB-3 — dois scripts em SessionStart:** manter `first-run-scaffold.sh` + `session-start-memory-inject.sh` como dois entries, ou consolidar num wrapper único no plugin (menos processos por sessão, um só ponto de falha)? Decisão de design no freeze.
- **QB-4 — alvo da projeção `.codex/hooks.json`:** a projeção é project-level; com o produto vindo de plugin (sem "pasta do Maestro"), em qual projeto a projeção é instalada — no(s) workspace(s) do usuário, em todos os projetos ativos, ou num projeto-âncora? Interage com o desenho do doctor (Spec E).
