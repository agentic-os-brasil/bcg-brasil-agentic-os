# Diagnóstico — hooks não aparecem ou não executam

Use este roteiro quando os arquivos do Maestro estiverem instalados, mas as
rotinas automáticas não deixarem evidência de execução. Faça o diagnóstico na
nova instalação e preserve `Maestro-old-{{FROM_VERSION}}` para rollback.

## 1. Separar presença, carregamento e execução

São três verificações diferentes:

1. **Configurado:** scripts e `.claude/settings.json` existem.
2. **Carregado:** `/hooks` mostra os seis handlers com origem no projeto ou,
   quando o comando não existe, o evento `init` do traço identifica a raiz nova
   e os handlers do projeto deixam eventos correlacionáveis.
3. **Executado:** o log de debug e os efeitos esperados mostram que o evento
   disparou.

Arquivos corretos e executáveis comprovam somente o primeiro item.

## 2. Iniciar a partir da raiz correta

Feche a sessão atual. No Mac, abra o Terminal:

```bash
cd "/caminho/para/Maestro"
claude doctor
claude --debug hooks
```

No Windows PowerShell:

```powershell
Set-Location "C:\caminho\para\Maestro"
claude doctor
claude --debug hooks
```

Não use apenas um diretório adicional para este teste. A raiz corrente deve ser
a pasta que contém `CLAUDE.md`, `VERSION` e `.claude/settings.json`.

Se o Claude pedir confiança no workspace, confira o caminho completo e aceite
somente a nova pasta `Maestro` extraída do ZIP cujo SHA-256 já foi validado. Um
pedido recusado, pendente ou referente a outra pasta é uma possível causa para
os hooks do projeto não serem carregados.

## 3. Conferir a configuração efetiva

Dentro da sessão, execute, se estes comandos existirem nesta versão:

```text
/status
/hooks
```

Se `/status` ou `/hooks` não aparecerem na lista de comandos, não os invente e
não classifique isso sozinho como falha. Gere uma sessão controlada com saída
`stream-json`, `--setting-sources project`, `--verbose` e
`--include-hook-events`, sempre a partir da raiz nova. Preserve o traço bruto
somente localmente e extraia apenas: `cwd` do evento `init`, nomes dos eventos,
matcher, exit code, origem de projeto e correlação do PreToolUse com a chamada
real de Yoda. SessionStart deve ter duas respostas bem sucedidas;
UserPromptSubmit, PreToolUse/Agent e Stop devem aparecer sem erro. Os seis
handlers continuam sendo contados em `.claude/settings.json`.

Registre sistema operacional, versão do Claude Code, diretório do projeto e,
sem incluir conteúdo pessoal, as fontes de configuração exibidas. Procure:

- erro de schema em `.claude/settings.json`;
- `.claude/settings.local.json` sobrepondo a configuração do pacote;
- `disableAllHooks`;
- política gerenciada `allowManagedHooksOnly` bloqueando hooks do projeto;
- ausência dos eventos SessionStart, UserPromptSubmit, PreToolUse ou Stop.

Não altere nem contorne uma política corporativa. Registre `UNAVAILABLE`, faça
rollback e envie o diagnóstico ao time BCG Brasil AI.

Não envie o log bruto de `claude --debug hooks`: ele pode conter prompts,
caminhos locais e outros dados do contexto. Registre e compartilhe somente o
evento, matcher, exit code e stderr sanitizado, removendo conteúdo pessoal ou
de cliente.

## 4. Distinguir runtime de despacho

Execute um hook diretamente somente numa extração limpa e descartável do ZIP da
plataforma. Não copie a instalação real: ela contém `data/` pessoal e de casos.

Mac:

```bash
PROBE_ROOT=$(mktemp -d)
ditto -x -k "/caminho/para/Maestro-v{{TO_VERSION}}-macos.zip" "$PROBE_ROOT"
(
  cd "$PROBE_ROOT/Maestro"
  CLAUDE_PROJECT_DIR="$PWD" bash .claude/hooks/first-run-scaffold.sh
)
```

Windows PowerShell:

```powershell
$probeRoot = Join-Path $env:TEMP ("Maestro-hook-probe-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $probeRoot | Out-Null
Expand-Archive -LiteralPath "C:\caminho\para\Maestro-v{{TO_VERSION}}-windows-powershell.zip" -DestinationPath $probeRoot
$hadProjectDir = Test-Path Env:CLAUDE_PROJECT_DIR
$previousProjectDir = $env:CLAUDE_PROJECT_DIR
Push-Location (Join-Path $probeRoot "Maestro")
try {
  $env:CLAUDE_PROJECT_DIR = (Get-Location).Path
  & .\.claude\hooks\first-run-scaffold.ps1
} finally {
  Pop-Location
  if ($hadProjectDir) {
    $env:CLAUDE_PROJECT_DIR = $previousProjectDir
  } else {
    Remove-Item Env:CLAUDE_PROJECT_DIR -ErrorAction SilentlyContinue
  }
}
```

Se a política normal do PowerShell impedir a execução, não use Bypass. Registre
`UNAVAILABLE` e encaminhe a evidência sanitizada ao time responsável.

- Se a chamada direta falhar, registre o erro como problema de runtime/script.
- Se funcionar, mas `/hooks` ou o traço não mostrarem carregamento, o problema é
  descoberta ou política do Claude Code.
- Se `/hooks` listar o handler, ou o `init` estiver correto, mas o evento não
  executar, use o log produzido
  por `claude --debug hooks` para registrar matcher, exit code e stderr.

## 5. Critério de saída

O update só recebe `PASS` quando os seis handlers são comprovados pela UI ou
pela combinação de settings e traço machine-readable, os eventos de início,
prompt, PreToolUse/Agent e Stop deixam evidência de execução, a proteção de
escrita está ativa e a chamada real ao Agent `yoda` retorna à sessão principal.

Caso contrário, mantenha `Maestro-old-{{FROM_VERSION}}`, classifique cada etapa
como `FAIL` ou `UNAVAILABLE` e não distribua esta instalação.
