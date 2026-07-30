# Onboarding completo — recuperação de trabalhos anteriores no SharePoint

## Resultado que esta capacidade entrega

Uma pessoa pode pedir explicitamente:

> “Quero o deck que apresentei para o CEO da Suzano em 2023 sobre PLANTIO.”

Depois que um catálogo assinado válido tiver sido importado, o Maestro procura
nesse catálogo organizacional governado e devolve candidatos com nome, link de
origem, sinais que justificam o match, data de modificação, frescor e nota de
autorização. O usuário decide qual arquivo abrir. A coleta corporativa ao vivo
e o release de piloto ainda não estão disponíveis.

O valor não é “dar acesso ao SharePoint para um agente”. O valor é recuperar
capital intelectual anterior com menos busca manual, sem indexar o tenant
inteiro, sem misturar essa base com a memória geral do Maestro e sem criar um
atalho para a política corporativa.

Este guia é deliberadamente detalhado. Ele serve a:

- usuários do piloto, que querem recuperar um material;
- owners, que autorizam pastas e acompanham frescor;
- operadores Claude, que qualificam a coleta nativa;
- segurança e privacidade, que precisam inspecionar a fronteira;
- suporte, que precisa distinguir `unavailable`, catálogo vazio, stale e
  revogação;
- engenharia, que valida receipts, snapshots e publicação atômica.

## 1. A regra corporativa em uma frase

**Claude pode coletar metadados de pastas SharePoint explicitamente
matriculadas; Codex não pode se conectar ao SharePoint.**

No Codex:

- a capability de coleta permanece `unavailable`;
- o motivo é `corporate_policy`;
- não se instala plugin, token, proxy, script ou credencial para contornar a
  regra;
- só podem ser consumidos snapshot e receipt locais, produzidos pelo coletor
  Claude autorizado e verificados criptograficamente;
- a busca ocorre no índice local já publicado.

No Claude:

- uma configuração local não é prova de integração nativa;
- o coletor só fica `supported` depois de um trial nativo qualificado;
- o acesso fica limitado aos roots matriculados;
- a chave privada de assinatura pertence ao runtime autorizado e nunca entra no
  Maestro, no Codex, no repositório ou no chat.

## 2. O que existe hoje — e o que ainda não existe

| Camada | Estado | O que isso prova |
| --- | --- | --- |
| Contrato, schemas e threat model | Implementado e validado localmente | Enrollment, snapshot, receipt, revogação e consulta têm regras testáveis. |
| Índice local determinístico | Implementado e validado localmente | Um snapshot assinado pode ser publicado e consultado sem SharePoint ao vivo. |
| CLI `bcgos prior-work` | Implementado e validado localmente | Actor local, enrollment, status, import e find funcionam sobre artefatos locais; sync-due é diagnóstico fail-closed no build atual. |
| Skill de recuperação | Implementada | A capacidade só deve ser ativada por pedido explícito de trabalho anterior. |
| Coletor SharePoint no Claude | Protocolo definido; trial nativo pendente | Ainda não existe evidência suficiente para declarar `supported`. |
| Coleta no Codex | Proibida por política corporativa | Não há fallback técnico autorizado. |
| Release de piloto | Gate separado | Código e testes não equivalem a distribuição assinada ou aceite em dispositivo limpo. |

O estado atual é útil para desenvolvimento, revisão de segurança e teste com
fixtures sanitizadas. Uso com tenant corporativo exige o trial Claude e os
gates de distribuição aplicáveis.

## 3. Arquitetura e separação de responsabilidades

```text
SharePoint corporativo
        │
        │ somente roots matriculados
        ▼
Claude nativo qualificado
  coleta metadados → normaliza snapshot → assina receipt
        │
        │ arquivos locais; nenhuma chave privada
        ▼
Importador determinístico do Maestro
  valida schema + assinatura + tenant + roots + sequência + watermark
        │
        │ publicação atômica + audit record
        ▼
Wiki organizacional de prior-work
  separada de memória, owner context e workspaces de cliente
        │
        │ somente após intenção explícita
        ▼
bcgos prior-work find
  resultados + origem + frescor + justificativa
```

### Por que a wiki é separada

A wiki de prior-work não é:

- a wiki geral do produto;
- a memória pessoal do usuário;
- o workspace de um cliente;
- um transcript de conversas;
- uma cópia local de todos os documentos;
- uma base vetorial invisível.

Ela é um catálogo organizacional governado, ativado somente quando o usuário
pede para recuperar trabalho anterior. Essa separação reduz exposição
acidental, permite revogação e evita que contexto histórico apareça em tarefas
não relacionadas.

### O que o V1 indexa

O V1 é metadata-first. Cada item elegível pode conter:

- identidade opaca do item e do root;
- nome e segmentos de caminho;
- URL SharePoint autorizada;
- timestamps, tamanho, media type e ETag;
- facetas limitadas de cliente, projeto, tema, ano, audiência, pessoas e
  apresentadores;
- termos de busca delimitados;
- sensitivity e status.

O V1 não precisa baixar ou extrair o conteúdo integral do deck para localizar o
arquivo. Enriquecimento de conteúdo com MarkItDown ou Docling é uma evolução
posterior, com autorização, fidelidade, retenção e revogação próprias.

## 4. Papéis e responsabilidades

| Papel | Responsabilidade | Não pode |
| --- | --- | --- |
| Usuário | Fazer pedido explícito, revisar frescor e escolher o resultado. | Expandir roots ou ignorar stale/revocation. |
| Owner da matrícula | Aprovar tenant, roots, prazo, cadence, limites e finalidade. | Autorizar “tenant inteiro” por conveniência. |
| Operador Claude | Executar e evidenciar o trial nativo; proteger a chave privada. | Copiar chave privada para Maestro, Codex, Git ou chat. |
| Maestro local | Validar, publicar, consultar e revogar de forma determinística. | Conectar-se silenciosamente ao SharePoint. |
| Codex | Desenvolver, testar e consultar artefatos locais autorizados. | Usar connector SharePoint ou simular a coleta. |
| Segurança/privacidade | Aprovar escopo, retenção, evidência e resposta a incidente. | Tratar configuração local como prova de controle efetivo. |
| Suporte | Diagnosticar estados e preservar last-known-good. | Forçar import, apagar barreira ou editar índice manualmente. |

## 5. Pré-requisitos do piloto

Antes da primeira matrícula, confirme:

- [ ] release privado autorizado e verificado;
- [ ] owner nominal do catálogo organizacional;
- [ ] finalidade única `prior_work_retrieval`;
- [ ] lista explícita de pastas, não sites ou tenant genéricos;
- [ ] origens `https://...sharepoint.com` aprovadas;
- [ ] prazo de autorização e campo reservado para futura reconfirmação de
  expansão;
- [ ] timezone IANA, por exemplo `America/Sao_Paulo`;
- [ ] cadence e limite de stale;
- [ ] tipos e tamanhos máximos;
- [ ] chave pública Ed25519 e key ID do coletor Claude;
- [ ] política de revogação e contato de incidente;
- [ ] fixture sanitizada para o trial;
- [ ] confirmação de que Codex não terá conexão SharePoint.

Se algum item estiver aberto, mantenha a capability de coleta como
`unavailable`.

### Handoffs administrativos obrigatórios no estado atual

O onboarding ainda não é autossuficiente para um operador comum porque três
insumos dependem do coletor/administrador que ainda será qualificado:

1. **Refs opacas dos roots:** um administrador SharePoint ou o trial Claude
   autorizado precisa resolver site, drive/library e folder dos caminhos
   aprovados e entregar somente as refs necessárias. O Maestro não faz
   descoberta tenant-wide para obtê-las.
2. **Par de chaves do coletor:** o ambiente seguro do coletor gera a chave
   Ed25519, mantém a chave privada e entrega ao owner somente key ID e chave
   pública. O CLI não provisiona nem guarda a chave privada.
3. **Primeiro snapshot/receipt:** o coletor Claude qualificado produz e assina
   os arquivos. Enquanto esse collector não existir, somente fixtures
   sanitizadas de engenharia podem exercitar o importador.

Esses são blockers administrativos/native-runtime, não passos que devam ser
improvisados com scripts, connector no Codex ou edição do store.

## 6. Descobrir a identidade local autorizada

Execute:

```text
bcgos prior-work actor
```

Resposta esperada:

```json
{
  "schema_version": 1,
  "actor_ref": "actor-local-<opaque-hash>",
  "binding": "local_os_principal"
}
```

O `actor_ref` é derivado do principal autenticado do sistema operacional. Ele
não deve ser inventado nem passado como flag. O valor da matrícula precisa ser
exatamente o valor emitido no dispositivo autorizado.

Se outro usuário do sistema operacional tentar consultar a mesma matrícula, a
autorização falha fechada.

## 7. Construir a matrícula

A matrícula é create-only. Uma nova execução não substitui silenciosamente a
anterior. O V1 ainda não implementa migrate, revoke, unenroll ou key rotation.
Portanto, mudança de root, key, prazo ou política fica **indisponível** até um
procedimento governado e uma superfície determinística serem implementados.
Nunca edite ou apague manualmente o store para simular rematrícula.

Exemplo estrutural — substitua todos os valores indicados e valide antes de
usar:

```json
{
  "schema_version": 1,
  "tenant_ref": "tenant-corporate-opaque",
  "purpose": "prior_work_retrieval",
  "policy_version": "prior-work-policy-v1",
  "authorized_by": "actor-local-REPLACE_WITH_ACTOR_OUTPUT",
  "collector_key_id": "claude-sharepoint-collector-key-v1",
  "collector_public_key": "REPLACE_WITH_BASE64_ED25519_PUBLIC_KEY",
  "enrolled_at": "2026-07-29T09:00:00-03:00",
  "authorization_expires_at": "2026-10-29T09:00:00-03:00",
  "scope_expansion_confirm_after": "2026-08-29T09:00:00-03:00",
  "refresh_hours": 24,
  "stale_hours": 72,
  "schedule_timezone": "America/Sao_Paulo",
  "max_item_bytes": 1073741824,
  "max_snapshot_items": 25000,
  "allowed_item_types": ["file", "folder"],
  "allowed_origins": ["https://company.sharepoint.com"],
  "roots": [
    {
      "site_ref": "site-opaque-approved",
      "drive_ref": "drive-opaque-approved",
      "folder_ref": "folder-opaque-approved"
    }
  ]
}
```

Notas importantes:

- refs são opacas; não use nome de cliente como ID técnico;
- a chave pública Ed25519 codificada em base64 precisa ter o formato exigido
  pelo schema; o placeholder acima é intencionalmente inválido;
- `authorization_expires_at` limita o direito de consulta;
- `scope_expansion_confirm_after` registra a intenção de uma futura
  reconfirmação, mas o V1 ainda não implementa nem impõe expansão/rematrícula;
- `refresh_hours` controla a ocorrência; `stale_hours` controla o aviso de
  frescor;
- cada root é a combinação `site_ref + drive_ref + folder_ref`;
- todos os roots precisam aparecer como completos em um full snapshot.

## 8. Confirmar e gravar a matrícula

Revise o arquivo por duas pessoas quando a política exigir. Depois:

```text
bcgos prior-work enroll --stdin --confirm < enrollment.json
```

O `--confirm` é obrigatório. A resposta deve indicar `state: enrolled`, tenant,
policy version e quantidade de roots.

Pare se ocorrer:

- actor diferente do principal local;
- origem fora de SharePoint;
- prazo inválido ou expirado;
- timezone inválido;
- root duplicado ou não aprovado;
- chave pública malformada;
- store já matriculado.

Não “corrija” o erro alterando arquivos internos.

## 9. Qualificar o coletor Claude

Esta etapa acontece no Claude, nunca no Codex.

O trial nativo precisa demonstrar:

1. autenticação usando a identidade corporativa aprovada;
2. acesso somente aos roots matriculados;
3. full snapshot inicial com resultado completo para cada root;
4. normalização determinística e limites respeitados;
5. receipt assinado pela chave correspondente à chave pública matriculada;
6. `producer_runtime: claude`;
7. tenant, roots, policy, sequence, watermark, digest e trigger corretos;
8. nenhum prompt, token ou segredo no snapshot/receipt;
9. revogação ou deleção representada por tombstone/barrier;
10. falha fechada quando um root fica parcial, inacessível ou fora de escopo.

A evidência mínima inclui ambiente, versão, horário, comando/ação nativa,
resultado, hashes dos artefatos sanitizados e revisão do owner. Screenshot de
configuração ou presença de connector não basta.

Até o trial ser aceito, a capability
`sharepoint_work_collection` permanece `unavailable`.

## 10. Importar a primeira publicação

O coletor Claude produz:

- `snapshot.json`: catálogo normalizado full ou delta;
- `receipt.json`: receipt externo assinado, ligado ao digest do snapshot e à
  ocorrência autorizada.

No Maestro:

```text
bcgos prior-work import \
  --snapshot snapshot.json \
  --receipt receipt.json
```

O importador verifica antes de publicar:

- schema estrito e ausência de chaves JSON duplicadas;
- runtime e capability;
- assinatura Ed25519;
- key ID e fingerprint da matrícula;
- tenant, policy e igualdade exata dos roots;
- sequence, previous watermark e watermark;
- digest do snapshot;
- trigger da ocorrência;
- limites de itens, tipos, tamanhos e origens;
- integridade de full/delta e tombstones.

A publicação é atômica e preserva last-known-good. Um receipt declarativo sem
manifest ativo e audit record correspondente não completa uma ocorrência.

## 11. Verificar status e frescor

Execute:

```text
bcgos prior-work status
```

Campos principais:

| Campo | Leitura operacional |
| --- | --- |
| `state` | Estado do catálogo local. |
| `due` | Já existe uma ocorrência de refresh vencida. |
| `stale` | O catálogo passou do limite de frescor definido. |
| `last_sync_at` | Última publicação validada. |
| `watermark` | Ponteiro opaco da coleção ativa. |
| `collection_sequence` | Sequência monotônica aceita. |
| `fingerprint` | Fingerprint da publicação ativa. |
| `items` | Quantidade de itens no catálogo. |
| `refresh_hours` | Cadence planejada. |
| `stale_hours` | Limite para declarar stale. |

`due` não significa necessariamente `stale`. Um refresh pode estar devido e o
catálogo ainda estar dentro da janela aceitável. Resultado stale deve ser
mostrado ao usuário, não escondido.

## 12. Recuperar um trabalho anterior

A busca exige intenção explícita e lê a consulta por stdin. Isso evita ativação
acidental e evita oferecer a query como argumento fácil de persistir em
histórico de shell.

Execute:

```text
bcgos prior-work find --explicit --stdin --limit 5
```

Digite a consulta, por exemplo:

```text
quero o deck que apresentei pro CEO da Suzano em 2023 sobre PLANTIO
```

Finalize o stdin conforme o terminal. O resultado inclui:

- `name` e `source_url`;
- facetas de cliente, tema, ano, audiência e apresentador;
- `matched_terms` e score determinístico;
- data de modificação;
- `catalog_freshness`;
- `authorization_note`;
- watermark e fingerprint do catálogo consultado.

### Como interpretar os candidatos

1. Confirme se o catálogo está fresh ou stale.
2. Compare cliente, ano, tema e audiência.
3. Use `matched_terms` para entender por que o item apareceu.
4. Abra somente o link do candidato autorizado.
5. Não trate o primeiro resultado como verdade sem inspeção humana.
6. Se não houver resultado, refine os termos; não expanda roots
   automaticamente.

O ranking V1 é lexical, explicável e determinístico. Não existe afirmação de
busca semântica perfeita.

## 13. Sincronização periódica

O scheduler deriva as ocorrências de `enrolled_at`, `refresh_hours` e
`schedule_timezone`, com no máximo um catch-up por presença.

Interface:

```text
bcgos prior-work sync-due --runtime claude
bcgos prior-work sync-due --runtime codex
```

No build atual, essa superfície é **diagnóstica e fail-closed**:

- `--runtime claude` sempre retorna `unavailable`, porque o CLI ainda não possui
  um caminho implementado para injetar collector e publication verifier;
- no Codex, retorna `unavailable` com `corporate_policy`;
- uma futura integração Claude só poderá completar depois de implementar a
  injeção nativa e verificar a publicação;
- ocorrência `unavailable` ou failed continua due;
- somente publicação auditada e ligada ao trigger atual gera sucesso;
- o claim interprocesso impede dois coletores concorrentes;
- owner vivo não é tomado por idade; processo morto libera o guard do sistema
  operacional;
- erros persistidos usam taxonomia fechada e não carregam nomes, URLs ou paths
  profissionais.

Não configure um cron alternativo que chame SharePoint pelo Codex.

## 14. Revogação, deleção e catálogo parcial

Revogação tem precedência sobre conveniência de busca.

- delta com tombstone remove item elegível;
- barreira de revogação bloqueia a consulta quando a aplicação fica parcial;
- mudança de policy ou fingerprint invalida publicação incompatível;
- autorização expirada bloqueia consulta;
- root parcial não pode ser apresentado como full completo;
- falha de import preserva last-known-good, mas não apaga uma barreira válida.

Se um arquivo foi removido ou perdeu autorização, o objetivo não é “manter o
resultado até o próximo ciclo”; o objetivo é impedir que ele continue
recuperável.

## 15. Troubleshooting

| Sintoma | Causa provável | Ação segura |
| --- | --- | --- |
| `corporate_policy` no Codex | Comportamento esperado. | Use Codex apenas para índice local; faça coleta no Claude autorizado. |
| `unavailable` no Claude | Trial nativo ou injeção do collector pendente. | Não emule; conclua qualificação. |
| actor mismatch | Matrícula pertence a outro principal local. | Use o dispositivo/usuário autorizado; rematrícula ainda é indisponível no V1. |
| enrollment already exists | Matrícula é create-only. | Não edite o store; registre o blocker e aguarde o lifecycle determinístico. |
| signature/key mismatch | Receipt não corresponde à chave matriculada. | Rejeite e investigue key rotation/proveniência. |
| roots mismatch | Snapshot tentou ampliar ou alterar escopo. | Rejeite e registre um blocker de lifecycle; expansão/rematrícula ainda é indisponível. |
| sequence/watermark replay | Snapshot antigo ou fora de ordem. | Preserve o ativo e gere coleção correta. |
| `due: true` | Refresh venceu ou tentativa não completou. | No build atual, registre o estado; sync Claude ainda é diagnóstico-only. |
| `stale: true` | Última publicação ultrapassou o limite. | Avise o usuário; restaure coleta no Claude. |
| consulta sem resultados | Termos não encontrados ou root não matriculado. | Refine cliente/ano/tema; não amplie escopo automaticamente. |
| revocation fence | Aplicação de remoção ficou incompleta. | Conclua a correção e valide antes de reabrir consultas. |
| claim ocupado | Outro processo está executando a ocorrência. | Aguarde; não apague guard/metadata manualmente. |

## 16. Checklist de aceite do piloto

### Segurança e política

- [ ] Codex não possui nem usa conexão SharePoint.
- [ ] Claude acessa somente roots matriculados.
- [ ] chave privada não aparece em repositório, Maestro, Codex, logs ou chat.
- [ ] actor local e finalidade são validados.
- [ ] expiração é exercitada; o campo de reconfirmação permanece reservado e
  não é tratado como controle implementado.
- [ ] teste de symlink/escape e concorrência está verde.

### Qualidade funcional

- [ ] full snapshot publica os roots exatos.
- [ ] delta preserva watermark e sequência.
- [ ] query Suzano/2023/PLANTIO retorna fixture esperada.
- [ ] resultado explica match e frescor.
- [ ] query sem intenção explícita é rejeitada.
- [ ] catálogo stale é sinalizado.
- [ ] resultado vazio não dispara expansão automática.

### Revogação e resiliência

- [ ] tombstone remove item.
- [ ] aplicação parcial ativa revocation fence.
- [ ] receipt forjado/replayed é rejeitado.
- [ ] dois processos não coletam a mesma ocorrência.
- [ ] processo morto permite recuperação.
- [ ] import inválido preserva last-known-good.

### Operação e produto

- [ ] owner, suporte e incidente têm DRI nominal.
- [ ] trial Claude tem receipt de evidência.
- [ ] release é privado, assinado e aceito em dispositivo limpo.
- [ ] usuário não técnico completa a primeira busca sem ferramenta de
  desenvolvimento.
- [ ] métricas não contêm query, nome de cliente, URL ou conteúdo.

## 17. Métricas que demonstram valor

Métricas recomendadas, sempre metadata-only:

- tempo da intenção até abrir o material correto;
- taxa de sucesso no top 1, top 3 e top 5;
- percentual de consultas com catálogo fresh/stale;
- quantidade de refinamentos até o clique;
- ocorrências due, succeeded, unavailable e failed;
- taxa de revogação aplicada dentro do SLA;
- intervenções de suporte por usuário;
- roots e matrículas revisados dentro do prazo.

Não registrar a frase de busca, nome de cliente, nome de arquivo, URL, path,
conteúdo do documento ou identidade pessoal em telemetria central.

## 18. Roadmap específico desta capacidade

1. **V1 — metadata-first governado:** roots explícitos, busca lexical,
   assinatura, auditoria, frescor e revogação.
2. **V1.1 — experiência de busca:** formulário guiado, filtros explicáveis,
   preview metadata-only e feedback sem texto livre.
3. **V1.2 — cadence e mudanças:** delta otimizado, webhooks aprovados,
   observabilidade de drift e key rotation.
4. **V2 — enriquecimento local seletivo:** MarkItDown para formatos simples e
   Docling para estrutura/OCR/tabelas, somente em arquivos e campos aprovados.
5. **V2.1 — ranking híbrido:** lexical + semântico, com avaliação, explicação,
   ACL/revogação e fallback determinístico.
6. **V3 — escala organizacional:** taxonomia multi-office, ownership por
   domínio, retenção por classe e compartilhamento governado.

Cada etapa exige nova evidência. “Mais inteligência” nunca autoriza “mais
dados”.

## 19. Documentos relacionados

- [Contrato técnico e threat model](../../specs/037-sharepoint-work-retrieval-wiki.md)
- [Protocolo do coletor Claude](../../adapters/claude/sharepoint-work-collector.md)
- [Skill de recuperação explícita](../../bundles/base/skills/find-prior-work/SKILL.md)
- [Onboarding geral do Maestro](maestro-user-onboarding.md)
- [Roadmap de evolução](../roadmap/maestro-evolution-roadmap.md)
- [Licença proprietária](../../LICENSE.md)

## Assinatura do projeto

Maestro — BCG Brasil Agentic OS

Desenvolvido por:

- Daniel Scardini
- Julia Ribeiro
- Marcelho Sanches

Postura pretendida: **Maestro Proprietary License v1.0**, licença fechada e
all-rights-reserved. A minuta não autoriza distribuição externa; entidade
proprietária, cadeia de titularidade, jurisdição e termos comerciais precisam
de aprovação jurídica.
