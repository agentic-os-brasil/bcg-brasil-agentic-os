# PRD — Maestro v0.3.0 por plugin

**DRAFT / F1 BLOCKED.** Autoridade de escopo:
[epic #428](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/428).
Evidência de runtime e sequência: [plano](migration-plan.md).

## Problema e resultado

A base de aproximadamente 40 colegas recebe ZIP por email. Produto e estado
coabitam a instalação; atualizar exige preservar a memória manualmente.
O objetivo é instalar uma vez no **Code tab do Claude desktop**, receber uma
versão publicada na próxima sessão e preservar o estado fora do produto.

O fluxo alvo usa um caminho previamente aprovado e disponível localmente:

```text
claude plugin marketplace add "<pasta-compartilhada-aprovada>/<target>"
claude plugin install maestro@maestro-releases
```

Depois, `/maestro:doctor` identifica versão, marketplace e origem do data home.
`<target>` é `darwin-arm64`, `darwin-amd64` ou `windows-amd64`, escolhido
antes da instalação conforme a [Spec B](specs/spec-B-plugin-packaging.md).
Esse fluxo direto ainda depende da decisão de admissão pré-execução descrita
na [Spec F](specs/spec-F-trust-release-pipeline.md); não é um fluxo qualificado.
Onboarding menciona `/maestro-doctor` como nome do ZIP e orienta para o novo
namespace; alias executável ainda exige implementação. Abertura, autenticação
e permissões normais do host permanecem requisitos. Os dois comandos não
garantem instalação pronta sem os gates de plataforma e migração.

## Baseline verificada

| Fato | Fonte em main |
|---|---|
| 0.2.0 incorporada em 2026-09-29 por #425; candidata em HOLD | Histórico Git; [evidência local](../../releases/0.2.0-local-evidence.md) |
| Campo 0.1.11; 0.1.12 candidata arquivada | [Consolidação](../../releases/0.2.0-consolidation.md), epic #428 |
| 53 skills base, 12 tech-core, sete agentes; pm-core ausente | `bundles/*/skills/*/SKILL.md`, `bundles/base/manifest.json` |
| Nove handlers com `.sh` e `.ps1` | Manifest base e settings dos dois sistemas no template ZIP |
| Guard obrigatório para quatro tools | Matcher e criticidade na [Spec B](specs/spec-B-plugin-packaging.md) |
| Motor Go de migração existente | `internal/zipmigration/`, `acceptance/migration/CONTRACT.md` |

## Decisões de escopo

1. `maestro` distribui base; `maestro-tech-core` distribui o bundle técnico
   opcional. São dois plugins, sem criação de bundle pm-core.
2. `maestro-runtime` permanece helper CLI nativo em `${CLAUDE_PLUGIN_ROOT}/scripts/`.
   Nenhum MCP server ou binário em `~/.maestro/bin/` integra este desenho.
3. Uma migração do `data/` do ZIP de campo vai diretamente ao data home
   `~/.maestro/`; não exige instalar versão intermediária com brain no produto.
   Conteúdo de casos continua no workspace autorizado; separar estado do dono
   não globaliza roots de trabalho nem reduz isolamento entre casos.
4. Marketplace `directory`, fontes relativas e pasta compartilhada somente
   leitura são o canal padrão. `DISABLE_AUTOUPDATER=1` não será contornado.
5. A próxima sessão deve carregar a publicação sem `plugin update`: é um
   critério a provar no Code tab, não garantia derivada do spike CLI.
6. Preservar nove hooks, guard e agentes; mover a orientação do `CLAUDE.md`
   para injeção curta em SessionStart e skill operador, conforme #435.

## Aceite do produto

- Mac e Windows limpos instalam e chegam a doctor PASS.
- Nove hooks executam no Code tab nos dois sistemas; guard bloqueia escrita
  fora do caso ativo pelas quatro tools declaradas.
- Publicar N+1 e abrir outra sessão carrega N+1, com snapshot de `~/.maestro/`
  byte-idêntico ao anterior durante o ensaio controlado de atualização.
- Uma instalação real 0.1.11 migra com equivalência e rollback testado.
- Dois colegas, um técnico e um não técnico, cobrem Mac e Windows e completam
  instalação e atualização sem ajuda; ZIP retirado após um release de convivência.

## Defaults pendentes de ratificação

| Decisão | Default do epic | Estado neste draft |
|---|---|---|
| Specs C/D na v0.3.0 | Retiradas | Propostas futuras preservadas |
| Pasta compartilhada | OneDrive do responsável no spike; SharePoint no rollout | Caminho concreto ainda não selecionado nem aprovado |
| Comando | `/maestro:doctor`, nome antigo mencionado no onboarding | Alias ainda não implementado |

Não há compromisso de data, telemetria de frota disponível, nova paridade
Codex, connector Desktop, Chat ou Cowork. Política restritiva de marketplace/hooks
aciona o plano B organizacional da [Spec F](specs/spec-F-trust-release-pipeline.md).
