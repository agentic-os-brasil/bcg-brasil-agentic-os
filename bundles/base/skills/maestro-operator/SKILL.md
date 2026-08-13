---
name: maestro-operator
description: Método operacional do Maestro, carregado no início de cada sessão (spec 050). Roteia operações de controle para o skill ou ação certa.
---

# Maestro Operator

Método de controle instalado pelo Maestro. Loaded at SessionStart before any task
routing so Claude always knows how to handle control-plane requests.

## Interaction profile

Resolve the canonical `interaction-profile` skill before responding. Adjusts vocabulary and depth only — never the routing rules or authority boundaries.

## Responsabilidades

Rotear cada pedido de controle para o destino certo, sem expor comandos internos ao usuário.

| Tipo de pedido | Ação |
|---|---|
| Saúde da instalação / "está tudo OK?" | Invocar `/maestro-doctor` |
| Onboarding / primeira configuração | Invocar `/maestro-onboarding` |
| Atualização do Maestro | Invocar `/maestro-setup-update` |
| Versão instalada | Ler `VERSION`; responder com versão em uma linha |
| Recuperação de erros de instalação | Invocar `/maestro-doctor`; seguir recomendações |
| Diagnóstico de memória ou scaffold | Invocar `/maestro-runtime-checkup` se disponível |

## Loop operacional (spec 050)

1. **Inspecione antes de alterar** — leia o estado atual antes de propor qualquer mudança.
2. **Roteie corretamente** — trabalho normal → skill de tarefa; controle do plano → este método.
3. **Execute mecânicas rotineiras silenciosamente** — surfaça apenas o que o usuário precisa ver.
4. **Responda com resultado + próximo passo** — nunca exponha jargão técnico sem necessidade.
5. **Verifique o estado resultante** — use `/maestro-doctor` ou outro surface de status depois de
   qualquer operação de setup ou update.
6. **Recupere de erros tipados** — sem contornar proteções ou fazer suposições destrutivas.

## O que NÃO fazer

- Não instale, atualize ou repare arquivos fora dos skills autorizados.
- Não exponha caminhos de arquivo internos, hashes ou metadados técnicos salvo pedido explícito.
- Não tome ações irreversíveis (deletar `data/`, sobrescrever config) sem confirmação explícita do
  usuário e sequência declarada por um skill.

## Autoridade

Este método não concede nenhuma autoridade adicional. Isolamento de workspace, confirmação
explícita para efeitos externos ou destrutivos, proteção da managed-root e permissões nativas do
runtime continuam autoritativas independentemente deste método.
