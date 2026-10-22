# Stories — instalação, memória e publicação

**DRAFT.** Critérios futuros; nenhum checkbox representa teste concluído.
Dependências e issues: [plano F0–F4](migration-plan.md).

## US-1 — Colega instala no Code tab

Como colega sem experiência técnica, quero copiar dois comandos e confirmar
permissões normais do host para começar com diagnóstico claro.

- [ ] Em Mac e Windows limpos, adicionar a pasta aprovada e instalar `maestro`
  chega a `/maestro:doctor` PASS, sem editar configuração manualmente.
- [ ] Doctor mostra plugin, runtime, marketplace e origem do data home.
- [ ] Onboarding explica nome novo e `/maestro-doctor` usado no ZIP.
- [ ] Política, autenticação ou binário bloqueados produzem diagnóstico e
  próxima ação suportada; não são tratados como instalação bem-sucedida.

## US-2 — Colega preserva memória do ZIP

Como colega em 0.1.11, quero migração verificável para `~/.maestro/` para
continuar com meu contexto e recuperar o ZIP se necessário.

- [ ] Motor existente estendido copia, verifica e faz commit, contabilizando
  todo original e mantendo namespaces legados acessíveis.
- [ ] Repetição é no-op; interrupção é retomável; destino ambíguo não é sobrescrito.
- [ ] Rollback preserva edições posteriores e valida instalação ZIP funcional.
- [ ] Instalação direta do plugin pergunta sobre uso de ZIP antes de qualquer
  escrita de estado; resposta afirmativa exige origem explícita.

## US-3 — Colega recebe versão publicada

Como colega, quero abrir outra sessão e receber a versão íntegra publicada
sem repetir o ritual de copiar memória.

- [ ] No Code tab, N→N+1 aparece em doctor sem `plugin update`, com snapshot
  do data home byte-idêntico; Mac e Windows têm evidência própria.
- [ ] Guard bloqueia quatro tools fora do caso ativo e permite escrita de controle.
- [ ] Nove hooks executam; conteúdo novo não mascara runtime antigo.
- [ ] Versão parcial, adulterada ou incompatível produz FAIL nomeado.

## US-4 — Responsável publica e recolhe

Como responsável pela distribuição, quero publicar e fazer recall de uma
versão para reduzir suporte e interromper novas ativações defeituosas.

- [ ] Verificar hashes no destino precede a troca de `marketplace.json`.
- [ ] Mesma versão com bytes diferentes é recusada; falha preserva ponteiro anterior.
- [ ] Recall aponta para versão anterior verificada, sem modificar dados do usuário.
- [ ] Sessões abertas, offline e sincronização pendente têm limites explícitos;
  recall não promete revogação remota imediata.

## US-5 — Piloto prova o percurso

Como responsável pela adoção, quero observar colegas técnico e não técnico
cobrindo Mac e Windows para decidir rollout com evidência.

- [ ] Ambos migram ZIP, instalam e recebem atualização sem ajuda.
- [ ] Doctor PASS e fricções registrados nos dois momentos e máquinas.
- [ ] Após um release de convivência, cada destinatário está migrado ou com
  acompanhamento explícito; só então o ZIP é retirado.

## US-6 — Revisão de confiança

Como revisor, quero distinguir hashes, assinatura e qualificação nativa para
avaliar bytes executáveis e a fronteira entre produto e estado.

- [ ] Manifest cobre hooks, conteúdo e helper; raiz de confiança independente
  da pasta compartilhada. Divergência bloqueia conformidade.
- [ ] Relatório distingue candidata unsigned/HOLD, CLI observado, Code tab,
  Windows corporativo e release autorizada, sem inferir um pelo outro.
