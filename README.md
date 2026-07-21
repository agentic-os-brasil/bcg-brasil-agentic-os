# BCG Brasil Agentic OS

Second brain profissional e camada operacional de agentes para o BCG Brasil.

> Status: fundacao inicial. O repositorio ainda nao contem uma CLI funcional nem um Agent OS pronto para uso.

## Visao

Construir progressivamente um sistema de trabalho assistido por agentes que possa servir consultores classicos, BCG X, cientistas de dados e engenheiros. O sistema sera exclusivamente profissional e devera combinar baixa friccao para o usuario com uma base deterministica, modular e atualizavel.

O objetivo nao e copiar o Kowalski OS. Vamos reaproveitar seletivamente principios que ja provaram valor, especialmente separacao de responsabilidades, injecao de contexto, hooks, validacao, modularidade e evolucao governada.

## Experiencia pretendida

Para o usuario piloto, o Agentic OS deve se comportar como uma ferramenta interna:

```text
instalar bcgos
bcgos init <workspace>
bcgos doctor
bcgos update
```

O usuario nao precisara clonar o repositorio, entender Git ou manter manualmente os componentes do OS. O proprio agente podera invocar a CLI em nome do usuario.

## Principios fundadores

- **CLI-first:** instalacao, diagnostico e atualizacao sao parte do produto desde a primeira versao.
- **Baixa friccao:** a referencia de experiencia e o consultor que nao trabalha com engenharia de software.
- **Core separado do trabalho:** runtime gerenciado nunca se mistura com memoria, credenciais ou conteudo de clientes.
- **Distribuicao por releases:** o piloto recebera artefatos versionados a partir de GitHub Releases privadas, nao `git pull`.
- **Evolucao progressiva:** primeiro um nucleo pequeno e verificavel; agents, skills, hooks e automacoes entram conforme casos de uso reais.
- **Wiki compilada para navegacao:** conteudo reutilizavel, incluindo memoria autorizada, e navegado por um atlas derivado no padrao Karpathy; fontes canonicas continuam sendo a verdade e escopos privados permanecem separados.
- **Claude-first, Codex-compatible:** Claude sera o runtime principal, mas nao a fonte canonica da arquitetura. Um core compartilhado e adapters finos devem preservar os mesmos invariantes observaveis em Claude e Codex, com lacunas mecanicas declaradas explicitamente.
- **Seguranca desde a origem:** nenhum dado real, segredo ou material de cliente pertence ao repositorio ou aos bundles distribuiveis.

## Publico do piloto

O primeiro piloto sera de aproximadamente dez pessoas e devera incluir perfis com diferentes niveis de familiaridade tecnica. Windows e macOS serao plataformas de primeira classe, com a mesma experiencia e os mesmos criterios de aceitacao. Linux permanece um alvo suportado de build e desenvolvimento, mas nao e requisito de paridade do piloto inicial.

## Estado atual

Implementado agora:

- visao e principios fundadores;
- contrato inicial de colaboracao;
- roadmap progressivo;
- specs de fundacao, distribuicao, fronteiras de dados e sucesso do piloto;
- estrutura reservada para CLI, bundles, adapters, schemas, migrations e installers.
- harness Go exclusivamente de desenvolvimento para decision log, skills, formatacao, testes e `go vet`;
- skills de desenvolvimento para onboarding, inicio de trabalho, implementacao, decisoes, recuperacao e preparacao de PR;
- `doctor`, hooks locais e guia Claude para contribuidores iniciantes;
- CI com o mesmo gate em Windows, macOS e Linux.
- contrato inicial de memoria com dreaming diario leve, deep dreaming semanal, L1/L2/L3 e lifetime versionado;
- politica de memoria distribuivel, schemas, engine Go testado e skills operacional/de desenvolvimento.
- entrypoint `bcgos` com `version` e bridge inicial de memoria para capture, status e contexto; dreaming falha explicitamente como indisponivel ate existir adapter aprovado.
- contrato de wiki compilada com atlas gerenciado separado do atlas privado e navegacao governada de L1/L2/L3/lifetime.

Ainda nao implementado:

- instalacao e distribuicao do binario `bcgos`;
- instalador;
- autenticacao com GitHub;
- bundle do OS;
- pipeline de releases;
- update, rollback e assinatura de artefatos.
- adapters de sintese e elegibilidade, agendamento, recovery de locks e dreaming executavel no `bcgos memory`.
- gerador da wiki, schemas de pagina/indice, comandos de navegacao e atlas privado de owner/workspace.

## Desenvolvimento da solucao

O harness abaixo existe apenas no source repo. Ele nao faz parte da CLI `bcgos`, dos bundles ou da instalacao dos usuarios.

```text
go run ./dev/harness validate
go run ./dev/harness validate --full
go run ./dev/harness decision check
go run ./dev/harness decision available ABCD
go run ./dev/harness doctor
go run ./dev/harness setup
```

O fluxo esta definido na [Spec 005](specs/005-development-harness.md). Decisoes duraveis vivem no [project decision log](docs/decisions/decision-log.md) com codigos de quatro letras maiusculas.

Contribuidor novo: comece pelo [guia de contribuicao](CONTRIBUTING.md). No Claude, diga: **"Use start-contributing e me guie passo a passo."**

Para um primeiro clone no Windows, Daniel pode enviar o [prompt de onboarding para o Claude](docs/onboarding/windows-contributor-prompt.md). Questoes ainda nao decididas ficam em [Open Questions](docs/OPEN-QUESTIONS.md), separadas do decision log.

## Estrutura

```text
cmd/bcgos/       entrypoint futuro da CLI
internal/        implementacao interna da CLI
bundles/base/    conteudo gerenciado do Agentic OS
adapters/        integracoes finas com runtimes de agentes
schemas/         contratos de configuracao e manifests
migrations/      mudancas versionadas de schema
installers/      instaladores por plataforma
specs/           contratos antes da implementacao
docs/            decisoes e explicacoes para humanos
```

O contrato de memoria esta em [Spec 006](specs/006-memory-persistence.md). A politica sanitizada vive em `bundles/base/memory/policy.json`, o engine em `internal/memory` e a skill operacional em `bundles/base/skills/dream-memory/`. Dados e rollups reais permanecerao fora do repositorio, em armazenamento local do usuario.

O contrato de navegacao esta na [Spec 007](specs/007-content-navigation.md). A wiki e um atlas compilado e regeneravel: navega conteudo gerenciado e, futuramente, os rollups da memoria privada escopada. Dreaming produz L2/L3/lifetime; a wiki organiza rotas temporais e semanticas sobre esses rollups sem substitui-los nem promover conteudo privado para o bundle compartilhado.

Leia primeiro:

- [Decisoes fundadoras](docs/FOUNDING-DECISIONS.md)
- [Roadmap](ROADMAP.md)
- [Contrato de colaboracao](COLLAB.md)
- [Guia de contribuicao](CONTRIBUTING.md)
- [Spec de fundacao](specs/000-foundation.md)
- [Spec de portabilidade entre runtimes](specs/004-runtime-portability.md)
- [Spec do harness de desenvolvimento](specs/005-development-harness.md)
- [Spec de memoria e dreaming](specs/006-memory-persistence.md)
- [Spec de navegacao por wiki compilada](specs/007-content-navigation.md)

## Confidencialidade

Este repositorio privado armazena somente codigo, templates, contratos e conteudo distribuivel sanitizado. Dados de pessoas, clientes, cases, projetos, memoria operacional, logs e credenciais devem permanecer fora do repositorio e fora dos bundles oficiais.
