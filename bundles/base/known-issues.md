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

## claude-project-dir-nonstandard-path

- **Sintoma:** Ao abrir a pasta em `/tmp`, em um caminho com espaços, em drive externo ou em qualquer path não-padrão, a workspace `data/` pode não ser criada na primeira sessão e nenhum erro visível aparece.
- **Causa:** O hook `first-run-scaffold.sh` depende da variável `CLAUDE_PROJECT_DIR` injetada pelo Claude Code CLI. Em paths não-padrão essa variável pode não ser setada e o fallback para `.` cai no diretório corrente do CLI (não da pasta do projeto).
- **Contorno:** Abrir a pasta em um caminho canônico (`~/Documents/Maestro`, `~/Desktop/Maestro`) ou rodar `maestro-doctor` para regenerar `data/` explicitamente. Ver seção "Runtime dependencies" do `CLAUDE.md` para detalhes.
