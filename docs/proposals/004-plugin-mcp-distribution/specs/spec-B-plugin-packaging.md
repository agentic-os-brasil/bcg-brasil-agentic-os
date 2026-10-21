# Spec B — Dois plugins, helper CLI e nove hooks

**DRAFT / F1 BLOCKED.** Issues #435 (bootstrap), #436 (build), #431 (Windows).
Fontes: `bundles/base/manifest.json`, template ZIP e settings Mac/Windows.

| Plugin | Conteúdo da baseline |
|---|---|
| `maestro` | 53 skills base, sete agentes, templates referenciados, recursos/políticas, nove hooks e helper CLI |
| `maestro-tech-core` | 12 skills técnicas; base verificada como pré-requisito pelo doctor |

`pm-core` não existe em main e não integra este release. O builder reutiliza
cópia/hashing da fábrica ZIP por allowlist: nenhum harness, Git, credencial,
configuração local ou estado de usuário entra no produto.

Layout proposto: `.claude-plugin/plugin.json`, `skills/`, `agents/`,
`hooks/hooks.json`, recursos referenciados e `scripts/`. O helper
`maestro-runtime`/`.exe` e handlers ficam em `${CLAUDE_PLUGIN_ROOT}/scripts/`;
targets darwin-arm64, darwin-amd64 e windows-amd64 são selecionados explicitamente.
Sem `bin/` de topo nem `.mcp.json` para um servidor Maestro.

## Nove handlers preservados

Cada nome abaixo tem `.sh` e `.ps1`; preservar stdin, saída, exit codes e
semântica de erro existentes. `Stop` é o evento atual, não `SessionEnd`.

| Evento Claude | Handler | Criticidade |
|---|---|---|
| SessionStart | first-run-scaffold | required |
| SessionStart | session-start-memory-inject | required |
| UserPromptSubmit | context-inject-userprompt | recommended |
| PreToolUse | block-cross-case-writes | required |
| PreToolUse | announce-agent-dispatch | recommended |
| Stop | session-stop-dream | recommended |
| Stop | session-stop-agent-check | recommended |
| Stop | session-stop-brain-index | recommended |
| Stop | session-stop-eod-check | recommended |

Guard mantém **`^(Edit|MultiEdit|Write|NotebookEdit)$` e `required`**;
delegação mantém `^(Agent|Task)$` até observar nome real. Ausência de hook
crítico não é conformidade degradada silenciosa. O marker do spike bloqueia
somente um nome de arquivo: não substitui o guard entre casos do produto.

## Padrão Windows — NOT QUALIFIED, aguardando #431

O draft de #431 usa manifests gerados por plataforma, sem campo per-OS
inventado. Mac mantém comando Bash com path `${CLAUDE_PLUGIN_ROOT}` entre
aspas. Para handlers PowerShell, candidato exec-form é `command: powershell.exe`
com vetor `args: [-NoProfile, -File, ${CLAUDE_PLUGIN_ROOT}/scripts/<handler>.ps1]`.
Cada path ocupa um argumento; não inserir aspas literais no vetor.
O spike Go usa diretamente `.exe` e vetor de argumentos, sem wrapper.
Direcionar cada cliente ao manifesto correto na pasta compartilhada também
precisa ser fechado em #431/#436; não presumir seleção automática por OS.

Essa escolha ainda precisa provar Code tab, PowerShell 5.1/7, stdin/exit codes,
paths com espaços, política corporativa e processo não elevado em Windows.
Cross-compilation, shell PowerShell no Mac ou CI sintético não qualificam o host.
Nenhuma opção ExecutionPolicy Bypass ou remoção de proteção faz parte do padrão.
Se a versão suportada não aceitar exec-form, #431 deve registrar o resultado
e revisar o manifesto antes do freeze; este texto não finge solução validada.

## Bootstrap e aceite

Plugin não transporta `CLAUDE.md` como bootstrap. #435 mapeia cada instrução
do template para SessionStart curto, skill operador ou remoção justificada.
Não presumir que plugin settings entrega permissões ou env de sessão.

- [ ] Validar ambos plugins e marketplace; comparar inventário com a baseline.
- [ ] Orientação e primeiro check obrigatório observados em pasta arbitrária.
- [ ] Code tab Mac/Windows executa nove handlers, guard real nas quatro tools
  e escrita de controle permitida; nome real de delegação registrado.
- [ ] Paths de produto via plugin root, estado via [Spec A](spec-A-data-home-decoupling.md);
  sem paths absolutos de fábrica ou escrita no produto.
- [ ] `/maestro:doctor` e demais comandos namespaced alcançáveis; onboarding
  documenta nome antigo do ZIP. Nova paridade Codex fica fora da v0.3.0.
