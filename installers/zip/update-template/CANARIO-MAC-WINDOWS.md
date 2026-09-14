# Canário do update {{FROM_VERSION}} → {{TO_VERSION}}

Este roteiro fecha a evidência em três camadas. Uma camada não substitui a
outra.

## 1. Contrato determinístico do ZIP

O mantenedor executa `installers/zip/eval-release.sh` no ZIP exato. O resultado
deve ter zero `FAIL` e zero `SKIP`. Ele cobre estrutura, checksum, scaffold,
settings, hooks, Python, caminhos Windows, guard de casos, router, anúncio e
projeções de agentes.

## 2. Execução nativa por plataforma

No macOS, rode o gate em Terminal/Bash. No Windows, rode em Git Bash — não em
PowerShell-only. Em ambos os casos, registre:

- sistema e arquitetura;
- versão do Claude Code, Bash e Python;
- SHA-256 do ZIP;
- contagem final de PASS/FAIL/SKIP;
- resultado do caminho nativo do guard de casos.

O mesmo SHA-256 deve ser usado nas duas máquinas. Se o ZIP for reconstruído,
os dois canários devem ser repetidos.

## 3. Sessão real do Claude Code

Abra a pasta extraída como um workspace confiável e cole
`PROMPT-2-VERIFICAR.txt`. A evidência mínima é:

- SessionStart executado na abertura;
- UserPromptSubmit entregando a rota de Yoda;
- ferramenta Agent chamada com `subagent_type: yoda`;
- PreToolUse devolvendo o pedido de anúncio ao hub;
- retorno real de Yoda à sessão principal.

Uma busca de arquivos ou a saída direta de um script não prova chamada real.
Sem os cinco eventos acima, registre `UNAVAILABLE` ou `FAIL`, nunca `PASS`.

## Critério de release

O bug fix pode ser distribuído apenas quando o mesmo ZIP tiver:

- gate determinístico verde;
- canário macOS verde;
- canário Windows + Git Bash + Python 3 verde;
- chamada real do Agent observada pelo menos uma vez em cada plataforma;
- atualização {{FROM_VERSION}} → {{TO_VERSION}} preservando `data/` em ambas.
