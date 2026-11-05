# Maestro — ensaio no Windows

Este kit prepara o MCP, o plugin Code e a extensão Desktop com dados sintéticos e uma HOME privada nova. É um **ensaio de desenvolvimento Windows x64**, sem identidade de release qualificada, assinatura Authenticode ou atualização automática. O servidor informa `unavailable`; `0.0.0` é somente o rótulo dos pacotes. Windows ARM64 permanece sem qualificação.

## Preparar

1. Extraia o ZIP inteiro para uma pasta local comum, com todos os arquivos disponíveis. Não execute dentro da visualização do ZIP nem de uma pasta que seja um link.
2. Abra `Preparar Maestro.cmd` como usuário comum. Ele usa o Windows PowerShell5.1 do sistema, com a política já definida no computador. Não use “Executar como administrador”.
3. A preparação cria `%USERPROFILE%\Developer\MaestroPilot`, separando produto, usuário privado, workspace sintético e exportações.
4. Abra `COMECE-AQUI.txt` nessa pasta. Ele contém os comandos de registro do plugin Code e o caminho do MCPB a importar no Desktop.
5. Siga `ROTEIRO.md` e registre os resultados dos dois clientes separadamente.

Code e Desktop precisam estar instalados e autenticados pelo fluxo normal. O participante não precisa de Go, Python, Git ou acesso ao repositório. O preparador não instala clientes, modifica login ou usa um modelo. Se o computador bloquear o script ou o executável, preserve a mensagem e pare; não altere a política de execução, SmartScreen, WDAC ou AppLocker para continuar.

Os pacotes são gerados no seu computador porque contêm seus caminhos absolutos. Compartilhe somente o ZIP genérico e seu checksum; não compartilhe a pasta `user`, o plugin exportado ou o MCPB gerado localmente. Checksum identifica os bytes e não substitui a confiança do computador/aplicativo.

## Estado e limites

A preparação recusa `MAESTRO_HOME` já definido, unidades remotas/removíveis, links/reparse, diretórios de outro usuário, permissões desconhecidas, pastas estrangeiras ou parciais e arquivos gerenciados modificados. Ela não assume propriedade nem repara uma instalação existente.

Reabrir o mesmo preparador verifica os arquivos gerenciados e lê o diagnóstico, preservando o estado privado e o workspace. O ensaio não migra seu Maestro anterior nem importa documentos reais. Use somente `EXEMPLO.md` e os textos sintéticos do roteiro.

Docling sem runtime admitido, agentes/hooks nativos sem qualificação, Yoda/modelo, wiki privada Windows e instalação/atualização assinada permanecem indisponíveis. A presença de receitas não comprova que suas dependências executam. Testes do SDK, arquivo MCPB e registro do plugin não substituem o carregamento real dos clientes.

## Trocar de kit ou remover

Uma versão diferente requer uma pasta nova de ensaio e pacotes regenerados. Antes de registrar o novo kit, remova pelo gerenciador normal do Code o plugin e o marketplace `maestro-pilot-20261105` anteriores e remova a extensão anterior do Desktop. O estado privado antigo permanece preservado; não há migração automática entre ensaios.

O preparador PowerShell aceita `-Root` somente dentro de seu diretório local `Developer`, para um destino novo ou para repetir o mesmo kit completo. Se a preparação estiver incompleta, preserve-a para suporte e escolha uma pasta nova; este kit não faz reparo automático.

Para encerrar, desconecte o plugin e a extensão pelos gerenciadores normais, feche os clientes e preserve a pasta se houver algo a investigar. Quando decidir descartar os dados sintéticos, remova somente a pasta específica do ensaio pelo Explorador. Isso não remove outro Maestro.

Ao reportar um problema, envie sistema/arquitetura, versão dos clientes, `KIT-ID.txt`, etapa, resultado esperado e mensagem real. Diga se houve confirmação. Não envie credenciais, memória privada ou documentos de cliente.
