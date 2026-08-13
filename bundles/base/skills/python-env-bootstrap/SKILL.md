---
name: python-env-bootstrap
description: Prepara um ambiente Python local isolado (UV + venv em `data/runtime/venv`) e instala pacotes sob demanda, para que skills que dependem de Python (extração de PDF, DOCX, XLSX, PPTX via MarkItDown, etc.) funcionem sem pedir terminal ao usuário. Use quando o pedido for "instalar pacote python", "habilitar markitdown", "preparar python", "ingerir PDF/DOCX/XLSX/PPTX" ou quando outra skill chamar esta como dependência.
---

# Python Env Bootstrap

Preparar de forma idempotente um ambiente Python local isolado para o Maestro. UV como gestor, venv em `data/runtime/venv`, registro em `data/runtime/python-env.json`, regra operacional em `data/owner/self/preferences.md`. Sem terminal, sem edit de JSON pelo usuário.

## Interaction profile

Resolver `interaction-profile` se disponível. O perfil ajusta profundidade de explicação, nunca o envelope de segurança nem o destino de escrita.

## Contrato de comunicação

- Nunca perguntar ao owner antes de instalar UV ou pacotes; a skill opera em modo silencioso por default (ver "Instalação silenciosa" e Tier 1/Tier 2).
- Sem "você", "tu" ou "te". Impessoal ou 3ª pessoa.
- Sem em-dash em texto externo.
- Nunca pedir terminal, edit de JSON, script ou instalação manual de dependência.
- Nunca enviar conteúdo do owner ou de documento para provedor remoto como fallback.

## Explainer de primeira execução

Na primeira invocação em uma workspace (marcador: `data/runtime/python-env.json` ausente), emitir um bloco curto de contexto antes de agir. Sem prompt, sem perguntar permissão. Apenas informar em ≤6 linhas:

> "Preparando ambiente Python local do Maestro. UV (gestor de pacotes da Astral) instalado em `~/.local/bin`, venv criado em `data/runtime/venv`. Pacotes desta skill vivem só nessa workspace, não afetam Python global do sistema.
>
> Primeiro pacote instalado agora: **MarkItDown**. É um conversor open-source da Microsoft que transforma PDF, DOCX, XLSX e PPTX em Markdown limpo, preservando estrutura de títulos, listas e tabelas. Fica importante porque desbloqueia a ingestão de arquivos Office e PDFs texto-nativos direto pelo `ingest-content`, sem pedir para copiar-colar."

Nas invocações seguintes na mesma workspace: silêncio, só reportar o resultado da instalação/verificação em uma linha.

## Instalação silenciosa

- Se `uv` já está presente no PATH: seguir direto.
- Se `uv` está ausente: instalar sem perguntar, via `curl -LsSf https://astral.sh/uv/install.sh | sh`. Registrar em `python-env.json`.
- Se `curl` está ausente: parar (fail-closed) e informar em uma linha; oferecer alternativa de copiar-colar.

## Fluxo

1. **Confirmar workspace.** Verificar `${CLAUDE_PROJECT_DIR}/data/runtime/` (criar se ausente) e `${CLAUDE_PROJECT_DIR}/data/owner/self/`. Se raiz da workspace ausente, orientar reabrir Maestro e parar.

2. **Detectar UV.** `command -v uv`. Se presente, registrar `uv --version` e seguir para passo 4. Se ausente:
   - Verificar `command -v curl`. Se ausente, parar e informar: "Este ambiente não tem `curl`. Sem `curl` a instalação automática de Python via Maestro não é possível nesta release." (fail-closed).
   - Instalar UV silenciosamente: `curl -LsSf https://astral.sh/uv/install.sh | sh`. Confirmar sucesso via `uv --version`.

3. **Registrar UV.** Escrever/atualizar `data/runtime/python-env.json` com `uv_version` fixada nesta bootstrap. Formato canônico:
   ```json
   {
     "uv_version": "0.4.x",
     "python_version": "3.12.x",
     "venv_path": "data/runtime/venv",
     "packages": []
   }
   ```

4. **Criar venv (se ausente).** Verificar `data/runtime/venv/bin/python`. Se ausente: `uv venv data/runtime/venv --python 3.12`. Se `--python 3.12` falhar por ausência de toolchain, UV baixa automaticamente. Registrar `python_version` em `python-env.json`.

5. **Instalar pacote solicitado.** A skill aceita como entrada uma lista de nomes de pacote (via argumento programático da skill chamadora ou via nome citado pelo owner). Duas trilhas:

   **Tier 1 — allowlist curada (silenciosa, sem prompt):**
   - `markitdown` (extração PDF/DOCX/PPTX/XLSX → Markdown)
   - `pypdf` (extração PDF texto-nativo)
   - `python-docx` (leitura DOCX)
   - `openpyxl` (leitura XLSX)
   - `python-pptx` (leitura PPTX)
   - `pandas` (dataframes)

   Para pacotes desta lista: instalar direto via `uv pip install --python data/runtime/venv/bin/python <pkg>`.

   **Tier 2 — pacote fora da allowlist (log-and-proceed):**

   Instalar direto, sem prompt. Antes do install, emitir uma linha de disclosure informativa (não bloqueante):

   > "Instalando `<pkg>` (última versão do PyPI) no venv local. Pacote fora da lista curada do Maestro, registrado em `python-env.json` como `discretionary` para auditoria."

   Se o install falhar (pacote inexistente, conflito de dep), parar com erro claro em uma linha e não registrar.

6. **Registrar pacote.** Após install bem-sucedido, acrescentar entrada em `python-env.json`:
   ```json
   {
     "name": "<pkg>",
     "version": "<versão resolvida>",
     "installed_at": "<ISO-8601>",
     "requested_by": "<skill-caller | owner>",
     "tier": "curated | discretionary"
   }
   ```
   Idempotência: se `<pkg>` já está registrado na mesma versão, no-op silencioso.

7. **Persistir regra operacional (uma vez só).** No primeiro bootstrap com sucesso, acrescentar a `data/owner/self/preferences.md` (criar arquivo se ausente):

   > `## Python`
   >
   > `Todo trabalho Python nesta workspace usa o venv em` `data/runtime/venv`. `Pacotes novos entram por skill python-env-bootstrap, não por pip global.`

   Idempotência: se a seção `## Python` já existe, no-op.

8. **Confirmar.** Reportar em uma linha: pacote(s) instalado(s), caminho do venv, se houve disclosure ou foi silencioso.

## Callers previstos

- `ingest-content`: quando o arquivo é PDF, DOCX, XLSX ou PPTX, chama esta skill com `markitdown` antes de tentar extração.
- Owner direto: "instale pandas", "preciso de <pacote>".
- Skills futuras que dependem de runtime Python específico.

## Invariantes

- Nunca escreve fora de `data/runtime/`, `data/owner/self/preferences.md` e `~/.local/bin/uv`.
- Nunca chama provedor remoto além de `astral.sh` (UV installer) e PyPI (default do `uv pip`).
- Nunca faz fallback para pip global do sistema.
- Uma bootstrap que falha ou é cancelada não altera `python-env.json` nem `preferences.md`.
- Reingerir o mesmo pacote na mesma versão é no-op.

## Failure modes

- `curl` ausente: parar com orientação clara e oferecer alternativa manual (copiar-e-colar via ingest-content).
- Rede indisponível durante `uv pip install`: erro capturado, `python-env.json` não é atualizado, retomável na próxima invocação.
- `uv venv` falha por falta de espaço/permissão: erro claro, parar.

## Fora do escopo

- Gestão de múltiplos venvs paralelos nesta workspace (um venv só).
- Pinning de versões arbitrárias solicitadas pelo owner nesta release (sempre latest do PyPI para simplicidade); pinning explícito entra em release posterior.
- Publicar pacotes ou rodar scripts arbitrários do usuário.

## Encerramento

Uma linha com: pacote(s) resolvido(s), versão(ões), tier (curated/discretionary), caminho do venv. Se nada foi instalado, dizer explicitamente por que (owner disse não, curl ausente, rede) e qual é o próximo passo seguro.
