# Spec C — Proposta futura de servidor MCP

**OUT OF v0.3.0.** Preservada como hipótese futura; sem compromisso de versão,
implementação ou paridade. O default do epic #428 retira este trabalho.

A ideia original era expor operações tipadas de estado, memória e prior-work
por MCP stdio, reutilizando contratos Go. Ela precisaria de design específico
para schemas de ferramentas, autoridade de escrita, consentimento, concorrência,
bootstrap verificável e limites entre casos. Existência de pacotes Go não
comprova uma API MCP pronta nem justifica sua exposição automática.

No escopo atual, `maestro-runtime` é helper CLI chamado por hooks/skills,
empacotado em `scripts/` pela [Spec B](spec-B-plugin-packaging.md).
Não há binário `maestro-mcp`, instalação em `~/.maestro/bin/`, servidor novo,
download no primeiro uso ou gate MCP em F0–F4.

Reabrir somente com necessidade demonstrada, decisão própria e contrato de
segurança. [Spec D](spec-D-connector-surface.md) depende dessa decisão futura.
