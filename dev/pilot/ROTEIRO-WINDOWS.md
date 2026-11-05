# Teste Windows entre Code e Desktop

Use somente o workspace sintético criado pela preparação. Os prompts usam sua conta normal dos clientes; o kit não inclui credenciais ou chamadas de modelo.

1. No Code, peça: “Use somente o conector Maestro deste ensaio. Chame `maestro_context` e mostre o `workspace_id`. Compare-o com a criação real em `COMECE-AQUI.txt` e confirme a pasta `MaestroPilot\workspace`. Se houver dúvida sobre o conector ou divergência, pare antes de escrever.”
2. Peça: “Crie um trabalho sintético chamado ‘Ensaio Windows do Maestro’, usando somente `EXEMPLO.md`, com próximo passo ‘Retomar no Desktop’. Inicie, salve um checkpoint com resumo exato `MAESTRO_WINDOWS_PILOT_2026_11_05` e pause. Mostre `item_id`, revisão e estado; não duplique um trabalho equivalente.” Anote os valores reais.
3. No Desktop, informe os valores anotados e peça: “Leia `maestro_context` e confira o mesmo workspace. Leia o item antes de retomar. Somente se estiver paused, na revisão anotada e com o resumo exato do ensaio Windows, retome uma única vez com essa revisão. Se houver divergência, mostre-a e pare.”
4. Reinicie a conexão MCP ou feche/reabra o cliente. Peça a leitura do mesmo item, sem mudar nada. O item, histórico e próximo passo devem persistir; a retomada cria uma nova tentativa.
5. Peça: “Proponha memória somente com o texto sintético ‘No ensaio Windows do Maestro, o próximo passo permanece explícito’. Mostre a revisão para minha confirmação antes de salvar. Se o cliente não suportar a confirmação necessária, informe indisponibilidade e não use outro caminho.” Aceite uma vez, reinicie a conexão e leia a captura pelo identificador retornado. Depois proponha outra captura e recuse: ela não deve aparecer na descoberta.

Registre separadamente: preparação local; plugin carregado no Code; extensão carregada no Desktop; continuidade após reinício; memória aceita; recusa sem efeito. Marque “não executado” ou “indisponível” onde necessário. Ferramentas listadas, pacote gerado e testes SDK não comprovam carregamento real nem confirmação humana.
