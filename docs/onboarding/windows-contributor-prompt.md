# Prompt de onboarding Windows para o Claude

Este texto e enviado ao contribuidor antes do clone. Ele deve colar somente o bloco abaixo em uma sessao do Claude Code no Windows.

```text
Voce vai me ajudar a entrar como contribuidor no repositorio privado
agentic-os-brasil/bcg-brasil-agentic-os. Eu sou iniciante em Git, GitHub e
desenvolvimento. Execute o onboarding comigo, nao apenas me entregue uma lista
de instrucoes.

Objetivo final:
- obter acesso ao repositorio sem expor credenciais;
- clonar em uma pasta segura dentro do meu perfil de usuario;
- preparar Git e Go;
- executar o bootstrap oficial do repositorio;
- terminar com uma nova sessao do Claude aberta dentro do clone e pronta para
  usar $start-contributing.

Regras obrigatorias:
1. Fale em portugues simples e forneca uma unica proxima acao por vez.
2. Antes de instalar software, mudar configuracao, autenticar, clonar ou criar
   diretorios, explique o que sera feito e peca minha confirmacao.
3. Nunca peca que eu cole senha, token, chave ou codigo de recuperacao no chat.
   Autenticacao deve usar browser ou Git Credential Manager.
4. Nao use curl pipe shell, scripts remotos, bypass de politica corporativa,
   ExecutionPolicy Bypass, elevacao administrativa, bypass de proxy/Defender/SSO,
   prompt administrativo escondido ou comandos destrutivos de Git.
5. Se uma politica da maquina bloquear algo, pare, confirme que nada foi perdido
   e me de uma unica mensagem de erro para encaminhar ao Daniel.
6. Se a pasta ou o clone ja existir, nao clone novamente e nao sobrescreva nada;
   diagnostique primeiro.

Fluxo:
A. Confirme que estamos no PowerShell e proponha a pasta
   $env:USERPROFILE\Developer\bcg-brasil-agentic-os. Se a pasta Developer nao
   existir, mostre o path absoluto, explique e peca confirmacao antes de cria-la
   com New-Item.
B. Verifique, sem alterar nada: git --version, gh --version, go version e
   winget --version.
C. Se Git, GitHub CLI ou Go estiver ausente, use winget search/show para
   verificar respectivamente Git.Git, GitHub.cli ou GoLang.Go. Mostre o pacote
   e o publisher e peca confirmacao antes de winget install. Depois da
   instalacao, confirme se um novo PowerShell e necessario para atualizar PATH.
   Se winget tambem estiver ausente ou bloqueado, nao improvise outro instalador:
   pare e identifique Software Center ou suporte de TI como proxima acao.
D. Rode gh auth status. Se nao houver autenticacao, explique e, com minha
   confirmacao, use gh auth login --hostname github.com --web --git-protocol
   https. Nunca use token digitado no chat nem aceite outro host.
E. Confirme acesso com gh repo view agentic-os-brasil/bcg-brasil-agentic-os. Se houver
   404 ou acesso negado, pare e diga que Daniel precisa adicionar meu usuario
   GitHub ao repositorio privado.
F. Se a pasta alvo nao existir, clone com:
   gh repo clone agentic-os-brasil/bcg-brasil-agentic-os "$env:USERPROFILE\Developer\bcg-brasil-agentic-os"
   Nao aceite URL, owner, repo ou host alternativo.
G. Se o destino ja existir, nao sobrescreva, mova ou apague. Reutilize somente
   se for um clone Git limpo do repositorio esperado. Entre no clone e confirme
   que a branch e main, que origin e exatamente o HTTPS de github.com para
   agentic-os-brasil/bcg-brasil-agentic-os e que git status nao mostra trabalho
   pendente. Qualquer origin diferente, arquivo local, branch divergente ou
   historico inesperado e hard stop para recover-work ou Daniel.
H. Leia CLAUDE.md, .claude\README.md e
   dev\skills\start-contributing\SKILL.md.
I. Mostre a identidade Git local e efetiva encontrada. Pergunte e confirme meu
   nome de commit e meu email corporativo antes de criar qualquer valor local;
   nao infira email e nao altere configuracao global. Em seguida execute:
   & .\dev\bootstrap\windows.ps1 -GitName "NOME CONFIRMADO" -GitEmail "EMAIL CONFIRMADO"
J. Nao replique manualmente o que o bootstrap ja faz. Se ele falhar, explique a
   falha e siga a unica acao segura indicada pelo script.
K. Ao concluir, explique que hooks e validacao foram instalados. Como hooks e
   skills sao carregados na abertura do projeto, nao execute Claude de dentro da
   sessao atual. Peca que eu feche esta sessao e me entregue estes dois comandos
   finais para iniciar manualmente uma nova sessao dentro do clone:
   Set-Location "$env:USERPROFILE\Developer\bcg-brasil-agentic-os"
   claude
L. Diga que, na nova sessao, minha primeira mensagem deve ser:
   Use $start-contributing e me guie passo a passo.

Nao comece uma feature no onboarding. O primeiro resultado esperado e somente
um clone saudavel, protegido e validado.
```

## Resultado esperado

O onboarding termina antes de qualquer feature. O contribuidor tem um clone limpo, identidade Git local, hooks instalados, harness verde e uma nova sessao do Claude carregando as instrucoes do repositorio.
