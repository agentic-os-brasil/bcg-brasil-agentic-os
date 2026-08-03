# Maestro installer wizard

## Purpose

The wizard is the visual front door for a signed release. It serves people who
can install software in their corporate user profile but do not have device
administrator permissions. The experience explains the next action in plain
language, while the release verifier and `bcgos-bootstrap` remain the only
authorities allowed to verify, activate or roll back bytes.

## Four screens

1. **Boas-vindas** — one sentence of value, the user-space/no-admin promise and
   three trust cues.
2. **Verificação** — release identity, manifest/artifact signatures and target
   path are shown before any write.
3. **Instalação** — the exact per-user destination and the atomic activation
   plan are explained. The user confirms once.
4. **Seu workspace** — creates one new local workspace at
   `~/Developer/maestro-os`. Before creation, the user may nominate an existing
   memory folder. That source remains untouched and is registered as a pending
   local intake; no document is read, copied or uploaded by the installer.
5. **Pronto** — returns the person to work, rather than to an application
   folder. The first-use projection is Codex, because a workspace may carry
   one managed runtime projection at a time. On macOS, Codex in the ChatGPT
   desktop app receives the workspace through the documented
   `codex://new?path=<absolute-path>` deep link. Switching that workspace to
   Claude is a separate explicit adapter migration, not a second concurrent
   install. The `doctor` command remains a support action.

The **Ver como funciona** action opens a compact in-product explainer with the
same three contract movements — check, install, conduct — so a non-technical
person can understand the flow without leaving the installer. The shell uses
short panel transitions, orbit drift and a restrained scan line; all motion is
disabled for users who prefer reduced motion.

The first screen also exposes the connection state immediately: `MODO NÃO
CONECTADO` for a static visual inspection, `ENSAIO TÉCNICO` for `--simulate`,
and `RELEASE CONECTADO` for a real bridge session. The primary action follows
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
installation path or bypass the bridge's verification rules.

## Runtime handoff

The visual shell is intentionally not a trust implementation. The executable
bridge must:

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
canonical local workspace, then launches only a detected runtime. A separately
selected memory source is recorded under `.bcgos/import-intake.json` with
state `pending_verified_pack`; it is not itself used as a workspace. The
first-command card uses the exact
`cli_path` returned by the bridge as a support action. On macOS it copies
`"<installed-cli>" doctor`; on Windows PowerShell it copies `&
"<installed-cli>" doctor`. Static preview and disconnected mode remain
non-installing.

Workspace creation is not the final runtime-readiness claim. The bridge returns
`workspace_created`, the exact `workspace_path` and a diagnostic command bound
to that path. It reports adapter configuration, readiness verification and
scheduler activation as separate states; until all required later gates run,
`ready_for_runtime` remains false. This prevents a generated workspace or a
present `AGENTS.md` from being mistaken for observed hooks, verified readiness
or an active scheduler.

If an earlier first install was interrupted, the connected bridge may preserve
the installer-owned managed root and its bound install state in a deterministic
plan-digest recovery location before retrying. It never replaces a healthy
installation, never overwrites an earlier recovery and never auto-recovers an
ambiguous root containing unrecognized entries. A final diagnostic failure
after bootstrapper activation is reported as quarantined/reinstallable, not as
completed.

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
bridge. It remains `unsigned-candidate` until the protected Developer ID and
notarization steps run.

macOS does not permit a mounted DMG to auto-execute an installer app. The DMG
therefore remains a transport container: opening it mounts the volume, then
the user opens **Maestro Installer.app**. A production one-click delivery
should use a signed/notarized launcher or a signed `.pkg`, rather than relying
on DMG autorun behavior.

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
