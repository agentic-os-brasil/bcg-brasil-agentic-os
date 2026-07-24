# Validação dos modelos do Atlas humano

**Status:** pedido de input — não altera o produto nesta proposta.

**Revisores convidados:** Marcelo Petrof Sanches e Julia Ribeiro.

## Decisão que precisamos tomar

O Agentic OS deve criar modelos de arquivos para ajudar uma pessoa a manter
contexto de trabalho em clientes, projetos, pessoas e dias de trabalho. A
pergunta não é se os arquivos podem guardar toda a informação possível. A
pergunta é se eles ajudam alguém a registrar o essencial rapidamente e a
reencontrar esse contexto semanas depois.

Os quatro modelos abaixo estão implementados no PR #14 como ponto de partida.
Eles não criam busca, tarefas, memória automática ou uma Wiki. São apenas
arquivos Markdown locais que a pessoa pode abrir e editar.

## Modelo: cliente

```md
# Client: <name>

## Snapshot
- Organization / business unit:
- Relationship context:
- Sensitivity: client_restricted
- Source / as of:

## Stakeholders
- Name / role / relevance:

## Current context

## Related
- Projects
- Daily
```

**Intenção:** registrar o mínimo necessário sobre a empresa, a relação e as
pessoas relevantes para este workspace.

## Modelo: projeto ou workstream

```md
# Project / workstream: <name>

## Snapshot
- Client:
- Owner / role:
- Status: on_track | at_risk | blocked
- Next milestone / date:

## Objective

## Current truth
| Fact | Value | As of | Source |

## Current state

## Workplan
- [ ] <step> — <owner> — <due date>

## Decisions
- Decision:
- Context / source:
- Review by:
- Status: active | superseded

## Risks / blockers

## Key artifacts
```

**Intenção:** manter uma visão de trabalho: objetivo, estado atual, fatos
importantes com fonte, decisões, riscos e materiais relevantes. Ele não é um
substituto para o sistema oficial de tarefas.

## Modelo: pessoa

```md
# Person: <name>

## Snapshot
- Role / organization:
- Relationship to this workspace:
- Sensitivity: professional_restricted
- Source / as of:

## Working context
- Collaboration preferences observed:
- Communication considerations:

## Workspace interactions
- YYYY-MM-DD — <factual, necessary note>
```

**Intenção:** registrar somente contexto profissional necessário para colaborar
bem. Não é um dossiê comportamental e não deve guardar informação pessoal sem
justificativa. Registre somente fatos profissionais observáveis e necessários
para a colaboração. Nunca registre saúde, vida pessoal, inferências
psicológicas, avaliação de desempenho ou qualquer dado sensível. Se não houver
uma finalidade clara de colaboração, não registre.

## Modelo: diário

```md
# Daily — YYYY-MM-DD

## Related scope
- Projects:
- Clients:

## Priorities
1.
2.
3.

## Notes

## Decisions surfaced

## Learning candidates

## Carry forward
```

**Intenção:** um registro humano do dia, para organizar prioridades, anotar
fatos e não perder pendências. Hoje ele não alimenta automaticamente a memória
do agente.

## Perguntas para revisão

1. Para cada modelo, indique uma opção: **criar agora**, **criar quando
   necessário** ou **não criar no piloto**.
2. Que campo está faltando para tornar o modelo realmente útil no trabalho?
3. Que campo parece burocracia e deveria ser removido ou deixado opcional?
4. A página de pessoa é útil desde o piloto? Se sim, quais limites de conteúdo
   precisam ficar ainda mais explícitos?
5. Projeto e workstream devem usar o mesmo arquivo ou modelos diferentes?
6. Os modelos devem nascer em português, inglês ou permitir ambos?
7. Qual é o menor ritual realista para atualizar essas páginas: durante o
   trabalho, ao fim do dia, após reunião ou uma vez por semana?

## Como usar o feedback

Responda diretamente neste PR, apontando o modelo e o campo. Para cada campo,
use **manter**, **remover**, **tornar opcional** ou **adicionar**. Ao propor um
campo novo, inclua um exemplo concreto de quando ele ajudaria. O feedback será
consolidado antes de mudar os templates distribuídos pelo comando `bcgos atlas
init`.
