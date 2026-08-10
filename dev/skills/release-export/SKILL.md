---
name: release-export
description: Exportar releases Maestro de forma reproduzível, incluindo o ZIP portátil Windows, checksums, provenance e publicação opcional em GitHub Releases. Usar quando alguém pedir para gerar, empacotar, guardar em releases/, validar ou publicar um release.
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
6. Verificar o ZIP, o checksum, o provenance e a lista de assets antes de
   qualquer publicação.
7. Publicar no GitHub somente com `--publish-github --confirm-publish`, depois
   reconsultar a release/tag e guardar a URL, SHA do commit e digests.

## Exportar o ZIP portátil Windows

Execute a partir da raiz do repositório, preferencialmente numa factory Windows
porque a inspeção real do seed e do Authenticode requer Windows:

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
- Se a factory não tiver Windows ou não conseguir inspecionar Authenticode/seed,
  reportar `unavailable`; não substituir por uma função falsa fora dos testes.
- Se a pasta `releases/<versão>/` já contiver arquivos, parar para evitar
  sobrescrever ou misturar artefatos de runs diferentes.
- Depois de exportar, a instalação Windows ainda exige extrair o ZIP em caminho
  fixo e executar `Activate-Maestro.cmd` uma vez. Isso não prova clean-device,
  endpoint approval, pilot-ready ou produção.

## Evidência mínima de entrega

Registrar separadamente:

- commit de origem e versão/tag;
- caminho local e URL do GitHub Release, quando publicado;
- SHA-256 do ZIP, checksum, provenance, registry e bootstrapper;
- resultado de `go run ./dev/release verify`/`portable-windows` e `unzip -t`;
- status dos gates externos e qualquer evidência Windows real.
