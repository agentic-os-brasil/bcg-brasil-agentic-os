# Spec E — Doctor, origem e migração verificável

**DRAFT / F1 BLOCKED.** #434 estende migração; #437 implementa doctor.
Consome [Spec A](spec-A-data-home-decoupling.md) e [Spec F](spec-F-trust-release-pipeline.md).

## Origem e diagnóstico somente leitura

`/maestro:doctor` mostra versão do plugin/runtime, pasta ativa do marketplace,
data home resolvido e origem: `migrated`, `fresh-confirmed` ou
`fresh-unconfirmed`. Mostra PASS/FAIL por check, autoridade usada e uma próxima
ação suportada. Observar ausência não equivale a confirmar instalação nova.

Home ausente ou `fresh-unconfirmed`: onboarding pergunta sobre ZIP anterior
antes de qualquer escrita de estado. “Sim” requer path explícito e mesma
migração #434; “não” autoriza origem `fresh-confirmed`. Home populado e legado
sem prova de vínculo é ambíguo: FAIL com os dois paths, sem merge automático.
Resolver e doctor não escrevem para transformar esse estado em PASS.

Checks incluem assinatura/autoridade quando disponível, hashes dos arquivos,
helper correto para plataforma, igualdade plugin↔runtime, resolução do home e
integridade da transação. Mismatch é FAIL nomeado: reabrir sessão na versão
completa verificada ou seguir recall; não recomendar update como correção genérica.
Prova de execução de hook é separada de configuração e validação do manifesto.
Verificação local offline não atesta freshness de recall; informar UNKNOWN.

## Uma migração, reaproveitando o motor

O motor atual, [ZIP migration v1](../../../../acceptance/migration/CONTRACT.md),
preserva `data/`, mapeia conteúdo para brain e retém namespaces. Seu rollback
atual apenas revoga a transação (`restored_runtime=false`), preserva arquivos
e manda reabrir a instalação anterior. Não restaura runtime por conta própria.

#434 amplia esse motor para o home final, sem reimplementar migração em
Python, shell ou MCP. Fixtures 0.1.11 e candidata 0.1.12 cobrem:

1. Inventariar origem completa, inclusive ocultos e diretórios vazios; gerar
   plano imutável com digests, mapeamento, retidos e transformações versionadas.
2. Copiar para destino confinado, sem sobrescrever estado existente; staging
   e gravação atômica mantêm as proteções atuais de path, symlink e colisão.
3. Verificar origem, destinos e acesso aos retidos; comparar bytes autorais e
   saída esperada das transformações. Divergência bloqueia commit.
4. Commit durável vincula origem, home, plano e recibo. Depois disso, cumprir
   a preservação `data.migrated/` pedida em #434, atualizando referências aos
   retidos com segurança. Renomear não pode invalidar recibo ou isolamento.
5. Repetição reconhece transação concluída; interrupção retoma apenas outputs
   pertencentes ao mesmo plano. Conflito, drift ou edição desconhecida bloqueiam.
6. Rollback preserva origem e edições posteriores, revoga ativação e valida
   abertura do ZIP anterior. Restauração funcional exigida em #434 precisa de
   prova adicional ao atual recibo de revogação; não declarar já implementada.

O contrato do layout físico final, marker e controle/legacy map fecha em #432
antes do freeze F1; #433/#434 o implementam e validam. Uma fase intermediária com brain no produto não
é pré-requisito para o colega de campo. Qualificação nova nunca reutiliza
receipt histórico como PASS fresco após alterações autorais.

## Aceite

- [ ] Dry-run/status/diagnóstico não criam estado. Fresh-unconfirmed não dispara scaffold.
- [ ] Fixtures positivas, ambíguas, interrompidas e adulteradas verificam
  equivalência, idempotência, retomada e rollback sem perda.
- [ ] Último ZIP pelo fluxo `build-release.sh`/`build-update-package.sh` e uma
  instalação real migrada com doctor PASS satisfazem F2; distribuição é HITL.
- [ ] Install/update/recall do plugin reportam versão, home/origem e mismatch;
  hashes alterados, runtime errado ou autoridade ausente não viram PASS.
- [ ] Colega que pulou o último ZIP é encaminhado à migração explícita antes
  de qualquer escrita nova; execução de hooks respeita essa mesma barreira.
