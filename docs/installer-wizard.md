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
4. **Pronto** — the first `bcgos doctor` command and the recovery promise are
   visible.

The **Ver como funciona** action opens a compact in-product explainer with the
same three contract movements — check, install, conduct — so a non-technical
person can understand the flow without leaving the installer. The shell uses
short panel transitions, orbit drift and a restrained scan line; all motion is
disabled for users who prefer reduced motion.

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

## Visual identity

The wizard converges with the presentation material: deep green-black field,
luminous mint orbit lines and typography, quiet white copy and a restrained
 secondary aqua accent. The Maestro avatar is visible as a small conductor
 identity plaque and a native app-icon medallion; the faceless conductor mark
 remains the visual anchor and the baton/orbit mark remains the secondary visual
 anchor. The orbit/constellation treatment carries the “digital second brain”
 idea into the installation moment. SVG keeps the assets small, deterministic
 and inspectable in the release factory; no remote fonts, JavaScript packages or
 network calls are required. Native `.ico`/`.icns` packaging is specified in
[`docs/installer-icons.md`](installer-icons.md) and still requires platform
signing evidence.

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
