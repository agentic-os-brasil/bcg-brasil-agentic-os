# Maestro — ensaio no Mac

Este kit permite testar o MCP, o plugin Code e a extensão Desktop com um workspace sintético e estado privado novo. É um **ensaio de desenvolvimento**, sem identidade de release qualificada, instalação assinada ou atualização automática. A versão do servidor aparece como `unavailable`; `0.0.0` é somente o rótulo dos pacotes.

O alvo exercitado deste artefato é **Mac Apple Silicon**. Ele inclui também um binário Mac Intel compilado e com formato inspecionado, mas **sem execução nativa Intel observada**. Windows x64 tem um kit separado e sua própria prova nativa. Um executável ou `.mcpb` compilado não comprova carregamento real no cliente.

## Preparar

1. Extraia o ZIP inteiro para uma pasta local. Não execute arquivos de dentro da visualização do ZIP.
2. Abra `Preparar Maestro.command` pelo fluxo normal do macOS. O kit não remove quarentena, modifica políticas do computador ou copia credenciais.
3. A preparação cria `~/Developer/MaestroPilot`, com pastas separadas para produto, usuário privado, workspace sintético e exportações.
4. Abra `~/Developer/MaestroPilot/COMECE-AQUI.txt`. Ele indica `exports/Abrir Code.command` e o MCPB a importar no Desktop. O launcher carrega o plugin apenas nessa janela, sem registro global.
5. Faça o teste em `ROTEIRO.md`.

Você precisa ter Code/Desktop instalados e autenticados pelo fluxo normal. Não precisa de Go, Git, Python ou acesso ao repositório. A preparação não instala clientes, altera login nem usa um modelo.

Se sua sessão já define `MAESTRO_HOME`, a preparação recusa antes de criar o ensaio. Esse redirecionamento não é removido silenciosamente. Uma variável conflitante também será recusada pelo vínculo canônico ao conectar os clientes.

Os pacotes são gerados **no seu computador** porque contêm seus caminhos absolutos. Não compartilhe o MCPB, plugin exportado ou a pasta privada gerados no seu computador; compartilhe somente este kit genérico.

## Funcionalidades e limites

- O MCP oferece o ciclo de trabalho, estado/contexto e as operações de memória sujeitas à confirmação que o cliente suporta.
- `methods.list/read/plan` permitem descobrir os 92 métodos, ler suas instruções e consultar requisitos reais. Selecionar uma receita não executa seus efeitos nem supre dependências ausentes.
- `brain.*` prepara templates, cria, lê, edita e liga páginas profissionais com revisão e controle de versão. A wiki de memória privada compila e navega fontes já capturadas e consolidadas; ela é separada dessas páginas autorais.
- O plugin do Claude Code inclui três hooks de contexto: `SessionStart` lê memória gerada e aprovada; `UserPromptSubmit` fornece metadados de contexto; `Stop` registra uma observação limitada. A preparação cria as pastas e arquivos necessários; os hooks não inicializam nem reparam o estado durante uma sessão.
- Code e Desktop devem apontar para o mesmo workspace e a mesma HOME do ensaio.
- Esses hooks são configurados sem afirmar execução observada no cliente. Eles não fornecem proteção antes de ferramentas, agentes nativos, aprovação ou conclusão de trabalho; o perfil completo continua exigindo qualificação separada. A extensão Desktop mantém o MCP sem hooks do Code.
- A memória nova aparece no início de uma sessão somente depois de captura revisada e consolidação explícita. Não há captura de conversas, dreaming, indexação ou manutenção recorrente automática pelos hooks.
- Docling sem runtime admitido, agentes/guardas nativos sem qualificação, Yoda/modelo e o ciclo de instalação/atualização assinada permanecem indisponíveis.
- A presença de uma receita ou ferramenta na lista não comprova que todas as suas dependências estão disponíveis.
- Este kit usa somente material sintético. Não migra seu Maestro existente nem importa arquivos de cliente.

Se o sistema ou o aplicativo recusar a execução, preserve a mensagem e pare. Não desative controles corporativos para fazer o teste passar.

## Retomar, atualizar e remover

Abrir novamente a preparação com o mesmo kit verifica os arquivos gerados e lê o diagnóstico; ela preserva o estado privado. Uma pasta estrangeira ou preparação incompleta permanece intacta e é recusada.

Não substitua binários com os clientes abertos. Este ensaio não inclui atualização assinada: uma versão diferente usa um novo diretório de ensaio e pacotes regenerados, preservando a pasta anterior. Não há migração automática de memória entre ensaios.

Para um kit diferente, feche a janela antiga do Code e use o launcher da nova pasta. Se cadastrou manualmente o plugin anterior no perfil global, desative-o pelo gerenciador normal para evitar conectores ambíguos. Remova a extensão anterior pelo Desktop antes de importar a nova. O estado privado antigo permanece na pasta anterior.

Para encerrar, feche a janela do Code e remova a extensão do ensaio pelo Desktop. Se instalou um plugin global manualmente, desative-o pelo gerenciador normal. Preserve a pasta `~/Developer/MaestroPilot` se houver algo a investigar; quando decidir descartar os dados sintéticos, remova somente essa pasta pelo Finder. Isso não remove outro Maestro.

## Reportar um problema

Envie: sistema/arquitetura, versão do Code/Desktop, `KIT-ID.txt`, etapa do roteiro, resultado esperado e mensagem real. Diga se o problema aconteceu antes ou depois de uma confirmação. Não envie a pasta `user`, credenciais, memória privada ou documentos reais.

Os arquivos `ARTIFACT.txt` e `SHA256SUMS` identificam o conteúdo deste kit; checksum não substitui a aprovação de confiança do computador/aplicativo.
