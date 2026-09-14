# Atualização do Maestro {{FROM_VERSION}} para {{TO_VERSION}}

Este kit atualiza o núcleo do Maestro sem mudar a estrutura que você já usa.
Seu conteúdo pessoal continua em `data/`. O novo ZIP não contém uma pasta
`data/`. Não extraia o novo ZIP por cima da instalação atual.

## Antes de mexer nas pastas

1. Abra seu Maestro atual normalmente.
2. Cole todo o conteúdo de `PROMPT-1-PREPARAR.txt` no chat.
3. Só continue quando o Maestro disser que a instalação atual é a versão
   {{FROM_VERSION}}, que `data/` foi encontrada e que o manifesto de baseline
   `.maestro-update-baseline-{{FROM_VERSION}}.json` foi validado.
4. Feche o Claude Code e qualquer terminal aberto dentro da pasta `Maestro`.

## Fazer a atualização

1. Renomeie a pasta atual `Maestro` para `Maestro-old-{{FROM_VERSION}}`.
2. Escolha e extraia o ZIP da sua plataforma ao lado dela:
   - Windows: `Maestro-v{{TO_VERSION}}-windows-powershell.zip`
   - Mac: `Maestro-v{{TO_VERSION}}-macos.zip`
   O ZIP criará uma nova pasta chamada `Maestro`.
3. Copie somente a pasta `data/` de `Maestro-old-{{FROM_VERSION}}` para dentro
   da nova pasta `Maestro`.
4. Não copie `.claude/`, `bundles/`, `CLAUDE.md` ou outros arquivos antigos.
   Eles são o núcleo que está sendo atualizado.
5. Abra a nova pasta `Maestro` no Claude Code.
6. Cole todo o conteúdo de `PROMPT-2-VERIFICAR.txt` no chat.

## Quando considerar concluído

O Maestro deve confirmar, com evidência:

- versão {{TO_VERSION}};
- todos os arquivos preexistentes de `data/` preservados conforme o manifesto
  criado antes da troca. A primeira abertura pode mudar somente os metadados de
  lifecycle documentados e criar backfills ausentes;
- runtime correto para a plataforma: PowerShell nativo no Windows; Bash e
  Python 3 no Mac;
- hooks de início, prompt, proteção de escrita e anúncio carregados;
- projeções de agentes presentes;
- uma chamada real ao Agent `yoda`, com retorno observado.

Se qualquer item falhar, não apague a pasta antiga. Feche o Claude, renomeie a
nova pasta para `Maestro-falhou-{{TO_VERSION}}` e devolva o nome `Maestro` para
`Maestro-old-{{FROM_VERSION}}`. Guarde a pasta antiga por pelo menos sete dias,
mesmo quando tudo passar.

Se o Windows informar que uma política corporativa bloqueou scripts
PowerShell, não altere a política e não tente contornar o bloqueio. Faça o
rollback acima e envie o diagnóstico ao time BCG Brasil AI.

## Perfil suportado

- macOS: Claude Code, Bash e Python 3.
- Windows: Claude Code com Windows PowerShell 5.1 ou PowerShell 7. Os hooks não
  dependem de Git Bash nem Python.
