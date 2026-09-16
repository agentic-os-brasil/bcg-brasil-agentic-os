# Mensagem para homologação do Maestro 0.1.12

Oi! Preparei a release **Maestro 0.1.12**, separada das mudanças estruturais,
com foco exclusivo nas correções de hooks e chamadas de Agents.

O Windows agora usa **PowerShell nativo 5.1 ou 7** em todos os hooks. Não há
dependência de Git Bash nem Python no caminho Windows. O macOS continua usando
Bash e Python 3.

O código, os testes e a receita de atualização estão neste PR:
**https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/pull/424**

Antes de distribuirmos o update, preciso da homologação final abaixo:

## 1. Windows real

- testar o ZIP `Maestro-v0.1.12-windows-powershell.zip` no Windows PowerShell
  5.1 e no PowerShell 7;
- confirmar a execução dos seis hooks;
- confirmar que nenhum hook chama Bash ou Python;
- executar uma chamada real ao Agent `yoda` e confirmar o retorno;
- testar a proteção de escrita usando um junction/reparse point.

## 2. macOS

- testar o ZIP `Maestro-v0.1.12-macos.zip`;
- confirmar a execução dos hooks;
- executar uma chamada real ao Agent `yoda` e confirmar o retorno.

## 3. Ensaio de atualização

Em cada plataforma, usar uma **cópia** de uma instalação real `0.1.11`:

1. executar o `PROMPT-1-PREPARAR.txt` no Maestro antigo;
2. conferir a criação do manifesto SHA-256 de `data/`;
3. renomear a instalação antiga para `Maestro-old-0.1.11`;
4. extrair o ZIP correto da plataforma;
5. copiar somente `data/` para o novo Maestro;
6. executar o `PROMPT-2-VERIFICAR.txt`;
7. confirmar que todos os arquivos cobertos pelo manifesto continuam presentes
   e idênticos; somente a allowlist documentada de metadados de lifecycle pode
   mudar na primeira abertura;
8. manter a pasta antiga por pelo menos sete dias para rollback.

Por favor, registre cada item como `PASS`, `FAIL` ou `UNAVAILABLE`, incluindo a
versão do PowerShell, sistema operacional e evidência da chamada real ao Agent.

O pacote que será enviado aos usuários, depois da homologação, é
`Maestro-Update-v0.1.12.zip`. Ele contém os ZIPs das duas plataformas e a receita
passo a passo. **Não distribuir antes de concluirmos os testes de qualificação
acima.**
