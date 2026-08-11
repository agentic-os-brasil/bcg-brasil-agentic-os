# Maestro installer wizard

## Purpose

The wizard is the visual front door for a signed release. It serves people who
can install software in their corporate user profile but do not have device
administrator permissions. The experience explains the next action in plain
language, while the release verifier and `bcgos-bootstrap` remain the only
authorities allowed to verify, activate or roll back bytes.

## Core screens and workspace journeys

1. **Boas-vindas** — one sentence of value, the user-space/no-admin promise and
   three trust cues.
2. **Verificação** — release identity, manifest/artifact signatures and target
   path are shown before any write.
3. **Instalação** — the exact per-user destination and the atomic activation
   plan are explained. The user confirms once.
4. **Seu workspace** — the intent selected on the welcome screen becomes the
   single primary action after installation: create a clean workspace, create
   one and choose an authorized source, or inspect an update. Advanced
   migration/import journeys remain available behind an explicit secondary
   link, rather than repeating the same choice a second time.
5. **Análise e plano** — the transactional core returns a typed classification plus the
   mapped, excluded, ambiguous and capability-unavailable items. It also says
   whether the existing workspace is preserved and which version/migration is
   required. Selecting a folder is only a temporary analysis pointer; it is
   never called ingestion.
6. **Confirmação, staging e receipt** — confirmation is bound to the exact
   plan and approval. The transactional core must report staging, validation and rollback state;
   the UI shows “Pronto” only for a valid, committed receipt. An invalid or
   unavailable capability fails closed and never offers an unsigned or implicit
   fallback.

The welcome scene also makes the owner's intent explicit with three reversible
directions: starting fresh, bringing an existing workspace/second brain for a
later bounded import, or updating an existing Maestro installation. Selecting
one changes the guidance copy only; it never reads, copies or ingests a source
without the subsequent workspace and approval gates.

Verification and installation expose a determinate progress bar tied to the
wizard's observed phases. It starts at zero, advances through release,
integrity and user-space checks, then reaches 100% only after the connected
core returns successfully. While the request is in flight, the bar holds at an
explicit waiting state instead of implying that a background operation has
finished.

The **Ver como funciona** action opens a compact in-product explainer with the
same three contract movements — check, install, conduct — so a non-technical
person can understand the flow without leaving the installer. The shell uses
short panel transitions, orbit drift and a restrained scan line; all motion is
disabled for users who prefer reduced motion.

The first screen also exposes the connection state immediately: `MODO NÃO
CONECTADO` for a static visual inspection, `ENSAIO TÉCNICO` for `--simulate`,
and `RELEASE CONECTADO` for a real core session. The primary action follows
that state (`Abrir fluxo visual`, `Simular instalação`, or `Instalar no meu
perfil`) so the user never has to infer whether a click will mutate anything.

## Accessible progression

The visual progression is also a keyboard and assistive-technology contract:

- future steps are native-disabled until the preceding action succeeds;
- the active step exposes `aria-current="step"` and the panel title receives
  focus after navigation;
- verification, installation and workspace outcomes use live status regions,
  while failures use an alert region;
- the visual focus ring is only added for keyboard navigation, preserving the
  quiet presentation for pointer users.

These affordances describe the same gated flow; they do not create a second
installation path or bypass the core's verification rules.

## Runtime handoff

The visual shell is intentionally not a trust implementation. The executable
installer core must:

1. materialize the signed release in a staging directory;
2. call the stable, native-signed `bcgos-bootstrap` with the exact release
   directory and owner-data root;
3. stream structured verification/activation status into the wizard;
4. close with the bootstrapper's exit status and activation receipt;
5. render a failed gate as a stop with the safe next action — never as an
   unsigned retry.

The normal destination is user-level application storage (`%LOCALAPPDATA%\\BCGOS`
on Windows and `~/Library/Application Support/BCGOS` on macOS). The wizard
does not modify the global `PATH`, workspace content or credentials.

After a connected install, the wizard detects compatible runtimes locally. It
never accepts a browser-provided workspace path: it creates and verifies the
canonical local workspace, then launches only a detected runtime. The new
transactional workspace-flow core exposes these endpoints:

- `POST /api/workspace-flow/select` selects `update`, `workspace_migration` or
  `external_import`; native folder selection is kept server-side.
- `POST /api/workspace-flow/analyze` returns the typed classification and plan
  without mutation. External import calls `internal/workspaceimport` for
  inspect/plan and reports mapped, excluded, ambiguous and unavailable items.
- `POST /api/workspace-flow/confirm` accepts the exact plan digest plus
  `action=IMPORT`, binds `approved_by` and `approval_plan_id` to that plan, and
  returns a receipt containing `staging=completed`, `validation=completed` and
  `rollback=available`.
- `POST /api/workspace-flow/rollback` accepts `action=ROLLBACK` and the exact
  receipt ID. Source, target and rollback effects are separate fields.

External import preserves its source, writes only bounded allowlisted target
entries after confirmation, and can roll them back. A Maestro workspace is
classified by `internal/workspacemigration`, but public migration execution is
`pending_core_activation`/`unavailable` until bootstrapper authority is wired;
the UI blocks confirmation with HTTP 503 and does not execute a fixture.
Capability gaps, conflicts, invalid plans, changed sources and replay/tamper
conditions fail closed. `--simulate` uses sanitized fixtures only and is the
sole fixture path. A pointer-only source is reported as
`pointer_recorded_pending_analysis` / `not_ingested_pointer_only`, never as
ingestion. The first-command card uses the exact
`cli_path` returned by the installer core as a support action. On macOS it copies
`"<installed-cli>" doctor`; on Windows PowerShell it copies `&
"<installed-cli>" doctor`. Static preview and disconnected mode remain
non-installing.

Workspace creation alone is not the final runtime-readiness claim. The installer core
reports adapter configuration, readiness verification and scheduler activation
as separate states, alongside the exact `workspace_path` and a diagnostic
command bound to that path. It returns `ready_for_runtime=true` only after the
workspace, selected Claude/Codex projection, durable orchestration state,
owner context, Case Agent dossier/scaffold and per-user scheduler pass their
respective checks; a failure returns an error and never marks the workspace
ready. A generated workspace or a present orientation file is therefore not
mistaken for observed hooks, verified readiness or an active scheduler.

The final handoff presents **Abrir no Claude Code Desktop** as the primary
action when the app is detected. The launcher opens the Claude bundle with the
workspace deep link and the Maestro onboarding prompt, then asks macOS to
activate that app in front of the installer. Codex remains an explicit
secondary path when available.

If an earlier first install was interrupted, the connected core may preserve
the installer-owned managed root and its bound install state in a deterministic
plan-digest recovery location before retrying. It never replaces a healthy
installation, never overwrites an earlier recovery and never auto-recovers an
ambiguous root containing unrecognized entries. A final diagnostic failure
after bootstrapper activation is reported as quarantined/reinstallable, not as
completed.

Runtime project hooks are intentionally a separate owner-consent boundary. The
wizard installs and verifies seven workspace-local Claude hooks or five Codex
hooks, then states that the first opening must review their exact
commands before they execute automatically. It never manufactures entries in
global runtime settings, bypasses the native hook-review UX, or presents a
configured hook as a native observation. This keeps local hooks both wired and
revocable by their owner.

Once the owner accepts those commands, each hook resolves and validates the
same strict workspace-local orchestration snapshot before recording bounded
metadata evidence. Session Start also emits a non-blocking presence wake to
the already enrolled maintenance boundary. That wake is occurrence-idempotent,
does not run a model inline and remains separate from native hook
qualification.

## Visual identity

The wizard uses a small Maestro identity system rather than a generic themed
installer: a transparent conductor avatar is the primary hero, the
`MAESTRO / AGENTIC OS` lockup is the wordmark, and the baton/orbit mark is the
secondary seal. The palette converges with the presentation material — deep
green-black field, luminous mint, quiet white and a restrained aqua accent.
The orbit/constellation treatment carries the “digital second brain” idea into
the installation moment, while the staff and two notes give the regent a
musical signature without turning the screen into a concert poster. The
conductor mark anchors the rail and hero while the completion state keeps the
baton/orbit seal as a compact “show is ready” stamp; the native app-icon
remains a separate square asset for the operating system shell.

SVG keeps the assets small, deterministic and inspectable in the release
factory; no remote fonts, JavaScript packages or network calls are required.
Native `.ico`/`.icns` packaging is specified in
[`docs/installer-icons.md`](installer-icons.md) and still requires platform
signing evidence.

For a real package candidate, the macOS factory is
[`dev/release/build-macos-installer.sh`](../dev/release/build-macos-installer.sh).
Unlike the rehearsal DMG, it requires the exact release directory, authority
registry and native bootstrapper as explicit inputs and passes them to the
installer core. It remains `unsigned-candidate` until the protected Developer ID and
notarization steps run.

macOS does not permit a mounted DMG to auto-execute an installer app. The DMG
therefore remains a transport container: opening it mounts the volume, then
the user opens **Maestro Installer.app**. A production one-click delivery
should use a signed/notarized launcher or a signed `.pkg`, rather than relying
on DMG autorun behavior. The local-beta app launcher starts the local bridge
detached, writes its bounded diagnostic log under the user's temporary
directory and exits the short-lived Finder process immediately; this prevents
macOS from displaying the app as “not responding” while the browser wizard
continues to run on its loopback endpoint.

### Musical references

Music stays in the composition rather than becoming a theme costume: the
baton is the primary diagonal, orbit lines behave like sustained phrases, a
small staff and two notes sit in the background as a quiet signature, and the
single `♪` in the top bar marks the conductor without competing with the
installation action. The user-facing copy remains operational and direct.

## Evidence boundary

Opening the static wizard proves only the UX layer. It is not evidence of a
signed release, a corporate-device acceptance or pilot readiness. A connected
installer runs the real verify/install transaction; `--simulate` runs the same
user journey against an isolated rehearsal sandbox and labels its checks
`simulado`. Neither mode claims that unsigned bytes were validated. Those gates
remain defined by
`specs/020-release-distribution.md` and `specs/022-guided-pilot-release.md`.
