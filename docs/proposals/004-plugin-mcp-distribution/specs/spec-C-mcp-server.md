# Spec C — `maestro-mcp`: runtime Go empacotado como MCP server

**Status:** DRAFT — proposal 004 · **Decisões implementadas:** D3 (Go permanece, empacotado como MCP), D7 (scheduler fora do plugin) · **Depende de:** Spec A (`~/.maestro/`); paralela à Spec B (declaração no `.mcp.json` do plugin base) · **Referências:** [PRD.md §4](../PRD.md), [user-stories.md](../user-stories.md) (US-C2, US-S2), [migration-plan.md P3/G3](../migration-plan.md)

---

## Problem

O achado central do mapeamento v0.2.0: **o Go não foi removido do Maestro — ficou invisível.** O CLI `bcgos` saiu (PR #349), mas 70+ pacotes Go continuam vivos como substrate sob os slash-commands: agent state, session hooks, configuração de adapters, scheduling, síntese de memória/dreaming, adapters macOS/Windows, canary, verificação de release. (Lista de pacotes conforme mapeamento v0.2.0; inventário file-a-file de `internal/` não re-verificado nesta spec — ver QC-5.)

Hoje esse runtime chega ao usuário como código-fonte dentro do ZIP, compilado/invocado localmente pelos hooks e skills. Problemas:

1. **Toolchain implícita:** o colega precisa de um ambiente capaz de executar o runtime — classe de bugs "funciona na máquina do maintainer".
2. **Dessincronia conteúdo↔runtime:** skills de uma versão invocando runtime de outra é um estado alcançável e já observado na família de bugs 0.1.10–11.
3. **Sem superfície fora do harness:** o runtime só é alcançável de dentro de uma sessão Claude Code com a árvore instalada.

A migração MCP é **empacotamento, não extração**: o código já existe e já tem papel de library (`go.mod`: módulo `github.com/agentic-os-brasil/bcg-brasil-agentic-os`, Go 1.26). O trabalho é dar a ele um binário, um contrato MCP e um canal de distribuição verificado.

## Solution

Um binário único `maestro-mcp` (MCP server via stdio), cross-compilado para `darwin-arm64`, `darwin-amd64` e `windows-amd64`, declarado no `.mcp.json` do plugin `maestro` (Spec B), instalado em `~/.maestro/bin/` por um bootstrap fail-closed com verificação criptográfica pinada no plugin. Versão do runtime travada à versão do plugin; `/maestro:doctor` verifica a igualdade.

O mesmo binário é a superfície do connector no Claude Desktop (Spec D) — zero lógica duplicada.

## Design

### 1. Grupos de capacidade → tools MCP

Agrupamento proposto (a lista exata de tools é decisão de design-time no freeze — QC-1):

| Grupo | Pacotes de origem (mapeamento v0.2.0) | Exemplos de tools |
|---|---|---|
| **Agent state** | `agentstate`, `sessionhook` | ler/atualizar estado de agents governados, checkpoints de execução, ledger de continuidade |
| **Memória & dreaming** | síntese de memória/dreaming | consolidação de memória (dream), rollups, query de memória por escopo |
| **Prior-work & catálogo** | catálogo prior-work (SharePoint local), `bundles/catalog/` (ver QB-1 da Spec B) | query explicável do catálogo local de trabalho anterior |
| **Canary & versão** | `canary`, `releaseverify` | report de telemetria tipada (plugin_version + runtime_version, Spec 054), verificação de manifest de release |
| **Config de adapters** | `adaptercfg`, adapters macos/windows | inspeção de capacidade por plataforma (read-only via MCP) |

Fora da superfície MCP (deliberado):
- **Scheduler/scheduling (D7):** o runtime contém a lógica, mas o REGISTRO de jobs de OS não é tool MCP — ver §4.
- Qualquer tool que escreva fora de `~/.maestro/` (invariante herdada do manifest do bundle).

Todas as tools operam sobre `~/.maestro/` como único estado persistente. Nenhuma tool lê ou escreve no diretório do plugin.

### 2. Distribuição do binário + bootstrap verificado (US-S2)

**Default: binários shipados DENTRO do plugin** (os 3 targets em `${CLAUDE_PLUGIN_ROOT}/bin/`), assinados no pipeline (codesign + notarytool em macOS; Authenticode em Windows — Spec F). Racional (Walter pressure-test): binário Go não notarizado baixado em runtime numa máquina macOS gerenciada pela BCG cai em quarentena do Gatekeeper quase certamente, e um bootstrap que contorna quarentena é exatamente o que reprova em security review. No modo embedded, o "bootstrap" apenas verifica (sha256+Ed25519 pinados) e copia atomicamente o target da plataforma para `~/.maestro/bin/` — sem rede.

**Fallback (size-driven, gated):** download-on-first-run só entra se o tamanho medido do plugin com 3 targets embutidos for proibitivo para o harness/marketplace, E com sign-off explícito do security review (decisão documentada, nunca default silencioso). O fluxo abaixo descreve o fallback; no modo embedded, os passos 3 são substituídos por cópia local do `${CLAUDE_PLUGIN_ROOT}/bin/`, e os passos 4–6 aplicam-se igualmente.

O `.mcp.json` do plugin base aponta para um script de bootstrap, não direto para o binário:

```json
{
  "mcpServers": {
    "maestro": {
      "command": "${CLAUDE_PLUGIN_ROOT}/bin/maestro-mcp-bootstrap.sh",
      "args": []
    }
  }
}
```

Fluxo do bootstrap (primeira execução ou pós-update):

1. Detecta plataforma (`darwin-arm64` / `darwin-amd64` / `windows-amd64`; Windows usa variante `.ps1`/`.cmd` — QC-6).
2. Se `~/.maestro/bin/maestro-mcp` já existe **e** `maestro-mcp --version` == versão pinada no plugin → exec direto (caminho quente, sem rede).
3. Senão: download do binário da release correspondente à versão do plugin (canal: GitHub Releases do repo marketplace — interage com Q1).
4. **Verificação fail-closed:** sha256 do artefato comparado ao sha256 pinado em `${CLAUDE_PLUGIN_ROOT}/pinned/checksums.json` **e** assinatura Ed25519 do manifest de release (Spec 020, F6) verificada com a chave pública pinada em `${CLAUDE_PLUGIN_ROOT}/pinned/release-signing.pub`. **Chave e checksums viajam DENTRO do plugin — nunca são baixados do mesmo canal do binário.** Comprometer o canal de download não basta; seria preciso comprometer também o canal do plugin (marketplace com branch protection + access control, Spec F).
5. Verificação OK → instala atomicamente em `~/.maestro/bin/` (write em temp + rename) → exec.
6. Verificação FALHA → binário descartado, **nunca executado**, erro claro no output + evento tipado pro canary. Não existe flag de override.

### 3. Lockstep de versão

- A release (Spec F) publica plugin + binários com a MESMA versão; os sha256 pinados no plugin são gerados no mesmo pipeline.
- `maestro-mcp --version` responde a versão embutida em build time (ldflags).
- `/maestro:doctor` verifica `plugin.version == runtime.version`; desigualdade = FAIL com fix de 1 comando (re-run do bootstrap via update).
- Update de plugin ⇒ sha pinado muda ⇒ passo 2 do bootstrap falha a igualdade ⇒ re-download verificado da versão nova. Update do binário é, portanto, consequência automática e atômica do update do plugin — sem passo manual (US-C2).

### 4. Scheduler fica fora do plugin (D7)

launchd (macOS) e Task Scheduler (Windows — herda status pendente da 0.2.0) continuam sendo **enrollment consentido** via `/maestro:setup-update`, exatamente como hoje (presence pulse). Razões:

1. Plugins não devem registrar jobs de OS — update/uninstall de plugin não tem hook confiável para desfazer efeitos no OS; jobs órfãos seriam a nova classe de bug.
2. Consent: registrar um job persistente na máquina do colega é decisão explícita do colega, não efeito colateral de `plugin install` (doutrina de consent do Maestro, PRD §7).

O job registrado pelo setup-update invoca `~/.maestro/bin/maestro-mcp` (subcomando de manutenção, não o modo server) — o binário é o mesmo; apenas o REGISTRO permanece fora do plugin. `/maestro:doctor` reporta o estado do enrollment (ativo/inativo/desatualizado) sem alterá-lo.

### 5. Failure modes (contrato)

| Falha | Comportamento |
|---|---|
| Download falha (offline, proxy BCG, repo inacessível) | Degradação: conteúdo do plugin (skills/agents/commands) funciona normalmente; tools MCP indisponíveis com mensagem clara ("Maestro runtime não instalado — rode /maestro:doctor quando online"). Nunca bloqueia a sessão. |
| sha256 ou assinatura não confere | **Hard fail:** binário descartado, nunca executado; erro acionável + evento canary; doctor FAIL com instrução de reportar ao maintainer. |
| Plataforma não suportada (ex.: linux, win-arm64) | Mensagem explícita de plataforma não suportada + matriz de targets; conteúdo segue funcionando; sem tentativa de download. |
| Binário corrompido pós-install (não executa) | Doctor detecta (exec de `--version` falha) → re-bootstrap verificado. |
| Dessincronia versão plugin ≠ runtime | Doctor FAIL (estado distinto no relatório, US-D2) + fix de 1 comando. |

## Acceptance Criteria

1. **Build:** pipeline produz os 3 targets a partir do mesmo commit; falha de 1 target aborta a release inteira (US-D1).
2. **Bootstrap feliz:** instalação limpa do plugin → primeira invocação de tool MCP dispara bootstrap → binário verificado em `~/.maestro/bin/` → tool responde. Observável em Claude Code (gate G3).
3. **Fail-closed (US-S2):** fixture com binário adulterado (1 byte) → bootstrap recusa, binário não executa, erro claro. Fixture com assinatura de chave errada → idem. Teste automatizado em CI.
4. **Lockstep:** pós `claude plugin update`, `maestro-mcp --version` == versão do plugin sem ação manual; doctor PASS. Update interrompido entre plugin e binário → doctor FAIL com dessincronia nomeada.
5. **Offline:** máquina sem rede com plugin recém-instalado → sessão abre, skills funcionam, tool MCP retorna mensagem de indisponibilidade (não timeout silencioso).
6. **D7:** `plugin install` + onboarding NÃO criam nenhum job launchd/Task Scheduler (verificável por inspeção de `launchctl list` / `schtasks`); job só existe após consent explícito via `/maestro:setup-update`; uninstall do plugin não deixa job órfão sem aviso do doctor.
7. **Estado:** nenhuma tool MCP escreve fora de `~/.maestro/` (teste de sandbox/trace no CI de release).

## Out of Scope

- Server MCP remoto hospedado/OAuth — v0.4+ (PRD §7).
- Registro no Claude Desktop (mesmo binário) — Spec D.
- Pipeline de release, assinatura do manifest e canal de revogação — Spec F (esta spec consome os artefatos: chave pinada, checksums, manifest Spec 020).
- Windows Task Scheduler parity — herda pendência da 0.2.0, não é regressão desta proposta.
- Mudança de comportamento dos pacotes Go — empacotamento, não refactor.

## Open Questions

Herdadas: Q1 (repo de release define o canal de download do binário), A4 (declaração MCP via plugin no Codex — fallback `mcp_servers` config-level, Spec B camada 2).

Novas desta spec:
- **QC-1 — lista exata de tools:** o agrupamento §1 é proposta; a enumeração final de tools (nomes, schemas de input/output) é decisão de design-time no freeze, derivada dos contratos existentes em `schemas/`. Dona: Daniel + security review.
- **QC-2 — nome do server:** PRD §2 usa `maestro-runtime` na tabela de primitivas; esta spec usa `maestro-mcp` (binário). Unificar nomenclatura (binário vs nome do server no `.mcp.json`) antes do freeze.
- **QC-3 — endpoint security BCG:** RESOLVIDA em parte pelo §2 (Walter pass): codesign+notarytool macOS e Authenticode Windows entram como etapas DEFAULT do pipeline Spec F. Resta validar com security review se há exigência adicional (MDM allowlist, etc.).
- **QC-4 — tamanho do plugin com binários embutidos:** medir tamanho real dos 3 targets; se proibitivo, o fallback de download (§2) requer funcionar atrás do proxy BCG + sign-off do security review. Default permanece embedded até evidência contrária.
- **QC-5 — inventário `internal/`:** a lista de 70+ pacotes vem do mapeamento v0.2.0; re-verificar o inventário contra a branch de baseline no início da implementação (pacotes citados: agentstate, sessionhook, adaptercfg, scheduling/scheduler, memória/dreaming, adapters macos/windows, canary, releaseverify).
- **QC-6 — bootstrap em Windows:** `.mcp.json` com `command` shell script não roda nativo em Windows sem bash; definir variante (binário bootstrap mínimo? PowerShell?) na implementação.
