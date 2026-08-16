# Maestro OS — Pílulas #041 a #090

> Batch de 50 pílulas de conhecimento sobre o Maestro OS. Foco: agentes internos (Yoda, Darwin, Maestro), memória, skills, arquitetura, método de trabalho, rotinas e hooks. Cada pílula traz o JSON Block Kit pronto pra colar em canal e um preview em markdown pra revisão.

**Regras aplicadas:**
- Bookends `💊` só no título do header
- Corpo 100% em itálico, ≤900 caracteres, 1 emoji temático por parágrafo/bullet
- `:maestro:` só no footer
- Só cita skills, agentes, hooks e arquivos que existem no repo

---

## Bloco A — Agentes internos (#041–#050)

### Pílula #041 — Yoda revisa antes do owner ver

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Yoda revisa antes do owner ver 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧙 _Yoda é o revisor sênior interno do Maestro — pressure-test de proposta antes de chegar em owner ou stakeholder._\n\n⚡ _Não é rubber stamp. Só entra quando o custo de errar importa:_\n\n• _decisão cara ou difícil de reverter_ 🎯\n• _output pra fora do loop privado_ 📤\n• _mudança de regra persistente_ 🔒\n\n📖 _Skill em `bundles/base/skills/yoda/SKILL.md`. Persona: Mestre Yoda de Star Wars._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #041 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Yoda revisa antes do owner ver** 💊
> 🧙 *Yoda é o revisor sênior interno do Maestro.* ⚡ *Entra quando errar custa caro.* 📖 *Ver `bundles/base/skills/yoda/SKILL.md`.*

---

### Pílula #042 — Yoda tem quatro veredictos e só

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Yoda tem quatro veredictos 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧙 _Yoda não conversa — devolve exatamente um veredicto do conjunto canônico:_\n\n• _`approve` — proposta defensável, pode seguir_ ✅\n• _`refine` — gap corrigível, volta com a correção específica_ 🔧\n• _`clarify` — intenção mal-especificada, pergunta bounded pro owner_ ❓\n• _`hold_exceptional` — fora de escopo ou risco que exige decisão explícita do owner_ 🛑\n\n📖 _Contrato em `bundles/base/agents/yoda/AGENT.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #042 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Yoda tem quatro veredictos** 💊
> 🧙 *`approve` · `refine` · `clarify` · `hold_exceptional`.* 📖 *Ver `bundles/base/agents/yoda/AGENT.md`.*

---

### Pílula #043 — Yoda nunca fala com o owner

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Yoda nunca fala com o owner 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧙 _Yoda é conselheiro interno — o veredicto volta pro agente que pediu, nunca aparece direto pro owner._\n\n🎭 _Isso preserva a autoria do agente produtor: quem entrega ao owner responde pela entrega, com Yoda embutido no raciocínio, não estampado no output._\n\n🔒 _Yoda também é read-only — não escreve arquivo, não edita canon, não promove self._\n\n📖 _Boundaries em `bundles/base/agents/yoda/AGENT.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #043 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Yoda nunca fala com o owner** 💊
> 🧙 *Veredicto volta ao agente que pediu.* 🎭 *Autoria permanece com o produtor.* 🔒 *Read-only.*

---

### Pílula #044 — Darwin cuida do próprio OS

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Darwin cuida do próprio OS 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧬 _Darwin é o cirurgião do Maestro — observa drift, propõe mudança estrutural e passa toda proposta pelo Yoda antes de virar canon._\n\n🔍 _Cobre governance, memória, hooks, skills e rotinas. É quem enxerga o sistema de fora enquanto o resto executa._\n\n🛠 _Também é read-only por default: escreve só quando a proposta é aprovada. Detecta context rot, ponteiros quebrados, camadas stale._\n\n📖 _Agente em `bundles/base/agents/darwin/AGENT.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #044 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Darwin cuida do próprio OS** 💊
> 🧬 *Cirurgião de governança.* 🔍 *Detecta drift.* 🛠 *Toda proposta passa por Yoda.*

---

### Pílula #045 — Darwin ajuda Yoda sem invadir

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Darwin ajuda Yoda sem invadir 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧬 _Darwin observa o padrão de refinos que Yoda pede ao longo do tempo._\n\n📊 _Se Yoda começa a nitpickar demais, Darwin flagra: 'refino virou naysayer'. Se Yoda deixa passar risco material, Darwin flagra: 'threshold caiu'._\n\n🎯 _É meta-review: Yoda avalia proposta, Darwin avalia como Yoda tem avaliado. Nenhum dos dois fala com o owner direto._\n\n📖 _Ver `docs/personas.md` para o loop completo._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #045 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Darwin ajuda Yoda sem invadir** 💊
> 🧬 *Observa padrão de refinos.* 📊 *Flagra drift do próprio Yoda.* 🎯 *Meta-review sem canal com owner.*

---

### Pílula #046 — Maestro é o hub, não o executor

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Maestro é o hub, não o executor 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🎼 _Maestro é o único agente que fala com o owner. Recebe pedido, decide rota, delega — não executa trabalho técnico direto._\n\n🧭 _É superfície conversacional + roteador. Escolhe skill, aciona case agent, chama Yoda quando o risco pede._\n\n🔁 _Isso mantém contexto do owner concentrado num lugar só e libera especialistas pra fazer o trabalho fundo._\n\n📖 _Ver `bundles/base/agents/maestro/AGENT.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #046 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Maestro é o hub, não o executor** 💊
> 🎼 *Única superfície com o owner.* 🧭 *Roteia pra skill ou agente.* 🔁 *Especialistas fazem o fundo.*

---

### Pílula #047 — Três personas, três autoridades

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Três personas, três autoridades 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🎼 _Maestro fala com o owner e delega._\n\n🧙 _Yoda pressure-testa proposta antes de sair._\n\n🧬 _Darwin observa o sistema e propõe evolução, passando por Yoda._\n\n🔒 _Só Maestro tem canal direto com o owner. Yoda e Darwin agem por dentro — o output do sistema é sempre falado pelo Maestro, com o refino de Yoda embutido._\n\n📖 _Contrato completo em `docs/personas.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #047 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Três personas, três autoridades** 💊
> 🎼 *Maestro fala.* 🧙 *Yoda refina.* 🧬 *Darwin evolui.* 🔒 *Só Maestro tem canal com o owner.*

---

### Pílula #048 — Case agent guarda contexto do projeto

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Case agent guarda o projeto 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📁 _Cada case tem seu próprio agente — memória, decisões, stakeholders e histórico ficam contidos ali._\n\n🧭 _O Maestro chama o case agent quando o pedido é sobre o projeto. Case agent responde com contexto denso; Maestro traduz de volta pro owner._\n\n🔒 _Isso impede que contexto de cliente A vaze pra cliente B. Isolamento é por design, não por acidente._\n\n📖 _Ver `bundles/base/skills/case-agent-setup/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #048 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Case agent guarda o projeto** 💊
> 📁 *Cada case = agente dedicado.* 🧭 *Maestro chama, case responde.* 🔒 *Isolamento por design.*

---

### Pílula #049 — Client account é a conta, não o case

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Conta e case são coisas diferentes 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🏢 _Client account = relação com o cliente ao longo dos anos: stakeholders, histórico comercial, contratos._\n\n📁 _Case = projeto específico dentro dessa conta: escopo, decisões, entregas._\n\n🎯 _Separar os dois evita bagunça: uma conta pode ter vários cases em paralelo, um case pertence a uma conta só._\n\n📖 _Setup em `bundles/base/skills/account-case-setup/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #049 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Conta e case são coisas diferentes** 💊
> 🏢 *Account = relação com cliente.* 📁 *Case = projeto dentro dela.* 🎯 *N cases por conta.*

---

### Pílula #050 — Agentes têm nome, mandato e limite

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Agente tem nome, mandato e limite 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🎭 _Todo agente do Maestro tem persona customizável — nome e emoji controlados pelo owner._\n\n📜 _Mas o contrato é do sistema: mandato, boundaries, ferramentas permitidas ficam no `AGENT.md` e não mudam com o rename._\n\n🔒 _Trocar Walter por Yoda mudou a persona apresentada; o contrato de pressure-test seguiu igual. Persona é fachada, mandato é infraestrutura._\n\n📖 _Ver `bundles/base/skills/agent-identity-setup/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #050 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Agente tem nome, mandato e limite** 💊
> 🎭 *Persona é fachada.* 📜 *Contrato é sistema.* 🔒 *Renomear não muda mandato.*

---

## Bloco B — Memória (#051–#060)

### Pílula #051 — Memória tem quatro andares

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Memória tem quatro andares 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧠 _A memória do Maestro é uma pirâmide de 4 andares, cada um com propósito distinto:_\n\n• _recent — o que aconteceu hoje_ 📅\n• _weekly — o que sobrou da semana_ 📆\n• _medium-term — janela temática rolante_ 🌊\n• _lifetime — canon durável_ 🏛\n\n📖 _Contrato em `bundles/base/memory/policy.json`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #051 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Memória tem quatro andares** 💊
> 🧠 *recent · weekly · medium-term · lifetime.*

---

### Pílula #052 — Memória rola sozinha

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 A memória rola sozinha 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🌊 _Cada andar tem sua janela: o diário roda todo dia, o semanal fecha a semana, o médio prazo guarda o que sobrevive a ela._\n\n♻️ _O conteúdo que importa sobe de andar; o resto expira. Não é o owner que decide item a item — a rotina promove ou descarta._\n\n📥 _No início da sessão entram sozinhos o lifetime, o semanal e o diário mais recente._\n\n📖 _Rollups em `data/memory/`, um diretório por andar._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #052 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **A memória rola sozinha** 💊
> 🌊 *Cada tier tem janela.* ♻️ *Importante sobe, resto expira.* 📥 *SessionStart injeta.*

---

### Pílula #053 — Dormir fixa memória

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Dormir é o que fixa memória 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🌙 _`dream-memory` tem dois ciclos: o diário comprime o dia na memória recente, e o semanal é que promove o que virou aprendizado durável._\n\n🧠 _Sem esse passo, o diário vira lixo acumulado e a sessão seguinte começa com contexto ruim._\n\n💡 _O digest é denso e curto, não narrativo. O log bruto fica arquivado; o que sobe de andar é a compressão._\n\n📖 _Skill em `bundles/base/skills/dream-memory/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #053 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Dormir é o que fixa memória** 💊
> 🌙 *`dream-memory` comprime o dia.* 🧠 *Sem isso, daily vira lixo.* 💡 *Digest denso, log arquivado.*

---

### Pílula #054 — Lifetime é o que fica pra sempre

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Lifetime é o que fica pra sempre 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🏛 _O andar `lifetime` é canon: decisões estruturais, princípios, personas, arquitetura._\n\n🔒 _Não expira, não rola. Só entra por promoção deliberada, e só no ciclo semanal — via `case-canon-ingest`, `craft-update` ou proposta Darwin aprovada por Yoda._\n\n🎯 _Isso protege o núcleo do sistema: ninguém enfia coisa aleatória no canon no meio de um dia corrido._\n\n📖 _Vive em `data/memory/lifetime/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #054 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Lifetime é o que fica pra sempre** 💊
> 🏛 *Canon durável.* 🔒 *Só entra por promoção deliberada.* 🎯 *Núcleo protegido.*

---

### Pílula #055 — Cada agente guarda seu próprio estado

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Cada agente tem sua memória 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗜 _Estado por agente não é um andar da memória — é uma área própria em `data/agents/`, uma pasta por agente._\n\n📉 _Cada um guarda ali sua memória de trabalho, suas decisões e seu contexto. Sem isso, todo agente carregaria o histórico inteiro toda vez._\n\n⚡ _Contexto denso, custo baixo, comportamento consistente sessão após sessão._\n\n📖 _Ver `data/agents/` na sua pasta do Maestro._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #055 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Cada agente tem sua memória** 💊
> 🗜 *Uma pasta por agente.* 📉 *Não recarrega o histórico todo.* ⚡ *Comportamento consistente.*

---

### Pílula #056 — Decision-log é memória com timestamp

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Decision-log é memória datada 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗓 _Toda decisão estrutural vira linha no decision-log: data, tema, status, tag, resumo._\n\n🔎 _Isso permite recuperar 'por que decidimos X em Y' meses depois sem depender de transcript perdido._\n\n📊 _Um script gera routing summary compacto no SessionStart — chega só o essencial, o log inteiro fica pra consulta on-demand._\n\n📖 _Skill: `bundles/base/skills/case-decision-log-entry/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #056 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Decision-log é memória datada** 💊
> 🗓 *Toda decisão vira linha.* 🔎 *Recupera "por quê" meses depois.* 📊 *Rollup no SessionStart.*

---

### Pílula #057 — Contexto apodrece se ninguém olhar

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Contexto apodrece sem olhar 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🦠 _Contexto poluído degrada resposta: camada velha que ninguém limpou, ponteiro duplicado, ordem de injeção errada._\n\n👀 _O Darwin tem sensor pra isso: mede o crescimento do envelope, as camadas paradas e os ponteiros repetidos._\n\n🧹 _Sem sensor, o sistema piora em silêncio — respostas piores, custo maior, ninguém sabe por quê._\n\n📖 _Agente em `bundles/base/agents/darwin/AGENT.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #057 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Contexto apodrece sem olhar** 💊
> 🦠 *Context rot mensurável.* 👀 *Darwin sensoreia.* 🧹 *Silencioso sem observação.*

---

### Pílula #058 — Weekly rollup enxerga o que sobrou

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Weekly mostra o que sobrou 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📆 _No fim do ciclo, o `dream-memory` consolida os diários da semana num rollup denso._\n\n🧭 _O que aparece: entregas, itens que viraram carry, blockers._\n\n♻️ _O rollup fica em `data/memory/weekly/` e alimenta o médio prazo. O diário original fica arquivado — recuperação sob demanda, injeção só do compacto._\n\n📖 _Skill em `bundles/base/skills/dream-memory/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #058 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Weekly mostra o que sobrou** 💊
> 📆 *Consolida 7 dailies.* 🧭 *Runtime + entregas + carries.* ♻️ *Alimenta medium-term.*

---

### Pílula #059 — Medium-term é a janela temática

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Medium-term é janela temática 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🌊 _O médio prazo agrega várias semanas num digest temático: projetos ativos, questões recorrentes, carries._\n\n🎯 _É o meio-termo entre o barulho do diário e o silêncio do canon: mostra o que está evoluindo agora._\n\n🔁 _Consultado sob demanda, dá contexto de trajetória sem rebobinar conversa._\n\n📖 _Vive em `data/memory/medium-term/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #059 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Medium-term é janela temática** 💊
> 🌊 *Várias semanas em digest.* 🎯 *Trajetória sem rebobinar.* 🔁 *Consultado sob demanda.*

---

### Pílula #060 — A memória é sua, e dá pra abrir

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 A memória é sua, e dá pra abrir 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗂 _Tudo que o Maestro lembra fica em `data/memory/`, uma pasta por andar, em arquivos de texto comum._\n\n📖 _Você pode abrir, ler, editar e apagar qualquer um deles pelo seu próprio computador. Não tem banco de dados nem formato fechado._\n\n🎯 _Nada sai da sua máquina, e a pasta `data/` nunca é sobrescrita quando chega uma versão nova._\n\n📖 _Abra `data/memory/` na sua pasta do Maestro._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #060 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **A memória é sua, e dá pra abrir** 💊
> 🗂 *Texto comum em `data/memory/`.* 📖 *Você abre, lê e edita.* 🎯 *Nunca sai da sua máquina.*

---

## Bloco C — Skills (#061–#070)

### Pílula #061 — Skills atendem no gatilho certo

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Skills atendem no gatilho certo 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🎯 _Toda skill do Maestro tem triggers — frases que fazem ela pular na frente automaticamente._\n\n⚡ _Não precisa lembrar do nome. Basta descrever o que quer:_\n\n• _`kickoff de caso novo` → dispara `bcg-case-kickoff`_ 🚀\n• _`revisa esse deck` → dispara `deck-review`_ 📊\n• _`fechando o dia` → dispara `eod`_ 🌙\n\n📖 _Ver todos em `bundles/base/skills/INDEX.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #061 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Skills atendem no gatilho certo** 💊
> 🎯 *Triggers descrevem intenção.* ⚡ *Não precisa lembrar nome.*

---

### Pílula #062 — INDEX é o mapa das skills

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 INDEX é o mapa das skills 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗺 _`INDEX.md` lista toda skill do Maestro: nome, quando usar e pointer pro SKILL.md completo._\n\n📖 _O SKILL.md só é aberto quando a task pede — o resto do tempo, o INDEX resolve o roteamento._\n\n⚡ _Isso é progressive disclosure: contexto denso na entrada, detalhe on-demand._\n\n📖 _Ver `bundles/base/skills/INDEX.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #062 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **INDEX é o mapa das skills** 💊
> 🗺 *Lista + pointer.* 📖 *SKILL.md carrega on-demand.* ⚡ *Progressive disclosure.*

---

### Pílula #063 — Skill descreve, não impõe

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Skill descreve, não impõe 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📜 _Skill é procedimento explícito: quando usar, o que produz, anti-padrões que evitar._\n\n🧠 _Diferente de ability (que é capacidade do modelo), skill é procedure escrita — não some se o modelo esquecer._\n\n🔒 _Formato locked garante consistência: mesma estrutura, mesmo contrato, saída previsível._\n\n📖 _Todas em `bundles/base/skills/`, uma pasta por skill._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #063 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Skill descreve, não impõe** 💊
> 📜 *Procedure explícita.* 🧠 *Persiste onde ability some.* 🔒 *Formato locked.*

---

### Pílula #064 — Skill atômica compõe

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Skills atômicas compõem 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧩 _Skill pequena com escopo claro compõe melhor do que skill grande que tenta tudo._\n\n🎯 _Ex: montar um deck usa `bcg-deck`, revisar usa `deck-review`, treinar a apresentação usa `deck-drill` — três skills, três momentos._\n\n♻️ _Cada uma é reutilizável sozinha: dá pra revisar um deck que não foi feito aqui. Composição > monolito._\n\n📖 _Ver `bundles/base/skills/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #064 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Skills atômicas compõem** 💊
> 🧩 *Escopo pequeno e claro.* 🎯 *Montar, revisar e treinar são três.* ♻️ *Reutilizável.*

---

### Pílula #065 — Skill de gate protege entrega

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Gate protege a entrega 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🔒 _`client-delivery-gate` é o check final antes de qualquer entrega ir pro cliente. Três lentes:_\n\n• _Kahneman — solidez analítica: viés de confirmação, ancoragem, falsa precisão_ 🧠\n• _Taleb — risco de cauda: o que precisa ser verdade, o que quebra sob estresse_ 🎲\n• _Ariely — integridade de enquadramento: o número que ancora, o que ficou de fora_ 🖼\n\n🚦 _Devolve LIBERADO, LIBERADO COM RESSALVAS ou SEGURAR. Não reescreve nada — aponta e sugere._\n\n📖 _Diga \"pode entregar?\" antes de mandar deck, memo ou análise._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #065 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Gate protege a entrega** 💊
> 🔒 *Kahneman, Taleb e Ariely.* 🚦 *LIBERADO, COM RESSALVAS ou SEGURAR.* 🎯 *Aponta, não reescreve.*

---

### Pílula #066 — Investigate antes de consertar

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Investigate antes de consertar 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🔍 _Output errado? Não corrige no chute — dispara `investigate`._\n\n🧭 _A skill puxa root cause: input, contexto injetado, skill acionada, agente chamado, resposta gerada._\n\n🎯 _Diagnosticar antes de patch evita que o mesmo erro volte em outra forma amanhã._\n\n📖 _Skill em `bundles/base/skills/investigate/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #066 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Investigate antes de consertar** 💊
> 🔍 *Root cause primeiro.* 🧭 *Rastreia input → resposta.* 🎯 *Patch sem diagnóstico volta.*

---

### Pílula #067 — Ingest content puxa doc pra memória

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Ingest content traz o doc pra dentro 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📥 _PDF de texto, markdown, HTML, planilha em CSV — `ingest-content` extrai e registra na memória certa._\n\n🔒 _Tudo acontece na sua máquina, nada sobe pra nuvem. Word, Excel e PowerPoint pedem uma instalação extra e o Maestro avisa antes; PDF escaneado ainda não dá — nesse caso, copie o texto e cole no chat._\n\n🧠 _Depois de ingerir, o material vira canon consultável — não precisa colar de novo._\n\n📖 _Skill em `bundles/base/skills/ingest-content/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #067 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Ingest content traz o doc pra dentro** 💊
> 📥 *Extrai local.* 🔒 *Compliance-safe.* 🧠 *Vira canon consultável.*

---

### Pílula #068 — Find prior work recupera passado

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Find prior work recupera passado 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🔎 _'Cadê aquele deck do case X do ano passado?' — `find-prior-work` recupera deliverables anteriores do workspace._\n\n🗂 _Vasculha por título, contexto, tag. Devolve pointer + resumo do que era, não abre nem move._\n\n🎯 _Bounded por design: recupera especificamente o que foi pedido, não faz garimpo genérico._\n\n📖 _Skill em `bundles/base/skills/find-prior-work/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #068 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Find prior work recupera passado** 💊
> 🔎 *Deliverable antigo, pointer + resumo.* 🎯 *Bounded, sem garimpo.*

---

### Pílula #069 — Wayfinder quebra problema fuzzy

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Wayfinder quebra problema fuzzy 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🧭 _Problema aberto sem primeiro passo claro? `wayfinder` estrutura em issue tree e propõe first move._\n\n🎯 _Não resolve o problema — dá forma pra ele. Downstream você aciona a skill específica (deck, análise, kickoff)._\n\n⚡ _Uso típico: 'não sei por onde começar com X'. Sai daí com árvore, hipóteses e ação inicial._\n\n📖 _Skill em `bundles/base/skills/wayfinder/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #069 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Wayfinder quebra problema fuzzy** 💊
> 🧭 *Issue tree + first move.* 🎯 *Dá forma, não resolve.*

---

### Pílula #070 — Interaction profile ajusta o tom

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Interaction profile ajusta o tom 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🎚 _`interaction-profile` calibra profundidade de explicação por agente e por owner._\n\n📏 _Não muda contrato de skill nem verdict de Yoda — muda quanto de raciocínio explicitar._\n\n🎯 _Owner que prefere direto recebe direto; agente que precisa de contexto denso recebe denso. Um lugar só pra ajustar._\n\n📖 _Skill em `bundles/base/skills/interaction-profile/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #070 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Interaction profile ajusta o tom** 💊
> 🎚 *Calibra profundidade.* 📏 *Não muda contrato.* 🎯 *Um lugar só.*

---

## Bloco D — Arquitetura (#071–#080)

### Pílula #071 — Bundle é a unidade portável

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Bundle é a unidade portável 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📦 _Bundle empacota agentes, skills, runtime e canon num diretório que outro OS consegue instalar._\n\n🎯 _`bundles/base/` é o núcleo compartilhado. `bundles/tech-core/` traz as skills de engenharia, carregadas sob demanda._\n\n♻️ _Separação por bundle deixa release incremental: subo tech-core sem mexer no base._\n\n📖 _Ver `bundles/base/manifest.json` e `distribution.json`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #071 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Bundle é a unidade portável** 💊
> 📦 *Agentes + skills + runtime + canon.* 🎯 *base / tech-core / dev.* ♻️ *Release incremental.*

---

### Pílula #072 — Catalog é o índice do bundle

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Catalog é o índice do bundle 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗂 _`catalog.json` lista tudo que o bundle expõe: para cada skill, o id, o nome, o gatilho e o caminho do arquivo._\n\n🔒 _É gerado, não escrito à mão. Editar direto quebra o build — quem manda é o arquivo de origem._\n\n⚡ _É a fonte de verdade que o instalador lê. Sem ele, o bundle é só uma pasta de markdowns._\n\n📖 _Ver `bundles/base/skills/catalog.json` e `agents/catalog.json`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #072 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Catalog é o índice do bundle** 💊
> 🗂 *Lista agentes + skills.* 🔒 *Ordem estrita.* ⚡ *Fonte de verdade do instalador.*

---

### Pílula #073 — Adapter fala com o cliente

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Adapter fala com o cliente 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🔌 _O Maestro é escrito contra contratos, não contra um runtime específico. Hoje ele roda no Claude Code._\n\n🎯 _As skills e os agentes descrevem o que deve acontecer; quem traduz isso para cada ambiente fica de fora do núcleo._\n\n♻️ _É o que torna a portabilidade possível mais adiante — ainda não é uma capacidade entregue._\n\n📖 _Contratos em `bundles/base/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #073 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Adapter fala com o cliente** 💊
> 🔌 *Escrito contra contratos.* 🎯 *Núcleo não conhece o runtime.* ♻️ *Portabilidade como projeto.*

---

### Pílula #074 — Conformance garante contrato

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Conformance garante contrato 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "✅ _`adapters/conformance/` guarda JSONs que descrevem o contrato esperado por adapter — ex: `yoda-review.json`, `darwin-cadence.json`._\n\n🧪 _Teste roda o adapter contra o JSON e falha se o comportamento não bate._\n\n🔒 _É o freio que impede um runtime novo de dizer que suporta Yoda sem realmente implementar o contrato._\n\n📖 _Ver `adapters/conformance/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #074 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Conformance garante contrato** 💊
> ✅ *JSON descreve esperado.* 🧪 *Teste falha se drift.* 🔒 *Freio de compliance.*

---

### Pílula #075 — Runtime carrega no SessionStart

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Runtime carrega no SessionStart 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🚀 _`bundles/base/runtime/` define o que o Maestro carrega em toda sessão nova: método operacional, orientação, maintenance._\n\n📥 _SessionStart injeta esse pacote automaticamente. O agente abre a sessão já sabendo quem é e como opera._\n\n🎯 _Sem runtime injetado, cada sessão começaria do zero. Com runtime, comportamento é consistente._\n\n📖 _Ver `bundles/base/runtime/maintenance.json` e `orientation.md.tmpl`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #075 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Runtime carrega no SessionStart** 💊
> 🚀 *Método + orientação + maintenance.* 📥 *Injetado auto.* 🎯 *Consistência.*

---

### Pílula #076 — Atlas guarda conceito derivado

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Atlas é conceito derivado 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗺 _`bundles/base/atlas/managed/concepts/` guarda conceitos gerados a partir de canon — não escritos a mão._\n\n🔄 _Um script compila princípios do decision-log, personas e runtime em cards de conceito consultáveis por agente._\n\n🎯 _Vantagem: quando canon muda, atlas regenera. Sem duplicação manual, sem drift._\n\n📖 _Ver `bundles/base/atlas/managed/concepts/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #076 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Atlas é conceito derivado** 💊
> 🗺 *Compilado do canon.* 🔄 *Regenera quando canon muda.* 🎯 *Sem drift.*

---

### Pílula #077 — Governance fence protege núcleo

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Governance fence protege o núcleo 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🚧 _O núcleo do Maestro — agentes, skills, contratos — vem pronto no pacote e não é alterado pelo uso do dia a dia._\n\n🔒 _Mudança nele passa por revisão humana antes de virar uma versão nova: pressure-test do Yoda, decisão registrada, PR revisado._\n\n🎯 _Isso impede que uma conversa qualquer mude os princípios do sistema num dia agitado._\n\n📖 _Sua parte fica em `data/`, e ela nunca é sobrescrita no update._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #077 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Governance fence protege o núcleo** 💊
> 🚧 *Regras/agents/skills atrás de cerca.* 🔒 *Yoda + decision-log + PR.* 🎯 *Sem drift.*

---

### Pílula #078 — Manifest declara versão

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Manifest declara versão 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📋 _`bundles/base/manifest.json` declara nome, versão e conteúdo do bundle._\n\n🎯 _Instalador lê o manifest, resolve dependências e decide se roda migração antes de aplicar._\n\n🔒 _Versão bump é intencional: patch pra fix, minor pra skill nova, major pra breaking change de contrato._\n\n📖 _Ver `bundles/base/manifest.json` e `distribution.json`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #078 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Manifest declara versão** 💊
> 📋 *Nome + versão + conteúdo.* 🎯 *Instalador resolve deps.* 🔒 *SemVer intencional.*

---

### Pílula #079 — Distribution empacota release

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Distribution empacota release 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📦 _`distribution.json` diz o que entra no ZIP de release: manifesto, agentes, skills, contratos e schemas._\n\n🚀 _`installers/zip/` gera o pacote instalável. Colega roda o installer e recebe o Maestro configurado._\n\n🎯 _Sem distribution, o repo é código-fonte. Com ele, virou produto instalável._\n\n📖 _Ver `bundles/base/distribution.json` e `installers/zip/`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #079 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Distribution empacota release** 💊
> 📦 *ZIP com bundles + adapters + docs.* 🚀 *Installer roda.* 🎯 *Produto instalável.*

---

### Pílula #080 — Doctor mede saúde do install

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Doctor mede saúde do install 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🩺 _`maestro-doctor` confere a instalação: arquivos do núcleo, pastas da sua área, hooks ligados, versão e problemas conhecidos._\n\n🚨 _Se algo quebrou, doctor lista o que checar antes de dar report vago tipo 'não funciona'._\n\n🎯 _É a diferença entre pedir ajuda com sintoma vs pedir ajuda com diagnóstico._\n\n📖 _Skill em `bundles/base/skills/maestro-doctor/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #080 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Doctor mede saúde do install** 💊
> 🩺 *Bundles + adapters + memória.* 🚨 *Sintoma vs diagnóstico.*

---

## Bloco E — Método, rotinas e hooks (#081–#090)

### Pílula #081 — Start day abre com briefing

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Start day abre com briefing 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "☀️ _`start-day` (gatilho: 'bom dia') abre o dia útil com briefing escopado às horas que restam._\n\n🎯 _Injeta: agenda do dia, carries do daily anterior, prioridades vivas, blockers explícitos._\n\n⚡ _Sem start-day, primeira hora vira arqueologia de contexto. Com ele, a hora zero já é execução._\n\n📖 _Skill em `bundles/base/skills/start-day/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #081 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Start day abre com briefing** 💊
> ☀️ *Gatilho: "bom dia".* 🎯 *Agenda + carries + prioridades.* ⚡ *Hora zero = execução.*

---

### Pílula #082 — EOD fecha o dia

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 EOD fecha o dia 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🌙 _`eod` (gatilho: 'fechando o dia', 'eod') consolida o dia útil: entregas, decisões, carries pra amanhã._\n\n📝 _Não é ritual vazio — é o que faz o `start-day` de amanhã ter conteúdo bom._\n\n🎯 _Skipar o EOD hoje = perder qualidade do briefing amanhã. Custo do atalho aparece defasado._\n\n📖 _Skill em `bundles/base/skills/eod/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #082 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **EOD fecha o dia** 💊
> 🌙 *Entregas + decisões + carries.* 📝 *Alimenta start-day de amanhã.* 🎯 *Skip = perda defasada.*

---

### Pílula #083 — Dream memory consolida a noite

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Dream memory consolida a noite 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🌙 _`dream-memory` roda depois do EOD: comprime o dia num digest e, no ciclo semanal, promove o que virou aprendizado._\n\n🧠 _É o passo que faz memória virar canon consultável em vez de log inflado._\n\n⚡ _Dispara sozinho na abertura da sessão seguinte, a partir do marcador deixado no fechamento da anterior._\n\n📖 _Skill em `bundles/base/skills/dream-memory/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #083 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Dream memory consolida a noite** 💊
> 🌙 *Digest diário, promoção semanal.* 🧠 *Log vira canon.* ⚡ *Dispara sozinho.*

---

### Pílula #084 — Meeting close vira packet

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Meeting close vira packet 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🗣 _`meeting-close` transforma notas de reunião em packet estruturado: decisões, tasks, follow-ups, dono._\n\n📤 _Output é reviewable: dá pra mandar pro cliente, arquivar no case, disparar `meeting-to-work-items`._\n\n🎯 _Sem meeting-close, reunião vira nota solta que ninguém revisita. Com ele, virou artefato de trabalho._\n\n📖 _Skill em `bundles/base/skills/meeting-close/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #084 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Meeting close vira packet** 💊
> 🗣 *Decisões + tasks + donos.* 📤 *Reviewable.* 🎯 *Artefato, não nota solta.*

---

### Pílula #085 — Retro fecha a semana

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Retro fecha a semana 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "📆 _`retro` fecha a semana contra os objetivos de desenvolvimento do owner._\n\n🎯 _Não é retrospectiva de time — é retro pessoal: o que avançou nos objetivos, o que atrasou, o que ajustar na semana seguinte._\n\n♻️ _Alimenta o weekly rollup e o medium-term. Semana sem retro é semana que não vira aprendizado._\n\n📖 _Skill em `bundles/base/skills/retro/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #085 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Retro fecha a semana** 💊
> 📆 *Objetivos vs execução.* 🎯 *Retro pessoal.* ♻️ *Semana vira aprendizado.*

---

### Pílula #086 — Execution continuity retoma trabalho

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Continuity retoma o trabalho 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🔄 _`execution-continuity` cria checkpoint e retoma trabalho entre sessões — tarefas, entregas, versão em progresso._\n\n🎯 _Fecha sessão hoje sem perder onde parou; abre amanhã sabendo o próximo passo._\n\n⚡ _É o oposto de 'começar do zero toda vez'. Contexto persiste por design._\n\n📖 _Skill em `bundles/base/skills/execution-continuity/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #086 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Continuity retoma o trabalho** 💊
> 🔄 *Checkpoint + retomada.* 🎯 *Não começa do zero.* ⚡ *Contexto persiste.*

---

### Pílula #087 — Hook SessionStart injeta contexto

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Hook injeta contexto na entrada 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🪝 _O hook de abertura roda toda vez que a sessão começa — injeta o teu SELF, a memória de longo prazo, o resumo semanal e o último diário._\n\n📥 _Não é mágica: é script que compõe o pacote inicial. Determinístico, testável, versionado._\n\n🎯 _Sem hook, cada sessão pediria ao owner pra recolar contexto. Com hook, contexto vem pronto._\n\n📖 _Setup canônico via `maestro-operator`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #087 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Hook injeta contexto na entrada** 💊
> 🪝 *SessionStart roda script.* 📥 *Pacote pronto.* 🎯 *Owner não recola.*

---

### Pílula #088 — Hook PreToolUse bloqueia risco

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 PreToolUse bloqueia risco 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🚦 _Existe uma checagem que roda antes de cada escrita em arquivo — dá pra auditar, alertar ou barrar._\n\n🔒 _O uso que já vem ativo é a separação entre casos: escrita em um caso que não é o ativo é barrada, com a instrução de trocar de caso primeiro._\n\n🎯 _É freio técnico, não convenção. Não depende de ninguém lembrar da regra._\n\n📖 _Hook em `.claude/hooks/block-cross-case-writes.sh`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #088 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **PreToolUse bloqueia risco** 💊
> 🚦 *Roda antes da escrita.* 🔒 *Separa os casos.* 🎯 *Freio técnico.*

---

### Pílula #089 — Craft update documenta método

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Craft update documenta método 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🎯 _Owner descobriu jeito de fazer que funciona? `craft-update` documenta como preferência ou método pessoal._\n\n📜 _Vira canon consultável: outras sessões seguem o mesmo padrão sem precisar reaprender._\n\n♻️ _Sem craft-update, mesma preferência é redescoberta várias vezes. Com ele, virou institutional knowledge._\n\n📖 _Skill em `bundles/base/skills/craft-update/SKILL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #089 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Craft update documenta método** 💊
> 🎯 *Método pessoal vira canon.* 📜 *Outras sessões seguem.* ♻️ *Sem redescoberta.*

---

### Pílula #090 — Update sem medo: a versão antiga fica

```json
{
  "blocks": [
    {"type": "header", "text": {"type": "plain_text", "text": "💊 Update sem medo 💊", "emoji": true}},
    {"type": "section", "text": {"type": "mrkdwn", "text": "🛠 _`maestro-setup-update` guia a atualização pelo chat e confere no fim se deu certo._\n\n🔒 _Não existe rollback automático — a rede de segurança é outra: você renomeia a pasta atual para `Maestro-old`, extrai a nova ao lado e **copia** a `data/`. Copiar é reversível; mover não._\n\n🎯 _Só apague a `Maestro-old` depois de confirmar que está tudo certo. Guarde por uns 7 dias._\n\n📖 _O ritual completo está no `README-INSTALL.md`._"}},
    {"type": "context", "elements": [{"type": "mrkdwn", "text": ":maestro: _Pílula #090 · Maestro OS_ :maestro:"}]}
  ]
}
```

> 💊 **Update sem medo** 💊
> 🛠 *Guiado pelo chat.* 🔒 *Sem rollback automático — a `Maestro-old` é a rede.* 🎯 *Copiar, não mover.*

---

## Notas de uso

- **Ordem sugerida de postagem:** girar entre blocos (agente → memória → skill → arquitetura → método) pra não saturar um tema só.
- **Numeração:** #041 continua da série interna (#001–#040 entregues inline em sessões anteriores). Próxima batch começa em #091.
- **Renumeração:** se quiser cadência diferente no canal, ajustar footer sem tocar em título/body.
- **Custom emoji `:maestro:`** precisa estar cadastrado no workspace Slack — validar antes da primeira postagem.
