# Atualização do Maestro {{FROM_VERSION}} para {{TO_VERSION}}

Este kit atualiza o núcleo do Maestro sem mudar a estrutura que você já usa.
Seu conteúdo pessoal continua em `data/`. O novo ZIP não contém uma pasta
`data/`. Não extraia o novo ZIP por cima da instalação atual.

## Antes de mexer nas pastas

1. Abra seu Maestro atual normalmente.
2. Cole todo o conteúdo de `PROMPT-1-PREPARAR.txt` no chat.
3. Só continue quando o Maestro disser que a instalação atual é a versão
   {{FROM_VERSION}}, que `data/` foi encontrada e que o pré-check terminou.
4. Feche o Claude Code e qualquer terminal aberto dentro da pasta `Maestro`.

## Fazer a atualização

1. Renomeie a pasta atual `Maestro` para `Maestro-old-{{FROM_VERSION}}`.
2. Extraia `Maestro-v{{TO_VERSION}}.zip` ao lado dela. O ZIP criará uma nova
   pasta chamada `Maestro`.
3. Copie somente a pasta `data/` de `Maestro-old-{{FROM_VERSION}}` para dentro
   da nova pasta `Maestro`.
4. Não copie `.claude/`, `bundles/`, `CLAUDE.md` ou outros arquivos antigos.
   Eles são o núcleo que está sendo atualizado.
5. Abra a nova pasta `Maestro` no Claude Code.
6. Cole todo o conteúdo de `PROMPT-2-VERIFICAR.txt` no chat.

## Quando considerar concluído

O Maestro deve confirmar, com evidência:

- versão {{TO_VERSION}};
- `data/` preservada;
- Bash e Python 3 disponíveis no perfil suportado;
- hooks de início, prompt, proteção de escrita e anúncio carregados;
- projeções de agentes presentes;
- uma chamada real ao Agent `yoda`, com retorno observado.

Se qualquer item falhar, não apague a pasta antiga. Feche o Claude, renomeie a
nova pasta para `Maestro-falhou-{{TO_VERSION}}` e devolva o nome `Maestro` para
`Maestro-old-{{FROM_VERSION}}`. Guarde a pasta antiga por pelo menos sete dias,
mesmo quando tudo passar.

## Perfil suportado

- macOS: Claude Code, Bash e Python 3.
- Windows: Claude Code executando o projeto com Git Bash e Python 3 resolvível
  como `python`, `python3` ou `py -3`.
- PowerShell sem Git Bash não faz parte desta atualização.
