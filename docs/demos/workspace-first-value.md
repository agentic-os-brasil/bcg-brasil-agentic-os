# Demo local — primeiro valor do Case Agent

Converse com Maestro para conduzir as seis perguntas do Case Agent. O skill
canônico é `$case-agent-setup`; ele usa a superfície CLI de compatibilidade
abaixo internamente:

```text
bcgos init <workspace>
bcgos workspace-agent value start <workspace>
bcgos workspace-agent interview <workspace>
bcgos workspace-agent value submit --run <id> --stdin <workspace>
bcgos workspace-agent value status <workspace>
```

O resultado é uma decisão Markdown classificada em `brain/deliverables/`, mais
um handoff e métricas locais. O fluxo não usa Docling, wiki, dreaming, pesquisa
externa ou agentes adicionais.
