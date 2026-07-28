<p align="center">
  <img src="docs/assets/maestro-hero-v2.png" alt="Maestro conduzindo trabalho contínuo, delegação governada e aprendizagem segura" width="1200">
</p>

<h1 align="center">Maestro</h1>

<p align="center">
  <strong>O sistema operacional profissional para trabalho que precisa continuar — com contexto, governança e privacidade.</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/MAESTRO-v0.1.0%20target-243047?style=for-the-badge&labelColor=0B1020" alt="Maestro v0.1.0 target">
  <img src="https://img.shields.io/badge/PILOT-private-12B886?style=for-the-badge&labelColor=0B1020" alt="Private pilot">
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

### 🏅 Toward the v0.1.0 pilot

| Capability | What it means today |
| --- | --- |
| ✅ Local workspace | `bcgos init`, `status` and `doctor` establish a local workspace without mixing it into the managed core. |
| ✅ Professional context | Owner profile, skills index, human atlas and bounded session pointers stay inspectable and local. |
| ✅ Long-running work | A local execution ledger supports contract, checkpoint, pause, resume, evidence, inspect and export. |
| ✅ Governed completion | High-stakes work can require a separately authenticated Walter review before completion. |
| ✅ Bounded delegation | Maestro can dispatch a narrow packet to the right agent; a signed packet never grants broad tool access or completion authority. |
| ✅ Professional capability bundles | Neutral engineering quality methods ship in the base bundle; specialized engineering and data-practice tracks remain catalogued and fail closed until a separate release contract exists. |
| ✅ Pilot canary | Local reports aggregate typed outcomes, capability failures, interventions and receipt metadata — never work content. |
| ✅ Privacy-safe improvement loop | The local Darwin can compile approved structural signals; central curation proposes advances for human acceptance. |

> **Truth in labeling.** `v0.1.0` is the product target and canary contract,
> not a claim that a signed end-user release is already
> available. Pilot distribution, native schedulers and hosted bridge operation
> remain release operations with their own evidence.

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

---

<p align="center">
  <strong>Professional work deserves continuity — without surrendering control.</strong><br>
  Built privately for the BCG Brasil pilot.
</p>
