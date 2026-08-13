---
date: 2026-08-13
version: 0.1.5
scope: dist/Maestro — onboarding + scaffold + hooks
author: Darwin (automated audit via Maestro Doctor + manual analysis)
status: open
---

# Onboarding & Scaffold Health Audit — v0.1.5

## Contexto

Auditoria manual executada em `dist/Maestro` em sessão de desenvolvimento (2026-08-13).
Cobriu: fluxo completo de onboarding (trilha completa), hooks SessionStart/Stop/PreToolUse/UserPromptSubmit,
catálogo de skills e integridade do workspace `data/`.

---

## Problemas encontrados

### P1 — Memory layers ausentes em workspace existente

**Severidade:** Alta — funcionalidade quebrada  
**Componente:** `first-run-scaffold.sh`, `session-start-memory-inject.sh`, `dream-memory`

**Sintoma:** Diretórios `data/memory/recent/`, `weekly/`, `medium-term/`, `lifetime/`, `policies/` e o arquivo
`data/memory/.schema-version` não existem em workspaces onde `data/.initialized` foi criado fora do scaffold
(por exemplo, por cópia manual, restore de backup ou pré-população em desenvolvimento).

**Causa:** O scaffold executa `mkdir -p` para os tiers de memória apenas no branch first-run
(`if [ -f "$MARKER" ]; then ... exit 0; fi`). Quando `.initialized` já existe, o script chega ao
`exit 0` sem criar os tiers. O bloco de backfill (GAP-D) roda antes do `exit 0` e cria
`.schema-version` e `.gitignore` **se** `data/memory/` existir — mas não cria os subdirs ausentes.

**Impacto:**
- `session-start-memory-inject.sh` → `emit_latest_file` e `emit_all_files` não encontram arquivos; injeção de memória muda.
- `dream-memory` skill recusa escrita quando `.schema-version` está ausente (guard de migração).
- Toda a pirâmide de memória (L1/L2/L3) não funciona até intervenção manual.

**Fix sugerido:** No bloco de backfill (antes do `exit 0`), adicionar criação idempotente dos tiers:

```bash
for tier in recent weekly medium-term lifetime policies; do
  mkdir -p "$DATA_DIR/memory/$tier" 2>/dev/null
done
```

Isso torna o scaffold seguro para workspaces restaurados, copiados ou pre-populados em dev.

---

### P2 — Onboarding não preenche `owner/self/` facet files

**Severidade:** Média — gap silencioso de injeção de contexto  
**Componente:** `bundles/base/skills/maestro-onboarding/SKILL.md`, `session-start-memory-inject.sh`

**Sintoma:** Após onboarding completo (trilha completa, 10 facetas confirmadas), os arquivos
`data/owner/self/<faceta>.md` permanecem como placeholders ou vazios.
O hook de session-start injeta esses arquivos via `emit_all_files "SELF do usuário" "$DATA_DIR/owner/self"` —
se vazios, a injeção do contexto do owner fica muda na segunda sessão.

**Causa:** A skill de onboarding grava exclusivamente em `data/profile/identity.json` e `data/profile/style.json`.
Não há instrução na SKILL.md para atualizar os 10 arquivos de faceta em `owner/self/`.

**Impacto:**
- `identity.json` e `style.json` têm o perfil correto.
- `owner/self/` fica com placeholders — contexto do owner ausente na injeção de sessão.
- O usuário não percebe; o Maestro silenciosamente perde o contexto entre sessões.

**Fix sugerido — opção A (preferida):** adicionar ao fluxo de conclusão da SKILL.md a gravação de cada faceta confirmada no arquivo correspondente em `owner/self/`. Ex:

```
After all facets are confirmed, in addition to profile/identity.json and profile/style.json,
write each confirmed facet to data/owner/self/<facet-name>.md using the reviewed draft content.
```

**Fix sugerido — opção B:** alterar o `session-start-memory-inject.sh` para ler `profile/identity.json`
e `profile/style.json` como fallback quando `owner/self/` estiver vazio — menos limpo, cria acoplamento duplo.

---

### P3 — `data/` do dist contaminado com dados de desenvolvimento

**Severidade:** Alta — bloqueante para release  
**Componente:** pipeline de build / geração do ZIP

**Sintoma:** O `dist/Maestro/data/` pré-populado em desenvolvimento contém:
- `data/cases/.active` → `hdi-ai-for-sales-2026` (caso de cliente real)
- `data/cases/hdi-ai-for-sales-2026/` com `case-brief.md` e `plan-v1.md`
- `data/agents/hdi/account-context.md`
- `data/canary/` com receipts de sessão de desenvolvimento

**Impacto imediato:** o hook `block-cross-case-writes.sh` está ativo e bloqueia qualquer write
em case diferente de `hdi-ai-for-sales-2026`. Um usuário novo que criar o primeiro case será
bloqueado pelo PreToolUse hook sem mensagem de erro clara.

**Impacto de privacidade:** dados de contexto de cliente (HDI) presentes em dist de release.

**Fix sugerido:** o pipeline de build (`generate-portable-zip` / `release-export`) deve garantir
que `data/` no ZIP contenha apenas os arquivos gerados pelo scaffold limpo:
- `data/.initialized` (timestamp de build ou ausente — deixar o first-run gerar)
- `data/profile/` vazio (ou ausente)
- Nenhum `cases/`, `agents/`, `canary/` pré-populado

Alternativa: excluir `data/` inteiramente do ZIP e deixar o scaffold criar na primeira sessão.
Isso é mais seguro e alinhado com o contrato do `README-INSTALL.md` ("sua `data/` nunca é tocada pelo ZIP").

---

### P4 — `catalog.json` incompleto

**Severidade:** Baixa — impacto indireto  
**Componente:** `bundles/base/skills/catalog.json`

**Sintoma:** `catalog.json` lista 2 entradas explícitas enquanto existem 40+ diretórios de skill
em `bundles/base/skills/`. O rollup emitido pelo scaffold (awk sobre SKILL.md) funciona independentemente
do catálogo, mas consumidores que dependem de `catalog.json` (wayfinder, UI futura, automações)
recebem uma lista incompleta.

**Fix sugerido:** automatizar a geração de `catalog.json` a partir dos SKILL.md no pipeline de build,
ou adicionar ao `dev/harness/validate` uma checagem de paridade entre diretórios e entradas do catálogo.

---

## Hooks — avaliação

| Hook | Avaliação | Observações |
|---|---|---|
| `first-run-scaffold.sh` | ✅ Bem projetado | Fail-open, idempotente, backfill GAP-C/D; afetado por P1 e P3 |
| `session-start-memory-inject.sh` | ✅ Bem projetado | L3→L2→L1 + profile; dream-trigger; upgrade-trigger; afetado por P1 |
| `context-inject-userprompt.sh` | ✅ Bem projetado | Budget correto; anti context-rot explícito; session marker funcional |
| `block-cross-case-writes.sh` | ✅ Bem projetado | Escopo cirúrgico; fail-open; bootstrap path via `.pending`; bloqueado por P3 |
| `session-stop-dream.sh` | ✅ Correto | Minimal e correto — `.dream-requested` para trigger na sessão seguinte |

---

## Onboarding — avaliação da trilha completa

| Aspecto | Avaliação |
|---|---|
| Fluxo de perguntas (1 por vez) | ✅ Correto |
| Loop de reflexão + confirmação por faceta | ✅ Implementado conforme SKILL.md |
| Escrita de `profile/identity.json` | ✅ |
| Escrita de `profile/style.json` | ✅ |
| Escrita de `profile/onboarding.json` com `status: complete` | ✅ |
| Escrita de `owner/self/<faceta>.md` | ❌ Não implementado (P2) |
| Pergunta SharePoint pós-onboarding | ✅ Fluxo correto — dois estágios (seleção ≠ autorização de leitura) |
| Check MarkItDown pós-onboarding | ✅ |
| Convite para naming de agentes | ✅ |
| tech-core suggestion por role | ✅ Correto — não sugere automaticamente para roles não-técnicas |

---

## Próximos passos sugeridos (por prioridade)

1. **[P3 — imediato]** Limpar `data/` do dist antes do próximo build — risco de privacidade.
2. **[P1 — v0.1.6]** Adicionar criação idempotente dos memory tiers no bloco de backfill do scaffold.
3. **[P2 — v0.1.6]** Adicionar gravação de `owner/self/<faceta>.md` ao fluxo de conclusão da skill de onboarding.
4. **[P4 — backlog]** Automatizar geração de `catalog.json` no pipeline de build.
