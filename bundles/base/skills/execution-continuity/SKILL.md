---
name: execution-continuity
description: Register, pause and resume bounded professional work with local checkpoints across sessions. Use for "register this task", "pause and checkpoint", "resume where I left off", "prep a version", or when work must survive a session close.
---

# Execution Continuity

## MCP product and plugin

When the installed runtime exposes `maestro_context`, confirm its admitted
workspace and use the current `maestro_query`/`maestro_apply` schemas. The product
ledger owns this route; model input selects no HOME or workspace root.

1. For a new owner-requested task, query `execution.list` and avoid an equivalent
   duplicate. Create with `execution.create`, the objective, next step and the
   selected existing workspace-relative artifacts. This binds their snapshot
   contract; it does not create missing artifacts or execute their contents.
2. Use `execution.start` with the actual current revision. It returns the
   attempt ID used by subsequent writes. Produce requested artifacts only
   through the separately authorized workspace or host capability.
3. Before pausing, save the owner-selected concise summary and next step with
   `execution.checkpoint`, then `execution.pause`, carrying the actual revision
   and attempt ID at each step. A lifecycle hook does not supply checkpoint text.
4. To continue in another session or client, query `execution.next` for the
   selected item. If paused, call `execution.resume` with its current revision,
   then read `execution.next` again and use the newly returned attempt ID.
   Reconcile a revision/attempt conflict by reading current state.
5. Read `execution.contract` for its actual criteria and relative artifact
   references. Use `execution.evidence` after producing the requested artifact;
   `execution.complete` revalidates the evidence. Report only the observed state.

Present the work and next step in the selected interaction profile. Product
continuity ends here. An unavailable operation stays pending; creating private
task files, raw ledger records or a replacement Markdown queue is not recovery.
Native Code hooks and Desktop hooks are not prerequisites for these operations.

## Positively identified legacy workspace

Use the remainder only when installation metadata identifies the older workspace
route and its owning filesystem operations are admitted. Missing MCP capability
does not select this legacy route.

Before owner-state file operations, follow [admitted owner-state paths](../interaction-profile/resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


Use this skill when the owner asks to register a task, continue open work,
pause a deliverable, create a checkpoint or resume work from a previous
session. This is the runtime-neutral execution ledger; it is not the owner
`work-state.md`, a transcript, a task provider or memory.

## Runtime-first continuity

Use the active workspace and its readable artifacts as the source of truth.
Do not expose internal commands, run IDs, revisions or JSON contracts to the owner. The
legacy deterministic execution ledger remains a compatibility/recovery surface
for older workspaces, but a new task is represented first by the agent in the
canonical workspace locations.

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) before presenting the task summary
or checkpoint. It controls communication style only; it does not choose the
task, grant authority or replace owner confirmation.

## Register a new task

1. Confirm the workspace and summarize the objective, next smallest step and
   one bounded completion criterion. When the owner explicitly asks to track
   the work, create it directly; ask only if the workspace, scope or next
   action is genuinely ambiguous.
2. Write a concise Markdown task under the active case's own
   `brain/accounts/<account-id>/cases/<case-id>/tasks/` and link it to the
   relevant artifact: the case brief at `<case-id>.md` in the case root, or a
   reviewed output under that case's `deliverables/`. The canonical case
   layout is declared by [`case-agent-setup`](../case-agent-setup/SKILL.md)
   and has no wrapper folder around the brief — a case already is one
   project, so a same-named folder around its single brief would be a
   redundant file in a redundant folder. Neither the brief nor
   `deliverables/` exists at the top of the brain either, so naming one
   there sends the task where nothing looks. Include only objective, next
   step, owner, completion criterion and logical artifact references.
3. Do not place prompts, transcripts, client bodies, credentials or absolute
   paths in the task file. Report the task as registered after the reviewed
   artifact is written. Uma tarefa registrada é uma nota em Markdown com objetivo, próximo passo e critério de conclusão.

The execution ledger is separate from `owner/operating/work-state.md`.
`ownerctx` open tasks may remain empty while a local execution item is active;
that is an intentional authority boundary, not a persistence failure.

## Pause and checkpoint

When the owner pauses, changes the next action or reaches a session boundary,
append a concise `## Checkpoint` section to the reviewed task or deliverable.
It may contain only a bounded summary, next step, blocker and logical artifact
references. Never synthesize one from a transcript, prompt, tool payload or
Stop hook without an explicit owner-approved next step.

## Resume in a new session

At SessionStart, inspect the active workspace and its bounded task/checkpoint
artifacts first. Confirm the scope and next step with the owner, then continue
from the most recent reviewed checkpoint. If there is no active item, say so
and offer to create one. If multiple items are active, ask which one to use.
Do not claim continuity until the new runtime session has actually observed the
bounded projection.

## Truth and safety

- Consultar a lista de tarefas registradas é operação somente de metadados; ver os detalhes de uma tarefa ou a próxima ação disponível apenas quando o owner autorizou leitura do handoff de trabalho delimitado.
- Preserve the current workspace identity and reject a path or workspace ID
  that does not match the SessionStart context.
- Keep external task systems, SharePoint, memory and agent delegation outside
  this skill unless their own governed capability is explicitly available.
- An unavailable native qualification label does not make the local execution
  ledger unavailable.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.
