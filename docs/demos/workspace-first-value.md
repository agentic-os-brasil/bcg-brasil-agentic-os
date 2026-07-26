# Demo local — primeiro valor do workspace

Converse com Maestro para conduzir as seis perguntas; o skill usa os comandos
determinísticos abaixo internamente:

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
