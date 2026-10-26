# Métodos de engenharia no Maestro

O Maestro distribui os 27 métodos estáveis do pacote Pocock 1.3.1 como parte
do Tech Core. Os IDs começam com `pocock-`, preservando os métodos existentes
do Maestro. Referências, templates e licença acompanham cada método.

## Usar o fluxo completo

Em um workspace profissional autorizado, escolha a etapa necessária:

| Necessidade | Método |
|---|---|
| Configurar o tracker e as convenções do repositório | `pocock-setup-matt-pocock-skills` |
| Esclarecer o problema e os termos do domínio | `pocock-grilling`, `pocock-domain-modeling` |
| Produzir uma especificação e tickets executáveis | `pocock-to-spec`, `pocock-to-tickets` |
| Implementar a entrega com testes nas interfaces acordadas | `pocock-implement`, `pocock-tdd` |
| Investigar uma falha | `pocock-diagnosing-bugs` |
| Revisar separadamente especificação e standards | `pocock-code-review` |
| Explicar a mudança e documentar métodos | `pocock-pr`, `pocock-writing-for-agents` |
| Transferir trabalho ou revisar aprendizados | `pocock-handoff`, `pocock-retro` |

No Claude Code com a projeção local, invoque `/pocock-pr`, por exemplo. No
Codex, use `$pocock-pr`. No plugin experimental `maestro-team-pilot`, o nome é
`/maestro-team-pilot:pocock-pr`. A chamada carrega o método e suas referências
somente quando necessários; não injeta todos os corpos no contexto inicial.

Se o host não oferece uma ferramenta Skill, abra o método pelo catálogo
instalado e siga suas instruções. Os 16 métodos marcados como explícitos
continuam exigindo uma chamada do usuário; um match de palavras no roteador
não os ativa. A instalação não executa o setup do tracker nem cria labels.

## O que é instalado

O Tech Core contém os 12 métodos anteriores e 27 métodos Pocock, totalizando
39. Com os 53 métodos base, o catálogo completo tem 92 IDs. O ZIP mantém as
fontes em `bundles/` e as cópias nativas em `.claude/skills/` e `.agents/skills/`.
A projeção Go usa as superfícies Claude/Codex já existentes e instala todos os
recursos dos métodos selecionados, incluindo `agents/openai.yaml`.

O plugin experimental contém os métodos Pocock e a dependência
`interaction-profile`, junto do operator/resume existente. Ele adapta as
referências de invocação para o namespace real do plugin. A extensão MCPB
continua sendo o transporte do servidor; ela não instala skills no chat Desktop.

## Integridade, atualização e limites

A atualização e a desinstalação verificam recursos auxiliares junto de
`SKILL.md`. Um arquivo personalizado, ausente ou redirecionado por symlink
gera conflito; o instalador preserva o conteúdo para revisão. Projeções legadas
sem registro de recursos precisam ser atualizadas antes de o diagnóstico e o
roteamento atestarem o pacote completo. Um recurso novo não substitui um
arquivo que já existia sem ownership, mesmo com os mesmos bytes.

Adicionar recursos em caminhos livres e reinstalar os mesmos recursos é
suportado. Alterar ou retirar um recurso gerenciado de uma versão anterior
interrompe a atualização até existir uma autoridade de migração revisada para
essa versão. O manifesto local não fornece essa autorização. A migração do
workspace inclui os recursos no snapshot e rollback; seus limites são 256
arquivos, 512 KiB por arquivo e 4 MiB no total.

As instruções dos métodos operam dentro do workspace e da autoridade já
concedidos ao Maestro. Publicações, mensagens, dependências, ações destrutivas
e gastos continuam sujeitos à autorização aplicável. O perfil de interação
canônico altera a comunicação; não concede acesso ou ferramentas.

Origem: `mattpocock/skills`, versão 1.3.1, commit
`49dd158d1076134a641b33efb035946536778336`. A licença MIT acompanha cada pacote;
`pocock-upstream.json` registra os hashes de origem e do conteúdo canônico
adaptado. No plugin, os hashes canônicos precedem a transformação de namespace.

A [Spec 056](../../specs/056-pocock-product-skillset.md) define o aceite da
entrega. Pacote, descoberta local, execução de modelo, qualificação de
plataforma e release assinado são evidências distintas. O piloto MCP sintético
mantém seu escopo; estes métodos não ampliam o acesso das três ferramentas MCP.
