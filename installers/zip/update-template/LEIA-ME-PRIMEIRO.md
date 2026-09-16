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
5. Inicie o Claude Code **a partir da raiz da nova pasta `Maestro`**. Abrir a
   pasta como diretório adicional não prova que as configurações do projeto
   foram carregadas:
   - Mac: no Terminal, execute `cd "/caminho/para/Maestro"` e depois
     `claude --debug hooks`;
   - Windows: no PowerShell, execute
     `Set-Location "C:\caminho\para\Maestro"` e depois
     `claude --debug hooks`.
   Se aparecer um pedido de confiança, confira o caminho exato e aceite somente
   a nova pasta extraída do ZIP cujo SHA-256 foi validado. Não aprove outra
   pasta por engano.
6. Dentro da nova sessão, execute `/status` e `/hooks`. Só continue se o
   diretório do projeto for a nova pasta `Maestro`, se não houver erro de
   configuração e se os seis handlers do projeto aparecerem em `/hooks` com
   origem em `.claude/settings.json`.
7. Se os hooks não aparecerem ou não dispararem, pare e siga
   `DIAGNOSTICO-HOOKS.md`. Arquivos presentes e executáveis não bastam para
   considerar as automações ativas.
8. Cole todo o conteúdo de `PROMPT-2-VERIFICAR.txt` no chat.

## Quando considerar concluído

O Maestro deve confirmar, com evidência:

- versão {{TO_VERSION}};
- todos os arquivos preexistentes de `data/` preservados conforme o manifesto
  criado antes da troca. A primeira abertura pode mudar somente os metadados de
  lifecycle documentados e criar backfills ausentes;
- runtime correto para a plataforma: PowerShell nativo no Windows; Bash e
  Python 3 no Mac;
- `/status` sem erro de configuração e `/hooks` mostrando os seis handlers do
  projeto;
- hooks de início, prompt, proteção de escrita e anúncio carregados **e
  executados**, não apenas presentes no disco;
- projeções de agentes presentes;
- uma chamada real ao Agent `yoda`, com retorno observado.

Se qualquer item falhar, não apague a pasta antiga. Feche o Claude, renomeie a
nova pasta para `Maestro-falhou-{{TO_VERSION}}` e devolva o nome `Maestro` para
`Maestro-old-{{FROM_VERSION}}`. Guarde a pasta antiga por pelo menos sete dias,
mesmo quando tudo passar.

Se `/status` indicar que uma política corporativa bloqueou hooks de projeto, ou
se o Windows informar que uma política bloqueou scripts PowerShell, não altere
a política e não tente contornar o bloqueio. Faça o rollback acima e envie o
diagnóstico ao time BCG Brasil AI.

## Perfil suportado

- macOS: Claude Code, Bash e Python 3.
- Windows: Claude Code com Windows PowerShell 5.1 ou PowerShell 7. Os hooks não
  dependem de Git Bash nem Python.
