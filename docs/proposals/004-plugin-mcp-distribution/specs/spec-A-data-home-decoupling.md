# Spec A — Um data home fora do produto

**DRAFT / F1 BLOCKED.** Implementação futura: #433; migração: #434.
Escopo e sequência: [PRD](../PRD.md), [plano](../migration-plan.md).

## Contrato

Todo leitor/escritor de estado do dono usa um único resolver: `MAESTRO_HOME` explícito,
depois marker de migração, depois `~/.maestro/` (`%USERPROFILE%\.maestro` no
Windows). Resolver apenas inspeciona: não cria pastas, markers ou estado.
Destino relativo, inacessível ou marker quebrado gera erro acionável; nenhum
fallback silencioso cria memória vazia. Produto permanece somente leitura.

Adoção no campo exige **uma migração direta** do `data/` de 0.1.11 para o
data home final. Ninguém precisa adotar brain numa instalação intermediária.
O helper fica no plugin em `scripts/`, não no data home. Estado persistente
não usa `${CLAUDE_PLUGIN_DATA}`, cujo ciclo de vida acompanha o plugin.

## Compatibilidade com o motor atual

O contrato existente é [ZIP migration v1](../../../../acceptance/migration/CONTRACT.md),
implementado em `internal/zipmigration/`. Hoje destino e controle ficam em
`brain/` dentro da instalação. `targetFor` mapeia profile→owner, achata atlas
e brain de casos, ajusta marcadores e retém agents/workspaces/canary na origem.
Portanto, mover todos os bytes 1:1 para `home/data` não descreve a baseline.

O desenho preserva os contratos lógicos de estado consumidos pelo runtime.
O layout físico do home permanece TBD: #433/#434 precisam fechar os paths
do estado, controle e namespaces retidos antes de implementar. O inventário
origem→destino/retido e transformações versionadas é a autoridade de equivalência;
payloads autorais preservam bytes, e transformações exigem validação própria.
Retidos continuam legíveis por um resolvedor validado; renomear a origem não
pode quebrar esse acesso. Nenhum descarte é inferido de namespace desconhecido.

Estado do dono e conteúdo de workspace são autoridades distintas. `brain`
em `internal/workspace`, `internal/atlas` e `internal/priorwork` pode designar
conteúdo local de caso; não substituir esses roots por um home global.
Pacotes que já recebem DataRoot mantêm essa injeção. Classificar callers,
fixtures e roots de caso antes de adaptar referências relativas ao produto.

## Escritas e origem

Onboarding, hooks e scaffold só escrevem estado após origem validada. Home
ausente ou `fresh-unconfirmed` exige a pergunta “Usava Maestro por ZIP nesta
máquina?” antes de qualquer escrita; “sim” pede a origem explícita e usa #434;
“não” confirma instalação nova. A [Spec E](spec-E-doctor-conformance-migration.md)
define estados e recuperação. Leitura do resolver e diagnóstico não confirmam
instalação nova por conta própria.

## Aceite de #433

- [ ] Nove pares `.sh`/`.ps1`, skills, scaffold e callers Go resolvem estado
  do dono pelo mesmo contrato; roots locais de caso e fixtures seguem seu escopo.
- [ ] Testes de prioridade env/marker/default, marker quebrado sem escrita,
  home ausente e Windows em drive diferente de C: com espaços.
- [ ] Execução do conjunto de hooks com produto somente leitura não escreve
  no produto; snapshot de estado fica intacto na troca N→N+1 sem trabalho concorrente.
- [ ] #434 comprova mapeamento, namespaces retidos e rollback no home final;
  layout interno e formato dos markers tornam-se contratos antes do freeze F1.
