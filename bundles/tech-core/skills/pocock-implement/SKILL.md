---
name: pocock-implement
description: "Implementa um ticket ou especificação com testes e revisão proporcionais."
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

Implement the work described by the user in the spec or tickets.

If the user passes a ticket reference, fetch it from the issue tracker and state its title before starting. If the reference is ambiguous, ask.

Call the Skill tool with "pocock-tdd" where possible, at pre-agreed seams.

Run typechecking regularly, single test files regularly, and the full test suite once at the end.

Once done, call the Skill tool with "pocock-code-review" to review the work.

Commit your work to the current branch.
