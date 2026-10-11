# User Stories — Maestro Distribution v0.3.0

**Formato:** Pocock `to-tickets` — stories por persona, acceptance criteria observáveis, task graph com dependências.
**Personas:** Carla (consultora não-técnica), Daniel (maintainer), Rafael (pilot técnico/Codex), Security Review BCG.

---

## 1. Stories — Carla (consultora, não-técnica)

### US-C1 — Instalar o Maestro sem ritual
**Como** consultora sem background técnico, **quero** instalar o Maestro com comandos que posso copiar/colar de um README, **para** começar a usar sem pedir ajuda ao maintainer.

**Acceptance criteria:**
- [ ] Instalação completa = `claude plugin marketplace add` + `claude plugin install` + abrir Claude Code + aceitar trust prompt + `/maestro:onboarding`. Zero passos além desses.
- [ ] Nenhum passo exige editar arquivo, mover pasta ou conhecer um path.
- [ ] `/maestro:doctor` pós-install reporta PASS em máquina limpa (macOS arm64/x64, Windows x64).
- [ ] Failure mode observável: se o GitHub auth falhar, a mensagem de erro do marketplace add é a do harness — README documenta o erro esperado e o fix (auth EMU/SAML).

### US-C2 — Atualizar com um comando
**Como** consultora, **quero** atualizar o Maestro com um único comando, **para** nunca mais adiar update por medo de quebrar minha memória.

**Acceptance criteria:**
- [ ] `claude plugin update` atualiza skills/agents/commands/hooks e o binário MCP (versão travada).
- [ ] `~/.maestro/` (memória, perfil, workspaces) permanece byte-idêntico após update (teste 3 do PRD §5).
- [ ] `/maestro:doctor` pós-update reporta PASS + versão nova + assinatura verificada.
- [ ] Failure mode: update interrompido no meio → doctor detecta dessincronia plugin↔runtime e instrui re-run (nunca estado corrompido silencioso).

### US-C3 — Saber que há update disponível no momento certo
**Como** consultora, **quero** ser avisada de versão nova quando já estou no terminal, **para** agir na hora em vez de perder o email.

**Acceptance criteria:**
- [ ] SessionStart hook compara versão local vs manifest remoto (budget: ≤1 check por dia, cache local, falha silenciosa offline).
- [ ] Notice de 1 linha: "Maestro v0.3.1 disponível — rode `claude plugin update`".
- [ ] Nunca bloqueia a sessão; nunca auto-atualiza (doutrina de consent).

### US-C4 — Migrar da instalação ZIP sem perder nada
**Como** consultora com Maestro 0.1.x/0.2.0 instalado por ZIP, **quero** que a migração para plugin preserve minha memória e perfil, **para** não recomeçar do zero.

**Acceptance criteria:**
- [ ] Doctor (shipado no último release ZIP) detecta `data/` co-locado → move para `~/.maestro/` → grava marker. Idempotente: segunda execução é no-op.
- [ ] Rollback documentado e testado (teste 4 do PRD §5).
- [ ] Pós-migração + pós-install do plugin: memória, workspaces e perfil acessíveis identicamente.

## 2. Stories — Daniel (maintainer)

### US-D1 — Publicar release com um comando
**Como** maintainer, **quero** publicar uma release com um comando/pipeline, **para** eliminar build local + envio por email + suporte individual.

**Acceptance criteria:**
- [ ] Pipeline: tag → cross-compile `maestro-mcp` (3 targets) → gerar manifest Ed25519 → atualizar marketplace.json → publicar. Um trigger.
- [ ] Nenhum passo manual entre tag e release disponível para `claude plugin update`.
- [ ] Failure mode: build de 1 target falha → release inteira aborta (nunca release parcial por plataforma).

### US-D2 — Saber quem está em qual versão
**Como** maintainer, **quero** telemetria de `plugin_version` + `runtime_version` por instalação, **para** suportar e revogar com precisão.

**Acceptance criteria:**
- [ ] Canary (Spec 054) reporta ambas as versões, anonimizado, sem conteúdo de usuário.
- [ ] Dessincronia plugin↔runtime aparece como estado distinto no relatório.

### US-D3 — Revogar uma release ruim
**Como** maintainer, **quero** um canal de revogação, **para** que uma release com defeito pare de ser instalada e usuários sejam avisados.

**Acceptance criteria:**
- [ ] Remover/pinar versão no marketplace.json impede novos installs da versão revogada.
- [ ] Doctor + SessionStart notice sinalizam versão revogada para quem já instalou.

## 3. Stories — Rafael (pilot técnico, Codex)

### US-R1 — Mesma instalação no Codex
**Como** early adopter que usa Codex, **quero** instalar os mesmos plugins via marketplace, **para** manter o promise "Claude-first · Codex-compatible".

**Acceptance criteria:**
- [ ] `marketplace add` + `install` + skills funcionando no Codex (já verificado 2026-10-06).
- [ ] Matriz de paridade por feature versionada em `acceptance/parity/` com status ✅/❌/degraded por harness.

### US-R2 — Degradação graciosa de hooks no Codex
**Como** usuário Codex, **quero** que hooks funcionem mesmo se o Codex não executar hooks-via-plugin, **para** não perder memory-inject/guards.

**Acceptance criteria:**
- [ ] Doctor detecta runtime Codex + hooks-via-plugin inativos → instala projeção `.codex/hooks.json` project-level (trust-gated por hash, mecanismo existente).
- [ ] Teste de 30min (plugin com SessionStart trivial no Codex) executado ANTES do design freeze da Spec B; resultado versionado.

### US-R3 — Reportar fricção via canary
**Como** pilot, **quero** que minha experiência de install/update alimente o gate de rollout, **para** que o time decida com evidência.

**Acceptance criteria:**
- [ ] Gate G2 do migration-plan: ≥1 colega completa install + 1 ciclo de update sem escalação ao maintainer.
- [ ] Fricção de auth GitHub (EMU/SAML) documentada como resultado do piloto, com decisão explícita sobre fallback.

## 4. Stories — Security Review BCG

### US-S1 — Auditar proveniência do que executa
**Como** reviewer, **quero** cadeia verificável da release ao código executando na máquina do colega, **para** aprovar a distribuição.

**Acceptance criteria:**
- [ ] Manifest Ed25519 (Spec 020) cobre: conteúdo do plugin, binário MCP (sha256 por target), versão.
- [ ] Doctor verifica assinatura pós-install/update e reporta no output.
- [ ] Hooks executáveis documentados no escopo do review desde a fase plugin (não só binários Go).
- [ ] Branch protection + access control da org documentados como parte do trust model.

### US-S2 — Binário Go com bootstrap verificado
**Como** reviewer, **quero** que o download/instalação do binário `maestro-mcp` seja verificado criptograficamente, **para** eliminar supply-chain no bootstrap.

**Acceptance criteria:**
- [ ] sha256 esperado + assinatura Ed25519 pinados DENTRO do plugin (não baixados do mesmo canal do binário).
- [ ] Verificação falha → binário descartado + erro claro; nunca executa binário não verificado.

---

## 5. Task Graph

```
T0  Merge v0.2.0 → main                              [pré-requisito duro — ação Daniel]
T1  Spec A: data home decoupling (~/.maestro/)        [depende: T0]
T2  Spec E: doctor migração (ship no ÚLTIMO ZIP)      [depende: T1]
T3  Teste Codex: SessionStart hook via plugin (~30m)  [independente; antes do freeze Spec B]
T4  Spec B: plugin packaging (3 plugins)              [depende: T1, T3]
T5  Spec F: release pipeline + marketplace + Ed25519  [depende: T4]
T6  Spec C: maestro-mcp (empacotamento Go + bootstrap)[depende: T1; paralelo a T4]
T7  Spec D: connector Claude Desktop                  [depende: T6]
T8  Gate G1: release ZIP final com migração (T2)      [depende: T2]
T9  Gate G2: piloto 1 colega install+update           [depende: T5, T8]
T10 Rollout geral + telemetria de versão              [depende: T9]
```

| Ticket | Stories cobertas | Spec |
|---|---|---|
| T1 | US-C4 (pré) | A |
| T2 | US-C4 | E |
| T3 | US-R2 | B |
| T4 | US-C1, US-C2, US-C3, US-R1 | B |
| T5 | US-D1, US-D3, US-S1 | F |
| T6 | US-C2 (runtime), US-S2 | C |
| T7 | (connector) | D |
| T9 | US-R3, US-D2 | — |
