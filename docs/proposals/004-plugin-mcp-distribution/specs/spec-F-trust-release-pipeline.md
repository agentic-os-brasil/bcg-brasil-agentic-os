# Spec F — Trust Model + Release Pipeline

**Status:** DRAFT — proposal 004 (pendente aprovação do owner)
**Implementa:** Decisão D4 do [PRD](../PRD.md) · Stories US-D1, US-D3, US-S1 · Ticket T5 do task graph
**Depende de:** Spec B (conteúdo dos 3 plugins), Spec C (binário `maestro-mcp`). Consumida por: Spec E (doctor verifica o que este pipeline produz).
**Gate associado:** G2 (piloto instala a partir deste pipeline).

---

## 1. Problem

**Maintainer (US-D1):** cada release hoje exige build local no macOS do maintainer, envio por email e suporte individual. Não há canal de revogação (US-D3), não há como saber quem está em qual versão, e um passo manual esquecido produz release inconsistente.

**Security review (US-S1):** o ZIP por email não oferece cadeia auditável da release ao código executando na máquina do colega. O Maestro **já tem** a resposta para isso — manifest de release assinado Ed25519 com registry local de chaves (Spec 020) — mas a migração para plugin cria a tentação de rebaixar o trust model para "confia no GitHub" (o marketplace É um repo git). A Decisão D4 veda esse rebaixamento: o plugin herda a cadeia de proveniência do ZIP, não a substitui pela confiança no provedor.

## 2. Solution

Um pipeline de release com **um trigger e zero passos manuais** entre tag e release instalável:

```
git tag vX.Y.Z
  → cross-compile maestro-mcp (darwin-arm64, darwin-amd64, windows-amd64)
  → gerar manifest de release assinado Ed25519 (reusa Spec 020)
  → atualizar .claude-plugin/marketplace.json
  → publicar
```

**Abort-all:** se o build de qualquer target falhar, a release inteira aborta — nunca release parcial por plataforma (US-D1). Falha em qualquer passo deixa o marketplace no estado anterior, intacto.

Revogação: operação no `marketplace.json` (remover/pinar versão) bloqueia novos installs; doctor + SessionStart notice sinalizam a base já instalada.

## 3. Design

### 3.1 Pipeline — passos e invariantes

| Passo | Produz | Invariante fail-closed |
|---|---|---|
| 1. Tag `vX.Y.Z` | trigger único | Versão canônica `MAJOR.MINOR.PATCH` (disciplina Spec 020); tag duplicada ou re-publicação de versão existente → rejeitada (versão de release é globalmente imutável) |
| 2. Cross-compile `maestro-mcp` | 3 binários: darwin-arm64, darwin-amd64, windows-amd64 | Qualquer target falha → **abort-all**; build determinístico por target |
| 2b. Assinar nativamente os binários | binários codesigned+notarized (macOS Developer ID + notarytool) e Authenticode (Windows) | **Passo default, não condicional** (Walter pass — binário não notarizado em macOS gerenciado BCG cai em Gatekeeper): notarização falha → abort-all. Binários embutidos no plugin (Spec C §2 default) |
| 3. Gerar manifest assinado | `release-manifest.json` + `.sig` (Ed25519 destacado sobre os bytes exatos — Spec 020) | Manifest cobre: hashes do conteúdo dos 3 plugins, sha256 de cada binário por target, versão única plugin+runtime. Sem assinatura válida não existe passo 4 |
| 4. Atualizar `.claude-plugin/marketplace.json` | entrada da nova versão, pinada por ref/sha | Escrita só após manifest assinado existir; commit atômico |
| 5. Publicar | release disponível para `claude plugin update` | Nenhuma intervenção manual entre 1 e 5 (US-D1 AC) |

O manifest é a **autoridade portátil** da release (Spec 020): o trust não está no nome do repo nem no provedor. O `marketplace.json` é canal de distribuição e descoberta; o manifest assinado é o que o doctor (Spec E §3.2.1) verifica na máquina do colega.

### 3.2 Trust model

Camadas, da mais forte para a mais contextual:

1. **Manifest Ed25519 (Spec 020, reusado — não rebaixado).** Assinatura destacada sobre os bytes exatos; verificação contra registry local de chaves aprovadas com janela de validade e revogação de chave (schema existente `release-authority-registry`). Cobre: conteúdo dos plugins (hash por arquivo), binário `maestro-mcp` (sha256 por target), versão. Chaves privadas **nunca** entram no repositório (disciplina Spec 020 mantida).
2. **Version pinning por ref/sha.** A entrada no `marketplace.json` pina a versão a um ref/sha imutável — install resolve bytes idênticos para todos, por construção ("mesmo comportamento em qualquer máquina", PRD §2).
3. **Branch protection + access control da org** no(s) repo(s) de release: documentados como parte do trust model entregue ao security review (US-S1 AC). São camada de defesa do canal, **não** a raiz de confiança — a raiz é a assinatura.
4. **Bootstrap do binário** (fronteira com Spec C): sha256 esperado + chave de verificação pinados DENTRO do plugin, não baixados do mesmo canal do binário (US-S2). Esta spec garante que o pipeline produz esses pins; a Spec C garante que o bootstrap os impõe.

**Nota de escopo para o security review (US-S1):** na fase plugin, o que executa na máquina do colega não são só binários Go — **hooks executáveis entregues pelo plugin estão no escopo do review desde a fase plugin**, com a mesma exigência de proveniência (cobertos pelo hash de conteúdo do manifest). O pacote de evidência para o review inclui: manifest + registry de chaves, provas de branch protection/access control, e o relatório de conformance do doctor como verificação no endpoint.

### 3.3 Marketplace: repo próprio vs dedicado (Q1 do migration-plan — decisão do Daniel)

| | **Opção 1 — marketplace no próprio repo** (`bcg-brasil-agentic-os`) | **Opção 2 — repo dedicado `maestro-releases`** (recomendada) |
|---|---|---|
| Acesso dos colegas | Exige conceder acesso de leitura ao repo de desenvolvimento inteiro (harness, histórico, conteúdo sensível) | Colegas acessam apenas o canal de distribuição; repo contém só artefatos reviewed de release |
| Superfície de exposição | Todo o histórico git + conteúdo não-release | Mínima — alinhada à distribution boundary da Spec 020 (releases nunca contêm harness de desenvolvimento, credenciais, dados de usuário) |
| Operação | Zero repos novos; pipeline publica in-place | Pipeline faz push cross-repo (um passo a mais, automatizado) |
| Fallback Q2 (se auth EMU/SAML falhar — Gate G2) | Tornar público é inviável (conteúdo sensível junto) | Tornar público é decisão isolável: só conteúdo auditado de release estaria exposto |
| Blast radius de um erro de permissão | Alto | Baixo |

**Recomendação: Opção 2 (dedicado).** Separa canal de distribuição de conteúdo sensível, reduz blast radius, e preserva o fallback da Q2 como decisão explícita e barata. Custo: um push cross-repo no passo 5 do pipeline. **Daniel decide antes do freeze desta spec** (migration-plan Q1).

### 3.4 Canal de revogação (US-D3)

**Bloquear novos installs:** remover (ou pinar para versão anterior) a entrada da versão defeituosa no `marketplace.json`. Efeito imediato: `claude plugin install`/`update` não resolve mais a versão revogada. A operação de revogação é a única escrita manual legítima no `marketplace.json` fora do pipeline — e deve ser um comando/script único, não edição à mão.

**Sinalizar a base já instalada** (quem já tem a versão revogada):

- **Doctor** (Spec E, check 5): versão instalada ausente/revogada no marketplace → FAIL com instrução de update.
- **SessionStart notice** (Spec B, US-C3): o check diário de versão (≤1/dia, cache local, falha silenciosa offline) compara também contra o estado de revogação → notice de 1 linha "versão vX.Y.Z revogada — rode `claude plugin update`". Nunca bloqueia a sessão, nunca auto-atualiza (doutrina de consent, PRD §7).
- **Telemetria** (US-D2 / Spec 054): canary reporta `plugin_version` + `runtime_version` → o maintainer observa a drenagem da versão revogada e sabe quem suportar.

Revogação ≠ rollback forçado: o Maestro nunca desinstala nada remotamente. Revogar bloqueia distribuição e avisa; o update é ação do usuário.

### 3.5 Failure modes do pipeline

| Failure mode | Comportamento |
|---|---|
| Build de 1 target falha | Abort-all; nada publicado; marketplace intacto (US-D1 AC) |
| Assinatura falha (chave indisponível/expirada no registry) | Abort antes do passo 4; release vira candidate não publicável — nunca release sem manifest assinado (sem override unsigned, disciplina Spec 020) |
| Publicação interrompida entre manifest e marketplace.json | Re-run do pipeline converge (passos idempotentes por versão); versão nunca fica instalável sem manifest verificável correspondente |
| Tag re-usada / manifest alterado atrás de versão existente | Rejeitado — imutabilidade global de versão (Spec 020) |

## 4. Acceptance Criteria

- [ ] **AC-F1** US-D1: do `git tag` à release disponível para `claude plugin update`, zero passos manuais; execução única do trigger produz os 3 binários + manifest assinado + marketplace atualizado.
- [ ] **AC-F2** Failure mode abort-all: falha induzida no build de 1 target → nada é publicado, `marketplace.json` permanece no estado anterior byte a byte.
- [ ] **AC-F3** Manifest da release cobre hashes do conteúdo dos 3 plugins + sha256 dos 3 binários + versão; doctor (Spec E) verifica assinatura e conteúdo em máquina limpa com PASS.
- [ ] **AC-F4** Adulteração pós-publicação (byte alterado em plugin ou binário) → doctor FAIL; a assinatura não valida bytes alterados (fail-closed herdado da Spec 020).
- [ ] **AC-F5** Version pinning: duas máquinas instalando a mesma versão resolvem bytes idênticos (hash comparado).
- [ ] **AC-F6** US-D3 revogação: remover/pinar versão no `marketplace.json` → novo install da versão revogada falha; doctor FAIL + SessionStart notice na instalação existente (par com AC-E9).
- [ ] **AC-F7** US-S1 pacote de auditoria: manifest + registry de chaves + documentação de branch protection/access control + escopo de hooks executáveis entregues ao security review antes do rollout (G2→G4).
- [ ] **AC-F8** Pipeline re-executado após interrupção em qualquer passo converge para release consistente ou aborto limpo — nunca estado intermediário instalável.
- [ ] **AC-F9** Tentativa de re-publicar versão existente com bytes diferentes → rejeitada.

## 5. Out of Scope

- Conteúdo e estrutura dos 3 plugins e do `.mcp.json` — Spec B.
- Bootstrap/instalação do binário na máquina do usuário — Spec C (esta spec entrega os pins; aquela os impõe).
- Verificação no endpoint (doctor) — Spec E (esta spec produz o que aquela verifica).
- MCP server remoto hospedado, OAuth — v0.4+ (PRD §7).
- ~~Assinatura/notarização nativa como condicional~~ — promovida a passo default do pipeline (2b) pelo Walter pass; o que resta fora de escopo é assinatura de ARTEFATOS NÃO-binários além do manifest Ed25519 (Spec 020 cobre).
- Auto-update silencioso — vedado pela doutrina de consent.

## 6. Open Questions

| # | Questão | Decide | Quando |
|---|---|---|---|
| QF-1 | **Q1 do migration-plan (reapresentada aqui):** marketplace no próprio repo ou `maestro-releases` dedicado — recomendação §3.3: dedicado | Daniel | Antes do freeze da Spec F |
| QF-2 | Onde o pipeline executa e custódia da chave de assinatura em execução automatizada (GitHub Actions com secret? passo de assinatura local do maintainer como único gate humano?) — a Spec 020 veda chave privada no repo; "um trigger" precisa conviver com essa custódia. Recomendação: pipeline automatizado com passo de assinatura sob custódia do maintainer, ainda assim sem edição manual de artefatos | Daniel + security review | Freeze Spec F |
| QF-3 | Notarização/Authenticode viraram passo default (2b). Resta: exigências ADICIONAIS do endpoint security BCG (MDM allowlist, EDR)? Certificados Developer ID/Authenticode — quem custodia e em nome de quê (BCG? org GitHub?) | Security review + Daniel | Antes do G3 |
| QF-4 | Formato da entrada de revogação: remoção da versão vs campo explícito `revoked` no `marketplace.json` (campo explícito permite distinguir "revogada" de "nunca existiu" no notice do doctor). Recomendação: campo explícito | Daniel | Freeze Spec F |
| QF-5 | O schema do manifest v1 (Spec 020) comporta os novos artefatos (3 plugins + 3 binários MCP) ou exige extensão versionada do schema? Verificação no repo sobre `schemas/release-manifest.schema.json` | implementação | Início de T5 |
