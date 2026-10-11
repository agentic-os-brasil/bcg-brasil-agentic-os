# Spec E — Doctor: Migração + Conformance Check

**Status:** DRAFT — proposal 004 (pendente aprovação do owner)
**Implementa:** Migração da Decisão D1 + Testing Decisions 1 e 4 do [PRD](../PRD.md) · Fallback Codex da Decisão D5 · Tickets T2 (migração) e parte do T4/T5 (conformance) · Stories US-C4, US-R2, parte de US-S1
**Depende de:** Spec A (resolver + markers definidos lá).
**Gate associado:** G1 (migração shipada e verificada) · G2/G3 (conformance em uso no piloto).

---

## 1. Problem

Dois problemas distintos, um veículo (`/maestro:doctor`):

**Migração.** A base instalada (0.1.x/0.2.0 por ZIP) tem `data/` co-locado com o produto. O plugin (Spec B) não pode migrar esse estado — ele nem existe na máquina do colega antes do install, e plugins são imutáveis. Se a migração não acontecer **antes** do primeiro install de plugin, o colega instala o Maestro novo e encontra memória vazia: o pior failure mode possível para adoção (US-C4). A migração precisa viajar no canal que a base instalada já tem: **o último release ZIP (v0.2.x)**.

**Conformance.** Hoje não há sinal pós-install/pós-update de que a instalação está íntegra: versão certa, bytes certos, assinatura válida, runtime sincronizado, hooks rodando. O ritual ZIP mascarava isso com "rode o doctor e torça"; a fase plugin exige um veredito PASS/FAIL observável e auditável (US-S1), que é também o instrumento dos gates G1–G3.

## 2. Solution

O doctor ganha duas responsabilidades separadas e sequenciadas no tempo:

- **(a) Migração** — shipada no último ZIP v0.2.x: detecta `data/` co-locado → move para `~/.maestro/` → grava markers → idempotente → rollback documentado. A base instalada chega à fase plugin **já migrada**.
- **(b) Conformance** — fase plugin em diante: pós-install e pós-update, emite relatório PASS/FAIL contra o manifest assinado da release, reusando a máquina de verificação do `eval-release.sh` existente, reapontada para os artefatos plugin+binário. Inclui detecção de runtime Codex e instalação do fallback de hooks (D5).

## 3. Design

### 3.1 Responsabilidade (a) — Migração

#### 3.1.1 Detecção

Doctor detecta instalação ZIP com `data/` co-locado na raiz da instalação (padrão 0.1.x/0.2.0). Pré-condições para iniciar:

- `data/` existe no local legado E o marker de migração (Spec A §3.4) **não** existe.
- Se `~/.maestro/data/` já existe E não há marker: estado ambíguo (duas fontes possíveis de verdade) → doctor **não migra**, reporta FAIL com as duas localizações e instrução manual. Nunca merge automático, nunca sobrescrita.

#### 3.1.2 Procedimento (copy → verify → commit, nunca move destrutivo direto)

1. **Inventário**: lista recursiva de `data/` legado com sha256 por arquivo.
2. **Cópia**: para `~/.maestro/data/` (criado se ausente), layout preservado 1:1 (Spec A §3.1).
3. **Verificação**: re-hash do destino; qualquer divergência → aborta, destino parcial é removido, origem intacta, FAIL com relatório.
4. **Commit**: grava `~/.maestro/home.json` (origem = path da instalação ZIP) e o marker de migração no local antigo (ponteiro para o home + hash do inventário). Só então a origem é renomeada para `data.migrated/` — **preservada, não deletada** (é o insumo do rollback).
5. **Relatório**: contagem de arquivos, bytes, hash do inventário, localizações de origem/destino.

#### 3.1.3 Idempotência

Segunda execução = no-op observável: marker presente + inventário do home válido → doctor reporta "já migrado em <data>, nada a fazer" e sai PASS. Execução após migração interrompida (ver §3.3) **retoma**, não duplica.

#### 3.1.4 Rollback (documentado e testado — teste 4, PRD §5)

Procedimento inverso, documentado no relatório de migração e no README do release v0.2.x:

1. Renomear `data.migrated/` de volta para `data/` no local legado.
2. Deletar o marker de migração do local antigo.
3. (Opcional, para rollback completo) remover `~/.maestro/data/` — somente após confirmar que o `data/` legado restaurado está íntegro.

Pós-rollback, a instalação ZIP v0.2.x anterior volta a funcionar como antes (circuit breaker do risco "migração corrompe estado", migration-plan §2).

### 3.2 Responsabilidade (b) — Conformance check

#### 3.2.1 Verificação contra o manifest assinado

Pós-install e pós-update, o doctor executa, nesta ordem fail-closed (cada passo só roda se o anterior passou):

1. **Assinatura**: verifica a assinatura Ed25519 destacada do manifest da release contra o registry local de chaves aprovadas — mesma disciplina da Spec 020 (assinatura sobre os bytes exatos; parsing não estabelece trust). **Reusa a máquina de verificação do `eval-release.sh` atual, reapontada** para os artefatos da release plugin (Testing Decision 1 do PRD) — não se escreve um segundo verificador.
2. **Conteúdo do plugin**: hashes dos arquivos instalados vs manifest. Qualquer arquivo ausente, excedente ou com hash divergente → FAIL nomeando o arquivo.
3. **Versão travada**: `plugin_version == runtime_version` (binário `maestro-mcp` em `~/.maestro/bin/` reporta versão; sha256 do binário vs manifest por target). Dessincronia → FAIL com estado distinto "plugin↔runtime dessincronizado" (US-D2 exige esse estado visível também na telemetria).
4. **Data home**: cadeia de resolução da Spec A íntegra (home existe, `home.json` válido, sem `data/` órfão co-locado).
5. **Revogação**: versão instalada ainda presente/não-revogada no marketplace (canal de revogação definido na Spec F §revocação). Versão revogada → FAIL com instrução de update.

Saída: relatório PASS/FAIL por check + veredito agregado + versão + data da release + "assinatura verificada" explícitos (trust signal da lente UX 3 do PRD §6). PASS exige todos os checks verdes.

#### 3.2.2 Fallback de hooks no Codex (D5)

Em runtime Codex, o doctor adicionalmente:

1. **Detecta se hooks entregues pelo plugin executaram** — evidência observável de execução do `session_start` (ex.: receipt gravado pelo próprio hook no data home), não inferência por configuração.
2. **Se não executaram**: instala a projeção `.codex/hooks.json` project-level — mecanismo **já existente e testado** no Maestro (Fato F7), trust-gated por hash: a projeção só é escrita se o hash do conteúdo projetado bater com o esperado pelo manifest da release.
3. Reporta no veredito o modo ativo: `hooks-via-plugin` ou `hooks-via-projeção (degraded)` — alimenta a matriz de paridade em `acceptance/parity/` (US-R1/US-R2).

### 3.3 Failure modes

| Failure mode | Detecção | Comportamento |
|---|---|---|
| **Migração interrompida** (kill/queda no meio da cópia) | Re-run do doctor: destino parcial sem commit (sem marker, sem `home.json` de origem ZIP) | Retoma do inventário: re-copia/re-verifica divergentes; origem nunca foi tocada antes do commit, então nenhum estado se perdeu. Nunca apresenta instalação parcial como migrada |
| **Interrupção pós-commit, pré-rename** | Marker existe + `data/` legado ainda com nome original | Doctor completa o rename para `data.migrated/` e reporta migração concluída |
| **Update parcial** (`claude plugin update` interrompido; plugin novo + binário antigo ou vice-versa) | Check 3 do conformance (versão travada) | FAIL "dessincronizado" + instrução de re-run do update; nunca estado corrompido silencioso (US-C2) |
| **Versão revogada instalada** | Check 5 do conformance + SessionStart notice (Spec F) | FAIL com aviso explícito e instrução de `claude plugin update` |
| **Estado ambíguo** (duas fontes de `data/`) | §3.1.1 | FAIL, sem migração automática, instrução manual |
| **Manifest inacessível** (offline) | Check 1 sem rede | Conformance degrada para verificação local (assinatura + hashes do manifest cacheado na release instalada); checks que exigem rede (revogação) reportam SKIPPED, nunca PASS falso |

### 3.4 Sequenciamento de release

- **v0.2.x (último ZIP)**: carrega (a) migração. É o único veículo que alcança a base instalada antes do plugin existir (migration-plan P1/G1).
- **v0.3.0 (primeiro plugin)**: carrega (b) conformance. A migração permanece no doctor como detecção/FAIL (não executa mais o move automático — instalação não-migrada na fase plugin é roteada para §3.5), coerente com a janela de compat da Spec A §3.5.

### 3.5 Descoberta de legado na fase plugin (anti-SPOF — colega que pulou o último ZIP)

O pior caminho silencioso: colega em 0.1.x que NUNCA roda o ZIP v0.2.x e instala o plugin direto. Sem tratamento, `first-run-scaffold` cria `~/.maestro/` fresco e a memória antiga fica encalhada numa pasta que o doctor não sabe localizar — perda de memória sem erro.

Tratamento obrigatório:

1. **Onboarding do plugin pergunta antes de escrever.** Ao detectar que `~/.maestro/` não existe ou tem origem `fresh` (sem marker de migração), `/maestro:onboarding` pergunta explicitamente: "Usava Maestro por ZIP nesta máquina?" — ANTES de popular qualquer estado.
   - "Sim" → migração assistida: usuário informa o path da instalação antiga; doctor executa o MESMO procedimento §3.1.2 (copy→verify→commit) a partir do path informado. O código de migração, portanto, NÃO sai do doctor na v0.3.0 — só o scan automático de `data/` co-locado sai; o procedimento permanece invocável com origem explícita.
   - "Não" → origem `fresh` gravada no `home.json` com a resposta registrada (auditável).
2. **Origem distinguível na telemetria.** `home.json` registra `origin: migrated | fresh-confirmed | fresh-unconfirmed`. O canary (Spec 054) reporta a origem — G4 consegue distinguir "migrado" de "fresh que deveria ter migrado" (`fresh-unconfirmed` em máquina de destinatário de ZIP = flag de follow-up do maintainer).
3. **Best-effort scan.** Antes de perguntar, o doctor tenta localizar instalações ZIP em paths convencionais documentados dos releases anteriores (lista fixa, sem varredura de disco); se encontrar, oferece o path como default da pergunta.

## 4. Acceptance Criteria

- [ ] **AC-E1** Teste 4 do PRD §5: fixtures sintéticas 0.1.11 e 0.2.0 com `data/` co-locado → doctor migra → equivalência byte a byte + markers gravados + relatório emitido.
- [ ] **AC-E2** Idempotência: segunda execução sobre instalação migrada = no-op com saída PASS explicitando "já migrado".
- [ ] **AC-E3** Rollback testado: procedimento §3.1.4 restaura a instalação ZIP funcional; doctor pós-rollback volta a detectar `data/` co-locado (estado pré-migração limpo).
- [ ] **AC-E4** Migração interrompida (kill simulado em cada passo 1–5): re-run converge para migração completa sem perda nem duplicação; origem intacta até o commit.
- [ ] **AC-E5** Estado ambíguo (home populado + legado sem marker): doctor NÃO migra, FAIL com ambas as localizações.
- [ ] **AC-E6** Conformance pós-install e pós-update em máquina limpa (macOS arm64/x64, Windows x64): PASS com versão + assinatura verificada no output (US-C1/US-C2).
- [ ] **AC-E7** Manifest adulterado, arquivo de plugin alterado, ou binário com sha256 divergente → FAIL nomeando o item; nunca PASS com assinatura inválida.
- [ ] **AC-E8** Update parcial simulado (plugin vN+1 + binário vN) → FAIL "plugin↔runtime dessincronizado" + instrução de re-run.
- [ ] **AC-E9** Versão revogada no marketplace → doctor FAIL sinalizando revogação (par com SessionStart notice, Spec F).
- [ ] **AC-E10** Codex sem hooks-via-plugin: doctor instala projeção `.codex/hooks.json` trust-gated por hash; hash divergente → projeção NÃO é escrita, FAIL explícito. Modo degraded registrado em `acceptance/parity/`.
- [ ] **AC-E11** Gate G1: release ZIP v0.2.x publicada com a migração; ≥1 instalação real migrada sem perda, verificada por doctor PASS; entrada em P2 condicionada a cobertura de migração MEDIDA contra a lista de destinatários de ZIP (migration-plan §1).
- [ ] **AC-E12** Fixture "pulou o último ZIP": instalação 0.1.x sintética + install direto do plugin → onboarding pergunta antes de escrever estado; resposta "sim" + path → migração assistida equivalente ao AC-E1; resposta "não" → `origin: fresh-confirmed` gravado e reportado via canary. Em NENHUM caminho a memória antiga é sobrescrita ou órfã silenciosa.

## 5. Out of Scope

- Definição do data home, resolver e markers — Spec A (esta spec os consome).
- Pipeline que gera e assina o manifest, e o canal de revogação em si — Spec F (esta spec os verifica/consome).
- Bootstrap do binário `maestro-mcp` — Spec C (esta spec verifica o resultado).
- Merge automático de dois estados divergentes — nunca; sempre FAIL + procedimento manual.
- Migração de instalações anteriores a 0.1.x — fora do suporte declarado (0.1.x/0.2.0).

## 6. Open Questions

| # | Questão | Decide | Quando |
|---|---|---|---|
| QE-1 | Localização e interface exatas da máquina de verificação do `eval-release.sh` atual (o que precisa mudar para "reapontar" para artefatos plugin+binário) — verificação no repo sobre a v0.2.0 merged | implementação | Início de T2, pós-G0 |
| QE-2 | Evidência canônica de "hook executou" no Codex (receipt file? qual?) — depende do resultado do T3 (teste de 30min, pré-freeze Spec B) | Daniel + resultado T3 | Gate G0 |
| QE-3 | O conformance roda automaticamente pós-update (trigger) ou permanece comando explícito recomendado pelo update notice? Recomendação: explícito, consistente com a doutrina de consent | Daniel | Freeze Spec E |
| QE-4 | `data.migrated/` legado: TTL de preservação antes de instruir o usuário a remover (hoje o ritual ZIP já pede "apagar a pasta antiga depois de uma semana") | Daniel | Freeze Spec E |
