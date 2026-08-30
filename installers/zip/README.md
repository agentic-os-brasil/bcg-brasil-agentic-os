# Installers/zip — factory de release Maestro

## O que este diretório é

Factory que produz dois artefatos portáteis e específicos de plataforma:

- `Maestro-Portable-<version>-macos-arm64-local-beta-unsigned.zip`
- `Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip`

Cada artefato inclui o Hub, o bootstrapper e o CLI instalado daquela
plataforma. O sufixo `unsigned` é deliberado: SHA-256 e verificação local não
equivalem a assinatura organizacional, notarização, release-ready ou
pilot-ready.

## Layout

```
installers/zip/
├── README.md                    ← este arquivo
├── build-release.sh             ← script principal (roda no macOS do maintainer)
└── user-template/               ← estrutura da pasta que o usuário recebe
    ├── WELCOME.md
    ├── README-INSTALL.md
    └── .claude/
        ├── settings.json
        └── hooks/
            └── first-run-scaffold.sh
```

O `build-release.sh` copia `user-template/` + `bundles/` para uma pasta
temporária, injeta `VERSION`, compila `bcgos` e `bcgos-bootstrap` para cada
target, gera `managed/install-manifest.json` com o digest do CLI e produz os
ZIPs em ordem e timestamps determinísticos.

## Como buildar

```bash
installers/zip/build-release.sh 0.1.13
# ou um único target:
installers/zip/build-release.sh 0.1.13 macos-arm64
```

Saída em `dist/`: os dois ZIPs `Maestro-Portable-*`, seus sidecars `.sha256` e,
quando ambos são gerados, `Maestro-v<version>.zip` como alias macOS temporário
para o avaliador legado do Hub. O alias não é um binário universal.

## Fluxo de release

1. Concluir os gates de código e revisão; não criar tag ou publicar a partir
   desta factory automaticamente.
2. Rodar `build-release.sh <version>`.
3. Rodar `eval-release.sh --zip <artefato compatível com o host>`.
4. Submeter os artefatos específicos de plataforma aos gates separados de
   assinatura, notarização, clean-device e publicação.

Esta factory não faz hosting, assinatura, notarização, publicação nem declara
prontidão de piloto. O manifesto é local ao artefato e serve apenas para o
bootstrapper verificar target, versão, caminho e digest do CLI antes do uso.

## Design do hook first-run-scaffold.sh

- Cria `data/{agents,memory,profile,workspaces}/` na primeira sessão.
- Idempotente (marker em `data/.initialized`).
- Coloca um `data/README.md` explicando que essa pasta é preservada em updates.

## Separação core vs workspace

- **Core** (dentro do ZIP, sobrescrito em cada release):
  `VERSION`, `managed/`, `.claude/`, `bundles/`, `CLAUDE.md`, `WELCOME.md`, `README-INSTALL.md`
- **Workspace** (do usuário, criado no first-run, nunca no ZIP):
  `data/` inteiro

O ZIP não contém `data/`, então a workspace do usuário nunca é sobrescrita por uma extração. Isso não torna extract-over seguro: extrair por cima deixa arquivos de versões anteriores misturados com a nova. O ritual publicado em `user-template/README-INSTALL.md` manda renomear a pasta antiga, extrair a nova e **copiar** a `data/` para dentro dela. Esse é o único fluxo a divulgar.

## Dois modos de entrada

- **Hub:** abrir a raiz `Maestro/`; o first-run verifica o CLI e preserva o
  fluxo conversacional existente.
- **Repo/Worktree:** o CLI instalado oferece somente
  `workspace enroll|status|repair|remove`. A projeção escreve hooks locais com
  caminho absoluto para esse CLI e mantém dados privados fora do checkout.

O CLI é control plane estreito, não uma segunda experiência de produto e não
fica no `PATH` global. O contrato completo está em
[`specs/055-direct-repository-worktree-entry.md`](../../specs/055-direct-repository-worktree-entry.md).
