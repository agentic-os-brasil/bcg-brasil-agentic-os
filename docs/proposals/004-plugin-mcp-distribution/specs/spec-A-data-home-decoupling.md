# Spec A — Data Home Decoupling (`~/.maestro/`)

**Status:** DRAFT — proposal 004 (pendente aprovação do owner)
**Implementa:** Decisão D1 do [PRD](../PRD.md) · Ticket T1 do [task graph](../user-stories.md#5-task-graph)
**Pré-condição de:** Specs B, C, D, E, F — nada downstream começa antes desta.
**Baseline:** v0.2.0 (branch `codex/release-0.2.0-consolidation`, merge pendente — Gate G0).

---

## 1. Problem

O estado do usuário (`data/`: memória, perfil, workspaces, owner, agents) coabita a árvore do produto, que é movida manualmente a cada update ZIP. Consequências estruturais, não incidentais:

- **25 PRs de fix nas v0.1.10–0.1.11** concentrados em três categorias — resolução de paths (Windows drive letters, symlinks, espaços, unicode), hooks apontando para arquivos inexistentes, onboarding divergindo do estado real (Fato F8 do migration-plan).
- O ritual de update exige copiar `data/` de volta "no lugar certo"; errar um passo = perda de memória ou quebra silenciosa.
- Plugins Claude Code são **imutáveis por construção** — o modelo de distribuição alvo (Spec B) é incompatível com estado mutável dentro do produto. Sem esta spec, a migração para plugin não existe.

## 2. Solution

Separar permanentemente produto e estado:

- **Produto** (skills, agents, commands, hooks, schemas, binários): imutável, versionado, resolvido via `${CLAUDE_PLUGIN_ROOT}` na fase plugin.
- **Estado do usuário**: vive em `~/.maestro/`, fora de qualquer árvore de produto, e **nunca é tocado por update**.
- Hooks e commands resolvem o data home por uma ordem de resolução determinística (env var → marker → default), **nunca por path relativo ao projeto**.

Resultado observável para a Carla (US-C2, US-C4): `claude plugin update` jamais toca memória/perfil; a classe inteira de bugs de path do ritual ZIP deixa de existir por construção.

## 3. Design

### 3.1 Layout de `~/.maestro/`

```
~/.maestro/
├── data/                      # estado do usuário (conteúdo migrado 1:1 do data/ co-locado)
│   ├── memory/                # memória / dreaming
│   ├── profile/               # perfil e interaction profile
│   ├── workspaces/            # metadata de workspaces
│   ├── owner/                 # owner context / onboarding registry
│   └── agents/                # estado de agents (agentstate)
├── bin/                       # binários de runtime (maestro-mcp — Spec C)
└── home.json                  # state marker do data home (ver §3.3)
```

Notas:
- A estrutura interna de `data/` preserva o layout existente da v0.2.0 — esta spec move a raiz, **não reorganiza o conteúdo** (equivalência byte a byte é critério do teste 4 do PRD §5).
- `bin/` é definido aqui como parte do layout, mas populado pela Spec C (bootstrap do `maestro-mcp`).

### 3.2 Equivalente Windows

- Default: `%USERPROFILE%\.maestro` (mesma semântica, mesmo layout).
- Toda resolução de path usa APIs portáveis do Go (`os.UserHomeDir` + `filepath.Join`) — nunca concatenação de strings com separador fixo. Esta é exatamente a categoria de bug (drive letters, separadores) que motivou parte dos 25 PRs de fix.

### 3.3 Ordem de resolução do data home

Todo componente que precisa do estado (hooks, commands, pacotes Go, doctor, onboarding) resolve na mesma ordem, implementada **uma vez** em um resolver compartilhado:

1. **`MAESTRO_HOME`** (env var) — override explícito. Casos: testes/CI com fixtures determinísticas (teste 3 do PRD §5), ambientes corporativos com home remapeado.
2. **Marker file** — janela de compatibilidade (ver §3.5): instalação ZIP migrada carrega, no local antigo do `data/`, um marker apontando para o novo home. Código legado que ainda resolve project-relative segue o ponteiro em vez de encontrar (ou recriar) um `data/` órfão.
3. **Default** — `~/.maestro/` (macOS/Linux) / `%USERPROFILE%\.maestro` (Windows). Se não existir, é criado idempotentemente no primeiro uso (fresh install nunca passa por migração).

Regras fail-closed:
- Resolver NUNCA retorna path relativo ao cwd ou à árvore do produto.
- Marker com destino inexistente ou ilegível → erro explícito com instrução de rodar `/maestro:doctor`; nunca fallback silencioso para criar `data/` novo (isso mascararia perda de estado — o anti-padrão exato do onboarding divergente, F8).

### 3.4 State markers

Dois markers com papéis distintos:

| Marker | Onde vive | Papel | Dono |
|---|---|---|---|
| `~/.maestro/home.json` | data home | Identidade do home: versão de schema, data de criação/migração, origem (`fresh` ou path da instalação ZIP de origem) | Spec A (escrita: doctor/onboarding) |
| Marker de migração no local antigo | raiz da instalação ZIP migrada | Ponteiro para o data home + hash do inventário migrado; sinaliza "data/ já migrado, não recriar" | Spec E (escrito pela migração; deletado no rollback) |

Ambos são JSON estrito com parsing fail-closed (campos desconhecidos rejeitados), consistente com a disciplina de parsing da Spec 020.

### 3.5 Janela de backward-compat

- A resolução via marker (passo 2) existe para **uma janela de convivência de 1 release** (alinhada ao migration-plan P4: convivência ZIP/plugin = 1 release, depois ZIP descontinuado).
- Pós-janela: passo 2 é removido do resolver; a ordem vira env var → default. Instalação não-migrada nesse ponto é detectada pelo doctor (Spec E) como estado FAIL com instrução de migração manual documentada.
- Nenhum código novo (fase plugin em diante) pode depender do passo 2.

### 3.6 O que fica na árvore do produto

**Nada user-mutável.** Critério operacional: qualquer arquivo que algum fluxo em runtime escreva, modifique ou precise preservar entre versões pertence ao data home. A árvore do produto (ZIP hoje, plugin amanhã) contém exclusivamente conteúdo reviewed e versionado — coerente com a distribution boundary da Spec 020 ("releases may never contain user preferences, memory, logs or local configuration").

### 3.7 Categorias de touchpoints de código

Inventário por categoria (enumeração exata arquivo a arquivo é tarefa de implementação sobre a v0.2.0 merged — ver Open Questions):

1. **Hooks** — `session_start`, `user_prompt_submit`, `pre_tool_use`, `session_end`: todo acesso a estado passa a usar o resolver §3.3. São o touchpoint mais crítico: rodam em toda sessão e eram a fonte da categoria "hooks apontando para arquivos inexistentes".
2. **Pacotes Go** — `agentstate` e demais pacotes do substrate (memória/dreaming, scheduler, adapters, canary — F5: 70+ pacotes vivos) que leem/escrevem estado: trocam resolução própria pelo resolver compartilhado. Um único ponto de resolução, não N reimplementações.
3. **Onboarding** — `/maestro:onboarding` inicializa owner/profile/workspaces diretamente em `~/.maestro/data/`; nunca cria `data/` co-locado, nem em fresh install.
4. **Doctor** — `/maestro:doctor` valida a cadeia de resolução (env/marker/default), reporta o home ativo e sua origem, e é o veículo da migração (Spec E).

## 4. Acceptance Criteria

Observáveis, incluindo failure modes:

- [ ] **AC-A1** Fresh install (sem `data/` legado): primeiro uso cria `~/.maestro/` com layout §3.1 + `home.json` com origem `fresh`. Nenhum `data/` surge na árvore do produto.
- [ ] **AC-A2** Com `MAESTRO_HOME` setado para diretório de fixture: todos os hooks e commands leem/escrevem exclusivamente lá (verificável no teste 3 do PRD §5 em CI).
- [ ] **AC-A3** Ordem de resolução respeitada: env var presente vence marker; marker presente vence default. Teste com os três cenários montados.
- [ ] **AC-A4** Teste de update com estado (teste 3, PRD §5): instalar vN, popular `~/.maestro/` com fixture, atualizar para vN+1 → `~/.maestro/` byte-idêntico + skills novas ativas.
- [ ] **AC-A5** Windows: AC-A1–A4 passam com `%USERPROFILE%\.maestro`, incluindo home em path com espaços e drive letter ≠ C:.
- [ ] **AC-A6** Nenhum fluxo (hook, command, onboarding, doctor, pacote Go) escreve dentro da árvore do produto — verificável com árvore de produto read-only durante a suite.
- [ ] **AC-A7** Failure mode — marker quebrado: marker aponta para destino inexistente → erro explícito instruindo `/maestro:doctor`; nenhum `data/` novo criado silenciosamente.
- [ ] **AC-A8** Failure mode — home parcialmente ausente: `~/.maestro/` existe mas subdiretório esperado falta → componente recria apenas estrutura vazia faltante quando seguro, ou reporta FAIL via doctor quando a ausência indica migração interrompida (coordenação com Spec E §failure modes).
- [ ] **AC-A9** Gate G1 (migration-plan): fixtures 0.1.11 e 0.2.0 migram com equivalência verificada — critério compartilhado com Spec E.

## 5. Out of Scope

- A migração em si (detecção, move, rollback) — Spec E.
- Bootstrap e verificação do binário em `~/.maestro/bin/` — Spec C.
- Empacotamento do produto como plugin e uso de `${CLAUDE_PLUGIN_ROOT}` — Spec B.
- Reorganização interna do conteúdo de `data/` — explicitamente não: move de raiz, layout preservado.
- Multi-profile / múltiplos data homes por máquina — fora da v0.3.0.

## 6. Open Questions

| # | Questão | Decide | Quando |
|---|---|---|---|
| QA-1 | Inventário exato dos pacotes Go que tocam `data/` (quais dos 70+ pacotes leem/escrevem estado) — precisa de varredura sobre a v0.2.0 **merged**, não sobre main 0.1.11 | implementação (verificação no repo) | Início de T1, pós-G0 |
| QA-2 | Nome e formato final do marker de migração no local antigo (coordenar com Spec E, que o escreve) | Daniel | Freeze Spec A+E |
| QA-3 | `~/.maestro/` fixo vs respeitar `XDG_DATA_HOME` no macOS/Linux — recomendação: fixo (menos superfície de bug de path, público não-técnico nunca customiza XDG) | Daniel | Freeze Spec A |
| QA-4 | `MAESTRO_HOME` deve ser documentado para usuário final ou permanecer interno (testes/CI)? Recomendação: interno — Carla nunca vê path nenhum (US-C1) | Daniel | Freeze Spec A |
