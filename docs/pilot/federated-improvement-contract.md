# Contrato do piloto — Maestro Federated Improvement Loop

Ao entrar no piloto, a pessoa autoriza uma única vez o envio automático de
artefatos definidos neste contrato. Não há pedido de aprovação por envio. A
participação pode ser revogada a qualquer momento com `bcgos federation
revoke`; a revogação interrompe novos envios e preserva o estado local para
suporte e auditoria.

## O que sai automaticamente

- sinais tipados e limitados de adoção, fricção, falha e padrão de fluxo;
- candidatos estruturais de skills, sem corpo, provenientes de uso em um
  workspace;
- skill completa apenas se foi criada na raiz local `born_portable`, tem
  manifesto, hash, declaração de generalização e não contém links para o
  workspace;
- versão do produto/runtime, período semanal e identificador opaco de
  instalação.

## O que nunca sai por este fluxo

- arquivos, caminhos, nomes de clientes, identificadores de workspace,
  prompts, respostas, logs, mensagens de erro, memória, documentos ou código
  do workspace;
- token GitHub, credencial da GitHub App ou qualquer chave de publicação;
- avaliação de desempenho individual.

## Como o aprendizado vira mudança

O Darwin local converte percepções em uma taxonomia fechada. A ponte central
agrega apenas coortes e o Darwin central gera uma Issue de proposta ou
incidente no GitHub. Um mantenedor humano precisa aceitar a proposta antes de
qualquer skill compartilhada, alteração de código, pull request ou release.

## Operação

A instalação gerenciada executa o job semanal do runtime e recupera itens
pendentes quando houver presença autorizada. Falhas de rede deixam o item no
outbox local e tentam novamente no máximo três vezes; o cliente não precisa
intervir para cada envio. O endpoint central, a identidade opaca e a
provisão da instalação são configurados pelo canal do piloto, não por token
GitHub do participante.
