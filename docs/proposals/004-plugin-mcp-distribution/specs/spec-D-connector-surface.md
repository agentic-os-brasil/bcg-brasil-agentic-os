# Spec D — Connector: `maestro-mcp` no Claude Desktop

**Status:** DRAFT — proposal 004 · **Decisão implementada:** D6 (connector = mesmo binário) · **Depende de:** Spec C (`maestro-mcp` instalado e verificado em `~/.maestro/bin/`) · **Referências:** [PRD.md §4](../PRD.md), [migration-plan.md P3/G3](../migration-plan.md)

---

## Problem

O Maestro só existe dentro do terminal (Claude Code/Codex). Colegas não-técnicos — exatamente o público-alvo da persona Carla — vivem no Claude Desktop. Hoje não há caminho para consultar memória, continuidade de execução ou prior-work fora de uma sessão de terminal, e qualquer caminho que duplicasse lógica criaria uma segunda implementação para manter em sincronia.

## Solution

Registrar o MESMO binário `maestro-mcp` (Spec C) no Claude Desktop via configuração local (`claude_desktop_config.json`). O Desktop fala com o mesmo server stdio, lendo/escrevendo o mesmo estado em `~/.maestro/`. Zero lógica duplicada: uma correção no runtime chega às duas superfícies no mesmo update.

## Design

### 1. Registro

Entrada em `claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`; Windows: `%APPDATA%\Claude\claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "maestro": {
      "command": "/Users/<user>/.maestro/bin/maestro-mcp",
      "args": ["serve"]
    }
  }
}
```

Nota: o Desktop não tem `${CLAUDE_PLUGIN_ROOT}` nem bootstrap — por isso o registro aponta para o binário JÁ instalado e verificado pelo bootstrap da Spec C em `~/.maestro/bin/`. O connector pressupõe uma instalação de plugin funcional; não existe caminho de instalação "só Desktop" na v0.3.0.

### 2. Setup consentido

Novo command no plugin base: `/maestro:setup-connector`.

1. Verifica pré-condições: binário presente em `~/.maestro/bin/`, versão == plugin, doctor PASS.
2. Mostra ao usuário o diff exato da config do Desktop (arquivo + entrada a adicionar).
3. Com consent explícito: escreve a entrada (merge preservando outros servers do usuário) ou, se o usuário preferir, apenas imprime o snippet para colar manualmente.
4. Instrui reiniciar o Claude Desktop e como verificar (tools `maestro` visíveis no ícone de conectores).

Consistente com a doutrina de consent: editar config de outro app nunca é efeito colateral de install/update.

### 3. O que funciona onde (expectativa honesta)

| Superfície | Claude Code (plugin) | Claude Desktop (connector) |
|---|---|---|
| Tools MCP (estado, memória, prior-work, canary) | ✅ | ✅ (mesmo binário) |
| Skills / commands (`/maestro:*`) | ✅ | ❌ — Desktop não carrega o plugin |
| Hooks (injeção de contexto, guards, dream no fim da sessão) | ✅ | ❌ — sem hooks no Desktop |
| Agents governados | ✅ | ❌ |

Consequências a comunicar no onboarding do connector:
- No Desktop, o Maestro é um CONSULTOR de estado + executor de operações tipadas via tools — não o OS completo. Sem injeção automática: o modelo só vê o estado quando chama uma tool.
- Operações que no Claude Code são disparadas por hook (ex.: dream no session_end) NÃO acontecem automaticamente no Desktop; continuam disponíveis como tool invocável.
- Setup, update e diagnóstico continuam acontecendo no Claude Code (`/maestro:doctor`, `claude plugin update`). O Desktop herda o binário atualizado automaticamente (mesmo path).

### 4. Estado compartilhado

Ambas as superfícies leem/escrevem `~/.maestro/`. É o mesmo contrato da Spec C (nenhuma escrita fora do data home). Sessões simultâneas Claude Code + Desktop tornam escrita concorrente possível pela primeira vez — ver QD-1.

## Acceptance Criteria

1. **Gate G3 (migration-plan):** na máquina do piloto, a mesma tool MCP responde em Claude Code E Claude Desktop; `runtime_version` idêntico nos dois.
2. **Setup consentido:** `/maestro:setup-connector` nunca escreve a config sem confirmação explícita; config existente do usuário (outros servers) é preservada byte a byte fora da entrada `maestro`.
3. **Continuidade observável:** estado escrito via tool numa superfície (ex.: checkpoint) é lido corretamente na outra, sem passo de sincronização.
4. **Update propagado:** após `claude plugin update` (+ re-bootstrap Spec C), o Desktop usa o binário novo no próximo restart — sem editar a config de novo.
5. **Failure mode — binário ausente/desatualizado:** `setup-connector` sem bootstrap concluído → recusa com instrução (rodar doctor), não escreve config apontando para binário inexistente. Desktop com binário removido depois do registro → erro de conexão do Desktop; doctor detecta e orienta re-bootstrap.
6. **Failure mode — verificação:** o Desktop executa o binário que o bootstrap da Spec C verificou; não existe caminho de instalação do binário iniciado pelo Desktop (nada a verificar fora do fluxo Spec C).

## Out of Scope

- **Server MCP remoto hospedado com OAuth (connector claude.ai web/mobile) — v0.4+, explicitamente fora (PRD §7).** Esta spec é local-only.
- UI própria no Desktop (painéis, mods) — fora; a superfície é exclusivamente tools MCP.
- Paridade Codex do connector — Codex consome MCP via `mcp_servers` (config-level); se desejado, é a camada 2 da Spec B, não esta spec.
- Qualquer alteração no contrato das tools — definidas na Spec C.

## Open Questions

- **QD-1 — concorrência em `~/.maestro/`:** duas superfícies vivas podem escrever o mesmo estado (ex.: dream + checkpoint simultâneos). O runtime precisa de lockfile/single-writer por escopo, ou o modelo de escrita atual (arquivos atômicos por operação) já é suficiente? Avaliar no design das tools (QC-1); até lá, documentar como limitação conhecida.
- **QD-2 — path com username no snippet:** `claude_desktop_config.json` não expande `~` de forma garantida em todas as versões; o setup-connector resolve o path absoluto na escrita. Confirmar comportamento por plataforma na implementação.
- **QD-3 — detecção de Desktop instalado:** o setup-connector deve detectar ausência do Claude Desktop e sair com mensagem clara (sem criar config órfã)? Recomendação: sim; confirmar no freeze.
