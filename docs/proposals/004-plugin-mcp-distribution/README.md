# Proposal 004 — Distribuição por plugin em pasta compartilhada

**Status: baseline de preparação; qualificação F0/F1 pendente.** Alvo: v0.3.0.
Baseline atualizada em 2026-10-22: `origin/main` em
`db2d3036e241acb2913648d66415754fb058e282`. Este texto corrige a proposta;
não declara implementação, qualificação de runtime ou autorização de release.

O merge desta baseline permite implementar e testar os contratos de estado,
migração e empacotamento em ambiente isolado, com fixtures sintéticas. Essa
preparação pode avançar antes da conclusão de F0. F0 e a admissão independente
da Spec F continuam exigidos para freeze operacional, instalação de colegas,
migração real, distribuição e piloto. Resultados ainda não observados
permanecem pendentes; código preparado não os converte em PASS.

O [epic #428](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/428)
autoriza a execução condicionada: dois plugins no Code tab do Claude desktop,
marketplace local em pasta compartilhada somente leitura e estado em
`~/.maestro/`. O runtime continua sendo o helper CLI `maestro-runtime`.
MCP server, connector, Chat, Cowork e nova paridade Codex ficam fora da v0.3.0.

| Fonte | Quando ler |
|---|---|
| [PRD](PRD.md) | Escopo, baseline, resultado e decisões pendentes |
| [Plano](migration-plan.md) | Dependências #429–#440 e critérios F0–F4 |
| [Stories](user-stories.md) | Jornadas e aceite observável |
| [Spec A](specs/spec-A-data-home-decoupling.md) / [E](specs/spec-E-doctor-conformance-migration.md) | Resolver, migração e doctor |
| [Spec B](specs/spec-B-plugin-packaging.md) | Dois plugins, bootstrap e nove hooks |
| [Spec F](specs/spec-F-trust-release-pipeline.md) | Publicação, integridade e recall |
| [Specs C](specs/spec-C-mcp-server.md) / [D](specs/spec-D-connector-surface.md) | Propostas futuras preservadas, fora deste release |
| [Decisão condicionada](decision-log-draft.md) | Autorização e condições ainda abertas |

A v0.2.0 foi incorporada por #425 em 2026-09-29; permanece
`unsigned-candidate / distribuição HOLD`. O campo está em 0.1.11, não na
consolidação. Main contém 53 skills base, 12 tech-core, sete agentes e nove
handlers com pares `.sh`/`.ps1`; `pm-core` não faz parte dessa baseline.

O spike #429 observou instalação CLI 2.1.233, cópia física em cache e,
numa inicialização real do CLI, seleção da origem publicada e SessionStart.
Outra inicialização leu uma alteração temporária no hook sem `plugin update`;
o artefato foi restaurado byte a byte. Ponteiro e versão permaneceram iguais:
isso não comprova troca de versão do marketplace nem revisão da skill.
Modelo, guard real e delegação ficaram bloqueados por autenticação **do CLI**;
não foi inspecionada a autenticação do Code tab. Code tab, Windows corporativo
e OneDrive/Gatekeeper continuam pendentes. Atualização na próxima sessão
permanece critério a provar nesses ambientes.

O coletor de política #442 e o ensaio Windows #444/#445 estão incorporados.
No [ensaio nativo Windows](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/actions/runs/39538995212),
o transporte exato de bytes/exit codes passou em Bash, PowerShell 5.1 e 7;
os probes sintéticos de guard/delegação passaram. SessionStart foi
`BLOCKED_ELEVATED`, porque o runner é elevado. Os jobs verdes validam esse
recibo parcial; Code tab e Windows corporativo permanecem `NOT_RUN`.

Os contratos propostos de layout/locator e roteamento por target estão nas
Specs A/E e B. A admissão confiável antes de executar código do plugin continua
como decisão de arquitetura aberta na Spec F. Detalhar esses contratos no
draft não aprova F1 nem implementa #433–#438.
