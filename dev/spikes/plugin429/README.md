# Spike #429 — marketplace local

Ferramenta de desenvolvimento retida sob `dev/`, com incorporação autorizada
em `main` pelo PR #441. Continua fora dos pacotes de produto e o gate F0
permanece aberto até a qualificação de runtime. Constrói um
marketplace `maestro-spike` com uma skill, SessionStart, uma sonda de bloqueio
de escrita e uma sonda dos nomes `Agent`/`Task`. O helper `maestro-runtime`
existente executa somente `preflight`, sem migração ou escrita de memória.

## Construir no macOS

Execute na raiz deste worktree com Go instalado. Escolha um destino novo,
fisicamente sob `Developer`, com espaços no nome. O builder recusa um destino
existente e mantém a saída parcial se a compilação falhar.

```sh
export TMPDIR="$HOME/Developer/tmp"
export GOTMPDIR="$TMPDIR"
export GOCACHE="$HOME/Developer/ToolData/go-build"
mkdir -p "$TMPDIR" "$GOCACHE"
go test ./dev/spikes/plugin429 -v
go run ./dev/spikes/plugin429 build "$HOME/Developer/CodexWorkspace/Marketplace Spike"
```

Os dois executáveis nativos ficam em `versions/0.0.1/maestro-spike/scripts/`.
O pacote não usa Python, MCP, dados de cliente, memória nem configuração do
repo. O builder escolhe o formato nativo do host: executáveis `.exe` e hooks
em execução direta no Windows; executáveis sem extensão e o launcher existente
no Unix. Para compilar e ensaiar no Windows PowerShell 5.1/7, use
[WINDOWS.md](WINDOWS.md). A qualificação do host em #431 continua pendente.
O build nativo Windows inclui uma variante separada de comparação literal
Bash/PowerShell e suas wrappers; sua ativação/restauração no artefato de ensaio
está no mesmo guia. O manifesto padrão permanece com execução nativa direta.

## Ensaio assistido no Code tab

1. Valide o marketplace e o plugin com `claude plugin validate <caminho>`.
2. Registre `claude --version`, versão do aplicativo, sistema operacional e
   os caminhos ativos em `RESULTS.md`. Validação de manifest não comprova
   execução do Code tab.
3. Registre o marketplace local e instale:

   ```sh
   claude plugin marketplace add "<marketplace-path>"
   claude plugin install maestro-spike@maestro-spike
   ```

4. Abra uma sessão nova do Code tab numa pasta de teste, fora do plugin.
   Registre a linha `MAESTRO_SPIKE revision=0.0.1` do SessionStart e invoque
   `/maestro-spike:probe`. O retorno esperado é
   `MAESTRO_SPIKE_SKILL revision=0.0.1`.
5. Peça uma escrita por `Write` em `maestro-spike-denied.txt`. Capture a
   tentativa real, o bloqueio e a ausência do arquivo. Faça também um teste
   com arquivo inofensivo de outro nome para confirmar que pode escrever.
   Os testes unitários cobrem os quatro nomes de tool; este ensaio precisa
   observar quais tools o runtime realmente emite.
6. Peça uma delegação inofensiva. Registre o nome observado em
   `${CLAUDE_PLUGIN_DATA}/delegation.jsonl`; o log contém somente `tool_name`,
   sem prompt ou conteúdo. Ausência de log não significa ausência da tool.
7. Edite somente a revisão da skill para `0.0.2` na pasta publicada. Abra
   outra sessão, sem executar `plugin update`, e invoque a skill de novo.
   Registre a resposta, a raiz ativa do plugin e se a nova revisão apareceu.
8. Preencha cada linha de `RESULTS.md` com evidência e veredito. O gate F0
   continua pendente enquanto houver `NOT_RUN` ou resultado inconclusivo.

Para o teste real da pasta sincronizada, o operador deve primeiro disponibilizar
um caminho aprovado sob `Developer`. Este spike local não escreve em OneDrive
nem prova sincronização ou aprovação Gatekeeper. Não remova quarentena para
fazer o teste passar. Registre qualquer bloqueio nativo como resultado.

A sonda bloqueia o nome de arquivo dedicado e aliases por symlink. **Não é o
guard de isolamento entre casos** e não o substitui. Não rode o spike em
trabalho de cliente. Desinstale pelo fluxo normal de plugins depois do ensaio;
este plugin não usa `~/.maestro/`.

## Referência verificada

Em 2026-10-20, a [referência oficial de carregamento](https://code.claude.com/docs/en/plugins/loading#in-place-and-copied-plugins)
descreve que plugins com source relativo em marketplace adicionado por caminho
local carregam os arquivos da origem a cada sessão ou reload. Isso sustenta o
desenho do ensaio; a versão instalada ainda precisa demonstrar o comportamento.

O matcher e exit code 2 seguem a [referência oficial de hooks](https://code.claude.com/docs/en/hooks).
O [formato de marketplace](https://code.claude.com/docs/en/plugins/marketplace-reference)
define a origem relativa à raiz que contém `.claude-plugin/`.

## Repetir instalação CLI sem alterar o perfil real

Use um diretório novo de configuração sob `Developer/ToolData`, mantendo
`HOME` intacto:

```sh
export CLAUDE_CONFIG_DIR="$HOME/Developer/ToolData/maestro-spike-cli-test"
export TMPDIR="$HOME/Developer/tmp"
claude plugin marketplace add "<marketplace-path>"
claude plugin install maestro-spike@maestro-spike
claude plugin list --json
```

Este ciclo comprova registro e instalação CLI. Em 2.1.233 o teste encontrou
uma cópia em cache e a mudança da skill na origem não alterou essa cópia.
A sessão real ainda precisa mostrar qual árvore carrega; consulte `RESULTS.md`.
