<p align="center">
  <img src="docs/assets/maestro-hero-v2.png" alt="Maestro conduzindo trabalho contínuo, delegação governada e aprendizagem segura" width="1200">
</p>

<h1 align="center">Maestro</h1>

<p align="center">
  <strong>O sistema operacional profissional para trabalho que precisa continuar — com contexto, governança e privacidade.</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/MAESTRO-v0.1.0%20target-243047?style=for-the-badge&labelColor=0B1020" alt="Maestro v0.1.0 target">
  <img src="https://img.shields.io/badge/STATUS-contract--validated-12B886?style=for-the-badge&labelColor=0B1020" alt="Contract validated; runtime qualification pending">
  <img src="https://img.shields.io/badge/PRIVACY-bounded%20by%20design-5B8DEF?style=for-the-badge&labelColor=0B1020" alt="Privacy bounded by design">
  <img src="https://img.shields.io/badge/RUNTIME-Claude--first%20%C2%B7%20Codex--compatible-F4B942?style=for-the-badge&labelColor=0B1020" alt="Claude-first and Codex-compatible">
</p>

<p align="center">
  <a href="#the-promise">Promise</a> ·
  <a href="#what-is-ready">What is ready</a> ·
  <a href="#pilot-boundaries">Pilot boundaries</a> ·
  <a href="#start-here">Start here</a> ·
  <a href="ROADMAP.md">Roadmap</a>
</p>

---

## The promise

Work should not reset because a conversation ended, a handoff happened, or an
agent lost the thread. Maestro keeps the important parts of professional work
**legible, bounded and recoverable** — without turning client material into a
generic prompt dump.

| 🧭 Continue the work | 🛡️ Keep the boundaries | ✨ Improve with evidence |
| --- | --- | --- |
| A durable local ledger records the objective, checkpoints, evidence and the next safe action. | Managed product, local owner context and client workspace stay physically and logically separate. | The canary observes only typed, metadata-only signals; no prompts, documents or client content leave the workspace. |

### A workday that does not start over

```text
Define the outcome  →  Execute deliberately  →  Pause or hand off  →  Resume with proof  →  Improve the product
       contract              evidence               checkpoint              ledger                typed canary
```

Maestro is designed for consultants, BCG X practitioners, data scientists and
engineers who need the same thing: useful continuity without silent access,
invented memory or ungoverned automation.

## What is ready

### 🏅 Toward the v0.1.0 pilot — contract layer validated

| Capability | What it means today |
| --- | --- |
| Workspace local | `bcgos init`, `status` e `doctor` criam e inspecionam o espaço sem misturar dados de trabalho ao core gerenciado. |
| Preferência de interação | `standard`, `advanced` e `power`, configuradas localmente e sem alterar permissões. |
| Contexto do dono | Arquivos locais, editáveis e auditáveis para papel profissional, estilo, voz, preferências e limites. |
| Atlas humano | Estrutura local não destrutiva para clientes, projetos, pessoas e diário de trabalho. |
| Memória | Núcleo local com resumos graduais e contexto limitado; automação de síntese ainda não está ativa. |
| Execução retomável | Ledger local com contrato imutável, checkpoint privado, pausa/retomada, contrato core para breadcrumbs metadata-only de tool calls, receipts de checks executados pelo core e conclusão revalidada; emissão nativa pelos adapters ainda está pendente. |
| Skills | Catálogo compacto e atualizado de procedimentos disponíveis. |
| Ingestão local | Contrato provider-neutral, adapter MarkItDown bounded e `bcgos ingest` com resolução fail-closed do runtime pack; conversão só fica disponível após instalação do pack verificado. |
| Desenvolvimento | Harness, testes, CI em Windows/macOS/Linux, decisões versionadas e fluxo de PR com revisão humana. |

> **Truth in labeling.** `v0.1.0` is the product target and contract/canary
> baseline, not a claim that native runtime activation, telemetry, a signed
> end-user release or a user pilot is available. Pilot distribution, native
> schedulers and hosted bridge operation remain release operations with their
> own evidence.

### Maturity ladder

Maestro advances only when the evidence for the next tier exists:

1. **Contract-validated** — deterministic core, skills, agent contracts and
   local harness pass; runtime capabilities may still be `unavailable`.
2. **Runtime-qualified** — one supported runtime/platform invokes the installed
   adapter in a fresh native session with bounded evidence.
3. **Technical shadow** — two human-in-the-loop users exercise one bounded use
   case with compensating controls and explicit stop criteria.
4. **Controlled pilot** — signed release, clean-device acceptance, support and
   incident ownership, plus the pilot gate are complete.
5. **Production** — the controlled pilot meets its success and safety criteria
   and the release owner records promotion.

The repository is currently at tier 1. Q-011 (one concrete use case, persona
and acceptance metric) must close before tier 3; no user pilot should be
described as active while native lifecycle evidence is pending.

## Pilot boundaries

<table>
  <tr>
    <td width="33%" valign="top">
      <h3>🔒 Data stays scoped</h3>
      <p>Client files, prompts, paths, people, conversation text and credentials do not enter telemetry or federated batches.</p>
    </td>
    <td width="33%" valign="top">
      <h3>🧱 Authority stays narrow</h3>
      <p>Maestro remains read-oriented. Any action travels through a bounded, authenticated delegation contract — never an implicit tool grant.</p>
    </td>
    <td width="33%" valign="top">
      <h3>👤 Humans keep the final say</h3>
      <p>Central Darwin may curate recurring patterns into proposals. It cannot merge code, publish skills or release software.</p>
    </td>
  </tr>
</table>

### What the canary measures

| Signal | Why it matters | What is excluded |
| --- | --- | --- |
| Time to first value | Whether the pilot becomes useful quickly | Task content, files and client names |
| Resume success | Whether long-running work survives interruption | Objective prose and checkpoint body |
| Install, update and rollback | Whether delivery is reliable | Device identity and credentials |
| Manual interventions | Where the experience still asks too much of people | Free-text support history |
| Capability failures | Which product surfaces need improvement | Error messages and runtime logs |
| Receipt metadata | Whether governed work completed safely | Inputs, outputs and evidence content |

## Start here

### 👋 Pilot participant

The repository is the factory, not the product installation. Pilot users will
use a verified release and `bcgos` — not Git, Go, Python, Node or Docker.
Distribution is intentionally not yet declared ready until signed artifacts and
clean-device evidence exist.

### 🧑‍💻 Contributor

```text
1. Read CONTRIBUTING.md
2. Run: go run ./dev/harness setup
3. Start a session and say: “Use start-contributing.”
4. Follow: start-work → develop-change → prepare-pr → human review
```

For the complete product map, see the [roadmap](ROADMAP.md). For the actual
delivery and validation contract, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Product map

| Surface | Purpose | State |
| --- | --- | --- |
| [`bcgos`](cmd/bcgos) | Local product CLI and bounded inspection surfaces | Building |
| [`specs/`](specs) | Runtime-neutral contracts and architectural boundaries | Active |
| [`adapters/`](adapters) | Claude and Codex projections of shared contracts | In progress |
| [`dev/`](dev) | Contributor-only harness, governance and development skills | Active |
| [`acceptance/`](acceptance) | Clean-device and pilot acceptance evidence | In progress |
| [`bundles/`](bundles) | Versioned professional capability catalogs and optional tracks | Available for inspection; activation unavailable |

Lifecycle adapter evidence is intentionally separated into configuration,
direct-contract tests, adapter-command receipts and native-session proof. See
[the lifecycle evidence matrix](specs/035-lifecycle-evidence-matrix.md).

```text
bcgos init <workspace>
bcgos doctor <workspace>
bcgos profile show
bcgos owner init
bcgos atlas init <workspace>
bcgos atlas status <workspace>
bcgos skills index
bcgos ingest --workspace <path> --source <local-file> --adapter markitdown
bcgos session packet [workspace]
bcgos work create --workspace <path> --stdin
bcgos work start --workspace <path> --item <id> --revision <n>
bcgos work checkpoint --workspace <path> --item <id> --revision <n> --attempt <id> --stdin
bcgos work pause --workspace <path> --item <id> --revision <n> --attempt <id>
bcgos work next --workspace <path> (--item <id> | --active)
bcgos work resume --workspace <path> --item <id> --revision <n>
bcgos work evidence --workspace <path> --item <id> --revision <n> --attempt <id> --criterion <id>
bcgos work complete --workspace <path> --item <id> --revision <n> --attempt <id>
bcgos work inspect --workspace <path> --item <id>
bcgos work export --workspace <path> --item <id>
bcgos work delete --workspace <path> --item <id> --revision <n> --confirm
```

---

<p align="center">
  <strong>Professional work deserves continuity — without surrendering control.</strong><br>
  Built privately toward the BCG Brasil pilot.
</p>
