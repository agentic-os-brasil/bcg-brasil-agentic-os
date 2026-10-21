# Decision Log Draft — Proposal 004

**Status: approved with conditions.** Aprovação para executar o epic #428;
este arquivo permanece DRAFT e não promove os gates ainda abertos.
Autoridade: [epic #428](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/428)
e [issue #432](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/432).
Não é nova decisão inserida no histórico canônico nem autorização de release.

## Decisão condicionada

Distribuir base e tech-core como dois plugins no Code tab do Claude desktop,
por marketplace local em pasta compartilhada somente leitura. Estado separado
em `~/.maestro/` por uma migração direta do ZIP de campo; helper CLI em
`scripts/`, nove hooks e guard preservados. Retirar MCP/connector da v0.3.0.
Atualização na próxima sessão sem `plugin update` é requisito a provar.

A v0.2.0 já foi incorporada por #425 em 2026-09-29; ainda é candidata
unsigned/HOLD. Aprovação do epic não transforma candidata em release assinada.

## Condições não satisfeitas

1. F0 (#429–#431): fechar OneDrive/paths, enforcement do guard no Code tab,
   Agent/Task real, nova revisão/sessão, Gatekeeper e política do tier padrão;
   qualificar Windows corporativo não elevado com seu padrão de hooks.
2. F1 (#432): reconciliar proposta, layout/contratos e achados; revisão e
   merge autorizados. Draft antecipado permanece bloqueado por F0.
3. F2 (#433/#434): resolver compartilhado e migração copy→verify→commit,
   equivalência, retidos acessíveis e rollback funcional; último ZIP e uma
   instalação real migrada sem perda.
4. F3 (#435–#438): bootstrap substitui CLAUDE.md, dois plugins/nove hooks,
   helper, doctor e publicação/recall íntegros; Code tab Mac/Windows prova
   install e N→N+1 com estado byte-idêntico no ensaio controlado.
5. F4 (#439/#440): dois colegas cobrem Mac/Windows sem suporte; uma versão
   de convivência e cada destinatário contabilizado antes de retirar ZIP.
6. Autoridade, assinatura e aprovação de distribuição são evidências próprias;
   testes locais, docs, cross-build e startup CLI não as substituem.

## Defaults ainda pendentes de ratificação

Specs C/D ficam fora. Caminho da pasta permanece em aberto: default de
OneDrive no spike e SharePoint no rollout não seleciona um path concreto.
Comando alvo `/maestro:doctor`; onboarding menciona o antigo `/maestro-doctor`,
sem declarar alias executável. Política restritiva aciona avaliação da
biblioteca organizacional como plano B, não contorno do canal.

Rastreio dos gates e riscos: [plano](migration-plan.md). Reabrir o desenho se
F0 contrariar resolução de fontes, bloqueio de escrita, disponibilidade da
pasta ou execução nativa; não reinterpretar resultado inconclusivo como PASS.
