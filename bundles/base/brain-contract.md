# Contrato de página do brain

Toda página escrita em `brain/` carrega este frontmatter. Ele não é decoração: é o que
permite ao índice (`brain_index.md` e os índices por pasta) listar a página com resumo
**sem abrir o arquivo**, e ao compilador do atlas construir backlinks e diagnóstico.

Uma página sem este bloco é invisível para a navegação.

## Campos obrigatórios

```yaml
---
id: <caminho sem .md>              # estável; nunca muda depois de criado
title: "<título humano>"           # o mesmo do H1 da página
summary: "<uma linha>"             # o que o índice mostra; sem markdown, sem emoji
type: <tipo>                       # ver vocabulário abaixo
scope: <owner | account/<a> | account/<a>/case/<c>>
status: <active | closed | draft | superseded>
sensitivity: <owner-private | client-confidential | internal>
updated: <YYYY-MM-DD>
---
```

## Campos opcionais

| Campo | Quando usar |
|---|---|
| `source` | a página deriva de um documento, entrevista ou sistema — nomeie e date |
| `tags` | lista curta, minúscula, com hífen |
| `related` | ids de outras páginas; alimenta os backlinks |
| `supersedes` | id da página que esta substitui |
| `coverage` | `partial` quando a página cobre menos que seu período nominal |

## Vocabulário de `type`

**Owner** — `daily`, `learning`, `craft-method`, `craft-style`, `person`, `objectives`,
`cdc`, `retro`, `project-feedback`, `upward-feedback`, `owner-facet`, `operating`, `index`

**Caso** — `account`, `project-brief`, `decision-log`, `task-list`, `deliverable`,
`source`, `canon` (com subtipo em `type` quando já existir: `data`, `framework`,
`hypothesis`, `benchmark`, `interview`)

**Memória** — `memory-l1`, `memory-l2`, `memory-l3`, `memory-lifetime`

## Regras

1. **`id` é estável.** Renomear arquivo quebra backlinks. Se precisar renomear, atualize
   os `related` que apontam para ele.
2. **`summary` é prosa, não título repetido.** Deve dizer o que a página contém para quem
   está decidindo se abre. Máximo ~150 caracteres, sem markdown e sem emoji.
3. **`sensitivity` segue o escopo, não o conteúdo.** Tudo sob `accounts/` é
   `client-confidential`, mesmo quando o conteúdo parece genérico.
4. **`status: closed`** para caso encerrado. A página continua navegável, mas o índice a
   separa do trabalho vivo.
5. **`updated` é a data do conteúdo**, não do arquivo. Só use a data de modificação como
   último recurso.
