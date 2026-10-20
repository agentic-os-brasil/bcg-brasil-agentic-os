# Resultados — spike #429

Data: 2026-10-20. Baseline: `origin/main` em `f92e44520915b562697a2fc00c1fa07efedb07d2`.
Estado: implementação local descartável; F0 aberto. Nenhum resultado local
substitui uma sessão real do Code tab ou a execução do Windows em #431.

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
