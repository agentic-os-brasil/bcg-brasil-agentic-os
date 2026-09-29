# Teste de qualificação do update {{FROM_VERSION}} → {{TO_VERSION}}

Este roteiro fecha a evidência em três camadas. Uma camada não substitui a
outra.

## 0. Preflight de descoberta dos hooks

O Claude Code deve ser iniciado a partir da raiz da pasta extraída `Maestro`.
Configurações de hooks não são comprovadas apenas por abrir a pasta como um
diretório adicional.

No Mac:

```bash
cd "/caminho/para/Maestro"
claude doctor
claude --debug hooks
```

No Windows PowerShell:

```powershell
Set-Location "C:\caminho\para\Maestro"
claude doctor
claude --debug hooks
```

Dentro da sessão, execute `/status` e `/hooks` quando disponíveis. Se não
existirem nesta versão, use o traço machine-readable descrito em
`DIAGNOSTICO-HOOKS.md`. Registre:

- diretório raiz reconhecido;
- confiança aceita somente para a raiz exata extraída do ZIP validado;
- versão do Claude Code;
- fontes de configuração mostradas por `/status` ou pelo `init` do traço;
- ausência de erro de schema;
- nove handlers em `.claude/settings.json` e execução correlacionada no traço;
- qualquer política gerenciada, como `allowManagedHooksOnly` ou
  `disableAllHooks`, que impeça hooks do projeto.

Se nem `/hooks` nem o traço comprovarem os handlers, pare e siga
`DIAGNOSTICO-HOOKS.md`. Não
continue o teste de qualificação como se a automação estivesse ativa.

Não anexe nem compartilhe o log bruto de `claude --debug hooks`: ele pode conter
prompts e caminhos locais. Registre apenas evento, matcher, exit code e stderr
sanitizado.

## 1. Contrato determinístico do ZIP

No ZIP macOS, o mantenedor executa `installers/zip/eval-release.sh`. No ZIP
Windows, executa `acceptance/zip-update/native-smoke.ps1`. O resultado deve ter
zero `FAIL`; o gate macOS também deve ter zero `SKIP`. Em conjunto, cobrem
estrutura, checksum, scaffold, settings, runtime da plataforma, guard de casos,
router, anúncio e projeções de agentes.

## 2. Execução nativa por plataforma

No macOS, rode o gate em Terminal/Bash sobre o ZIP `macos`. No Windows, rode
`acceptance/zip-update/powershell-hook-smoke.ps1` em Windows PowerShell 5.1 e,
quando disponível, repita em PowerShell 7, sobre o ZIP `windows-powershell`.
Em ambos os casos, registre:

- sistema e arquitetura;
- versão do Claude Code e runtime da plataforma;
- SHA-256 do ZIP;
- contagem final de PASS/FAIL/SKIP;
- resultado do caminho nativo do guard de casos.

Cada plataforma tem seu próprio SHA-256. O recibo deve apontar para o digest
exato do ZIP testado. Se qualquer ZIP for reconstruído, sua qualificação deve ser
repetida.

## 3. Sessão real do Claude Code

Abra a sessão pelo preflight acima e cole `PROMPT-2-VERIFICAR.txt`. Para um
recibo automatizado no Windows, execute de
forma atendida `acceptance/zip-update/live-agent-qualification.ps1` em PowerShell 5.1
e 7; ele não usa Git Bash nem Python. A evidência mínima é:

- SessionStart executado na abertura;
- os nove handlers comprovados pela UI ou por settings mais traço;
- UserPromptSubmit entregando a rota de Yoda;
- ferramenta Agent chamada com `subagent_type: yoda`;
- PreToolUse devolvendo o pedido de anúncio ao hub;
- retorno real de Yoda à sessão principal.

Uma busca de arquivos ou a saída direta de um script não prova que o Claude
carregou os hooks nem que houve chamada real. Sem o preflight e os cinco
eventos acima, registre `UNAVAILABLE` ou `FAIL`, nunca `PASS`.

## Critério de release

O bug fix pode ser distribuído apenas quando cada ZIP específico de plataforma tiver:

- gate determinístico verde;
- qualificação macOS verde;
- qualificação Windows PowerShell 5.1 verde e PowerShell 7 verde quando disponível;
- chamada real do Agent observada pelo menos uma vez em cada plataforma;
- atualização {{FROM_VERSION}} → {{TO_VERSION}} preservando cada arquivo
  preexistente de `data/` em ambas, sem modificar os originais;
- `/status` e `/hooks` ou o traço machine-readable comprovando que a
  configuração do projeto foi carregada em cada plataforma.

## Contrato v2 e host escolhido

O arquivo `UPDATE-CONTRACT.json` deste kit é a autoridade para versões, checks
obrigatórios, opcionais, revisão de Yoda e bindings do recibo. A cópia externa
deve ser idêntica à que acompanha o payload escolhido. Este kit foi preparado
para `{{PLATFORM}}`; use somente o ZIP correspondente à plataforma real.

O recibo durável fica em `brain/.maestro/updates/update-{{TO_VERSION}}.json`.
A migração precisa estar committed e o inventário original de `data/` deve
permanecer verificável; agentes e workspaces retidos são lidos pelo adaptador
legado validado. Um marcador `.initialized` não substitui o recibo de migração.

No Claude são nove handlers: dois SessionStart, um UserPromptSubmit, dois
PreToolUse e quatro Stop. No Codex, verifique os eventos SessionStart,
UserPromptSubmit, PreToolUse, PostToolUse e Stop de `.codex/hooks.json`. A
projeção portátil requer binding atendido à raiz confirmada da instalação antes
do uso; presença da configuração não prova binding nem execução nativa. Use a
receita do runbook para o host escolhido e registre configured, observed e
native-qualified separadamente. Não aplique comandos exclusivos do Claude ao
Codex. Uma chamada real de subagente e seu retorno exigem evidência do host.

caseOS, ferramentas Python opcionais e suporte a goal são checks opcionais.
Ausência deve aparecer como unavailable com motivo; não autoriza instalar
dependências, autenticar, exportar dados ou contornar limites do host. O handler
Stop de índice pode executar corretamente enquanto a indexação Python continua
unavailable. Migração e hooks críticos do Windows usam o runtime Go empacotado
e PowerShell nativo, sem Git Bash ou Python.

Este kit é um candidato de engenharia. Checksum e validação offline não provam
qualificação nativa, assinatura, autorização de distribuição ou release.
