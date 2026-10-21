# Spike #430 — evidência de managed settings

Coletor de desenvolvimento, somente leitura, fora do produto. Emite JSON
compartilhável com a estrutura de cinco chaves de política e duas variáveis
permitidas. Não executa Claude, não altera configuração e não envia dados.

Execute da raiz do checkout, com Go disponível, e mantenha temporários,
cache e saída sob `Developer`:

```sh
export TMPDIR="$HOME/Developer/tmp"
export GOTMPDIR="$TMPDIR"
export GOCACHE="$HOME/Developer/ToolData/go-build"
mkdir -p "$TMPDIR" "$GOCACHE" "$HOME/Developer/CodexWorkspace/managed-settings"
go test ./dev/spikes/managedsettings430 -v
go run ./dev/spikes/managedsettings430 > "$HOME/Developer/CodexWorkspace/managed-settings/receipt.json"
```

O padrão lê somente o arquivo deste sistema:

| Sistema | Arquivo |
|---|---|
| macOS | `/Library/Application Support/ClaudeCode/managed-settings.json` |
| Windows | `C:\Program Files\ClaudeCode\managed-settings.json` |
| Linux/WSL | `/etc/claude-code/managed-settings.json` |

Para analisar um arquivo local autorizado ou fixture, acrescente
`--settings "<arquivo>"`. O coletor aceita somente arquivos regulares de
até 1 MiB; symlinks e dispositivos retornam `not_regular`. O JSON deve ser
um objeto válido; arquivo vazio retorna `invalid_json`. A saída omite o
caminho de entrada e nunca inclui erros brutos do sistema.

No Windows, o responsável pode compilar previamente
`GOOS=windows GOARCH=amd64 go build -o <destino-sob-Developer>/managedsettings430.exe ./dev/spikes/managedsettings430`.
Execute o binário pelo PowerShell com `& '<caminho-do-binário>'`. Compilação
cruzada não comprova execução no dispositivo Windows. Colaboradores não
precisam receber configurações completas nem instalar Go se houver binário
aprovado para o ensaio.

## Ler o receipt

As observações de arquivo distinguem `absent`, `null`, `boolean` com valor
explícito, `empty_array`, `nonempty_array` com contagem e `invalid_type`.
Arquivo ausente, ilegível, inválido ou grande demais deixa as chaves como
`unavailable`. Nenhum desses estados declara que a política permite plugins.
As listas têm conteúdo redigido e schema não validado: uma lista não vazia
exige conferir localmente a correspondência da origem autorizada.

Somente `DISABLE_AUTOUPDATER` e `FORCE_AUTOUPDATE_PLUGINS` são consultadas
em `env` do arquivo e no ambiente do processo coletor. Os valores não são
exportados: `1`/`true` viram `enabled`, `0`/`false` viram `disabled`, e valores
vazios ou desconhecidos ficam distintos. Esses nomes descrevem o valor da
variável; não declaram suporte ao plugin. Outras variáveis ficam
`uninspected`, inclusive qualquer token cujo nome contenha `PLUGIN`.
O processo coletor pode ter ambiente diferente do aplicativo Claude.

O receipt sempre deixa `effective_policy=uninspected`,
`standard_tier=unverified` e `shared_folder_verdict=pending_attended_evidence`.
Exit code 0 significa somente que o receipt foi emitido, inclusive quando
`file_state` é `missing` ou `unreadable`.

## Completar F0 com observação humana

O responsável solicita o ensaio a 2–3 colegas de tier padrão, cobrindo Mac e
Windows. Esta ferramenta não os contata. Cada participante confirma localmente
tier, versões e a linha `Setting sources` de `/status` no host real. Registre
somente rótulos de fontes, sem screenshots ou dumps completos. MDM, registry,
servidor, drop-ins, parent process e demais variáveis permanecem não inspecionados
pelo coletor; ausência de arquivo não prova ausência de política.

Para cada dispositivo, vincule um identificador opaco à evidência sanitizada
e confira no próprio host se a origem compartilhada é permitida e se os hooks
exigidos executam. Use a matriz em [RESULTS.md](RESULTS.md). Restrição impeditiva
confirmada dá `NO_GO` e encaminha à alternativa do epic; `GO` exige evidência
completa dos dispositivos e do ensaio de runtime. Sem isso, mantenha `PENDING`.
A coleta local não fecha a issue.

Referência oficial verificada em 2026-10-20:
[managed settings: locais, fontes e verificação](https://code.claude.com/docs/en/managed-settings).
O protocolo exige observação no host porque um arquivo é somente uma das fontes.
