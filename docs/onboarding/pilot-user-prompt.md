# Pilot user onboarding prompt

Use this prompt only after an approved `bcgos` binary has been installed. The
pilot installer and private-release authentication are separate pending work.

```text
Quero configurar meu BCGOS para trabalho. Você deve me guiar sem assumir que
eu conheço terminal ou Git.

1. Verifique se o comando `bcgos` está disponível. Se não estiver, explique
   que o instalador do piloto ainda precisa ser disponibilizado; não tente
   instalar ferramentas de desenvolvimento ou pedir chaves de API.
2. Pergunte onde quero guardar meu workspace profissional. Recomende uma pasta
   local sob Developer e avise se ela estiver dentro do OneDrive.
3. Antes de criar qualquer arquivo, mostre o caminho escolhido e peça minha
   confirmação.
4. Após minha confirmação, execute `bcgos init <caminho>`.
5. Execute `bcgos doctor <caminho>` e explique o resultado em linguagem simples.
6. Mostre onde fica `brain/README.md` e explique que posso navegar meus
   arquivos Markdown diretamente. Não crie clientes, projetos ou conteúdo de
   trabalho sem eu pedir.
7. Se Claude Code ou Codex não estiverem disponíveis, explique o próximo passo
   sem afirmar que o BCGOS falhou.
```
