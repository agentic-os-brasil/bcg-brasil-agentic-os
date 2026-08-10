# Windows pilot distribution

This document separates a technically valid Maestro package from one that is
safe to hand to a corporate Windows cohort.

## Rule for a real pilot

Do **not** distribute an artifact labelled `unsigned-candidate` or
`windows-portable-local-beta` to a general Windows cohort. The latter binds the
Maestro release, registry and bootstrapper together with Ed25519 and SHA-256
pins, but it deliberately does not claim that Windows or endpoint protection
trusts the ZIP or its executables. It is only a controlled engineering canary.

Decision `PONB` makes the verified portable ZIP the current controlled-Canary
handoff. The owner extracts it once to a fixed path, opens the packaged
`maestro-os/` in Claude Code and sends a natural-language message. Claude asks
for one setup confirmation and invokes the package-internal activator; the
owner is never instructed to type or run a command. This changes the delivery
surface, not the pilot gates below.

The Windows pilot handoff is ready only when all of the following are true:

1. Every delivered PE file inside the portable ZIP, including the CLI and
   bootstrapper, has a valid Authenticode signature from the approved BCG
   publisher. If a future single-file wrapper is used, it must be signed too.
2. The signed release manifest, authority registry and package provenance are
   retained with the immutable SHA-256 of the delivered ZIP and its PE files.
3. BCG Endpoint Security has reviewed the exact hashes and confirms the
   Defender for Endpoint outcome. A false-positive submission is not the same
   as an allow decision.
4. A clean managed Windows device completes install, workspace initialization,
   Claude Code handoff and the first SessionStart hook observation.

## Immediate path for the current 40-person cohort

1. Stop circulation of the unsigned portable ZIP and any legacy unsigned EXE
   that Defender remediated. Do not ask users to disable Defender, add local
   exclusions or bypass a corporate block.
2. Give Endpoint Security the exact ZIP, its SHA-256, the hashes of its CLI and
   bootstrapper, release manifest, provenance and the exact detection name.
   They can submit a file or file hash to Microsoft Defender for Endpoint as a
   **Clean (false positive)** sample and decide the organization-level response.
3. In parallel, sign a fresh package with a BCG-owned certificate service.
   Prefer a BCG Azure Artifact Signing account/certificate profile or the
   organization's existing Authenticode service; the signing identity and its
   approval workflow must belong to BCG, not an individual developer.
4. Build a protected-factory successor to the portable local-beta profile from
   the reviewed tag, sign the CLI and bootstrapper before packaging, generate
   provenance and submit the exact signed ZIP and PE hashes for endpoint review.
   Do not weaken the current local-beta factory's exact `NotSigned` contract.
   Test the successor package on an enrolled BCG Windows device before the
   cohort download is reopened.

For managed BCG endpoints, Intune/Company Portal is the preferred delivery
channel after the signed artifact is qualified. The Microsoft Store/LOB path is
an alternative when its organization setup and packaging requirements are
available; it is not a workaround for an unsigned executable.

## Why there is no free bypass

Windows reputation is based on both publisher certificate and file hash. A new
unsigned hash starts with no publisher reputation, and each update starts that
process again. Authenticode improves provenance but does not guarantee that a
new file bypasses SmartScreen or Defender.

SignPath Foundation is a useful free option only for a fully public,
OSI-licensed, non-dual-licensed project without proprietary components and
with its required release, review and signing policy. Maestro must not use that
route unless the product and its distribution have actually been made eligible.
The current Maestro repository declares a proprietary, closed-source license,
so SignPath is **not** an available shortcut for this product today. Publishing
the repository alone would not change that eligibility.

## Engineering evidence versus release approval

| Package profile | Intended use | May be sent to the cohort? |
| --- | --- | --- |
| `windows-portable-local-beta` / `unsigned-candidate` | controlled Canary only | No general cohort |
| valid BCG Authenticode + Ed25519 release | endpoint review and clean-device pilot test | Not until endpoint approval |
| valid BCG Authenticode + Endpoint approval + clean-device evidence | managed pilot | Yes |

The release factory remains strict by default. `-LocalBeta` is explicit and
kept solely for controlled engineering evidence; it is never a production or
pilot distribution mode.
