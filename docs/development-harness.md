# Development harness

O harness de desenvolvimento protege o repositório-fonte do BCG Brasil
Agentic OS. Ele não é um runtime de agente e não é um canal de distribuição
para pilotos — a distribuição é feita por ZIP privado verificado.

## Mapa rápido

| Objetivo | Comando | Evidência produzida |
|---|---|---|
| Diagnosticar o clone | `go run ./dev/harness doctor` | Branch, remote, identidade Git, hooks e estado local; em um bare repo, orienta para um worktree |
| Configurar hooks | `go run ./dev/harness setup` | Hooks do clone apontando para `.githooks` |
| Validação rápida | `go run ./dev/harness validate` | Contratos estruturais, políticas, projeções, boundary, wiki gerenciada, formatação e testes internos |
| Validação completa | `go run ./dev/harness validate --full` | Gate rápido + `go vet ./...` + `go test ./...` |
| Verificar decisões | `go run ./dev/harness decision check` | Integridade do log append-only e dos códigos de quatro letras |
| Reservar código | `go run ./dev/harness decision available ABCD` | Disponibilidade local do código proposto |
| Diagnosticar Git | `go run ./dev/harness recover` | Estado não destrutivo e uma próxima ação segura |
| Verificar wiki | `go run ./dev/harness wiki verify` | Bundle managed OKF reproduzível e atualizado |
| Validar wiki | `go run ./dev/harness wiki validate` | Estrutura e perfil do bundle OKF |
| Reconciliar wiki | `go run ./dev/harness wiki reconcile` | Regeneração local do bundle a partir da allowlist; revisar o diff depois |
| Regerar índice de skills | `go run ./dev/harness skills-index` | Índices derivados dos bundles |
| Testar guard | `go run ./dev/harness guard pre-commit` | Gate do snapshot staged |
| Testar adapter Claude | `go run ./dev/harness claude <session-start\|pre-tool\|post-tool>` | Contrato local do hook Claude |

`validate --full` para no primeiro erro. Isso é intencional: o output mostra
qual camada bloqueou a entrega, enquanto os workflows de CI adicionam a
matriz de sistemas operacionais e os smoke tests específicos.

## O que cada camada prova

Um estado verde precisa ser qualificado:

1. **Implementado**: existe código, contrato ou documentação versionada.
2. **Validado localmente**: o comando relevante passou neste checkout.
3. **CI verde**: os workflows hospedados passaram no SHA exato.
4. **Reviewed**: uma revisão humana avaliou o diff e os riscos.
5. **Mergeable**: o servidor informa que o head pode entrar na base; isso não
   significa aprovação.
6. **Merged**: o commit foi confirmado na branch de destino.
7. **Pilot-ready**: release, assinatura, clean-device, suporte e critérios de
   piloto também têm evidência.

O harness local não prova CI, revisão, mergeabilidade, assinatura, runtime
nativo ou pilot readiness.

## Fluxo de contribuição

Em um clone normal:

```text
start-contributing → start-work → develop-change → prepare-pr → revisão humana
```

Para uma mudança documental, leia `AGENTS.md`, o spec relevante e rode a
validação aplicável. Para uma mudança comportamental, `develop-change` exige
contrato/teste proporcional ao risco e `validate --full` antes da entrega.

O fluxo de PR prepara a revisão, mas não decide o merge. Nunca use `git add .`,
push direto para `main`, force push, reset destrutivo ou `gh pr merge` a partir
do agente.

## Wiki e distribuição

O managed atlas é uma saída derivada da allowlist em
`dev/wiki/managed-allowlist.json`. Altere primeiro as fontes canônicas; depois
rode `wiki reconcile`, `wiki validate` e `wiki verify`. Não edite manualmente
`bundles/base/atlas/managed/`.

Esse atlas é conteúdo de produto sanitizado e não é a wiki privada de owner ou
workspace. O runtime ainda não oferece skills de wiki privada para
compilar ou consultar conteúdo de owner. Conteúdo de desenvolvimento,
onboarding de contribuidores, decisões e dados locais ficam fora do bundle
managed.

## Bare repository e worktrees

Algumas instalações mantêm os objetos Git em um repositório bare e os arquivos
em worktrees vinculados. O caminho bare é armazenamento, não um diretório de
trabalho: `git status`, hooks e edição devem ocorrer em um worktree real.

Se `doctor` reportar esse caso:

```text
git worktree list
cd <worktree-limpo>
go run ./dev/harness doctor
```

Não remova worktrees `prunable` automaticamente. Primeiro identifique o owner
e preserve qualquer mudança local.

## Troubleshooting honesto

- `doctor` falha no bare repo: entre em um worktree; nenhum arquivo foi
  alterado.
- `validate --full` falha em listener/rede apenas no sandbox: repita em um
  ambiente autorizado e classifique o resultado como limitação do ambiente,
  não como CI verde.
- `wiki verify` falha: verifique a fonte/allowlist e regenere o bundle; não
  corrija o arquivo gerado à mão.
- O hook bloqueia: leia a razão e execute somente a ação de recuperação
  indicada. O hook não apaga nem guarda arquivos automaticamente.
- CI não iniciou ou ficou sem steps: isso é evidência de infraestrutura/billing
  inconclusiva, não sucesso.

## Fora do escopo deste harness

Builds de release, assinaturas, publicação, clean-device, autenticação externa,
qualificação nativa Claude/Codex e prontidão de piloto possuem gates próprios.
Consulte `docs/release-gates-checklist.md`, `docs/releasing.md` e as matrizes
de evidência antes de transformar um passe local em uma promessa de produto.
