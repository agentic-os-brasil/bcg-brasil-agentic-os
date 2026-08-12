---
name: generate-portable-zip
description: Gerar o ZIP full portable do Maestro (macOS Apple Silicon e/ou Windows amd64) em um único fluxo, do commit atual até `releases/<versão>/`. Orquestra `dev/release candidate` → build de bootstrappers → `release-export`. Usar quando alguém pedir "gera o zip portable", "quero um ZIP novo pra testar", "monta o local-beta portable".
---

# Gerar ZIP portable full (local-beta canary)

Use esta skill como ponto de entrada único quando o pedido é "quero um ZIP
portable pra testar", "gera o portable dessa versão", "monta o local-beta". Ela
orquestra as três fases que o operador tinha que costurar à mão:

1. produzir o release candidato assinado (canary);
2. construir os bootstrappers macOS arm64 e Windows amd64;
3. chamar a factory `release-export` para emitir o(s) ZIP(s) + `.sha256` +
   `.provenance.json` em `releases/<versão>/`.

Esta skill **não** substitui `release-export`: ela é uma casca fina que resolve
os inputs e delega. Para publicação em GitHub Release, empacotamento self-
contained Windows, ou qualquer coisa fora do happy path local-beta, ir direto
para `dev/skills/release-export/SKILL.md`.

## Quando NÃO usar

- Publicação oficial (Developer ID / Authenticode / notarização) — usar o
  pipeline de release owner-approved.
- Instalador DMG macOS — usar `maestro-local-beta-installer`.
- Empacotar EXE self-contained Windows — passo separado do `release-export`.
- Republicar uma versão já em `releases/` — a factory recusa por design.

## Pré-condições

1. Working tree limpo em uma branch `codex/`; commit atual é o SHA do release.
2. Versão semver `MAJOR.MINOR.PATCH` inédita: sem tag `maestro-v<versão>` no
   remoto e sem `releases/<versão>/` local.
3. `go run ./dev/harness validate --full` passa.
4. Registry de autoridade canary aprovado disponível no host (caminho absoluto
   + SHA-256 lowercase). Não inventar registry; usar o pin ativo.

## Fluxo padrão

Executar da raiz do repositório. O driver desta skill assume que o operador
passa apenas versão + alvo + registry; ele deriva o resto.

```sh
dev/skills/generate-portable-zip/scripts/generate.sh \
  --version 0.2.0 \
  --targets macos,windows \
  --authority-registry /abs/path/to/release-authority-registry.json \
  --authority-registry-sha256 LOWERCASE_SHA256
```

Fases:

1. **Candidate**: `go run ./dev/release candidate --version <v> --channel canary
   --output <tmp>/signed-release`. Produz a árvore assinada consumida pela
   factory.
2. **Bootstrappers**: `go run ./dev/release binary` para cada alvo pedido,
   emitindo `bcgos-bootstrap_<v>_darwin_arm64` e/ou
   `bcgos-bootstrap_<v>_windows_amd64.exe`. SHA-256 lowercase é calculado logo
   após o build.
3. **Export**: chama `dev/skills/release-export/scripts/export-release.sh` com
   os pins resolvidos. Se apenas um alvo foi pedido, os pins do outro são
   omitidos e o export-release respeita a lista.
4. **Verificação mínima**: `unzip -t` no ZIP, confere `.sha256` batendo, lê
   `EXPORT-METADATA.txt` e imprime o resumo.

Nada é publicado no GitHub por padrão. Para publicar, chamar
`release-export` diretamente com `--publish-github --confirm-publish`.

## Saída esperada

Em `releases/<versão>/`:

- `Maestro-Portable-<v>-macos-arm64-local-beta-unsigned.zip` (se `--targets`
  incluir `macos`);
- `Maestro-Portable-<v>-windows-amd64-local-beta-unsigned.zip` (se incluir
  `windows`);
- `.sha256` e `.provenance.json` correspondentes;
- `EXPORT-METADATA.txt` (commit, tag, versão, digests).

Status permanece `unsigned-controlled-canary` — não é production release.

## Falhas comuns

- **`releases/<versão>/` já existe** → escolher versão nova; a factory recusa
  sobrescrita por segurança.
- **Registry SHA-256 não bate** → o registry mudou; obter o pin novo do owner
  antes de continuar.
- **`harness validate --full` falha** → resolver ANTES de tentar generate.
  Falhas comuns incluem `skills-index catalog.json` stale (`go run
  ./dev/harness skills-index` regenera).
- **Bootstrapper Windows não é PE GUI** → a factory recusa; conferir que o
  build usou `-H=windowsgui`.

## Limites

- Doc-only wrapper: qualquer mudança no comportamento real está em
  `release-export` ou `dev/release`. Não duplicar lógica aqui.
- Não executar em produção. Este é um caminho de local-beta canary.
- Se qualquer passo falhar, parar e reportar; nunca continuar com um pin
  ausente ou `--skip-signature`.

## Evidência mínima

Além do que `release-export` já registra, guardar:

- comando exato executado (versão, targets, registry, sha256);
- SHA do commit de origem;
- timestamps de cada fase;
- resultado do `unzip -t` por ZIP produzido.
