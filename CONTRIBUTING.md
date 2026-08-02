# Como contribuir

Este arquivo é o ponto de entrada curto para contribuidores do código-fonte.
Usuários do piloto instalarão o produto pelo `bcgos` e não precisarão aprender
Git.

## Comece pelo onboarding

Se o repositório ainda não foi clonado no Windows, siga o
[onboarding de contribuidores Windows](docs/onboarding/windows-contributor-prompt.md).
Depois do clone, abra uma nova sessão do Claude dentro do repositório e diga:

> Use start-contributing e me guie passo a passo.

## Guia operacional

O [guia do harness de desenvolvimento](docs/development-harness.md) é a
referência única para comandos, camadas de proteção, boundary entre
desenvolvimento e produto, troubleshooting e estados de evidência. Consulte-o
antes de repetir um comando ou interpretar um bloqueio.

O fluxo diário é conduzido pelas skills canônicas:

~~~
start-work -> develop-change -> prepare-pr -> revisão humana
~~~

Se algo der errado ou o estado não estiver claro, diga:

> Use recover-work. Estou perdido.
