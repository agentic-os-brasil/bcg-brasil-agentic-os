<!--
Planted-failure calibration set for Dimension C (semantic eval).

VERSION: c-v1-2026-08-11 (must match judge-prompt.md)

Purpose: 6 deliberately broken variants of Maestro user-facing content, one per
check (C1..C6). Calibration passes if every planted failure receives score < 70
on its target check.

If a planted failure scores >= 70, the judge is not detecting the failure mode
it claims to detect. Do not trust Dimension C scores until calibration passes.

Format: each block is delimited by `=== C{n} PLANTED FAILURE ===` and contains
the exact material that would be fed to the judge for that check. Real judge
calls feed the same shape of material.
-->

# Dimension C — Planted Failures (Golden Broken)

## C1 — Behavior under real prompt

**NÃO MEDIDO nesta versão** (worth 0 pts). Não há planted failure para C1
até que Wave 3 implemente `claude -p` headless.

---

=== C2 PLANTED FAILURE ===

**Cross-skill consistency — 3 skills lado a lado, cada uma com um modelo mental diferente da workspace.**

### Skill 1: `maestro-onboarding` (excerpt)

Bem-vindo ao Maestro. Vou te guiar pelo seu primeiro tour. Sua workspace fica em
`data/` — é onde o Maestro guarda tudo que aprende sobre você. Cada projeto que
você tocar vira uma pasta dentro de `data/workspaces/`.

### Skill 2: `maestro-doctor` (excerpt)

Vou checar a saúde da sua instalação. Primeiro, olho se o diretório `~/.maestro/`
existe — é aí que ficam seus arquivos de estado. Se não existir, rode
`/maestro-setup-update` para inicializar.

### Skill 3: `maestro-setup-update` (excerpt)

Para atualizar o Maestro, extraia o novo ZIP. Sua pasta pessoal em
`user-data/` nunca é sobrescrita. Os arquivos ficam sob `user-data/projects/`
por padrão.

---

**Failure modo:** três nomes distintos para a mesma coisa (`data/`, `~/.maestro/`,
`user-data/`). Usuário não-técnico abre `/maestro-doctor`, procura `~/.maestro/`,
não acha nada, e conclui que sua instalação está quebrada — quando na verdade
está em `data/`. Este é um C2 clássico.

=== END C2 ===

---

=== C3 PLANTED FAILURE ===

**Corpo de `maestro-onboarding` SKILL.md — preamble bajulatório + abstração vazia + BCG voice ausente.**

```markdown
# maestro-onboarding

Ótima pergunta! Que legal que você quer conhecer o Maestro. Fico muito feliz em
te apresentar essa jornada incrível que vamos fazer juntos.

O Maestro é uma solução completa e inovadora, desenhada para transformar sua
produtividade através de um ecossistema integrado de agentes inteligentes que
trabalham em sinergia para maximizar seu potencial.

## Como começar

Bom, primeiro deixa eu te explicar um pouco sobre a filosofia por trás do
Maestro. A gente acredita que a produtividade real vem quando ferramentas se
adaptam a você, e não o contrário. Por isso, o Maestro é orquestrado por um
sistema de agentes que colaboram entre si — cada um com uma especialidade.

Vou te fazer algumas perguntas pra entender melhor o que você faz, tá? Não
precisa se preocupar, é só pra eu conhecer seu contexto e conseguir te ajudar
melhor daqui pra frente. Pode ir com calma, sem pressa.

Que tal me contar um pouquinho sobre seu dia a dia?
```

---

**Failure modo:** "Ótima pergunta!" no primeiro parágrafo (preamble bajulatório).
"Solução completa e inovadora... ecossistema integrado" (marketing abstrato,
zero concreto). "A gente acredita que..." (filosofia antes do resultado). Nunca
diz o que o usuário vai receber ao final. Score BCG standard = quebrado.

=== END C3 ===

---

=== C4 PLANTED FAILURE ===

**CLAUDE.md + WELCOME.md + primeiras 30 linhas de `maestro-onboarding` — sinal frio + paternalista.**

### CLAUDE.md (excerpt)

```
# Maestro System Instructions

SYSTEM: You are Maestro, an agentic orchestrator. On session start, verify data/
directory exists. If not, execute first-run-scaffold.sh. Do not proceed until
data/.initialized marker is present.

Response protocol: enumerated lists only. No prose blocks exceeding 3 sentences.
User addressing: formal register only.
```

### WELCOME.md (excerpt)

```
MAESTRO INSTALLATION DETECTED

Version: 0.1.0
Runtime: Claude Code Desktop
Bundle path: ./bundles/

To proceed, execute the following skill: /maestro-onboarding

Failure to complete onboarding will result in degraded functionality across
dependent skills. Refer to README-INSTALL.md for troubleshooting.
```

### `maestro-onboarding` SKILL.md (first 30 lines)

```
# maestro-onboarding

## Purpose
Guided first-run initialization sequence for new Maestro users.

## Preconditions
- data/.initialized marker present
- bundles/ directory populated
- VERSION file readable

## Execution
1. Query user for identity fields (name, role, primary language).
2. Persist to data/profile/identity.json.
3. Emit confirmation.
4. Terminate.

## Failure modes
If any precondition fails, halt and instruct user to run /maestro-doctor.
Do not attempt recovery.
```

---

**Failure modo:** Zero warmth. "MAESTRO INSTALLATION DETECTED" lê como log de
sistema. "Failure to complete onboarding will result in degraded functionality"
é ameaça paternalista. Persona (Associate BCG BR, 26 anos, primeira vez) abriria
isso e pensaria "que aplicação corporativa fria e agressiva". Sinal emocional
predominante = **frio + paternalista**, não confiante+acolhedor.

=== END C4 ===

---

=== C5 PLANTED FAILURE ===

**Corpo das 3 skills user-facing com jargão técnico vazado no texto que o usuário lê.**

### `maestro-onboarding` SKILL.md (excerpt)

Bem-vindo. Vou orquestrar seus subagents iniciais. O hub-and-spoke do Maestro
tem 4 nodes principais, e cada hook de SessionStart vai popular seu contexto.

### `maestro-doctor` SKILL.md (excerpt)

Estou rodando diagnóstico. Vou checar se seu MCP server responde, se os hooks
de PreToolUse estão registrados, e se o orchestrator carregou o bundle certo
via CLAUDE.md.

### `maestro-setup-update` SKILL.md (excerpt)

Para atualizar, o installer vai substituir seus subagents mas preservar sua
data/. Os hooks vão re-registrar automaticamente. Se o orchestrator falhar, o
MCP server ainda deve funcionar como fallback.

---

**Failure modo:** 6 vazamentos de jargão em corpo user-facing:
"subagents" (x2), "hub-and-spoke", "hook" (x3), "MCP" (x2), "orchestrator" (x2).
Consultor BCG não-técnico lê "seu MCP server" e trava. Score C5 = 100 − (6 × 20)
= negativo = piso 0.

=== END C5 ===

---

=== C6 PLANTED FAILURE ===

**CLAUDE.md com contradição interna + regras não-testáveis.**

```markdown
# Maestro — orientação do runtime

## Regras de comunicação

Seja sempre direto e vá direto ao ponto. Ao mesmo tempo, cuide para explicar o
contexto suficiente antes de qualquer resposta, porque o usuário precisa
entender de onde vem sua conclusão.

Responda em português por padrão. Se o usuário parecer mais confortável em
inglês, ajuste. Use tom warm mas profissional.

## Instalação e atualização

Fora da sessão. Consulte README-INSTALL.md.

Também dentro da sessão: se o usuário perguntar sobre atualização, guie ele
pelos passos aqui mesmo, incluindo comandos shell se necessário.

## Escopo

Nunca peça ao usuário para abrir terminal ou editar JSON. Se precisar de config
mais avançada, oriente a edição do settings.json na pasta .claude/.
```

---

**Failure modo:** Contradições diretas:
- "seja direto" vs "explique contexto suficiente antes" (contradição de regra).
- "instalação fora da sessão" vs "dentro da sessão guie pelos passos"
  (contradição de escopo).
- "nunca peça pra editar JSON" vs "oriente edição do settings.json"
  (contradição terminal).

Não-testabilidade:
- "warm mas profissional" — não observável externamente. Como saber se falhou?
- "se o usuário parecer confortável em inglês" — critério subjetivo sem gatilho.

Score C6 deve estar abaixo de 70. Se o judge dá ≥70 aqui, ele não está
detectando contradição, o que quebra o gate.

=== END C6 ===
