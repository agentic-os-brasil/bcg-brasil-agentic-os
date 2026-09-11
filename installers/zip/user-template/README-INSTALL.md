# Instalação e atualização do Maestro

## Requisitos

- **Claude Code desktop** instalado (Mac ou Windows).
- Nada mais.

## Instalação (primeira vez)

1. Descompacte o ZIP no local que preferir. Sugestão: `Documents/Maestro/` (Mac) ou `Documentos\Maestro\` (Windows).
2. Abra o Claude Code.
3. `File > Open folder…` e escolha a pasta `Maestro/` que você acabou de extrair.
4. Aceite quando o Claude Code perguntar se pode carregar os hooks (é normal e obrigatório).
5. Ao iniciar a sessão, o Maestro cria automaticamente a pasta `brain/` (sua workspace).

Pronto. Rode `/maestro-onboarding` para a apresentação guiada.

## Atualização (novas versões)

Quando o time BCG Brasil AI mandar um email com nova versão. O ritual é "renomear, extrair novo, mover `brain/`". Uma única operação destrutiva visível, sem risco de arquivos velhos sobrando entre versões.

**Antes de começar:** abrir a pasta `Maestro/` atual e ver qual destas duas existe lá dentro:

- **`brain/`** — é a sua workspace. Siga os passos abaixo usando esse nome.
- **`data/`** — você está numa versão anterior ao renome da workspace. Siga exatamente os mesmos passos, só que copiando `data/` no lugar de `brain/`. **Não renomeie a pasta à mão:** ao abrir a primeira sessão, o Maestro reconhece a `data/`, traz o conteúdo para o formato novo e deixa a original intacta. Ele escreve um resumo do que mudou de lugar em `brain/.maestro/migration-report.md`.

Se nenhuma das duas existir, parar e rodar `/maestro-doctor` antes de atualizar.

1. Fechar o Claude Code por completo.
2. No mesmo diretório onde está a pasta `Maestro/`, renomear ela para `Maestro-old/`.
3. Baixar o ZIP novo e extrair no mesmo diretório. Isso cria uma pasta `Maestro/` fresca ao lado de `Maestro-old/`.
4. **Copiar** (não mover) a sua pasta de workspace — `brain/` ou `data/`, a que existir — de dentro de `Maestro-old/` para dentro da nova `Maestro/`. Copiar é reversível; mover não é. Se algo der errado no meio do caminho, a de `Maestro-old/` continua intacta.
   - **Mac (Finder):** abrir `Maestro-old/`, segurar `Option (⌥)` e arrastar a pasta para dentro da nova `Maestro/` (arrastar sem Option move; com Option copia).
   - **Windows (Explorer):** abrir `Maestro-old/`, copiar a pasta (`Ctrl+C`), colar dentro da nova `Maestro/` (`Ctrl+V`).
5. Conferir que a nova `Maestro/` contém: `VERSION`, `CLAUDE.md`, `.claude/`, `bundles/` e a sua workspace. Se ela não estiver lá, refazer o passo 4 antes de continuar.
6. Reabrir a nova pasta `Maestro/` no Claude Code e rodar `/maestro-doctor` para confirmar. Se você copiou uma `data/`, a primeira sessão vai avisar que migrou a workspace e dizer onde está o resumo.
7. Só depois de confirmar que a workspace está dentro da nova `Maestro/` **e** que `/maestro-doctor` reporta tudo verde, apagar `Maestro-old/`. Manter por pelo menos 7 dias (ou até a próxima atualização) como rede de segurança.

Esse fluxo elimina o risco de arquivos velhos de versões anteriores sobrarem misturados com a versão nova. Como o passo 4 é uma cópia, um erro no meio do caminho não destrói nada: a workspace em `Maestro-old/` continua intacta até o passo 7.

**Vindo de uma versão com `data/`:** depois da migração, a `data/` continua dentro da nova `Maestro/`, intacta e sem uso. Ela é a sua rede de segurança. Apague junto com `Maestro-old/` no passo 7, não antes.

## Estrutura de pastas

```
Maestro/
├── VERSION                ← versão instalada
├── WELCOME.md             ← primeira leitura
├── README-INSTALL.md      ← este arquivo
├── CLAUDE.md              ← bootstrap do Claude Code
├── .claude/               ← configuração (hooks, skills, settings)
├── bundles/               ← skills e agentes (núcleo)
└── brain/                 ← SUA workspace — nunca sobrescrita
    ├── accounts/          ← clientes, cada um com seus projetos
    ├── memory/            ← memória consolidada (recente → permanente)
    ├── owner/             ← quem você é: identidade, estilo, estado de trabalho
    ├── daily/             ← a página de cada dia
    ├── learnings/         ← aprendizados profissionais duráveis
    ├── craft/             ← métodos e calibrações de estilo
    ├── people/            ← perfis de colegas
    ├── development/       ← objetivos, retros e feedback
    ├── tasks/             ← visão de tarefas, derivada dos casos
    └── .maestro/          ← índices e diagnóstico, regeneráveis
```

Se você vem de uma versão anterior, vai reconhecer nomes diferentes: `profile/`
virou parte de `owner/`, e `agents/` e `workspaces/` deixaram de existir — eram
pastas que nunca chegaram a ser usadas. A migração cuida disso sozinha.

## Solução de problemas

**"O Claude Code não reconheceu os hooks."**
Feche e reabra o Claude Code com a pasta. Se persistir, rode `/maestro-doctor`.

**"Sumiu minha memória depois do update."**
Provavelmente a pasta `brain/` foi movida por engano. Ela deve estar dentro de `Maestro/`. Se não estiver, verifique se você extraiu para o lugar certo.

**"Não sei qual versão tenho."**
Abra o arquivo `VERSION` na raiz da pasta.

**"Não recebi o email da nova versão."**
Peça no canal BCG Brasil AI ou escreva para o time.

**"Meu Claude Code não abre a pasta."**
Confirme que está usando o Claude Code desktop (não o navegador). Reinstale se necessário: https://claude.ai/download

## Desinstalação

Para remover: fechar o Claude Code, apagar a pasta `Maestro/`. Se quiser preservar sua memória, copiar `brain/` antes.

- **Windows:** esvaziar a Lixeira depois. Antes de esvaziar, "apagado" é reversível; depois, não.
- **Mac:** esvaziar a Lixeira depois. Mesma lógica.

## Suporte

Escreva para o time BCG Brasil AI no canal habitual. Inclua a saída de `/maestro-doctor` se possível.
