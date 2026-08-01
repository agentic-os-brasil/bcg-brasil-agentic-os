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
  <a href="docs/onboarding/maestro-user-onboarding.md">Onboarding</a> ·
  <a href="docs/onboarding/sharepoint-prior-work-onboarding.md">Prior-work retrieval</a> ·
  <a href="docs/roadmap/maestro-evolution-roadmap.md">Product evolution</a> ·
  <a href="ROADMAP.md">Engineering roadmap</a> ·
  <a href="LICENSE.md">Closed-source license</a>
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

## Why Maestro

Maestro turns agent assistance into a professional operating model. It keeps
the work legible across sessions, bounds authority by workspace and capability,
and improves from typed operational evidence rather than exporting client
content. The result is a product that can be adopted by a non-technical pilot
user while still giving engineering, security and leadership a contract they
can inspect.

| Product advantage | What the user experiences |
| --- | --- |
| Continuity | Resume a long-running work item from an explicit checkpoint and next safe action. |
| Governed execution | Scope, identity, evidence and human review remain visible at the point of action. |
| Local-first knowledge | Ingestion is designed around local Docling with approved deterministic fallbacks, never an implicit remote upload. |
| Organizational prior-work | Retrieve an earlier deck from explicitly enrolled SharePoint folders through an explainable local catalog, without giving Codex a SharePoint connection. |
| Runtime neutrality | Claude-first and Codex-compatible projections share product contracts and conformance language. |
| Privacy-safe improvement | The canary can measure friction and reliability without collecting prompts, documents or client names. |

For the full user journey, read the [Maestro user onboarding](docs/onboarding/maestro-user-onboarding.md).
For the business-facing evolution path, read the [product evolution roadmap](docs/roadmap/maestro-evolution-roadmap.md).

### Maestro's bounded routing topology

Maestro is the only user-facing hub. Its typed planner makes two independent
decisions before a Case runs: whether the work needs a Client Account's
strategic/stakeholder lens, and whether the resulting work is high-leverage
enough for Walter's calm Senior Advisor & Refiner review. The first decision is
based on client strategy, relationship, stakeholder pressure-testing, client
narrative, cross-case context or promotion signals—not technical size. The
second is based on consequence, leverage, reversibility, external exposure and
reputational risk.

The runtime keeps one active spoke at a time, depth one and zero children. An
account-assisted Case uses `Maestro → Client Account → Maestro → Case →
Maestro → Client Account validation → Maestro`; a direct execution-only Case
omits only the pre-brief and converges at `Maestro`. Both paths then invoke
Walter only when the independent high-leverage decision requires it, or record
an auditable low-leverage skip, before delivery. Claude and Codex share the
same controller and durable installation-state contract; capability reporting
distinguishes configured, adapter-observed and native-qualified evidence.

Walter's review packet is an ephemeral, digest-bound `IntentReviewPacket`: it
contains the literal request, selected route, bounded draft/context, audience,
consequence, reversibility and a `UserSelfSnapshot` projection. Walter's
intrinsic-intent assessment is a hypothesis with evidence and confidence, not
mind-reading. Owner Context facets remain the sole self authority. Maestro
evaluates every interaction, but persists only material, authenticated owner
signals as normalized local metadata; prompts, client documents and generated
output never become self evidence. The local CLI exposes inspection/export and
owner-confirmed rejection/redaction, revert and deletion controls.

When explicitly enabled, the owner-local PromptHistoryStore retains only user
prompts under bounded global/workspace/account/case scopes. Walter receives a
small relevance-scored selection alongside the current prompt; the current
prompt is normalized first, then history is normalized into the configured
working language. Each representation and the combined current-plus-history
packet are hard-bounded; translator expansion and oversized owner facets fail
closed. The packet is sealed before the current dispatch occurrence is
recorded, preserving earlier same-session/repeated prompts without duplicating
the current occurrence. Scores and reason codes are packet metadata only; historical text is
quoted data and kept only in the ephemeral review packet. Prompt history never enters receipts, telemetry, managed bundles or release artifacts,
and remains separate from self learning. The reachable `bcgos maestro dispatch
--stdin` boundary records an authenticated prompt, constructs and persists the
Maestro chain, and returns metadata-only dispatch state while model execution
is unavailable.

## What is ready

### 🏅 Toward the v0.1.0 pilot — contract layer validated

| Capability | What it means today |
| --- | --- |
| ✅ Local workspace | `bcgos init`, `status` and `doctor` establish a local workspace without mixing it into the managed core. |
| ✅ Guided runtime projection | `bcgos adapter install` installs hooks, a concise but complete `CLAUDE.md`/`AGENTS.md`, and the real base skills with idempotent, conflict-safe ownership. |
| ✅ Professional context | Owner profile, skills index, human atlas and bounded session pointers stay inspectable and local. |
| ✅ Long-running work | A local execution ledger supports contract, checkpoint, pause, resume, evidence, inspect and export. |
| ✅ Governed completion | High-stakes work can require a separately authenticated Walter review before completion. |
| ✅ Bounded delegation contract | The deterministic core can dispatch a narrow packet to the right agent; native runtime activation remains unavailable until adapter qualification. |
| ✅ Professional capability bundles | Neutral engineering quality methods ship in the base bundle; specialized engineering and data-practice tracks remain catalogued and fail closed until a separate release contract exists. |
| ✅ Canary contract | The local store can aggregate typed outcomes, capability failures, interventions and receipt metadata — native telemetry remains unavailable and no work content is exported. |
| ✅ Privacy-safe improvement loop | The local Darwin can compile approved structural signals; central curation proposes advances for human acceptance. |
| ✅ Darwin 🧬 operational surgeon | The same Darwin contract supports interactive and headless housekeeping with scoped `health/maestro-system` repairs and metadata-only receipts; native runtime invocation remains unavailable. |
| 🛠️ Automatic maintenance contract | A base-bundle catalog prebuilds memory, wiki, runtime-health, self-observation and update jobs; each remains unavailable until its executor and native evidence are qualified. |
| ✅ Local ingestion | Provider-neutral contract, Docling-first fallback selector, bounded MarkItDown adapter and fail-closed `bcgos ingest`; conversion remains unavailable until an approved managed runtime pack is verified. |
| ✅ Governed prior-work retrieval | Enrollment, signed snapshot import, deterministic local indexing, explicit search, freshness, revocation and scheduling are implemented and locally validated. Native Claude collection remains unavailable pending a qualifying trial; Codex collection is prohibited by corporate policy. |

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

### Automatic maintenance, with honest boundaries

`bcgos maintenance status` shows whether the maintenance plane is merely
prebuilt or actually executable. `bcgos maintenance catalog` exposes the
platform-neutral job contracts; `bcgos maintenance wake --trigger presence`
returns an unavailable state until a qualified executor exists. macOS and
Windows wake templates live under [`adapters/`](adapters) and are disabled
reference artifacts, not raw tasks inside the immutable base bundle. A wake-up
never counts as a successful memory commit or wiki publication.

### For a pilot user

The detailed onboarding covers workspace choice, first value, pause/resume,
ingestion, profiles, safe recovery and the boundary between an implemented
contract and a released runtime.

For the organizational retrieval journey — including the Claude/SharePoint and
Codex boundary, enrollment, signed import, explicit search, scheduling,
revocation and pilot acceptance — use the
[prior-work onboarding](docs/onboarding/sharepoint-prior-work-onboarding.md).

### For a decision-maker

The product roadmap explains the value unlocked by each evolution horizon,
which evidence is required before the promise expands and why distribution,
ingestion and governance are intentionally sequenced.

## Product map

| Surface | Purpose | State |
| --- | --- | --- |
| [`bcgos`](cmd/bcgos) | Local product CLI and bounded inspection surfaces | Building |
| [`specs/`](specs) | Runtime-neutral contracts and architectural boundaries | Active |
| [`adapters/`](adapters) | Claude and Codex projections of shared contracts | In progress |
| [`dev/`](dev) | Contributor-only harness, governance and development skills | Active |
| [`acceptance/`](acceptance) | Clean-device and pilot acceptance evidence | In progress |
| [`bundles/`](bundles) | Versioned professional capability catalogs and optional tracks | Available for inspection; activation unavailable |
| [`internal/priorwork/`](internal/priorwork) | Governed organizational prior-work catalog and explicit retrieval | Local core validated; native Claude collection pending; Codex collection prohibited |

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
bcgos adapter install --runtime claude <workspace>
bcgos adapter status --runtime claude <workspace>
bcgos ingest --workspace <path> --source <local-file> --adapter markitdown
bcgos prior-work actor
bcgos prior-work enroll --stdin --confirm
bcgos prior-work status
bcgos prior-work import --snapshot <json> --receipt <json>
bcgos prior-work find --explicit --stdin --limit 5
bcgos prior-work sync-due --runtime <claude|codex>
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
  Built privately toward the BCG Brasil pilot.<br>
  Developed by Daniel Scardini · Julia Ribeiro · Marcelho Sanches<br>
  <a href="LICENSE.md">Maestro Proprietary License v1.0 · All rights reserved</a>
</p>
