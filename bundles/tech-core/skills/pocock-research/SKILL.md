---
name: pocock-research
description: "Investiga uma pergunta e registra conclusões com fontes verificáveis."
---

## Contrato de uso no Maestro

Resolva `$interaction-profile` pelo ID canônico antes de apresentar resultados.
Este método opera no workspace profissional autorizado e segue os contratos de
escopo, dados e autoridade do Maestro. As etapas abaixo não concedem novas
permissões para publicar, enviar mensagens, instalar dependências, acessar
outros casos, executar ações destrutivas ou gastar recursos. Use somente os
adaptadores e ferramentas disponíveis e autorizados; reporte `unavailable`
quando o host não oferecer uma capacidade. Regras locais explícitas de saída,
privacidade e aprovação prevalecem sobre exemplos de diretórios e comandos.
Se o host não tiver uma ferramenta Skill, carregue o método pelo ID no catálogo
instalado; preserve as restrições de invocação explícita pelo usuário.

Spin up a **background agent** to do the research, so you keep working while it reads.

Its job:

1. Investigate the question against **primary sources** (official docs, source code, specs, first-party APIs), not a secondary write-up of them. Follow every claim back to the source that owns it.
2. Write the findings to a single Markdown file, citing each claim's source.
3. Save it where the repo already keeps such notes; match the existing convention, and if there is none, put it somewhere sensible and say where.
