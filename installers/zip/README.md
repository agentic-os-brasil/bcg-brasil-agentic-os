# Installers/zip — factory de release Maestro

## O que este diretório é

Factory que produz `Maestro-v<version>.zip` — o entregável para os 40 beta users. Distribuição é 100% por email; não há check de versão remoto.

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

O `build-release.sh` copia `user-template/` + `bundles/` + `CLAUDE.md` para uma pasta temporária, injeta `VERSION`, e produz o ZIP.

## Como buildar

```bash
installers/zip/build-release.sh 0.1.0
```

Saída em `dist/`:
- `Maestro-v0.1.0.zip`
- `Maestro-v0.1.0.sha256`

## Fluxo de release

1. `git tag v0.1.0 && git push --tags` (após code freeze).
2. Rode `build-release.sh 0.1.0`.
3. Envie `dist/Maestro-v0.1.0.zip` por email para o batch beta com instruções de extract-over.

Sem manifest, sem hosting público, sem checagem automática. O email é o único canal de notificação e o único canal de entrega.

## Design do hook first-run-scaffold.sh

- Cria `data/{agents,memory,profile,workspaces}/` na primeira sessão.
- Idempotente (marker em `data/.initialized`).
- Coloca um `data/README.md` explicando que essa pasta é preservada em updates.

## Separação core vs workspace

- **Core** (dentro do ZIP, sobrescrito em cada release):
  `VERSION`, `.claude/`, `bundles/`, `CLAUDE.md`, `WELCOME.md`, `README-INSTALL.md`
- **Workspace** (do usuário, criado no first-run, nunca no ZIP):
  `data/` inteiro

Extract-over funciona porque o ZIP não contém `data/` — ficheiros no destino que não estão no ZIP são preservados por Finder/Explorer.

## Deprecação do bcgos

Esta factory substitui completamente o instalador Go (`cmd/bcgos`). Todas as referências ao `bcgos` como runtime foram removidas do produto — hooks, skills e registry atualizados em `refactor/remove-bcgos-cli`.
