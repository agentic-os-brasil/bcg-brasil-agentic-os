# Spec E — Doctor, origem e migração verificável

**DRAFT / F1 BLOCKED.** Contrato proposto em #432; #434 estende migração e #437
implementa doctor. Consome [Spec A](spec-A-data-home-decoupling.md) e
[Spec F](spec-F-trust-release-pipeline.md). Nenhum fluxo abaixo está implementado.

## Origem e diagnóstico somente leitura

`/maestro:doctor` mostra versão plugin/runtime, marketplace ativo, home,
`selected_by`, origem e permissão lógica de escrita. Origem é `migrated`,
`fresh-confirmed` ou `fresh-unconfirmed`; diagnóstico também expõe erro,
fase pendente ou revogação, sem disfarçá-los de instalação nova. Cada check
mostra PASS/FAIL/UNKNOWN, autoridade usada e próxima ação suportada.
Resolver, dry-run, status e doctor não criam nem consertam registros.

Home sem confirmação exige a pergunta sobre ZIP anterior antes da primeira
escrita. “Sim” requer SOURCE explícito e esta migração; “não” confirma
instalação nova. Home populado mais legado sem vínculo comprovado é ambíguo:
FAIL com os paths, sem merge. Erros de leitura nunca contam como ausência.

Checks cobrem assinatura/autoridade, hashes, helper da plataforma, igualdade
plugin↔runtime, resolução/origem e integridade da transação. Mismatch é FAIL:
reabrir sessão na versão completa verificada ou usar recall. Configuração de
hook não prova execução. Offline não atesta freshness de recall: UNKNOWN.
Doctor não executa helper ainda não autenticado para avaliar a própria confiança.

## Uma extensão versionada do motor existente

Reusar `internal/zipmigration` e
[ZIP migration v1](../../../../acceptance/migration/CONTRACT.md): mesmo inventário,
`targetFor`, colisões portáveis, confinamento `os.Root`, staging, hashes,
no-overwrite e validação de retidos. Não criar outro motor Python/shell/MCP.
O v1 permanece legível sob seu contrato original; não converter seu recibo
em prova de uma transação para HOME que ele nunca realizou.

Propor schema `2` para plano, estado e recibo do destino externo. O plano
preserva campos v1 e adiciona `home`, `source_before="data"`,
`source_after="data.migrated"`, `mapping_version="zip-to-home-v1"` e
`previous_runtime`. Este último contém root absoluto da instalação anterior,
versão e digest do manifest de runtime verificado. Não inventar root nem
admitir um pacote anterior já substituído: reusar backup/artefato verificável
do fluxo ZIP, ou exigir a instalação anterior intacta antes de começar.

`project` continua sendo SOURCE. `entries[].source` e `legacy_map` passam a
representar paths lógicos relativos à árvore legada; `entries[].target` é
relativo a `HOME/brain/`. O schema distinto impede interpretar esses paths
como o antigo `SOURCE/brain/`. Preservar ID aleatória, hash dos bytes exatos
do plano e fingerprints do inventário definidos em v1. Não alterar o plano
para acompanhar rename; validar a raiz física a partir da fase.

Usar somente `HOME/brain/.maestro/migration/state.json` como journal mutável.
Além do resultado v1, estado v2 vincula `home`, `phase` e
`rollback_runtime_verified` (inicialmente false). Fases:
`copying`, `verified`, `source_retained`, `active`,
`rollback_pending`, `source_restored`, `rollback_verified`.
Estados externos continuam `planned|partial|blocked|committed|rolled_back`;
fase incompleta nunca autoriza escrita comum. O recibo de commit v2 é imutável,
vincula source/home/plano/fingerprints e `previous_runtime`; revogação e prova
nova de rollback são eventos separados no journal, sem reescrever o recibo.
Esses vínculos são consistência local, não assinatura organizacional nem
proteção contra um dono que reescreva todas as autoridades.

## Ordem de efeitos e recuperação

1. **Planejar.** Parar writers do ZIP e do plugin por este fluxo explícito;
   não migrar com sessões escrevendo em paralelo. Validar SOURCE, HOME e
   runtime anterior, inventariar arquivos ocultos/diretórios vazios e mapear
   cada item. `--dry-run` só retorna o plano em memória. Aplicação explícita
   grava plano e journal antes de reservar o locator da Spec A. Se existir
   locator/origem de outra tentativa ou conteúdo de destino não pertencente
   ao plano, bloquear sem sobrescrever. Retomada usa o mesmo ID.
2. **Copiar.** Só criar outputs ausentes e pertencentes ao plano em
   `HOME/brain/`; staging fica no mesmo filesystem do destino. Copiar,
   fsync, rehash e publicar sem overwrite, como no motor atual. Falta de
   hard-link seguro retorna partial; não trocar por cópia insegura.
3. **Verificar.** Revalidar árvore completa da origem, destinos, transformações
   e runtime anterior. Publicar fase verified após digests corretos. Até aqui
   `SOURCE/data/` permanece intacto e origin não existe.
4. **Reter.** Renomear `SOURCE/data/` para `SOURCE/data.migrated/` somente
   com destino de rename ausente; ambos ficam no filesystem da SOURCE.
   Revalidar inventário completo pela nova raiz antes de source_retained.
   Não é rename entre SOURCE e HOME. Falha de rename/flush no Windows é
   observável e retomável; não presumir atomicidade conjunta entre roots.
5. **Commit e ativação.** Gravar recibo imutável, depois estado committed com
   fase active, depois publicar origin v1 como último limite de ativação.
   Locator reservado só permite descoberta; somente a concordância
   origin↔locator↔journal↔plano↔recibo libera estado. Falha intermediária
   mantém HOME bloqueado até retomar e revalidar. O motor não executa
   scaffold para “completar” bytes ausentes.
6. **Retomar.** Ler locator/plano/journal e revalidar outputs já pertencentes
   à tentativa. Se rename ocorreu antes do journal, verified mais somente
   data.migrated com o fingerprint original permite concluir retenção.
   Se recibo existe sem journal/origin final, validar todos os vínculos antes
   de concluir a ativação. Plano sem locator ainda pode ser retomado com
   SOURCE e HOME explícitos; não procurar destinos pelo disco.

Nunca existe commit atômico global entre origem, home e USERHOME. A ordem
registrada e inspeção das árvores reais resolvem interrupções. `data/` e
`data.migrated/` coexistindo é conflito, mesmo com hashes iguais: não apagar,
fundir nem escolher por data/mtime. Nenhuma das duas presente é perda de
origem e bloqueia. Em copying/verified, apenas data/ é o estado esperado;
em source_retained/active, apenas data.migrated/. Uma diferença legítima
entre rename e journal só é aceita na transição descrita, com inventário
exato. Qualquer drift, symlink/reparse ou mudança de tipo bloqueia.

Após commit, edições regulares no destino geram `target_evolved`, preservam
uso normal e não provocam recópia, como em v1. Recibo histórico não é PASS
fresco. Symlink, tipos incorretos e falhas de IO nunca viram evolução normal.

## Retidos e propriedade dos hooks

`--resolve-legacy agents|workspaces|canary` verifica o namespace pedido contra
o inventário original e retorna `SOURCE/data.migrated/<namespace>` quando
active. Não retorna path da fase antiga nem transforma um diretório ausente
em namespace vazio. Edições autorais comuns no destino não bloqueiam a leitura
dos retidos, mas alteração nos retidos bloqueia essa resolução. Nenhuma cópia
de namespace retido para HOME é implicitamente autorizada.

O último ZIP deve detectar locator vinculado ou `data.migrated/` antes de
logging, recovery ou scaffold. Com migração ativa, seus hooks não escrevem
estado legado nem recriam `data/`; mostram a orientação para a sessão plugin.
Com transação pendente/revogada, informam recuperação e bloqueiam inicialização.
Ausência do locator com data.migrated presente não é instalação nova.

Abrir o root do ZIP com plugin global pode despachar hooks de ambos. O desenho
não presume execução exclusiva: startup e doctor identificam o conflito.
Hooks legados não podem desabilitar o guard do plugin; seu guard deve consumir
o resolver verificado ou recusar a operação, nunca permitir silenciosamente.
Até provar a seleção de hooks no host real, trabalhar a partir desse root é
estado não qualificado. Handoff de writers e guard são critérios próprios;
não desabilitar ambos para obter um primeiro turno bem-sucedido.

## Rollback funcional, separado de revogação

O rollback v1 só revoga e retorna `restored_runtime=false`; ele não satisfaz
#434. A extensão começa por revogar permissão de escrita no HOME, registrando
rolled_back/rollback_pending. Preserva HOME inteiro, inclusive edições novas,
e todos os recibos; não tenta aplicar essas edições na árvore legada.

Com writers parados, verificar fingerprint de data.migrated e ausência de
data/, então renomear de volta para data/ sem overwrite e revalidar. Antes da
retenção, rollback apenas verifica data/ intacto. Registrar source_restored;
se cair após rename e antes do journal, somente data/ com inventário exato
permite retomar. Conflito ou drift bloqueia, preservando ambas as árvores.

Reabrir o runtime anterior verificado em `previous_runtime.root`, com data/
restaurado disponível no root que ele de fato consome. Se for outro root, o
plano só é admissível se já documentar o vínculo real de dados do fluxo ZIP;
não inventar cópia/merge automático. Validar abertura, leitura de conteúdo
legado e uma operação normal sem perda. Registrar versão, manifest verificado
e evidência nova; só então marcar rollback_verified e
`rollback_runtime_verified=true`. `restored_runtime` só pode ser true após
essa prova, nunca pelo sucesso do rename.

A sessão de rollback precisa de seleção de hooks suportada pelo host que
mantenha o runtime ZIP e seu guard como autoridade efetiva. Não alterar
managed settings, remover segurança nem supor que abrir outra janela desliga
o plugin global. Se a seleção exclusiva não puder ser demonstrada, restauração
de arquivos é parcial e F2 não passa. Locator permanece como revogado para
impedir reativação silenciosa do plugin; retomada da migração exige ação
explícita. Repetir rollback verifica o mesmo resultado, sem apagar HOME.

## Aceite

- [ ] Resolver, dry-run/status e doctor não escrevem; fresh-unconfirmed não
  dispara scaffold nem bypass por env explícita.
- [ ] Fixtures 0.1.11/0.1.12 provam equivalência, retidos, colisões e ambiguidades;
  interrupções em cada limite acima retomam sem overwrite ou perda.
- [ ] Recibos v1 não viram v2 implicitamente; adulteração, conflito de locator,
  coexistência data/data.migrated e tipos inseguros ficam bloqueados.
- [ ] Windows nativo prova rename/retomada com arquivos abertos, paths com
  espaços e HOME em drive diferente; teste cruzado não é qualificação.
- [ ] Guard e writers têm handoff observado em sessão real; rollback inclui
  runtime anterior funcional e preserva edições posteriores no HOME.
- [ ] Último ZIP pelo fluxo existente de build/release e uma instalação real
  migrada com doctor PASS satisfazem F2; distribuição permanece HITL.
- [ ] Install/update/recall reportam versão, home/origem e mismatch; colega que
  pulou o último ZIP recebe migração explícita antes de qualquer escrita nova.
