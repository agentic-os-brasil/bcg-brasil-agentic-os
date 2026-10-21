# Plano — ZIP para plugin em pasta compartilhada

**DRAFT / F1 BLOCKED por F0.** Sequência autorizada pelo
[epic #428](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/428).
Preparar documentação enquanto o spike avança não aprova o design nem libera
implementação dependente dos resultados abertos.

## Evidência e lacunas de F0

| Observação | Estado e limite |
|---|---|
| Registro e instalação isolada CLI 2.1.233/macOS arm64 | Observados; plugin enabled e cópia física em cache |
| Inicialização real CLI fora do produto | SessionStart despachado pelo host, exit 0; init selecionou origem publicada, não cópia em cache |
| Mudança temporária no hook publicado | Segunda inicialização CLI emitiu marcador novo sem update; origem restaurada e cache idêntico; mesma versão/ponteiro |
| Modelo, skill revisada, guard real e Agent/Task | Bloqueados por autenticação do CLI: authMethod=none; tentativa exit 1, zero custo/API |
| Code tab e troca de versão por marketplace | Não observados; CLI não qualifica Code tab nem informa sua autenticação |
| OneDrive com espaços e Gatekeeper nesse caminho | Não executados; artefato local com espaços não é pasta sincronizada |
| Windows corporativo não elevado | Não qualificado; cross-build e fixtures diretas não substituem host real |
| Managed settings do tier padrão | Frota desconhecida; um Mac Power User não representa os demais |

Evidência comprometida do spike: [README](../../../dev/spikes/plugin429/README.md)
e [RESULTS](../../../dev/spikes/plugin429/RESULTS.md). Observações CLI adicionais
de 2026-10-20: pacote de sessão `runtime-observation/REPORT.md`, entregue no
handoff de #432; não convertem NOT_RUN da matriz do Code tab em PASS.

## Sequência e critérios de conclusão

| Gate | Issues e dependências | Passa quando |
|---|---|---|
| F0 Spike | #429 marketplace/guard; #430 managed settings; #431 Windows | Seis incógnitas do epic respondidas com host, versão, caminho, evidência e limites |
| F1 Design | #432 após #429/#430/#431 | Proposta reconciliada com main e F0, revisão e merge autorizados |
| F2 Estado | #433 resolver após #432; #434 migração após #433 | Último ZIP publicado pelo fluxo existente e uma instalação real migrada, equivalência, doctor PASS e rollback |
| F3 Plugin | #435 bootstrap após #432; #436 build após #431/#433/#435; #437 doctor e #438 publish após #436 | Install Mac/Windows, nove hooks/guard no Code tab, N→N+1 sem update explícito com estado preservado |
| F4 Rollout | #439 após #431/#434/#437/#438; #440 após #439 | Dois pilotos sem suporte cobrem Mac/Windows e install+update; destinatários contabilizados; retirada após um release de convivência |

Issues pertencem a `agentic-os-brasil/bcg-brasil-agentic-os`.
Fonte de aceite: [#432](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/432)
e #433–#440. A v0.2.0 incorporada não é gate de merge pendente;
sua distribuição HOLD continua sendo uma restrição real.

## Riscos e respostas

- Política bloqueia canal/hooks: parar esse canal e avaliar biblioteca
  organizacional claude.ai; nenhuma política é alterada para obter PASS.
- Sincronização parcial: versões imutáveis, verificação no destino e ponteiro
  trocado por último; inconsistência é FAIL, nunca execução presumida segura.
- Runtime bloqueado nativamente: registrar Gatekeeper/SmartScreen e encaminhar
  custódia de assinatura; nenhuma remoção de quarentena ou bypass.
- Migração ambígua/interrompida: preservar origem, plano e dados autorais;
  retomar apenas o que a mesma transação comprova. Ver [Spec E](specs/spec-E-doctor-conformance-migration.md).
- Falta de sinal de frota: contabilizar destinatários no rollout autorizado;
  ausência de telemetria não significa adoção nem migração concluída.

Cada gate distingue implementação, testes locais, CI, execução nativa, revisão,
merge e distribuição. Nenhum desses estados implica os demais.
