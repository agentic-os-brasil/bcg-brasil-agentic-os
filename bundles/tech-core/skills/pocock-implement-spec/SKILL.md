---
name: pocock-implement-spec
description: "Coordena a execução dos tickets de uma especificação em uma branch integrada."
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

You have been provided a spec. This spec should have tickets associated with it, describing how to implement the spec.

The issue tracker should have been provided to you. If not, tell the user to run `/pocock-setup-matt-pocock-skills`.

The goal is the entire spec implemented on a single **integration branch**, with every ticket resolved the way the issue tracker closes work.

The tickets are not a list of steps. They are a **task graph** with blocking relationships between them. This means there is always a **frontier** of tickets which are ready to be grabbed.

Communication to and from subagents should be sparse. Communicate primarily through **context pointers**: to the spec, tickets, research notes, and previous commits. Don't duplicate information already available via pointers.

**Implementer subagents** should be run in the background where possible for maximum concurrency.

## Steps

1. Read the spec and tickets to understand the task graph.

2. (optional) Use an **exploration subagent** to conduct any exploration required by the tickets - relevant codebase files or external documentation. Ensure the exploration subagent can save files - it should save its markdown notes in a directory outside the repo, accessible by all future subagents. This lets **implementer subagents** focus on implementation rather than exploration.

3. Create the integration branch. If the issue tracker closes work through PRs, or the user asks for one, open a draft PR after the first merge in step 5 (a branch with no commits ahead of main can't open one), marked as closing the spec and tickets.

4. Use **implementer subagents** to implement each ticket, each in its own worktree on its own branch. Each implementer subagent:
   - confirms its worktree is based on the integration branch before starting, and resets onto it if not;
   - calls the Skill tool with `pocock-tdd` to build the ticket;
   - merges the integration branch tip into its own branch before reporting done

5. Once an **implementer subagent** completes, merge its work to the integration branch with a **merger subagent**.

6. If this changes the **frontier** of available tickets, kick off more **implementer subagents** to work on the new tickets. This allows for maximum concurrency.

7. Once all tickets are complete, call the Skill tool with `pocock-code-review` on the integration branch. Fix all issues raised by the code review in a single **implementer subagent**.

8. If a draft PR exists, mark it ready for review. Otherwise, resolve each ticket the way the issue tracker closes work, and report the integration branch.

9. Clean up all **implementer subagent** worktrees.
