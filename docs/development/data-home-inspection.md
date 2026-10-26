# Inspecionar o data home do Maestro

Use este diagnóstico ao preparar a separação entre o produto gerenciado e os
dados do owner. Ele identifica a origem do caminho e bloqueia conflitos antes
de uma futura ativação. O helper deve ter sido compilado da revisão que contém
a [Spec 055](../../specs/055-data-home-inspection.md).

## Executar

Execute o helper confiável do produto, informando a raiz absoluta que contém
o código gerenciado. Se houver um ZIP anterior a preservar, informe também
`--source-root` com sua raiz absoluta:

```text
maestro-runtime home inspect --product-root "/absolute/managed/product"
maestro-runtime home inspect --product-root "/absolute/managed/product" --source-root "/absolute/prior/ZIP"
```

No Windows, use o executável `.exe` e os caminhos absolutos nativos, inclusive
em outros drives. Espaços são aceitos quando o argumento está entre aspas.
O comando não instala software nem muda o perfil do Claude.

## Interpretar e resolver

O JSON mostra `home`, `brain_root`, `selected_by`, `origin`, `writable` e
`diagnostic`. A seleção segue `MAESTRO_HOME`, locator
`USERHOME/.maestro-home.json` e default `USERHOME/.maestro`.

| Resultado | Próxima ação |
|---|---|
| `fresh-unconfirmed`, `writable=false` | Identificar se existe ZIP anterior; aguardar onboarding ou migração explícitos, sem criar registros manualmente |
| `fresh-confirmed`, `writable=true` | Origem local consistente; a operação futura ainda precisa verificar confiança do código, escopo e permissões do host |
| `migrated`, `migration-unverified`, `writable=false` | Aguardar a validação de journal/plano/recibo de #434 |
| `fresh-confirmed`, `ownership-unverified`, `writable=false` no Windows | Aguardar a validação de owner/ACL; estrutura válida não comprova autoridade de escrita |
| Erro de locator, origem ou path; exit 2 | Preservar os arquivos e revisar o vínculo local; a inspeção não faz fallback, reparo ou merge |

Uma inspeção válida retorna exit 0 mesmo quando a origem ainda está
incompleta. Erros retornam exit 2, diagnóstico no stderr e nenhum JSON de
sucesso. A saída contém caminhos locais: compartilhe somente com quem pode
ver esses metadados. Ela não lê nem retorna conteúdo do brain.

## Limite desta entrega

O comando e o resolver são somente leitura. Não criam HOME ou locator, não
ativam uma instalação e não migram dados. Os hooks existentes continuam no
layout ZIP até a adaptação conjunta de código, estado e migração. Não usar
`writable=true` como prova de assinatura de release ou permissão de ferramenta.

O progresso integral permanece na
[checklist de paridade](../plans/2026-10-24-maestro-full-parity.md). #433 e B01
continuam abertos até todos os callers de estado serem adaptados e testados.
