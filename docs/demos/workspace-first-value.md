# Demo local — primeiro valor do Case Agent

Converse com Maestro para conduzir as seis perguntas do Case Agent. O skill
canônico é `$case-agent-setup`; ele orquestra o fluxo de entrevista, entrega
e checkpoint internamente através da superfície de agentes governados do
workspace. Cada fase do fluxo é um passo autenticado no estado compartilhado:

1. **Início:** Maestro inicia o run de valor do Case Agent para o workspace.
2. **Entrevista:** Maestro conduz as perguntas canônicas de escopo, hipóteses,
   fontes, lacunas, decisão e critério de sucesso.
3. **Entrega:** Maestro produz o artefato e registra o submit com evidência.
4. **Status:** Maestro consulta o estado do run e exibe o checkpoint.

O resultado é uma decisão Markdown classificada em `brain/deliverables/`, mais
um handoff e métricas locais. O fluxo não usa Docling, wiki, dreaming, pesquisa
externa ou agentes adicionais.
