---
name: release-export
description: Exportar releases Maestro de forma reproduzível, incluindo ZIP portátil Windows, instalador Windows autocontido, checksums, provenance e publicação opcional em GitHub Releases. Usar quando alguém pedir para gerar, empacotar, guardar em releases/, validar ou publicar um release.
---

# Exportar releases Maestro

Use esta skill para operar a factory de releases. O repositório é a factory; o
produto distribuído é um artefato versionado em GitHub Releases. Nunca comite
ZIPs, executáveis, chaves, certificados, tokens ou dados de usuário na `main`.

## Fluxo padrão

1. Trabalhar em uma branch `codex/` e confirmar que a árvore está limpa.
2. Usar um commit revisado de `main`; registrar o SHA no pacote de evidência.
3. Confirmar que a versão é semver (`MAJOR.MINOR.PATCH`) e ainda não possui tag
   `maestro-v<versão>`.
4. Usar uma release Canary assinada pelo registry aprovado, um bootstrapper
   Windows versionado e seus SHA-256 exatos. Não inventar pins nem substituir
   uma entrada ausente por `--skip-signature`.
5. Exportar para `releases/<versão>/` usando o script desta skill. A factory
   existente (`go run ./dev/release portable-windows`) continua sendo a única
   autoridade para construir o ZIP.
6. Se a entrega exigir um único executável Windows, anexar o payload ao bridge
   validado com `go run ./dev/release self-contained`. O diretório-fonte deve
   ser o pacote convencional completo produzido pela factory Windows; nunca
   usar o diretório de release assinado ou o ZIP como fonte direta.
7. Verificar o ZIP, o checksum, o provenance, o executável autocontido (quando
   houver) e a lista de assets antes de qualquer publicação.
8. Publicar no GitHub somente com `--publish-github --confirm-publish`, depois
   reconsultar a release/tag e guardar a URL, SHA do commit e digests.

## Exportar o ZIP portátil Windows

Execute a partir da raiz do repositório. A factory pode ser executada em macOS,
Linux ou Windows para **gerar** o artefato Windows. O ZIP produzido é Windows-
only para execução; no macOS, o usuário deve instalar pelo DMG. Em Windows, a
factory executa `seed-status` e usa `Get-AuthenticodeSignature`, mantendo a
exigência de `NotSigned` para o Canary:

```sh
dev/skills/release-export/scripts/export-release.sh \
  --version 0.2.0 \
  --release-directory /abs/path/to/signed-release \
  --authority-registry /abs/path/to/release-authority-registry.json \
  --authority-registry-sha256 LOWERCASE_SHA256 \
  --bootstrapper /abs/path/to/bcgos-bootstrap_0.2.0_windows_amd64.exe \
  --bootstrapper-sha256 LOWERCASE_SHA256
```

O resultado fica em `releases/0.2.0/`:

- `Maestro-Portable-0.2.0-windows-amd64-local-beta-unsigned.zip`;
- o `.sha256` correspondente;
- o `.provenance.json` correspondente;
- `EXPORT-METADATA.txt`, com commit, tag, versão e digests observados.

O ZIP é uma Canary controlada e permanece `unsigned-controlled-canary`. A
factory recusa manifest que não seja `canary`, issuer/key fora do registry,
drift de digest, bootstrapper incompatível, seed divergente e qualquer status
Authenticode diferente de exatamente `NotSigned`.

## Empacotar o instalador Windows em um único EXE

O ZIP continua sendo o handoff portátil padrão. Quando o usuário precisa
receber um único arquivo, use o pacote convencional completo que a factory
Windows já validou (contendo `maestro-installer.exe`, `wizard/`, `release/`,
registry e bootstrapper) como payload do wrapper:

```sh
go run ./dev/release self-contained \
  --base /abs/path/to/windows-package/maestro-installer.exe \
  --source /abs/path/to/windows-package \
  --output /abs/path/to/releases/0.2.0/Maestro-Installer-0.2.0-windows-amd64-self-contained-unsigned.exe
```

O `--base` e o `--output` devem ser arquivos diferentes. O comando cria um
footer autenticado por SHA-256 sobre o payload comprimido, rejeita colisão de
paths e exige que o wrapper e o `maestro-installer.exe` interno sejam PE
Windows GUI (`-H=windowsgui`); uma base CUI é recusada para evitar um EXE que
apenas pisca e encerra. O comando não substitui a verificação Ed25519, o
bootstrapper ou a assinatura nativa. Registre no evidence pack o tamanho e o
SHA-256 do payload, o SHA-256 do EXE e o resultado de uma extração/instalação
em Windows. O arquivo isolado continua `unsigned-candidate` até a etapa
Authenticode aprovada.

## Publicar no GitHub Release

Só publique depois de verificar os gates externos aplicáveis: signing aprovado,
registry autorizado, asset closure, política de tag imutável e aprovação do
release owner. A publicação é deliberadamente opt-in:

```sh
dev/skills/release-export/scripts/export-release.sh \
  --version 0.2.0 \
  --release-directory /abs/path/to/signed-release \
  --authority-registry /abs/path/to/release-authority-registry.json \
  --authority-registry-sha256 LOWERCASE_SHA256 \
  --bootstrapper /abs/path/to/bcgos-bootstrap_0.2.0_windows_amd64.exe \
  --bootstrapper-sha256 LOWERCASE_SHA256 \
  --publish-github --confirm-publish
```

O script exige `gh auth status`, recusa tag/release existente, usa a tag
`maestro-v<versão>`, fixa o target no commit atual e anexa somente o ZIP, seu
checksum, provenance e notas de release disponíveis. Sem `--confirm-publish`,
ele exporta e valida localmente, mas não altera o GitHub.

## Limites e recuperação

- Não chamar esta skill para transformar um candidato unsigned em release
  autenticado; candidato e release assinado são estados diferentes.
- Não publicar um artifact se `go run ./dev/harness validate --full`, a
  verificação do release ou a inspeção do ZIP falhar.
- Se a factory não conseguir validar o seed linker-bound ou a certificate table
  do PE, reportar erro e não substituir a verificação por `--skip-signature`.
- Se a pasta `releases/<versão>/` já contiver arquivos, parar para evitar
  sobrescrever ou misturar artefatos de runs diferentes.
- Depois de exportar, a instalação Windows ainda exige validar o ZIP ou o EXE
  autocontido em um caminho fixo e confirmar o fluxo de ativação. Isso não
  prova clean-device, endpoint approval, pilot-ready ou produção.

## Evidência mínima de entrega

Registrar separadamente:

- commit de origem e versão/tag;
- caminho local e URL do GitHub Release, quando publicado;
- SHA-256 do ZIP, checksum, provenance, registry e bootstrapper;
- resultado de `go run ./dev/release verify`/`portable-windows` e `unzip -t`;
- quando houver EXE autocontido: caminho, SHA-256 do arquivo, tamanho/digest do
  payload e resultado da extração/instalação;
- status dos gates externos e qualquer evidência Windows real.
