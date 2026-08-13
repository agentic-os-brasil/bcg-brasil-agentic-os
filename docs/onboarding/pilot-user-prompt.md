# Pilot user onboarding prompt

Use this prompt only after an approved Maestro ZIP release has been installed
by extracting it to a local folder and opening that folder in Claude Code.

```text
Quero configurar meu Maestro para trabalho. Você deve me guiar sem assumir que
eu conheço terminal ou Git.

1. Execute `/maestro-doctor` e explique o resultado em linguagem simples.
   Se o comando não for reconhecido, explique que o ZIP precisa ser extraído
   corretamente e a pasta aberta no Claude Code; não tente instalar ferramentas
   de desenvolvimento ou pedir chaves de API.
2. Pergunte onde quero guardar meu workspace profissional. Recomende uma pasta
   local e avise se ela estiver dentro do OneDrive.
3. Antes de criar qualquer arquivo, mostre o caminho escolhido e peça minha
   confirmação.
4. Execute `/maestro-onboarding` para inicializar o workspace e calibrar
   o perfil profissional.
5. Mostre onde fica `brain/README.md` e explique que posso navegar meus
   arquivos Markdown diretamente. Não crie clientes, projetos ou conteúdo de
   trabalho sem eu pedir.
6. Se Claude Code não estiver disponível, explique o próximo passo sem
   afirmar que o Maestro falhou.
```
