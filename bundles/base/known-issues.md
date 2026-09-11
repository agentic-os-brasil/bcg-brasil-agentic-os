# Maestro — problemas conhecidos desta versão

Lista curada de issues conhecidas do release atual. Mantida no ZIP para que `maestro-doctor` possa reportar problemas conhecidos sem depender de rede.

Cada entrada segue o formato:

```
## <id-curto>
- **Sintoma:** o que o usuário observa.
- **Causa:** por que acontece.
- **Contorno:** ação segura, sem terminal e sem edit manual.
```

Ordem: mais recente no topo. Remover entradas resolvidas quando a versão que corrige for publicada.

---

## isolamento-entre-casos-nao-e-barreira-dura

- **Sintoma:** nenhum. É um limite do desenho, não um defeito observável.
- **Causa:** o guard que impede escrita no caso errado lê dois arquivos de
  controle (`brain/accounts/.active` e `.pending`) que são graváveis pelas mesmas
  ferramentas que ele filtra. Duas chamadas em sequência — declarar outro caso
  como pendente, depois escrever nele — passam. Fechar isso quebraria a criação
  de caso e a troca de caso ativo, que não têm outro canal de escrita.
- **Contorno:** nenhum é necessário no uso normal. O guard protege contra engano,
  que é o risco real do dia a dia. Se o seu contexto exige barreira dura entre
  clientes — auditoria, cliente com restrição contratual — fale com o time BCG
  Brasil AI antes de usar a mesma instalação para os dois.

---

## hooks-nao-executados

- **Sintoma:** O Maestro funciona, mas algumas coisas automáticas não acontecem: ele não lembra do contexto entre conversas, o fechamento de fim de dia nunca é oferecido sozinho, e a separação entre casos não é aplicada. Em algumas máquinas aparece uma mensagem de erro repetida a cada mensagem enviada.
- **Causa:** As rotinas automáticas do Maestro são scripts que o Claude Code executa em segundo plano. Eles precisam do `bash`, que vem no macOS mas **não** vem no Windows por padrão — ele chega junto com o Git for Windows ou o WSL. Sem `bash`, os scripts não rodam. O Maestro continua abrindo porque o próprio assistente monta a pasta `brain/` na conversa, mas essa versão é mais simples: parte da estrutura do seu perfil não é criada, e nada é injetado no início da sessão.
- **Contorno:** Avisar o time BCG Brasil AI — é a instalação da máquina, não a sua pasta, e não tem conserto por dentro do chat. Enquanto isso o Maestro segue utilizável: peça explicitamente "lembra disso pra próxima" quando quiser guardar algo, e "fecha o dia" no fim do expediente. Não reinstalar nem apagar a `brain/`: o problema não está nela.

---

## preferences-json-migration

- **Sintoma:** Após atualizar para v0.1.9+, preferências de estilo definidas no onboarding (como `interaction_profile`) deixam de ser injetadas na sessão de forma silenciosa.
- **Causa:** Versões anteriores ao v0.1.9 criavam `brain/owner/preferences.json` como arquivo canônico de preferências. A partir do v0.1.9 o arquivo canônico é `brain/owner/style.json` (criado pelo onboarding). Se o `style.json` não existir, a injeção de preferências simplesmente não ocorre — sem erro visível.
- **Contorno:** Rodar `maestro-onboarding` novamente (diga "rodar onboarding" no chat). O onboarding cria `style.json` com as preferências confirmadas. O arquivo `preferences.json` antigo pode ser ignorado — não causa erro.

---

## claude-project-dir-nonstandard-path

- **Sintoma:** Ao abrir a pasta em `/tmp`, em um caminho com espaços, em drive externo ou em qualquer path não-padrão, a workspace `brain/` pode não ser criada na primeira sessão e nenhum erro visível aparece.
- **Causa:** O hook `first-run-scaffold.sh` depende da variável `CLAUDE_PROJECT_DIR` injetada pelo Claude Code CLI. Em paths não-padrão essa variável pode não ser setada e o fallback para `.` cai no diretório corrente do CLI (não da pasta do projeto).
- **Contorno:** Abrir a pasta em um caminho canônico (`~/Documents/Maestro`, `~/Desktop/Maestro`) ou rodar `maestro-doctor` para regenerar `brain/` explicitamente.
