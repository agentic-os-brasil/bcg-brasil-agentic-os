# Atualização do Maestro para {{TO_VERSION}}

## Isto não substitui seu Maestro pessoal

Este kit atualiza apenas a parte gerenciada: receitas, skills, agentes, hooks
e ferramentas de suporte. A memória, os perfis, os casos e suas personalizações
não vêm no ZIP. Eles continuam sendo seus e precisam ser preservados.

O kit pode conter os payloads Mac e Windows ou apenas o da sua plataforma.
O Windows usa PowerShell nativo; Git Bash, Python e Go não são pré-requisitos
dos hooks. No Mac, o helper de migração também já vem incluído.

Na 0.2.0 a área canônica passa a ser `brain/`, com clientes em
`brain/accounts/`. A migração copia e verifica dados; não apaga a `data/`
antiga. Agentes e workspaces legados permanecem acessíveis por mapa validado.

## Antes da troca

1. Abra a instalação atual e cole `PROMPT-1-PREPARAR.txt`.
2. Aguarde o inventário por SHA-256 da origem, incluindo arquivos ocultos e
   personalizações fora de data/. Origens suportadas: 0.1.11 e 0.1.12.
3. Feche sessões e terminais que escrevem nessa instalação.
4. Preserve a pasta inteira como `Maestro-old-<versão-original>`. Não extraia
   nada por cima dela. Um backup apenas de data/ não cobre hooks/agentes próprios.

## Aplicar e verificar

1. Confira o SHA-256 do ZIP recebido com o checksum entregue pelo time.
2. Extraia o payload da sua plataforma em uma pasta nova ao lado da antiga.
3. Copie, não mova, a `data/` original para a nova raiz. Não crie outra brain/
   manualmente antes da migração. Se já existem data/ e brain/ na origem,
   preserve ambas e peça diagnóstico: não escolha uma nem mescle por conta própria.
4. Não copie o núcleo antigo sobre o novo. O Maestro reconciliará suas
   personalizações a partir do inventário e da pasta antiga, de forma explícita.
5. Abra a raiz nova no Claude Code ou Codex. Revise os pedidos reais de
   confiança e permissão. Uma pasta adicional no chat não prova que seus hooks
   foram carregados.
6. Leia `UPDATE-RUNBOOK.md`, escolha modelo forte/esforço alto e cole
   `PROMPT-2-VERIFICAR.txt`. O Maestro deve retomar até verificar todos os
   checks obrigatórios, incluindo migração e chamada real ao Yoda.

Claude/Codex não encontram automaticamente este kit em Downloads. Precisam
receber o caminho ou abrir a nova raiz. Depois disso, CLAUDE.md/AGENTS.md e o
contrato orientam a atualização; ainda é preciso confiar nos hooks conforme
o host. Este pacote não ativa download automático nem muda políticas BCG.

## Conclusão

Exija receipt terminal `brain/.maestro/updates/update-{{TO_VERSION}}.json`
com `status: pass` e evidências do host e da plataforma usados. A verificação
vale para o ZIP exato; não é um teste preliminar. Arquivos presentes, contagens
iguais e scripts chamados manualmente não provam que hooks/subagents funcionam
na sessão real.

caseOS e ferramentas Python opcionais têm diagnóstico próprio. Credenciais,
permissão de case e endpoint oficial podem depender do time BCG; sua ausência
não deve impedir trabalho local nem ser registrada como conexão bem-sucedida.

Se houver falha, preserve ambas as pastas e reabra a instalação antiga.
Mantenha o backup por pelo menos sete dias depois da qualificação. Nenhum
bloqueio de PowerShell, hooks, Gatekeeper ou política corporativa deve ser
contornado.
