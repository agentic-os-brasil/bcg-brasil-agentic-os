# Resultados — spike #429

Data: 2026-10-20. Baseline: `origin/main` em `f92e44520915b562697a2fc00c1fa07efedb07d2`.
Estado: ferramenta de desenvolvimento sob `dev/`, com incorporação em `main`
autorizada pelo PR #441; F0 aberto. Incorporar o código de desenvolvimento
não qualifica o runtime nem inclui o spike nos pacotes de produto. Nenhum
resultado local substitui uma sessão real do Code tab ou a execução do
Windows em #431.

## Perguntas do handoff

| # | Pergunta | Veredito | Evidência exigida |
|---|---|---|---|
| 1 | Instalação e hooks em pasta OneDrive com espaços funcionam? | NOT_RUN | Caminho sincronizado aprovado, dois comandos e log de hook no Code tab. Build local com espaços é somente ensaio. |
| 2 | PreToolUse do plugin bloqueia uma escrita real no Code tab? | NOT_RUN | Tentativa real com marker, negação e arquivo não criado. Testes do helper não comprovam enforcement do host. |
| 3 | O nome da tool de delegação é Agent ou Task? | NOT_RUN | Ocorrência real em `delegation.jsonl`; testes sintéticos cobrem ambos. |
| 4 | Mudança na origem aparece na sessão seguinte sem plugin update? | NOT_RUN | Retorno da skill nas revisões 0.0.1 e 0.0.2, duas sessões, raiz ativa e versão do host. Comportamento documentado, ainda não observado. |
| 5 | Binário executado da pasta OneDrive passa Gatekeeper? | NOT_RUN | Binário e caminho exatos, resultado nativo e log do SessionStart, sem remover quarentena. |

## Evidência local

| Check | Estado | Limite |
|---|---|---|
| Teste antes da implementação | FAIL esperado | `undefined: run`; contrato ainda ausente. |
| Testes do helper | PASS | JSON inválido/oversized, quatro tools, caminhos relativos, traversal lexical e symlinks; logging somente do nome da tool. |
| Build nativo macOS arm64 em pasta com espaços | PASS | Marketplace local gerado com os dois executáveis em `scripts/`; a pasta não é sincronizada. |
| Validação Claude de marketplace e plugin | PASS com warnings | CLI 2.1.233; marketplace sem descrição e plugin sem author. Esses avisos não impedem validação. |
| SessionStart direto fora do produto | PASS | Helper chamado de pasta externa: `MAESTRO_SPIKE revision=0.0.1`; preflight `non_elevated`, `native_qualification=unattested`. Não comprova carregamento pelo Code tab. |
| Full suite / full gate | NOT_RUN | Checks locais limitados ao spike, conforme orientação. |
| Windows / Code tab / OneDrive / Gatekeeper | NOT_RUN | Precisam de ambiente e observação próprios. |

Nenhuma instalação de plugin no perfil real, migração, publicação em pasta
compartilhada, envio a colegas ou escrita em `~/.maestro/` foi executada.

Verificação do artefato final: recompilação de `./dev/spikes/plugin429` após
todas as alterações produziu o mesmo SHA-256 do helper no marketplace:
`cadaf34f875a91f8d106577d8ea225c3e931f653ac176f89c0d5b1b16fda635e`.
O check confirma que o artefato contém o bloqueio de symlink pendente.

## Ciclo de instalação CLI isolado — 2.1.233

`CLAUDE_CONFIG_DIR` foi redirecionado para uma pasta nova em `Developer/ToolData`;
`TMPDIR` ficou sob `Developer/tmp`. O perfil real e `~/.maestro/` foram preservados.
Os comandos executados foram `claude plugin marketplace add <caminho-com-espaços>`,
`claude plugin install maestro-spike@maestro-spike` e `claude plugin list --json`.
Todos concluíram com exit code 0, sem chamada de modelo.

| Observação | Resultado | Limite |
|---|---|---|
| Registro do marketplace | PASS: source `directory`, installLocation igual à pasta local com espaços | Não testa sincronização OneDrive nem hooks. |
| Instalação do plugin | PASS: versão 0.0.1, scope user, enabled true no perfil isolado | Não é instalação no Code tab. |
| Arquivos instalados | O CLI registrou `plugins/cache/maestro-spike/maestro-spike/0.0.1`; realpath distinto da origem, sem symlinks nos componentes | Registro e arquivos em disco, sem observação do resolvedor de uma sessão. |
| Alteração temporária da skill na origem para 0.0.2 | A cópia instalada continuou 0.0.1, sem executar `plugin update`; a origem foi restaurada e conferida | Não comprova qual árvore uma sessão nova carrega. |

Há uma diferença a investigar entre a cópia observada no CLI 2.1.233 e a
referência oficial atual que descreve carregamento in-place. Não promover a
pergunta 4 a PASS com base na documentação. O teste obrigatório é abrir a
sessão real e observar a revisão carregada e `CLAUDE_PLUGIN_ROOT`.
Nenhum Code tab, hook despachado pelo host ou modelo foi executado neste ciclo.

A inspeção de `claude --help` em 2.1.233 não anunciou `--init-only`.
O ensaio opcional de inicialização sem modelo não foi executado: falta um
modo explicitamente suportado no CLI instalado para essa observação.

## Ajustes de review — PR #441

A incorporação autorizada mantém esta ferramenta sob `dev/`, excluída do
produto. A qualificação F0 continua aberta. Os testes usam diretório absoluto
do próprio host; os cenários de symlink ficam em subteste isolado e podem
ser pulados em Windows quando o runner não permite criá-los. Os testes sem
symlink continuam executando.

Validação focal após os ajustes:

- `go test ./dev/spikes/plugin429 -v`: PASS no macOS arm64; quatro testes e
  o subteste de symlinks executados, sem skips.
- `go vet ./dev/spikes/plugin429`: PASS.
- `GOOS=windows GOARCH=amd64 go test -c ./dev/spikes/plugin429`: PASS de
  compilação cruzada. O executável de teste não foi executado em Windows.

O helper e o builder não foram alterados nesta revisão; a evidência anterior
de build/manifest permanece vinculada ao mesmo artefato. Sem full suite local.

## Observação CLI posterior — 2026-10-20, sem modelo

Um ensaio posterior com CLI 2.1.233/macOS arm64, perfil isolado e ferramentas
desabilitadas observou `hook_started`/`hook_response` de SessionStart com exit
0 e preflight `non_elevated`/`unattested`. `init.plugins` selecionou a pasta
publicada da origem, apesar da cópia física registrada no cache. Uma segunda
inicialização releu um marker temporário acrescentado ao hook da origem, sem
`plugin update`; o arquivo foi restaurado byte a byte e o hash do cache
permaneceu igual. Isso comprova somente despacho de SessionStart e releitura
do arquivo de hook nesse CLI. Não comprova revisão da skill, troca de ponteiro
de marketplace, inferência, ferramentas, delegação nem Code tab.

A autenticação estava ausente e a execução de modelo terminou com
`authentication_failed`, custo zero. As perguntas assistidas acima continuam
`NOT_RUN`. Evidência externa local preservada em
`Developer/CodexWorkspace/2026-10-20/maestro-epic-428/runtime-observation/REPORT.md`.

## Preparação Windows — #431

O builder agora escolhe os dois nomes `.exe` no Windows, incluindo o runtime
chamado por SessionStart. O manifesto Windows usa execução direta com
`command`/`args`; o Unix mantém seu launcher anterior. O runner PowerShell
5.1/7 e o workflow manual são sondas diretas sintéticas, descritas em
[WINDOWS.md](WINDOWS.md). A definição de workflow não comprova execução hosted.

Teste antes da implementação: `TestPlatformHookLaunch` falhou por ausência das
funções de seleção da plataforma. Após a implementação, testes focais passaram
no macOS arm64, incluindo o subteste de symlinks sem skip e os contratos de
launch Windows/macOS/Linux. Nenhum resultado Windows nativo ou corporativo,
Code tab, OneDrive, SmartScreen/WDAC/AppLocker ou reload de skill foi observado.
F0 e #431 permanecem abertos.

Validação adicional deste patch: `go vet ./dev/spikes/plugin429`, compilação
cruzada do teste e dos dois helpers para Windows amd64 (PE32+), parser PowerShell
7 e fast harness passaram. A fixture Windows foi aceita por `claude plugin
validate` 2.1.233 com o warning de author já conhecido; esse comando valida
manifestos e não prova execução dos handlers.

Para verificar `args` na versão instalada, uma cópia isolada via `--plugin-dir`
usou o helper macOS nativo com `command` separado de `args: ["session-start"]`.
O CLI 2.1.233 emitiu `hook_started` e `hook_response`, exit 0 e o marker correto
do modo SessionStart; sem esse argumento o helper não executaria o modo.
A chamada de modelo terminou sem autenticação, custo zero, sem tools nem MCP.
O artefato registrado de outro ensaio foi preservado. Essa observação prova
despacho exec-form somente no macOS, nunca no Windows. Evidência:
`Developer/CodexWorkspace/2026-10-20/maestro-epic-428/windows/mac-exec-form-observation.json`.
