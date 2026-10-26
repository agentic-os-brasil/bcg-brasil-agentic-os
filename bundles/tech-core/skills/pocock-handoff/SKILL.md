---
name: pocock-handoff
description: "Prepara um documento enxuto para outra sessão continuar o trabalho."
argument-hint: "What will the next session be used for?"
disable-model-invocation: true
---

## Contrato de uso no Maestro

Resolva `$interaction-profile` pelo ID canônico antes de apresentar resultados.
Este método opera no workspace profissional autorizado e segue os contratos de
escopo, dados e autoridade do Maestro. As etapas abaixo não concedem novas
permissões para publicar, enviar mensagens, instalar dependências, acessar
outros casos, executar ações destrutivas ou gastar recursos. Use somente os
adaptadores e ferramentas disponíveis e autorizados; reporte `unavailable`
quando o host não oferecer uma capacidade. Regras locais explícitas de saída,
privacidade e aprovação prevalecem sobre exemplos de diretórios e comandos.
Se o host não tiver uma ferramenta Skill, carregue o método pelo ID no catálogo
instalado; preserve as restrições de invocação explícita pelo usuário.

Write a handoff document summarising the current conversation so a fresh agent can continue the work. Save to the temporary directory of the user's OS (`$TMPDIR`, else `/tmp`; `%TEMP%` on Windows) - not the current workspace.

Include a "suggested skills" section in the document, naming which skills the next agent should call the Skill tool for.

Do not duplicate content already captured in other artifacts (specs, plans, ADRs, issues, commits, diffs). Reference them by path or URL instead.

Redact any sensitive information, such as API keys, passwords, or personally identifiable information.

If the user passed arguments, treat them as a description of what the next session will focus on and tailor the doc accordingly.
