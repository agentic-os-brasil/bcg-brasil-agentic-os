# Installers/zip — factory de release Maestro

## O que este diretório é

Factory que produz `Maestro-v<version>.zip` — o entregável para os 40 beta users. Distribuição é 100% por email; não há check de versão remoto.

## Layout

```
installers/zip/
├── README.md                    ← este arquivo
├── build-release.sh             ← script principal (roda no macOS do maintainer)
├── build-update-package.sh      ← empacota release + receita de update
├── eval-release.sh              ← gate determinístico do ZIP de release
├── eval-update-package.sh       ← gate do kit e ensaio de preservação de data/
├── update-template/             ← instruções e prompts entregues no kit
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
installers/zip/build-release.sh 0.1.0 macos
installers/zip/build-release.sh 0.1.0 windows-powershell
```

Saída em `dist/`:
- `Maestro-v0.1.0-macos.zip` e seu `.sha256`
- `Maestro-v0.1.0-windows-powershell.zip` e seu `.sha256`

## Como buildar um kit de atualização

O kit envolve o ZIP já construído e validado. Para atualizar a versão em campo
0.1.11 para 0.1.12:

```bash
bash installers/zip/build-update-package.sh 0.1.11 0.1.12
bash installers/zip/eval-update-package.sh \
  --zip dist/Maestro-Update-v0.1.12.zip \
  --from-version 0.1.11 --to-version 0.1.12
```

O resultado `Maestro-Update-v0.1.12.zip` contém os releases de Mac e Windows,
checksums, instruções
de rollback e dois prompts: um para o Maestro antigo preparar o update e outro
para o novo Maestro verificar hooks e fazer o canário real de Yoda. O wrapper e
o release continuam sem `data/`.

Antes da distribuição, rode o ZIP macOS com
`acceptance/zip-update/native-smoke.sh` e o ZIP Windows com
`acceptance/zip-update/native-smoke.ps1` em PowerShell nativo, depois o canário
opt-in de Agent. Cada recibo deve apontar para o SHA-256 do artefato exato. O workflow manual
`ZIP update native canary` verifica a portabilidade da factory, mas não
substitui os canários do mesmo artefato final nas duas máquinas.

## Fluxo de release

1. `git tag v0.1.0 && git push --tags` (após code freeze).
2. Rode `build-release.sh 0.1.0`.
3. Para uma instalação nova, envie o ZIP específico da plataforma. Para atualizar
   uma versão em campo, envie o `Maestro-Update-v*.zip`, que conduz o mesmo
   ritual definido por `README-INSTALL.md`.

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

O ZIP não contém `data/`, então a workspace do usuário nunca é sobrescrita por uma extração. Isso não torna extract-over seguro: extrair por cima deixa arquivos de versões anteriores misturados com a nova. O ritual publicado em `user-template/README-INSTALL.md` manda renomear a pasta antiga, extrair a nova e **copiar** a `data/` para dentro dela. Esse é o único fluxo a divulgar.

## Deprecação do bcgos

Esta factory substitui completamente o instalador Go (`cmd/bcgos`). Todas as referências ao `bcgos` como runtime foram removidas do produto — hooks, skills e registry atualizados em `refactor/remove-bcgos-cli`.
