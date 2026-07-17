# Contrato de colaboracao

Este documento define como o BCG Brasil Agentic OS sera construido enquanto a arquitetura ainda esta em formacao.

## Papeis iniciais

- **Daniel Scardini:** maintainer inicial, owner do repositorio e responsavel pelas decisoes fundadoras.
- **Marcelo:** co-construtor; GitHub username e acesso serao registrados quando o convite for enviado.
- **Pilotos:** usuarios e fontes de evidencia; nao sao obrigados a entender a arquitetura para participar.

## Forma de trabalhar

1. Registrar o problema e o resultado esperado antes de expandir a arquitetura.
2. Separar claramente decisao aceita, hipotese e pergunta aberta.
3. Preferir componentes pequenos, substituiveis e testaveis.
4. Nenhuma feature entra apenas porque existe no Kowalski ou em outro Agent OS.
5. Mudancas no core, distribuicao, seguranca ou dados exigem spec ou decisao registrada.
6. Toda experiencia prometida no README deve ser distinguida entre planejada, implementada e validada.

## Fluxo de contribuicao

- `main` representa o estado integrado.
- Trabalho substantivo entra por branch e pull request.
- Commits devem ser pequenos e intencionais.
- Specs precedem implementacoes que alterem contratos.
- Secrets, dados reais e materiais de cliente nunca entram em commits, issues ou exemplos.

## Registro de decisoes

Decisoes fundadoras vivem em `docs/FOUNDING-DECISIONS.md`. Conforme o sistema crescer, decisoes estruturais passam a ter ADRs individuais, numerados e imutaveis; mudancas posteriores criam novos ADRs em vez de reescrever o historico.
