# Spec A — Um data home fora do produto

**DRAFT / F1 BLOCKED.** Contrato proposto em #432; implementação futura #433 e
migração #434. Não é comportamento entregue nem decisão aceita.
Escopo e sequência: [PRD](../PRD.md), [plano](../migration-plan.md).

## Layout proposto

`HOME` é o data home Maestro; `USERHOME` é o diretório do usuário do SO,
obtido pela API do SO (`%USERPROFILE%` no Windows), não o cwd do caso nem o
root do plugin. Default: `USERHOME/.maestro/`.

| Path | Autoridade e uso |
|---|---|
| `HOME/brain/` | Estado do dono; preserva os paths lógicos atuais de brain |
| `HOME/brain/{owner,accounts,daily,craft,learnings,people}/` | Conteúdo mapeado pelo motor ZIP atual |
| `HOME/brain/.maestro/` | Estado de máquina, lifecycle/context e controle dos callers ZIP |
| `HOME/brain/.maestro/migration/state.json` | Único journal mutável de migração; fases e revogação |
| `HOME/brain/.maestro/migration/attempts/ID/{plan,receipt}.json` | Plano e recibo imutáveis da tentativa |
| `HOME/brain/.maestro/origin.json` | Ativação explícita e vínculo à migração, quando houver |
| `USERHOME/.maestro-home.json` | Locator estável, inclusive para descobrir HOME não default sem env |
| `SOURCE/data.migrated/` | Árvore legada preservada; namespaces retidos continuam aqui |

`SOURCE` é o root absoluto da instalação ZIP explicitamente selecionada.
Helper e manifests continuam no produto em `scripts/`; estado persistente
não usa `${CLAUDE_PLUGIN_DATA}`. Não criar executáveis em HOME.

O layout usa `HOME/brain/` para reaproveitar `targetFor` e os paths de
`internal/zipruntime`. Callers passam a distinguir produto e estado; pacotes
com `DataRoot` explícito mantêm a injeção e seu subdiretório contratado. Isto
não autoriza prefixar todos os DataRoot com `brain/.maestro`.

## Resolver único e somente leitura

Retorna `home`, `brain_root`, `selected_by` (`env`, `marker` ou `default`),
`origin`, `writable` e diagnóstico. `writable` é permissão lógica de usar
origem confirmada; resolver e doctor nunca criam ou ativam nada.

1. Se `MAESTRO_HOME` está definida, selecionar seu path absoluto. Valor vazio,
   relativo, inválido ou inacessível é erro, sem expansão de shell ou `~`.
2. Sem env, ler `USERHOME/.maestro-home.json`, se existir. Não buscar markers
   recursivamente em cases, cwd ou plugins.
3. Sem locator, selecionar `USERHOME/.maestro/`. Apenas inspecionar não cria
   home ou locator. Instalação nova no default pode usar somente origin record.
4. Validar path, registros e origem. Locator quebrado nunca cai no default.
   Mesmo com env explícita, locator existente inválido ou apontando para outro
   home é conflito: pode reportar o path selecionado, mas `writable=false`.
   Trocar de home exige operação explícita futura; não é efeito do resolver.

Locator existente cujo destino não é mais um diretório acessível retorna
FAIL `marker-target-missing` ou erro de acesso, com `writable=false`; não
recriar o destino, perguntar por instalação nova nem cair no default.
Sem locator, home ausente ou sem origem é `fresh-unconfirmed`, sem escrita.
Antes de scaffold, perguntar “Usava Maestro por ZIP nesta máquina?”. “Sim”
exige SOURCE explícito e #434; “não” confirma instalação nova. Home populado
sem origem, legado não vinculado ou origem contraditória é FAIL, sem merge.
Nenhum hook infere instalação nova pela ausência de dados.

Roots e ancestrais existentes devem ser diretórios comuns: rejeitar symlinks,
reparse points, troca de tipo e sobreposição HOME↔SOURCE/produto gerenciado.
Não exigir posse exclusiva de ancestrais de sistema; validar posse/ACL de
USERHOME, HOME e registros segundo a plataforma. Destino ausente é reportado
sem `mkdir`; erro de acesso não equivale a ausência. Revalidar caminho físico
e confinamento antes de cada efeito do motor, inclusive substituição concorrente.

## Registros de descoberta e origem

JSON UTF-8 sem BOM, objeto único, sem chaves duplicadas, campos desconhecidos
ou conteúdo após o objeto; máximo 16 KiB por locator/origin. Schema inteiro
diferente de `1` bloqueia com erro de versão. Paths absolutos preservam grafia
do filesystem; comparação e colisões seguem as regras portáveis do motor,
com semântica nativa de drive e separadores. Não assumir drive C:.

Locator v1 tem exatamente `schema_version`, `home` e `attempt_id`. O último
é ID de migração com 32 hex minúsculos, ou string vazia para instalação nova.
Só roteia e vincula a tentativa. Na migração, pode ser reservado depois do
plano durável e antes da cópia, permitindo redescobrir uma interrupção. Reserva
sem ativação válida mantém escrita bloqueada; o locator não autoriza execução.

Origin v1 tem exatamente `schema_version`, `home`, `origin`, `attempt_id` e
`plan_sha256`. `origin` é `fresh-confirmed` ou `migrated`. No primeiro caso,
os dois últimos campos são vazios; no segundo, ID e digest SHA-256 (64 hex
minúsculos) coincidem com plano, recibo e estado committed. `home` deve
coincidir com o path físico selecionado. Publicar origin atomicamente após
confirmação nova ou após commit validado. Commit sem origin exige retomada,
nunca scaffold; locator com ID divergente também bloqueia.

Somente onboarding/migração explícitos escrevem esses registros, com acesso
restrito ao dono e sem substituir autoridade conflitante. Para instalação
nova fora do default, origin precede locator; falha ao publicar locator é
ativação incompleta, exige concluir a mesma operação e preserva o home.
Mesmo selecionado por env, HOME não default exige locator válido correspondente
para liberar escrita. O default não requer locator novo. Registros são evidência local de vínculo,
não assinatura: [Spec F](spec-F-trust-release-pipeline.md) governa confiança do código.

## Equivalência e namespaces retidos

Reusar [ZIP migration v1](../../../../acceptance/migration/CONTRACT.md) e
`internal/zipmigration/`: profile→owner, atlas conhecido→raízes correspondentes,
casos→accounts e transformação `legacy_case_marker`. Destinos copiados:
`HOME/brain/<entry.target>`. Cada path original consta do plano, inclusive
ocultos, diretórios vazios e retidos. Bytes autorais permanecem; transformações
precisam de nome versionado e teste de saída esperada.

`agents`, `workspaces` e `canary` ficam em
`SOURCE/data.migrated/<namespace>`. README, controles e outros paths não
copiados continuam inventariados, sem descarte. `--resolve-legacy` valida o
namespace contra o plano e retorna root atual; não reutiliza o antigo path
`data/`. Paths do plano permanecem relativos à árvore legada; sua raiz física
é derivada da fase, conforme [Spec E](spec-E-doctor-conformance-migration.md).

Estado do dono e conteúdo de workspace são autoridades distintas. `brain`
em `internal/workspace`, `internal/atlas` e `internal/priorwork` pode designar
conteúdo local de caso: esses roots não viram home global. `accounts/` legado
já do dono migra preservando escopo, sem importar outros workspaces ou conceder
acesso entre casos. Credenciais continuam no credential store do SO.

## Aceite de #433

- [ ] Nove pares `.sh`/`.ps1`, skills, scaffold e callers Go usam resolver
  para estado do dono; roots locais de caso e fixtures mantêm seu escopo.
- [ ] Env/marker/default, env vazia, locator quebrado/conflitante, origem
  incompleta e symlink/reparse falham sem escrita.
- [ ] Redescoberta sem env de HOME não default, Windows em drive diferente de
  C: e paths com espaços; inspeção não cria locator.
- [ ] Hooks com produto somente leitura não escrevem nele; snapshot fica
  intacto na troca N→N+1 sem trabalho concorrente.
- [ ] #434 valida mapeamento, retidos, retomada e rollback neste layout.
  F0 e revisão dos contratos continuam obrigatórios antes de F1.
