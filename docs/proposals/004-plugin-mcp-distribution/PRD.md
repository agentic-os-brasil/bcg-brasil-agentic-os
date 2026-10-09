# PRD — Maestro Distribution v0.3.0: Plugin + MCP + Connector

**Formato:** Pocock `to-spec` (Problem → Solution → User Stories → Implementation Decisions → Testing Decisions → Out of Scope) com seção UX via `ux-critique` (6 lentes).
**Baseline assumida:** v0.2.0 merged (hoje em `codex/release-0.2.0-consolidation`; pré-requisito P0 do plano).

---

## 1. Problem Statement (perspectiva do usuário)

**Colega BCG (não-técnico):** "Recebi um ZIP por email. Para atualizar, preciso fechar o Claude Code, renomear uma pasta, extrair outro ZIP, copiar uma pasta `data/` de volta no lugar certo, rodar um comando de verificação e lembrar de apagar a pasta antiga depois de uma semana. Se eu errar um passo, perco minha memória ou o Maestro quebra de um jeito que não entendo."

**Evidência estrutural:** 25 PRs de correção nas releases 0.1.10–0.1.11 concentrados em exatamente três categorias — resolução de paths (Windows drive letters, symlinks, espaços, unicode), hooks apontando para arquivos inexistentes, e onboarding divergindo do estado real da instalação. Não são bugs incidentais: são consequência de o produto e o estado do usuário coabitarem uma pasta movida manualmente.

**Maintainer:** cada release exige build local no macOS do maintainer, envio por email, e suporte individual a cada colega que erra o ritual. Não há canal de revogação, não há telemetria de versão instalada, não há como saber quem está em qual versão.

## 2. Solution (perspectiva do usuário)

**Instalar (uma vez):**
```
claude plugin marketplace add agentic-os-brasil/maestro-releases
claude plugin install maestro@maestro-releases
# abrir Claude Code → aceitar trust prompt → /maestro:onboarding
```

**Atualizar (sempre):** `claude plugin update` — um comando. `data/` (memória, perfil, workspaces) vive em `~/.maestro/` e nunca é tocado por update.

**Mesmo comportamento em qualquer máquina, por construção:**
1. Plugin imutável e versionado — todos rodam bytes idênticos da mesma release.
2. Paths resolvidos por `${CLAUDE_PLUGIN_ROOT}` (produto) e `~/.maestro/` (estado) — elimina a classe inteira de bugs de path.
3. `/maestro:doctor` como conformance check pós-install e pós-update.
4. Runtime Go empacotado como MCP server com versão travada à versão do plugin — conteúdo e runtime nunca dessincronizam.
5. Connector: a mesma superfície MCP registrável no Claude Desktop — Maestro acessível fora do terminal sem lógica duplicada.

**Decomposição por primitiva:**

| Primitiva | Carrega | Spec |
|---|---|---|
| Plugin `maestro` | 44 skills base, 7 agents, commands, hooks (session_start, user_prompt_submit, pre_tool_use, session_end), `.mcp.json` | B |
| Plugins `maestro-tech-core` / `maestro-pm-core` | bundles opcionais (12 + 11 skills) — seleção de bundle vira escolha de install | B |
| MCP server `maestro-mcp` | pacotes Go (agentstate, memória/dreaming, scheduler, adapters, canary) como tools MCP | C |
| Connector | mesmo server MCP no Claude Desktop (local) | D |
| `~/.maestro/` | data/ do usuário — fora do produto | A |
| Doctor | conformance + migração da base instalada | E |
| Release pipeline | marketplace + manifest Ed25519 (reusa Spec 020) | F |

## 3. User Stories

Extraídas para [user-stories.md](user-stories.md) (formato `to-tickets` com task graph). Resumo das personas:
- **Carla, consultora (não-técnica):** instala, usa, atualiza sem ritual; nunca vê path nenhum.
- **Daniel, maintainer:** publica release com um comando; sabe quem está em qual versão; pode revogar.
- **Rafael, pilot/early adopter (técnico):** valida paridade Codex; reporta fricção via canary.
- **Time de security review BCG:** audita proveniência do que executa nas máquinas.

## 4. Implementation Decisions

**D1 — Estado separado do produto (pré-condição de tudo).** `data/` migra para `~/.maestro/`. Hooks resolvem via marker/env, nunca path relativo ao projeto. Migração automática e idempotente via doctor, shipada no ÚLTIMO release ZIP (v0.2.x), para que a base instalada chegue ao plugin já migrada. (Spec A, E)

**D2 — Três plugins, não um.** `maestro` (base), `maestro-tech-core`, `maestro-pm-core`. Motivos: (a) mapeamento 1:1 com os bundles existentes; (b) controle de custo de contexto — colega que não usa tech-core não paga os tokens das 12 skills; (c) cadência de release independente **apenas para os bundles opcionais** (content-only): o plugin base `maestro` e o runtime `maestro-mcp` são SEMPRE lockstep (mesma versão, mesmo pipeline — ver D3 e Spec F). (Spec B)

**D3 — Go permanece, empacotado como MCP server.** Achado do mapeamento v0.2.0: o Go NÃO foi removido — virou substrate invisível sob slash-commands (70+ pacotes vivos: agentstate, sessionhook, adaptercfg, scheduling, canary). A migração MCP é **empacotamento, não extração**: binário `maestro-mcp` cross-compilado (darwin-arm64/amd64, windows-amd64), declarado no `.mcp.json` do plugin, instalado em `~/.maestro/bin/`. **Default: binários shipados DENTRO do plugin assinado** (codesign+notarytool macOS / Authenticode Windows no pipeline, Spec F) — download-on-first-run é fallback size-driven, gated em security sign-off, nunca o default (binário não notarizado baixado em runtime falha Gatekeeper em macOS gerenciado e falha security review). Verificação sha256+Ed25519 pinada no plugin em ambos os modos. (Spec C)

**D4 — Trust chain reusa Spec 020.** O Maestro já tem release manifest com assinatura Ed25519. O plugin NÃO rebaixa isso para "confia no GitHub": o doctor verifica o manifest assinado pós-install/update, mantendo a cadeia de proveniência que o ZIP tinha, somada a branch protection + access control da org. (Spec F)

**D5 — Paridade Codex por degradação graciosa.** Verificado: Codex suporta marketplace git + plugin install (hands-on 2026-10-06). Não verificado: hooks entregues POR plugin. Fallback já existente: projeção `.codex/hooks.json` project-level (trust-gated por hash, já testada no Maestro). O doctor detecta runtime e instala a projeção quando hooks-via-plugin não rodarem. (Spec B §Codex)

**D6 — Connector = mesmo binário.** `maestro-mcp` registrado no Claude Desktop via config local. Sem server remoto hospedado na v0.3.0 (OAuth/hosting = out of scope). (Spec D)

**D7 — Scheduler fica fora do plugin.** launchd (macOS) / Task Scheduler (Windows) continuam sendo enrollment consentido via `/maestro:setup-update`. Plugins não registram jobs de OS — correto que não o façam. (Spec C §scheduler)

## 5. Testing Decisions

Comportamento externo testável (sem paths internos):

1. **Conformance suite do doctor:** pós-install e pós-update, `/maestro:doctor` emite relatório PASS/FAIL contra o manifest da release — mesma máquina de verificação do `eval-release.sh` atual, reapontada.
2. **Teste de paridade por feature** (matriz): skills, agents, commands, hooks, SessionStart injection, MCP decl — executado em Claude Code E Codex, resultado versionado no repo (`acceptance/parity/`).
3. **Teste de update com estado:** instalar vN, popular `~/.maestro/` com fixture, atualizar para vN+1, verificar estado intacto + skills novas ativas. Automatizável em CI com fixture determinística.
4. **Teste de migração (Spec E):** instalação 0.1.11/0.2.0 sintética com `data/` co-locado → doctor migra → asserts de equivalência + marker + rollback.
5. **Canary (Spec 054):** telemetria anonimizada já existente passa a reportar `plugin_version` + `runtime_version` — responde "quem está em qual versão" sem coletar conteúdo.

## 6. UX do install/update — crítica pelas 6 lentes (`ux-critique`)

1. **Cognitive load:** de 6 passos com decisão humana → 2 comandos + 1 trust prompt. O trust prompt é o único momento de decisão; o texto do prompt é do harness (não controlamos) — mitigação: README do marketplace explica o que será aceito ANTES do install.
2. **Decision velocity:** update de ~10 min com medo → 1 comando sem medo. Adoption killer atual ("deixo pra depois, estou atrasado") eliminado.
3. **Trust signals:** versão visível via `claude plugin list`; doctor imprime versão + assinatura verificada + data da release. Hoje: nenhum sinal além do nome do ZIP.
4. **Acionabilidade no fluxo real:** update notice via SessionStart hook ("v0.3.1 disponível — rode `claude plugin update`") — no momento em que a pessoa já está no terminal, não num email perdido.
5. **Defensibilidade stakeholder:** security review ganha um artefato auditável (manifest assinado + provenance de repo) em vez de "ZIP que circulou por email".
6. **Platform-native fit:** plugin É o mecanismo nativo de distribuição do Claude Code — paramos de lutar contra a plataforma (installers, wizards) e usamos o trilho dela.

**Adoption killer residual (honesto):** auth GitHub do colega (EMU/SAML BCG) é o novo passo de fricção único. Tratamento: piloto com 1 colega antes de rollout (gate G2 do plano); se fricção for proibitiva, fallback = marketplace em repo público com conteúdo não-sensível auditado (decisão explícita, não default).

## 7. Out of Scope (v0.3.0)

- MCP server remoto hospedado com OAuth (connector claude.ai web/mobile) — v0.4+ se houver demanda.
- Mods JS/TS (wizard de onboarding como UI mod) — feature de ~1 semana de idade, confiança MEDIUM, watch com TTL 2026-11-09.
- Windows Task Scheduler parity — herda status pendente da 0.2.0, não é regressão desta proposta.
- Auto-update silencioso — update continua ação explícita do usuário (consistente com doutrina de consent do Maestro).
- Qualquer mudança de comportamento das skills/agents — esta proposta muda DISTRIBUIÇÃO, não produto.

## 8. Further Notes

- Pesquisa validada hands-on: `${CLAUDE_PLUGIN_ROOT}` oficial e usado por plugins Anthropic; SessionStart injection via plugin comprovada (`learning-output-style`); marketplace+install+update funcionando em Claude Code e Codex nesta máquina (mattpocock-skills, 2026-10-06).
- Estudo de origem: `kowalski-os/wiki/investigations/2026-10-09-agentic-os-brasil-plugin-mcp-study.md` (Walter pressure-test absorvido: matriz Codex, migration spec, trust model).
- A v0.2.0 precisa mergear primeiro — esta proposta NÃO deve basear em main 0.1.11 (specs 040/049 de native agents mudam o conteúdo do plugin base).
