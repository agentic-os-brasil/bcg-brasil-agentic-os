# Construção completa do Maestro em MCP, plugin e filesystem

Data: 2026-10-31. Base verificada: main `9147ed4031768ac271cc6e02da20fd6b8ec07539`.
Execução autorizada; conclusão depende dos aceites abaixo, não da quantidade de ferramentas.
A [checklist integral](2026-10-24-maestro-full-parity.md) continua sendo o inventário de paridade.

## Resultado final

Uma pessoa instala o Maestro, escolhe um workspace, conclui onboarding e trabalha
no Code ou Desktop com o mesmo estado, métodos, memória e políticas. Instalação,
atualização, recuperação e troca de cliente preservam dados e recusam escritores
antigos. O inventário ativo inclui 92 métodos, sete agentes, nove handlers e as
capacidades de manutenção admitidas; cada efeito precisa de prova no cliente.

## Método de execução

Usar domain-modeling para termos e invariantes, codebase-design para a interface,
TDD em fatias verticais, code-review com revisão separada de spec e qualidade e
writing-for-agents para orientação e documentação. Cada PR fecha um fluxo completo
com dependências, teste funcional, pacote e documentação. Não criar PRs de scaffold
nem tickets de follow-up para terminar a própria funcionalidade anunciada.

Os testes cruzam as interfaces públicas já escolhidas para a arquitetura:
ClientSession.CallTool do SDK MCP, o comando de inicialização/lançamento e os
métodos públicos dos motores canônicos. Tests privados e mocks que reproduzem a
implementação não provam paridade. Registrar vermelho antes de verde.

## Entregas e dependências

| Entrega | Fluxo que deve funcionar | Aceite observável | Dependência |
|---|---|---|---|
| E1 — Trabalho pelo MCP de produto | Inicializar estado novo explicitamente, vincular workspace, criar/iniciar trabalho, pausar no Code, retomar no Desktop, coletar evidência e concluir | Mesmo ledger após reinício e atualização; tentativa antiga e artefato adulterado recusados; pacote local Code + MCPB no mesmo vínculo | Armazenamento limitado/no-follow e origem admitida |
| E2 — Estado externo e migração | Resolver HOME em todos os consumidores, migrar ZIP preservando o original e recuperar falhas | Fixtures 0.1.11/0.1.12 equivalentes; idempotência; rollback; produto somente leitura; ownership Windows comprovado | E1; #433/#434; prova nativa antes de migração real |
| E3 — Owner, onboarding e memória | Entrevista → revisão → confirmação; captura → contexto → correção/exclusão/retenção | Faceta e track corretos, digest/base vigente, consentimento e orçamento; material excluído não reaparece | E1/E2; completar operações canônicas ausentes |
| E4 — Conteúdo e integrações | Selecionar fontes, ingerir formatos admitidos, pesquisar conhecimento/trabalho anterior e usar caseOS | Escopo/enrollment/proveniência, revogação, runtimes reais e estados indisponíveis explícitos | E3; autenticação e providers externos |
| E5 — Agentes, qualidade e cadência | Preparar delegação, revisar, fechar o dia e executar manutenção admitida | Autoridade e evidência preservadas por host; agendamento opt-in e recuperação; sem alegar agentes/hooks nativos no Desktop | E3/E4; permissões de runtime |
| E6 — Distribuição e paridade por cliente | Instalar/atualizar plugin e MCPB pelo mesmo canal; descobrir e executar os 92 métodos | N→N+1 preserva dados; rollback/recall; métodos manuais continuam manuais; Code CLI/Code tab/Desktop classificados individualmente | E1–E5; destino SharePoint e assinatura organizacional |
| E7 — Piloto e rollout | Dois colegas instalam e atualizam sem assistência, incluindo pessoa não técnica | Mac e Windows admitidos; doctor e jornada funcional; evidência sanitizada; retirada do ZIP após sobreposição | E6 e gates externos completos |

E1 é a primeira entrega em implementação. E2–E7 permanecem abertas até a prova
inteira; nenhuma contagem de skills ou CI substitui aceite de um fluxo.
O plano não fixa uma data de rollout enquanto os gates externos não tiverem dono e evidência.

## E1: tarefas executáveis no mesmo PR

1. Endurecer o ledger canônico nas plataformas admitidas: leituras limitadas, folhas regulares, operações
   descriptor-relative/no-follow e proteção dos artefatos usados como evidência.
   Preservar locks, publicação imutável e recuperação existentes. O adapter exige
   OpenConfinedStore; Windows conserva compatibilidade legada, mas não é admitido
   para mutações de produto até entregar confinamento e ownership.
2. Acrescentar bootstrap explícito de HOME novo ao resolver canônico. Não ativar
   origem migrada não verificada, não seguir links e não sobrescrever dados ou locator.
3. Construir `adapters/mcp` como módulo Go de produto, com SDK já pinado, sem
   imports de `dev/`/`internal/dev/` e sem marker de fixture sintética.
4. Fechar o fluxo por três ferramentas: contexto de vínculo, consultas fechadas
   e mutações fechadas. Roots/WorkspaceID vêm do vínculo, nunca da chamada do modelo.
5. Gerar plugin e MCPB locais do mesmo vínculo e provar o fluxo com dois clientes
   SDK/stdio e versões de pacote distintas, preservando o estado externo.
6. Revisar spec/standards/segurança/interoperabilidade, corrigir findings, publicar
   o PR coeso, conferir CI e integrar source quando apto. Qualificação é gate separado.

## Decisões de desenho

A comparação de três interfaces favoreceu a interface agrupada de leitura e
mutação: `maestro_context`, `maestro_query` e `maestro_apply`. Os nomes de operação
são enums fechados com schemas por variante; não há shell, RPC de filesystem ou
nome de função arbitrário. A instalação fica simples pelo vínculo único por
workspace. Facetas de owner e memória entrarão pela mesma disciplina de autorização.

O primeiro fluxo admite evidência `artifact_snapshot`; `command_check` executa
processos e só entra quando sua autorização e política forem entregues. Isso não
remove o critério do motor nem fecha a paridade dos critérios ainda não expostos.

Não promover o spike retirando sua trava sintética. Seus pacotes e recibos ficam
históricos; o adapter de produto reutiliza os motores, sem copiar seu estado.

## Dependências reais e gaps confirmados

- `execution.Store` já tem ciclo de vida, revisão/tentativa e revalidação de evidência;
  seu acesso a arquivos precisa de endurecimento antes de exposição a raízes reais.
- `datahome.Resolve` inspeciona; não inicializa. Origem migrada e ownership Windows
  ainda não são autoridade de escrita. Bootstrap e migração são operações distintas.
- `ownerctx` contém onboarding/SELF; faltam operações públicas para algumas escritas.
  Não implementar escrita direta no adapter para contornar a ausência.
- `memory.Engine` tem captura, síntese e contexto; correção/exclusão/retenção precisam
  de fechamento canônico. `sanitized=true` informado pelo modelo não é atestado.
- Providers SharePoint/caseOS e execução model-backed são dependências externas;
  mocks ou catálogo local não provam autenticação, coleta ou entrega remota.

## Gates externos

- Login dos clientes para teste real do pacote pelo modelo.
- Windows sem elevação, com ownership/ACL e canários nativos comprovados.
- Biblioteca/pasta SharePoint escolhida, acesso dos participantes e sincronização real.
- Assinatura corporativa/notarização e autoridade de publicação/recall.
- Participantes e máquinas de piloto, com consentimento para instalação e migração.

Essas dependências não impedem a construção e os testes determinísticos locais.
Também não podem ser convertidas em PASS por uma revisão ou por merge de código.

## Registro de evidência e retomada

Para cada tarefa: interface pública, vermelho/verde, comandos focados, árvore/SHA,
verdicts e limitações. Rodar somente testes locais diretamente relacionados;
o harness completo fica no CI. Guardar recibos em Developer/CodexWorkspace,
temporários em Developer/tmp e caches em Developer/ToolData. Preservar o checkout
principal e seu WIP. O estado de execução deste plano fica no ledger da sessão.
