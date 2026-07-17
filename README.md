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
- **Runtime modular:** o core deve aceitar adapters finos para Codex, Claude e futuros hosts sem fingir paridade inexistente.
- **Seguranca desde a origem:** nenhum dado real, segredo ou material de cliente pertence ao repositorio ou aos bundles distribuiveis.

## Publico do piloto

O primeiro piloto sera de aproximadamente dez pessoas e devera incluir perfis com diferentes niveis de familiaridade tecnica. Windows sera tratado como plataforma primaria; macOS e Linux permanecem alvos suportados.

## Estado atual

Implementado agora:

- visao e principios fundadores;
- contrato inicial de colaboracao;
- roadmap progressivo;
- specs de fundacao, distribuicao, fronteiras de dados e sucesso do piloto;
- estrutura reservada para CLI, bundles, adapters, schemas, migrations e installers.

Ainda nao implementado:

- binario `bcgos`;
- instalador;
- autenticacao com GitHub;
- bundle do OS;
- pipeline de releases;
- update, rollback e assinatura de artefatos.

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

Leia primeiro:

- [Decisoes fundadoras](docs/FOUNDING-DECISIONS.md)
- [Roadmap](ROADMAP.md)
- [Contrato de colaboracao](COLLAB.md)
- [Spec de fundacao](specs/000-foundation.md)

## Confidencialidade

Este repositorio privado armazena somente codigo, templates, contratos e conteudo distribuivel sanitizado. Dados de pessoas, clientes, cases, projetos, memoria operacional, logs e credenciais devem permanecer fora do repositorio e fora dos bundles oficiais.
