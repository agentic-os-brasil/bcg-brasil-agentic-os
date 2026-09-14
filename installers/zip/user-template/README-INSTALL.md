# Instalação e atualização do Maestro

## Requisitos

- **Claude Code desktop** instalado.
- **Mac:** Bash do sistema e um interpretador Python 3 disponível.
- **Windows:** Windows PowerShell 5.1 (já incluído no Windows) ou PowerShell 7.
  Use o ZIP cujo nome termina em `windows-powershell`.

No Windows, os hooks de memória, proteção de escrita e chamada de agentes são
PowerShell nativo. Git Bash e Python não são requisitos desses hooks. No Mac,
os hooks continuam em Bash e usam Python 3. Se o diagnóstico inicial apontar
uma peça ausente, não é necessário abrir terminal nem instalar algo por conta
própria: envie a saída de `/maestro-doctor` ao time BCG Brasil AI.

## Instalação (primeira vez)

1. Descompacte o ZIP no local que preferir. Sugestão: `Documents/Maestro/` (Mac) ou `Documentos\Maestro\` (Windows).
2. Abra o Claude Code.
3. `File > Open folder…` e escolha a pasta `Maestro/` que você acabou de extrair.
4. Aceite quando o Claude Code perguntar se pode carregar os hooks (é normal e obrigatório).
5. Ao iniciar a sessão, o Maestro cria automaticamente a pasta `data/` (sua workspace).

Pronto. Rode `/maestro-onboarding` para a apresentação guiada.

## Atualização (novas versões)

Quando o time BCG Brasil AI mandar um email com nova versão. O ritual é
"renomear, extrair novo, copiar `data/`". Uma única operação destrutiva visível,
sem risco de arquivos velhos sobrando entre versões.

**Antes de começar:** conferir se a pasta `data/` existe dentro da pasta `Maestro/` atual. Se não existir, parar e rodar `/maestro-doctor` antes de atualizar.

1. Fechar o Claude Code por completo.
2. No mesmo diretório onde está a pasta `Maestro/`, renomear ela para `Maestro-old/`.
3. Baixar o ZIP novo e extrair no mesmo diretório. Isso cria uma pasta `Maestro/` fresca ao lado de `Maestro-old/`.
4. **Copiar** (não mover) a pasta `data/` de dentro de `Maestro-old/` para dentro da nova `Maestro/`. Copiar é reversível; mover não é. Se algo der errado no meio do caminho, `Maestro-old/data/` continua intacto.
   - **Mac (Finder):** abrir `Maestro-old/`, segurar `Option (⌥)` e arrastar `data/` para dentro da nova `Maestro/` (arrastar sem Option move; com Option copia).
   - **Windows (Explorer):** abrir `Maestro-old/`, copiar `data/` (`Ctrl+C`), colar dentro da nova `Maestro/` (`Ctrl+V`).
5. Conferir que a nova `Maestro/` contém: `VERSION`, `CLAUDE.md`, `.claude/`, `bundles/` e `data/`. Se `data/` não estiver lá, refazer o passo 4 antes de continuar.
6. Reabrir a nova pasta `Maestro/` no Claude Code e rodar `/maestro-doctor` para confirmar.
7. Depois de confirmar que `data/` está dentro da nova `Maestro/` e que `/maestro-doctor` reporta tudo verde, manter `Maestro-old/` por pelo menos 7 dias (ou até a próxima atualização) como rede de segurança. Só então apagar.

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
└── data/                  ← SUA workspace — nunca sobrescrita
    ├── agents/            ← estado de cada agente
    ├── memory/            ← memória de longo prazo
    ├── profile/           ← identidade e preferências
    └── workspaces/        ← projetos ativos
```

## Solução de problemas

**"O Claude Code não reconheceu os hooks."**
Feche e reabra o Claude Code com a pasta. Se persistir, rode `/maestro-doctor`.

**"Sem Git Bash no Windows."**
Nenhuma ação é necessária. Esta versão usa PowerShell nativo no Windows.

**"A política da empresa bloqueou um script PowerShell."**
Não altere a política nem tente contornar o bloqueio. Guarde `Maestro-old/` e
envie a saída de `/maestro-doctor` ao time BCG Brasil AI; o release só pode ser
liberado nessa máquina pela rota aprovada pela empresa.

**"Sem Python 3."**
No Windows, nenhuma ação é necessária para os hooks desta versão. No Mac, a
memória em Markdown e a conversa continuam, mas o roteamento automático fica
indisponível e escritas que não podem ser verificadas são recusadas.

Nos dois casos, envie a saída de `/maestro-doctor` ao time BCG Brasil AI. Não
é necessário abrir terminal nem instalar algo por conta própria.

**"Sumiu minha memória depois do update."**
Provavelmente a pasta `data/` foi movida por engano. Ela deve estar dentro de `Maestro/`. Se não estiver, verifique se você extraiu para o lugar certo.

**"Não sei qual versão tenho."**
Abra o arquivo `VERSION` na raiz da pasta.

**"Não recebi o email da nova versão."**
Peça no canal BCG Brasil AI ou escreva para o time.

**"Meu Claude Code não abre a pasta."**
Confirme que está usando o Claude Code desktop (não o navegador). Reinstale se necessário: https://claude.ai/download

## Desinstalação

Para remover: fechar o Claude Code, apagar a pasta `Maestro/`. Se quiser preservar sua memória, copiar `data/` antes.

- **Windows:** esvaziar a Lixeira depois. Antes de esvaziar, "apagado" é reversível; depois, não.
- **Mac:** esvaziar a Lixeira depois. Mesma lógica.

## Suporte

Escreva para o time BCG Brasil AI no canal habitual. Inclua a saída de `/maestro-doctor` se possível.
