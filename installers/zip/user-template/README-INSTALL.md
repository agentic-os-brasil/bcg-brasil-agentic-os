# Instalação e atualização do Maestro 0.2.0

Este pacote instala receitas, agentes projetados, hooks e ferramentas gerenciadas.
Não contém a memória, os casos ou as personalizações do Maestro de outra pessoa.
A pasta pessoal existente deve ser preservada e reconciliada, nunca substituída.

## Escolha do pacote e do host

- Mac: pacote macos, Bash do sistema e runtime nativo incluído para Intel/Apple Silicon.
- Windows: pacote windows-powershell, PowerShell 5.1 ou 7, binário incluído para x64/ARM64. Git Bash não é necessário.
- Host: Claude Code ou Codex. Cada um possui configuração e mecanismo próprio de agentes.
- Python é opcional para indexação e algumas ferramentas de conhecimento. Migração e proteção de escrita usam o runtime nativo incluído nos dois sistemas. Sem Python, recursos opcionais devem declarar indisponibilidade.

O runtime é um candidato não assinado enquanto a distribuição corporativa não o qualificar.
Se o sistema bloquear o executável ou script, não contorne a política. Use o canal oficial.

## Primeira instalação

1. Extraia o ZIP em uma pasta nova, por exemplo Documents/Maestro ou Documentos\Maestro.
2. Abra essa pasta no host escolhido. Revise e aceite somente a confiança/permissões normais exigidas pelo host.
3. Claude Code: os hooks gerenciados criam a workspace brain/; use maestro-onboarding.
4. Codex: leia AGENTS.md e adapters/codex/README.md quando disponível no repositório. O setup precisa vincular os hooks à raiz absoluta confirmada desta instalação. As definições portáteis recusam escrita quando carregadas e confiadas; antes da revisão de hooks pelo host, elas podem ser ignoradas e não são proteção ativa. Siga o bootstrap descrito em AGENTS.md. Depois do binding, revise os cinco comandos na tela nativa /hooks e confie nas definições exatas pelo fluxo normal. Reabra e peça a qualificação dos hooks e uma chamada real a Yoda. Confiança na pasta não substitui confiança nas definições.
5. Peça maestro-doctor. Configuração presente não prova execução: siga as verificações do host.

## Atualização de 0.1.11 ou 0.1.12

1. Na instalação antiga, peça ao Maestro que siga PROMPT-1-PREPARAR.txt do kit. Ele deve inventariar a árvore data/ inteira, inclusive ocultos, e as personalizações fora dela. O baseline não altera esses arquivos.
2. Feche todas as sessões que possam escrever nessa pasta. Preserve a instalação inteira com nome único de backup, sem sobrescrever um backup anterior.
3. Extraia o pacote novo em outra pasta. Nunca extraia por cima da instalação antiga.
4. Copie (não mova) data/ da pasta anterior para a nova. Preserve permissões e conteúdo. No Finder use copiar/colar ou Option ao arrastar; no Explorer use Ctrl+C/Ctrl+V.
5. Personalizações em CLAUDE.md, AGENTS.md, .claude/, .codex/ e skills próprias ficam preservadas no backup e entram numa reconciliação explícita. Não copie configurações antigas por cima dos hooks novos.
6. Abra a nova raiz no host escolhido e siga PROMPT-2-VERIFICAR.txt, UPDATE-CONTRACT.json e UPDATE-RUNBOOK.md. No Codex faça o binding específico da nova raiz.
7. O motor copia os caminhos mapeados para brain/, valida hashes e grava plano/recibo imutáveis em brain/.maestro/migration/. A origem data/ permanece byte a byte preservada.
8. A verificação só termina em PASS depois dos checks obrigatórios e do retorno real de Yoda. FAIL/UNAVAILABLE são resultados honestos que exigem próximo passo, não sucesso.

Se existirem data/ e brain/ sem um recibo de migração válido, pare e peça diagnóstico.
Não mescle automaticamente duas árvores autorais. Se a origem já for uma instalação
0.2.0 com brain/, o contrato de origem deste kit não a cobre: faça diagnóstico de
reparo em vez de forçar uma migração antiga.

**Não apague data/ após a migração.** Agentes, workspaces e recibos históricos podem
continuar sendo consumidos pelos caminhos legados retidos no plano. O resolver
gerenciado valida cada namespace antes da leitura. Também não há exclusão automática
da instalação anterior: ela é o caminho de recuperação e deve seguir a política de
retenção autorizada pelo dono.

Edições legítimas posteriores em brain/ não refazem a migração nem reescrevem o
recibo histórico. Uma nova qualificação usa verificações frescas; o PASS antigo não
serve como prova do estado atual. Links inseguros e adulteração continuam bloqueantes.

## Recuperação e rollback

Revogar uma tentativa preserva ambas as árvores e informa restored_runtime: false.
Isso não reinstala a versão antiga. Para recuperar o runtime anterior, reabra a
instalação antiga intacta. Não copie brain/ de volta para um runtime que espera data/.
Conteúdo autoral criado depois do update deve ser reconciliado separadamente, nunca
apagado para fazer um teste passar.

## caseOS MCP

Peça caseos-connect para instalação e uso ativo do conhecimento compartilhado.
É necessário consentimento, workspace/caso explícitos, URL oficial completa e acesso
corporativo. Não adivinhe o caminho do endpoint nem copie tokens. O host faz o login
normal. Indisponibilidade de caseOS não impede o trabalho local. Consultas usam
ferramentas descobertas na sessão atual; exportação exige aprovação específica.

## Estrutura

- VERSION, README-INSTALL.md, UPDATE-CONTRACT.json e UPDATE-RUNBOOK.md: versão e contrato.
- CLAUDE.md + .claude/: projeção Claude.
- AGENTS.md + .codex/ + .agents/skills/: projeção Codex.
- bundles/: skills e agentes canônicos. runtime/: executáveis verificados por manifesto.
- brain/: conteúdo pessoal e de casos. brain/.maestro/: índices e evidências locais.
- data/: quando presente após upgrade, origem preservada e namespaces legados ainda consumidos.

## Problemas frequentes

- Hooks não executam: reabra a raiz correta e peça maestro-doctor. Não conclua por presença dos arquivos.
- Script/executável bloqueado: guarde o backup e encaminhe o diagnóstico ao time BCG Brasil AI. Não mude políticas de segurança.
- Python ausente: mantenha uso local; maestro-environment-setup orienta instalação opcional gerenciada com consentimento.
- Memória parece ausente: confirme raiz e cópia da origem; não crie uma workspace substituta por cima.
- Pasta movida no Codex: faça novo binding para a raiz confirmada, sem executar launcher de um caso aninhado.

Para remover a instalação, feche os hosts e preserve antes uma cópia íntegra de
brain/, data/ e personalizações. Não esvazie a Lixeira automaticamente.
