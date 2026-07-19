# Como contribuir

Este guia e para contribuidores do codigo-fonte. Usuarios do piloto instalarao o produto pelo `bcgos` e nao precisarao aprender Git.

## Se voce nao conhece Git

Se o repositorio ainda nao foi clonado no Windows, comece com `docs/onboarding/windows-contributor-prompt.md`. Envie esse prompt ao Claude para verificar ferramentas, autenticar pelo browser, clonar e executar o bootstrap oficial.

Depois do clone, abra uma nova sessao do Claude dentro do repositorio e diga:

> Use start-contributing e me guie passo a passo.

O agente vai verificar seu ambiente, explicar os termos e oferecer uma unica proxima acao por vez. O caminho diario e:

```text
start-work -> develop-change -> prepare-pr -> revisao humana
```

Se algo der errado, diga: **"Use recover-work. Estou perdido."** O fluxo de recuperacao primeiro preserva seus arquivos e so depois propoe uma acao.

## Protecoes

- `go run ./dev/harness setup` instala os hooks deste clone.
- O pre-commit bloqueia trabalho em `main`, possiveis segredos/arquivos de cliente e snapshots que nao passaram pelo gate completo.
- O pre-push bloqueia envio direto para `main` e codigo diferente do snapshot validado.
- A CI repete o gate em Windows, macOS e Linux.
- Um humano revisa e decide o merge do pull request.

Os hooks locais nunca apagam ou guardam arquivos automaticamente. Quando bloqueiam, nada foi perdido.
