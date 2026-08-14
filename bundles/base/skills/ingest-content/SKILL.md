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
   - Office (DOCX, XLSX, PPTX): tentar extração nativa primeiro em macOS (ver "Extração nativa primeiro (macOS)"); se falhar ou o sistema não for macOS, cair no ambiente Python sob demanda (ver "Leitura de Office sob demanda").

4. **Sintetizar em Markdown.** Produzir um resumo estruturado com:
   - Título curto do documento.
   - Origem: caminho absoluto do arquivo original, tamanho aproximado, data de ingestão.
   - Sumário em 3 a 8 bullets do conteúdo principal.
   - Decisões, números ou trechos citáveis que o usuário provavelmente vai querer resgatar depois.
   - Ao final, uma linha "Ver original em: <caminho absoluto>" para permitir retorno à fonte.

5. **Escolher o destino em `data/memory/`.** Perguntar em uma frase qual é o tópico ("finanças", "cliente X", "leitura pessoal", etc.) e gravar em `data/memory/<topico>/<slug-do-doc>.md`. Se o subdiretório do tópico não existir, criá-lo com o mesmo Write (o Write cria diretórios intermediários).

6. **Confirmar registro.** Reportar em uma linha: tópico, caminho relativo do arquivo criado, número aproximado de bullets no sumário. Não colar o sumário na conversa salvo se solicitado.

## Extração nativa primeiro (macOS)

O princípio é evitar dependências desnecessárias: em macOS existem extratores embutidos que resolvem a maior parte dos DOCX, PPTX e XLSX sem qualquer instalação. Só escalar para o ambiente Python sob demanda quando a extração nativa falhar de fato.

1. **Guarda de plataforma.** Executar via Bash `uname -s` e comparar com `Darwin`. Se o sistema não for macOS (`uname -s` diferente de `Darwin`, ex.: Linux ou WSL), pular esta seção inteira e ir direto para "Leitura de Office sob demanda". Não tentar os comandos abaixo em outras plataformas.

2. **Escolher o comando pela extensão** e capturar a saída padrão via Bash:
   - DOCX: `textutil -convert txt "<caminho-absoluto>" -stdout`
   - PPTX: `unzip -p "<caminho-absoluto>" 'ppt/slides/slide*.xml' | xmllint --xpath "//*[local-name()='t']/text()" - 2>/dev/null`
   - XLSX: `unzip -p "<caminho-absoluto>" xl/sharedStrings.xml xl/worksheets/sheet*.xml`

3. **Detecção explícita de extração falhada.** Uma saída com exit code 0 não garante conteúdo útil (slide só com imagens, células só com fórmulas, SmartArt, textboxes fora do fluxo principal). Considerar a extração falhada, e escalar para "Leitura de Office sob demanda", quando qualquer uma destas condições for verdadeira:
   - Exit code diferente de 0.
   - Saída padrão com menos de 50 caracteres não-brancos.
   - Razão `caracteres_extraidos / tamanho_do_arquivo_em_KB` menor que 5 (por exemplo, um DOCX de 200 KB devolvendo 400 caracteres de texto puro é sinal de perda estrutural).

4. **Fallback de última tentativa antes de escalar** (opcional, só quando o comando principal deu exit 0 mas caiu no critério de vazio): gerar um preview via `qlmanage -t -s 2000 "<caminho-absoluto>" -o /tmp/` e reportar ao usuário o caminho do PNG gerado como material auxiliar. O texto principal continua vindo da rota Python.

5. **Se a extração nativa passa nos critérios do passo 3**, seguir direto para o passo 4 do Fluxo (Sintetizar em Markdown) usando a saída obtida como conteúdo do documento. Nenhum ambiente Python é criado.

Nunca instalar nada nesta seção. `textutil`, `unzip`, `xmllint` e `qlmanage` já vêm com o macOS.

## Leitura de Office sob demanda (DOCX, XLSX, PPTX)

Decisão PYUV autoriza este caminho pontual: um ambiente Python local, pinado e isolado por workspace, criado apenas quando a extração nativa (ver seção anterior) falhou ou não é aplicável (sistema não-macOS).

1. **Verificar se já existe um ambiente pronto.** Ler `data/runtime/python-env.json` (se existir) e confirmar `schema_version=1`, `markitdown_version=0.1.7`, `python_version=3.12`, e que o interpretador existe (`data/runtime/venv/bin/python` no Mac/Linux, `data/runtime/venv\Scripts\python.exe` no Windows). Se tudo bater, pular direto para o passo 5.
2. **Pedir confirmação antes de qualquer instalação**, em uma linha: "Para ler arquivos Word, Excel ou PowerPoint preciso criar um ambiente Python local isolado neste workspace (via uv, download de poucos MB, sem privilégios de administrador). Posso criar agora?" Se o usuário recusar, seguir para "Fora do escopo desta release".
3. **Se confirmado, garantir que `uv` está instalado.** Verificar via Bash (`uv --version`). Se já presente, seguir direto para o próximo passo. Se ausente, decisão UVIN autoriza o próprio Maestro a instalá-lo: pedir confirmação em uma linha separada ("`uv` não está instalado; posso baixar e rodar o instalador oficial de https://astral.sh agora? Não precisa de privilégios de administrador.") e, se aceito, executar via Bash exatamente o instalador oficial publicado pela astral.sh, sem espelhar ou modificar o script:
   - Mac/Linux: `curl -LsSf https://astral.sh/uv/install.sh | sh`
   - Windows: `powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"`
   Se o usuário recusar a instalação do `uv`, ou o comando falhar (rede indisponível, proxy corporativo, etc.), reportar em uma linha e seguir para "Fora do escopo desta release".
4. **Criar o ambiente Python via Bash.** Executar nesta ordem:
   - `uv venv data/runtime/venv --python 3.12`
   - `uv pip install --python <caminho-do-interpretador-do-passo-1> markitdown==0.1.7`
   - Gravar `data/runtime/python-env.json` via Write com `{"schema_version":1,"markitdown_version":"0.1.7","python_version":"3.12","platform":"<darwin|windows|linux>","created_at":"<hora UTC ISO 8601>"}`.
   Se qualquer comando falhar, reportar em uma linha o que falhou e seguir para "Fora do escopo desta release".
5. **Converter o documento.** Executar `<caminho-do-interpretador> -m markitdown "<caminho-absoluto-do-arquivo>"` via Bash e usar a saída padrão como o texto extraído. Continuar a partir do passo 4 do Fluxo (Sintetizar em Markdown).

Nunca recriar ou reinstalar o ambiente quando o receipt já é válido: reutilizar silenciosamente, sem perguntar de novo. A confirmação do passo 2 só é necessária na primeira criação.

## Invariantes

- O documento original nunca é movido nem copiado, permanece no local escolhido pelo usuário.
- Credenciais, tokens, dados de cliente ou material sensível dentro do documento nunca são gravados em `data/memory/` sem confirmação explícita do usuário sobre o que preservar.
- Uma ingestão que falha ou é interrompida não altera `data/memory/`.
- Nenhuma chamada a provedor remoto é feita nesta skill.
- A skill nunca grava fora de `data/memory/`.

## Fora do escopo desta release

PDFs escaneados (imagem pura) e OCR de imagens complexas dependem de runtime local dedicado que ainda não está incluído no ZIP do Maestro. O mesmo se aplica a DOCX, XLSX ou PPTX quando o usuário recusa a criação do ambiente Python sob demanda (seção "Leitura de Office sob demanda") ou quando `uv` não está disponível. Nesses casos:

- Informar de forma direta que a extração nativa desses formatos entra em release posterior do Maestro, ou depende de instalar `uv`, conforme o caso.
- Oferecer a alternativa atual: pedir ao usuário que abra o arquivo na aplicação nativa, copie o texto relevante e cole no chat. A skill então grava o material colado como se tivesse sido lido de um `.md`, mantendo o campo "Origem" apontando para o caminho absoluto do arquivo original.

## Encerramento

Uma linha com tópico, caminho absoluto do arquivo criado dentro de `data/memory/`, e (se aplicável) o item deixado para release futura. Se nada foi gravado, dizer explicitamente por que a ingestão não ocorreu e qual é o próximo passo seguro.
