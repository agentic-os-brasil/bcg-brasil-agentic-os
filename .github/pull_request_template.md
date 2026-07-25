## Resultado

<!-- Em uma frase: o que passa a funcionar, ficar mais seguro ou mais claro? -->

## O que mudou

<!-- Explique em 2-5 bullets. Cite arquivos ou contratos apenas quando ajudar a revisão. -->

-

## Como foi validado

<!-- Marque somente o que você realmente executou. CI é preenchido após abrir o PR. -->

- [ ] Teste novo ou caracterização do comportamento relevante
- [ ] `go run ./dev/harness validate --full`
- [ ] CI verde em Windows, macOS e Linux
- [ ] Diff revisado: somente arquivos intencionais

## Contratos e decisões

<!-- Marque apenas o que se aplica a este PR. -->

- [ ] Não altera contrato durável
- [ ] Atualiza spec existente
- [ ] Adiciona ou atualiza decisão de quatro letras: `____`

<!-- Explique em uma frase se houver mudança de contrato, decisão ou compatibilidade. -->

## Dados, privacidade e runtimes

<!-- Marque apenas o que se aplica a este PR. -->

- [ ] Não inclui dados de cliente, pessoas, credenciais, conversas ou memória real
- [ ] Mantém o core gerenciado separado de dados locais e workspaces
- [ ] Preserva a equivalência Claude/Codex, ou declara a diferença e o estado da capacidade

## Fora do escopo

<!-- Diga explicitamente o que este PR não implementa para não criar promessa implícita. -->

-
