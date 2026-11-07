# Teste de continuidade entre Code e Desktop

Use somente o workspace sintético criado pela preparação. A execução destes prompts usa sua conta normal do cliente; o kit não inclui credenciais ou chamadas de modelo.

## 1. Conferir o vínculo no Code

> Use somente o conector Maestro deste ensaio. Chame `maestro_context` e mostre o `workspace_id` e o estado disponível. Compare o identificador e o caminho exato do workspace com `COMECE-AQUI.txt`. Se houver ambiguidade entre conectores ou divergência nesse vínculo, pare antes de escrever.

Esperado: contexto do workspace local; a versão de servidor pode aparecer como `unavailable`, conforme o modo de desenvolvimento. Compare o `workspace_id` com o resultado real de criação em `COMECE-AQUI.txt` ou `diagnostico-init.json`. Divergência impede o teste de escrita.

## 2. Criar e pausar um trabalho

> Crie no Maestro um trabalho sintético chamado “Ensaio de continuidade do Maestro”, com próximo passo “Retomar no Desktop”. Use somente `EXEMPLO.md` como artefato. Inicie, salve um checkpoint com resumo exato `MAESTRO_PILOT_2026_11_05` e pause o trabalho. Mostre o item_id, a revisão final e o estado; não crie outro item se já existir um equivalente neste ensaio.

Esperado: um item `paused`, com revisão explícita e o resumo exato. Anote `item_id` e revisão. Uma confirmação recusada não deve causar mutação.

## 3. Retomar no Desktop

> Chame `maestro_context` e confira se o workspace_id é o mesmo que anotei no Code. Localize o item_id que informarei. Leia seu estado antes de retomar. Se estiver paused, na revisão esperada e com resumo `MAESTRO_PILOT_2026_11_05`, retome uma única vez usando essa revisão. Se algum valor divergir, pare e mostre a divergência.

Esperado: o mesmo item passa a `running`, com revisão sucessora e nova tentativa. Não repetir a retomada automaticamente.

## 4. Confirmar persistência

Feche e reabra o cliente ou reinicie a conexão MCP. Peça:

> Leia novamente o item_id do ensaio, sem mudar nada. Mostre estado, revisão e resumo do último checkpoint.

Esperado: o mesmo item e histórico persistem. Reiniciar o chat não cria outro workspace nem outra HOME.

## 5. Memória com confirmação

> Proponha uma captura de memória somente com o texto sintético “No ensaio do Maestro, o próximo passo deve permanecer explícito”. Mostre a revisão para minha confirmação antes de salvar. Se o cliente não suportar a confirmação necessária, informe indisponibilidade e não salve por outro caminho.

Aceite uma vez pelo fluxo apresentado. Reinicie a conexão e peça a leitura da captura pelo identificador retornado. Depois proponha outra captura e **recuse**; a captura recusada não deve aparecer na descoberta.

Esperado: a captura aceita pode ser lida após reinício; a recusada não existe. Não forneça flags de aprovação em argumentos do modelo.

## 6. Skills e páginas compartilhadas

> No mesmo Maestro, liste os métodos por `methods.list` e mostre o total. Leia a receita `maestro://methods/maestro-operator/resources/authored-brain.md` por `methods.read`. Use um template real para propor `projects/piloto-time.md`, com o texto sintético “Página profissional do piloto”. Mostre a proposta completa para confirmação pelo host. Se a revisão estiver indisponível, pare; use somente `brain.create` para salvar.

Aceite a proposta uma vez. No outro cliente, confira o mesmo workspace e leia essa página por `brain.read`. Compare texto e revisão com os valores retornados na criação. Peça uma edição na seção existente, confirme a proposta e confira a nova revisão.

## 7. Wiki de memória privada

> Consolide explicitamente a captura de memória aceita no passo5 no ciclo diário de hoje. Qualifique somente o job `wiki-incremental-sync`, sem enrolar nem ativar manutenção recorrente. Leia `wiki.status` e `index.md` da geração retornada. Se houver indisponibilidade de fonte ou confirmação, informe o resultado e pare.

Reinicie a conexão e leia a mesma geração. A wiki deve continuar vinculada à fonte aceita. Ela é diferente da página `projects/piloto-time.md`; preparar uma delas não cria nem autoriza a outra.

## Registrar o resultado

Anote separadamente: preparação local; plugin carregado no Code; extensão carregada no Desktop; continuidade após reinício; captura aceita; captura recusada sem efeito; catálogo de métodos; página autoral no outro cliente; wiki da mesma fonte após reinício. Marque “não executado” ou “indisponível” quando for o caso. Ferramentas listadas, testes SDK ou um arquivo MCPB criado não substituem o carregamento real do cliente.
