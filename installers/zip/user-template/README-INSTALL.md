# Instalação e atualização do Maestro

## Requisitos

- O ZIP correto para o computador: `macos-arm64` ou `windows-amd64`.
- **Claude Code** para o modo Hub; Claude Code ou Codex para uma projeção
  direta que já tenha sido explicitamente matriculada.
- Git somente para o modo Repo/Worktree.

Não instale `bcgos` globalmente e não adicione `managed/bin` ao `PATH`. O
bootstrapper e o CLI já vêm dentro do ZIP e são verificados localmente antes
do uso. Essa checagem não é assinatura organizacional ou notarização.

## Instalação (primeira vez)

1. Descompacte o ZIP no local que preferir. Sugestão: `Documents/Maestro/` (Mac) ou `Documentos\Maestro\` (Windows).
2. Abra o Claude Code.
3. `File > Open folder…` e escolha a pasta `Maestro/` que você acabou de extrair.
4. Aceite quando o Claude Code perguntar se pode carregar os hooks (é normal e obrigatório).
5. Ao iniciar a sessão, o bootstrapper verifica target, versão e digest do CLI;
   depois o Maestro cria automaticamente `data/` (sua área privada).

Pronto. Rode `/maestro-onboarding` para a apresentação guiada.

## Escolha do modo de entrada

### Hub

Continue trabalhando com a pasta `Maestro/` aberta. Esse é o modo original e
não exige comando no terminal. `managed/` é o core substituível; `data/` é a
área privada preservada.

### Git Repo/Worktree

Primeiro abra o Hub uma vez para concluir a ativação local. Depois peça ao
Maestro para matricular o caminho exato do repositório ou worktree e informe o
runtime (`claude` ou `codex`). O Maestro executa o control plane instalado; ele
não copia o core nem `data/` para o repositório.

Para operação técnica assistida, os comandos públicos são:

```text
managed/bin/bcgos workspace enroll --runtime claude|codex <repo-ou-worktree>
managed/bin/bcgos workspace status --runtime claude|codex <repo-ou-worktree>
managed/bin/bcgos workspace repair --runtime claude|codex <repo-ou-worktree>
managed/bin/bcgos workspace remove --runtime claude|codex <repo-ou-worktree>
```

Claude e Codex podem permanecer matriculados ao mesmo tempo no mesmo checkout.
Execute `enroll` uma vez para cada runtime que será usado; `status`, `repair` e
`remove` atuam somente na projeção indicada, preservando a outra.

No Windows, use `managed\bin\bcgos.exe`. Cada worktree criado com
`git worktree add` precisa de sua própria matrícula: eles compartilham um
`repository_id` opaco e recebem `workspace_id` distintos. A matrícula não
muda branch, HEAD, index, remote ou Git hooks e recusa configurações locais do
runtime já rastreadas pelo Git.

## Atualização (novas versões)

Quando houver uma nova versão autorizada, use o ZIP da mesma plataforma. O
ritual é "renomear, extrair novo, copiar `data/`". A cópia mantém a versão
anterior recuperável enquanto o novo core é verificado.

**Antes de começar:** conferir se a pasta `data/` existe dentro da pasta `Maestro/` atual. Se não existir, parar e rodar `/maestro-doctor` antes de atualizar.

1. Fechar o Claude Code por completo.
2. No mesmo diretório onde está a pasta `Maestro/`, renomear ela para `Maestro-old/`.
3. Baixar o ZIP novo e extrair no mesmo diretório. Isso cria uma pasta `Maestro/` fresca ao lado de `Maestro-old/`.
4. **Copiar** (não mover) a pasta `data/` de dentro de `Maestro-old/` para dentro da nova `Maestro/`. Copiar é reversível; mover não é. Se algo der errado no meio do caminho, `Maestro-old/data/` continua intacto.
   - **Mac (Finder):** abrir `Maestro-old/`, segurar `Option (⌥)` e arrastar `data/` para dentro da nova `Maestro/` (arrastar sem Option move; com Option copia).
   - **Windows (Explorer):** abrir `Maestro-old/`, copiar `data/` (`Ctrl+C`), colar dentro da nova `Maestro/` (`Ctrl+V`).
5. Conferir que a nova `Maestro/` contém: `VERSION`, `managed/`, `CLAUDE.md`, `.claude/`, `bundles/` e `data/`. Se `data/` não estiver lá, refazer o passo 4 antes de continuar.
6. Reabrir a nova pasta `Maestro/` no Claude Code e rodar `/maestro-doctor` para confirmar. Para cada Repo/Worktree matriculado, peça `workspace status`; se aparecer `repair_required`, execute o `workspace repair` explícito com o novo CLI.
7. Só depois de confirmar que `data/` está dentro da nova `Maestro/`, que `/maestro-doctor` reporta tudo verde **e** que as projeções diretas necessárias estão `enrolled`, apagar `Maestro-old/`. Manter por pelo menos 7 dias (ou até a próxima atualização) como rede de segurança.

Esse fluxo elimina o risco de arquivos velhos de versões anteriores sobrarem misturados com a versão nova. Como o passo 4 é uma cópia, um erro no meio do caminho não destrói nada: `Maestro-old/data/` continua intacto até o passo 7.

## Estrutura de pastas

```
Maestro/
├── VERSION                ← versão instalada
├── WELCOME.md             ← primeira leitura
├── README-INSTALL.md      ← este arquivo
├── CLAUDE.md              ← bootstrap do Claude Code
├── .claude/               ← configuração (hooks, skills, settings)
├── bundles/               ← skills e agentes (núcleo)
├── managed/               ← bootstrapper, manifesto e CLI verificado
└── data/                  ← SUA workspace — nunca sobrescrita
    ├── agents/            ← estado de cada agente
    ├── memory/            ← memória de longo prazo
    ├── profile/           ← identidade e preferências
    └── workspaces/        ← projetos ativos
```

## Solução de problemas

**"O Claude Code não reconheceu os hooks."**
Feche e reabra o Claude Code com a pasta. Se persistir, rode `/maestro-doctor`.

**"Sumiu minha memória depois do update."**
Provavelmente a pasta `data/` foi movida por engano. Ela deve estar dentro de `Maestro/`. Se não estiver, verifique se você extraiu para o lugar certo.

**"Não sei qual versão tenho."**
Abra o arquivo `VERSION` na raiz da pasta.

**"Mudei a pasta Maestro e os hooks do repositório pararam."**
Abra a instalação no novo local e peça `workspace status` para o checkout. Se o
estado for `repair_required` com `managed_root_moved`, confirme o caminho e
execute `workspace repair`. O reparo só troca ponteiros gerenciados intactos.

**"O repositório ficou em conflict."**
Pare. Não apague `.claude/`, `.codex/`, `.bcgos/`, `CLAUDE.md` ou `AGENTS.md` e
não use comandos destrutivos do Git. Rode `workspace status`, preserve o
conteúdo modificado e encaminhe o diagnóstico ao suporte.

**"Não recebi o email da nova versão."**
Peça no canal BCG Brasil AI ou escreva para o time.

**"Meu Claude Code não abre a pasta."**
Confirme que está usando o Claude Code desktop (não o navegador). Reinstale se necessário: https://claude.ai/download

## Desinstalação

Antes de apagar a instalação, execute `workspace remove` em cada Repo/Worktree
matriculado enquanto o CLI instalado ainda existe. O comando remove somente
blocos e arquivos gerenciados intactos; conteúdo alterado vira conflito e é
preservado. Depois feche o runtime e mova `Maestro/` para a Lixeira. Para
preservar memória e vínculos privados, copie `data/` antes.

- **Windows:** esvaziar a Lixeira depois. Antes de esvaziar, "apagado" é reversível; depois, não.
- **Mac:** esvaziar a Lixeira depois. Mesma lógica.

## Suporte

Escreva para o time BCG Brasil AI no canal habitual. Inclua a saída de `/maestro-doctor` se possível.
