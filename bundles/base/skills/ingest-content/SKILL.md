---
name: ingest-content
description: Registra o conteúdo de um documento profissional (PDF, arquivo Office, página web salva, imagem, email) na memória local do Maestro em `data/memory/`. Use quando o pedido for "ingerir", "processar documento", "adicionar este PDF na memória", "salvar este material" ou equivalente.
---

# Ingest Content

Registrar um documento apontado pelo usuário como memória local do Maestro. Ler o arquivo com a ferramenta Read, sintetizar em Markdown enxuto e gravar em `data/memory/` via Write. O conteúdo original nunca é copiado para dentro da workspace, apenas referenciado por caminho.

## Interaction profile

Resolver `interaction-profile` se disponível. O perfil ajusta profundidade de explicação e sugestões opcionais, nunca o envelope de segurança nem o destino da escrita.

## Contrato de comunicação

- Uma pergunta por vez quando faltar informação.
- Sem "você", "tu" ou "te". Preferir impessoal ou 3ª pessoa.
- Sem em-dash ("—") em texto externo. Usar vírgula, dois pontos ou parênteses.
- Nunca pedir terminal, edit de JSON, script ou instalação de dependência.
- Nunca enviar o conteúdo do documento para provedor remoto como fallback.

## Fluxo

1. **Confirmar workspace.** Verificar que `${CLAUDE_PROJECT_DIR}/data/memory/` existe. Se ausente, orientar: "feche a pasta Maestro no Claude Code e reabra. Na próxima abertura a workspace é criada automaticamente." e parar.

2. **Identificar a fonte.** Perguntar (uma linha apenas) qual é o caminho absoluto do documento se ainda não foi fornecido. Aceitar PDF, arquivo de texto, Markdown, HTML salvo ou imagem com texto simples.

3. **Ler o conteúdo com a ferramenta Read.** Escopos suportados nesta release:
   - PDF texto-nativo: extração direta via Read.
   - Markdown, HTML, TXT, JSON, CSV: extração direta via Read.
   - Imagem com texto: Read entrega o conteúdo visual ao modelo, que transcreve o texto relevante.
   - Office (DOCX, XLSX, PPTX) e PDF texto-nativo pesado: invocar antes a skill `python-env-bootstrap` com pacote `markitdown` para preparar UV + venv em `data/runtime/venv` e instalar MarkItDown sob demanda. Depois rodar `data/runtime/venv/bin/python -m markitdown <caminho>` e capturar o Markdown. Nunca instalar Python global nem chamar `pip` do sistema.
   - PDF 100% escaneado (imagem-pura, sem camada de texto): MarkItDown retorna Markdown vazio ou incompleto. Detectar antes de tentar extração; se detectado, cair na rota "Fora do escopo" (copiar-colar via aplicação nativa) sem fingir sucesso.

4. **Sintetizar em Markdown.** Produzir um resumo estruturado com:
   - Título curto do documento.
   - Origem: caminho absoluto do arquivo original, tamanho aproximado, data de ingestão.
   - Sumário em 3 a 8 bullets do conteúdo principal.
   - Decisões, números ou trechos citáveis que o usuário provavelmente vai querer resgatar depois.
   - Ao final, uma linha "Ver original em: <caminho absoluto>" para permitir retorno à fonte.

5. **Escolher o destino em `data/memory/`.** Perguntar em uma frase qual é o tópico ("finanças", "cliente X", "leitura pessoal", etc.) e gravar em `data/memory/<topico>/<slug-do-doc>.md`. Se o subdiretório do tópico não existir, criá-lo com o mesmo Write (o Write cria diretórios intermediários).

6. **Confirmar registro.** Reportar em uma linha: tópico, caminho relativo do arquivo criado, número aproximado de bullets no sumário. Não colar o sumário na conversa salvo se solicitado.

## Invariantes

- O documento original nunca é movido nem copiado, permanece no local escolhido pelo usuário.
- Credenciais, tokens, dados de cliente ou material sensível dentro do documento nunca são gravados em `data/memory/` sem confirmação explícita do usuário sobre o que preservar.
- Uma ingestão que falha ou é interrompida não altera `data/memory/`.
- Nenhuma chamada a provedor remoto é feita nesta skill.
- A skill nunca grava fora de `data/memory/`.

## Fora do escopo desta release

- OCR de imagens complexas e PDFs 100% escaneados (imagem pura sem camada de texto): depende de runtime OCR dedicado ainda não incluído no ZIP do Maestro. Nesses casos, informar de forma direta e oferecer alternativa: abrir o arquivo na aplicação nativa, copiar o texto relevante e colar no chat; a skill grava o material colado mantendo o campo "Origem" apontando para o caminho absoluto do arquivo original.
- DOCX/XLSX/PPTX e PDFs texto-nativos são cobertos via `python-env-bootstrap` + MarkItDown (passo 3).

## Encerramento

Uma linha com tópico, caminho absoluto do arquivo criado dentro de `data/memory/`, e (se aplicável) o item deixado para release futura. Se nada foi gravado, dizer explicitamente por que a ingestão não ocorreu e qual é o próximo passo seguro.
