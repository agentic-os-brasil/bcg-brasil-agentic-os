# Spec 051 - Platform-specific Claude portable distribution

Status: accepted for implementation as a controlled local-beta distribution
contract. Production native signing, clean-device acceptance and publication
remain separate release gates.

## Objective

Deliver the complete Maestro product to a non-technical owner through two
small, deterministic ZIP archives:

- `macos-arm64`; and
- `windows-amd64`.

The owner extracts the archive, opens its `maestro-os/` workspace in Claude
Code, sends a natural-language message and confirms local preparation once.
Claude conducts activation, verification and onboarding without asking the
owner to open a terminal, discover a command or run a script.

The portable profile changes transport and interaction only. It is not a lite
edition, does not define a second workspace model and must not remove a product
capability present in the same canonical release.

## Distribution profiles

The factory emits exactly one native target per archive:

```text
Maestro-Portable-<version>-macos-arm64-local-beta-unsigned.zip
Maestro-Portable-<version>-windows-amd64-local-beta-unsigned.zip
```

Each ZIP has one top-level directory, one `maestro-os/` workspace and only the
bootstrapper for its declared platform and architecture as an executable seed.
The closed release remains byte-for-byte complete, so it may retain signed,
inert artifacts for other targets; those artifacts are never selected,
installed, made executable or referenced by the activator. A universal seed,
cross-platform fallback, binary conversion, emulation and execution of the
other platform's payload are forbidden. macOS amd64 may be added later as
another explicit platform archive; it is not silently activated from the
arm64 package.

The package may move before activation. After activation, its top-level
directory is fixed because workspace hooks bind the installed CLI by absolute
path. The managed core and workspace remain siblings; managed product bytes do
not live inside client work.

## Product-parity invariant

Both packages carry the same exact Ed25519-authenticated `canary` release,
approved authority registry and platform-neutral base bundle. Only the native
bootstrapper seed differs by target. Factory validation derives package
content from the closed release manifest and rejects a package that omits,
rewrites or independently substitutes a governed artifact. The bootstrapper
selects only the matching CLI from that verified release.

Activation uses the canonical bootstrapper and one-and-done setup transaction
to materialize the ordinary installed product, including:

- initialized workspace metadata and owner-local scaffolds;
- managed `CLAUDE.md`, complete governed skills and selection policy;
- Client Account, Case, Yoda, Darwin and PA Expert native Claude agents;
- Claude lifecycle bindings for `SessionStart`, `UserPromptSubmit`,
  `PreToolUse`, `PostToolUse`, `Stop`, `SubagentStart` and `SubagentStop`;
- brain, task, decision, project, source and deliverable navigation;
- owner context, onboarding, memory, continuity and bounded operating methods;
- update, rollback, health, recovery, prior-work and ingestion contracts; and
- the platform's maintenance assets and status surfaces.

The archive does not carry user memory, client data, SharePoint content,
credentials, prompts, logs, private signing keys or a pre-approved owner
profile. Optional external dependencies are not product omissions: Claude
guides their activation when relevant. In particular, MarkItDown remains a
post-install managed-runtime follow-up and SharePoint remains an explicit
owner-selected source; neither blocks local first use.

Package parity means the portable route exposes the same capability contract
and governed bundle as the canonical release. It does not mean that an absent
external runtime, corporate connector or native OS authority is falsely
reported as active.

## Claude-first bootstrap

Before runtime projection exists, the seeded `maestro-os/CLAUDE.md` is the
sole user-facing entry point. It requires Claude to:

1. verify silently that the host matches the archive target;
2. explain in one sentence that it can prepare Maestro locally;
3. ask one short technical setup confirmation;
4. invoke the package-internal activator only after an affirmative answer;
5. let the stable bootstrapper verify and activate the closed release in the
   managed root;
6. invoke the newly installed CLI for canonical Claude `setup apply` and
   post-install adapter verification;
7. reload the now-managed `CLAUDE.md`; and
8. continue directly into `maestro-onboarding` or resume it when incomplete.

The bootstrapper owns release verification, managed-root installation and
managed-core rollback. The package-internal activator then invokes only the
exact newly installed CLI for `setup apply` and readiness/adapter verification.
It never runs a released CLI directly from the ZIP, duplicates release
verification, invents an alternate managed root or exposes those commands to
the owner. A setup-stage failure uses the existing setup/adapter transaction to
restore owned workspace projections while preserving user-authored files.

The technical setup confirmation covers safe, reversible, per-user local
preparation and may be reused by idempotent repair. Owner-profile review,
credential entry, account or tenant selection, new SharePoint scope, native OS
scheduler enrollment, external publication and destructive work remain their
own real decisions.

## Continuity and guided next actions

Successful activation must hand off to the ordinary installed Maestro rather
than end at a folder or technical status screen. The first and subsequent
Session Start packets load the deterministic BCGOS operating method, pending
onboarding when applicable and bounded continuity state.

At natural transition points, when the owner's intent or current state makes a
continuation useful, Maestro offers up to three contextual next actions. Each
suggestion states what can happen, why it matters and which skill, agent or
knowledge source can help. Suggestions are advisory and executable after an
ordinary owner request or selection; reversible local work does not require a
new product confirmation. Suggestions are never a generic checklist, a fixed
priority ladder or a new admission gate.

Typical first-use suggestions include completing owner onboarding, selecting
an existing local/SharePoint knowledge source, creating the first bounded work
item and installing an optional managed ingestion dependency. Later sessions
adapt suggestions to the owner's intent and the bounded evidence available,
such as unfinished onboarding, active work, a valid checkpoint, selected prior
work or a relevant Darwin observation. No prompt, client body or checkpoint
body is added to Session Start.

## Minimum-risk activation

The owner sees one technical confirmation and a short progress/outcome
summary. Internal activation performs only the checks needed to prevent a
broken or misdirected install:

1. target OS and architecture match;
2. release, authority-registry and bootstrapper identities match their exact
   version and SHA-256 pins;
3. the archive contains only regular allowlisted paths and the destination is
   writable without a conflicting managed root;
4. the bootstrapper completes or rolls back the local transaction; and
5. post-install readiness confirms the installed workspace, projection,
   agents, skills and Claude lifecycle bindings.

Native qualification, telemetry volume, model evals, remote authentication,
SharePoint availability, MarkItDown availability and scheduler evidence do not
gate otherwise safe local use. They remain observable follow-ups.

The package does not disable Gatekeeper, remove quarantine, bypass PowerShell
execution policy, suppress SmartScreen/WDAC/AppLocker or request
administrator access. Ed25519 and SHA-256 establish Maestro release integrity;
they do not substitute for Apple Developer ID/notarization or Windows
Authenticode.

The Windows local-beta factory accepts native Authenticode status exactly
`NotSigned`. The macOS local-beta factory requires an exact ad-hoc Mach-O
signature container for the bootstrapper and CLI. When built on macOS, native
`codesign` must independently report `Signature=adhoc`. A valid Developer ID
signature, an invalid signature, an unsigned executable, a partial signature or
disagreement between the structural and native probes is rejected. Ad-hoc
signing prevents Apple Silicon from killing the technical-beta executable; it
does not establish Apple trust, notarization or corporate acceptance.

If native policy blocks the bootstrapper, Claude stops, explains that nothing
was installed or lost and gives the one owner/support action required. It
never improvises a download or security bypass.

## Maintenance enrollment

The ZIP carries the same maintenance implementation as the canonical release.
Claude offers native scheduler enrollment only when the exact platform adapter
reports that enrollment is available and the current work makes it relevant.
An unavailable adapter creates no user-facing choice: Maestro remains usable
and honestly unscheduled. A declined available enrollment likewise does not
remove the maintenance feature from the package or roll back the core install.

When available, enrollment is per-user, reversible and administrator-free. It
may be retried through the installed operating method without reinstalling the
product. The current unavailable Windows native Task Scheduler creation
surface is never advertised as an actionable option.

## Failure and recovery

Activation is idempotent and transactional. An interrupted attempt never marks
the release or workspace ready before required local invariants are committed.
Retry uses the same package and setup authorization. A failed update or setup
restores the last verifiable managed state while preserving user-authored
workspace files.

Existing compatible activation resumes or reports `already_ready`. A version
or managed-root conflict routes to the canonical update/recovery method rather
than overwriting an unknown installation. Optional capability failures are
reported once as follow-ups and never converted into a full-install failure.

## Factory and acceptance evidence

The release factory must prove:

- deterministic byte-for-byte ZIP output for identical inputs;
- exact closed-release artifact closure, one executable target bootstrapper
  seed and executable mode preservation;
- absence of symlinks, traversal, source code, development harnesses, secrets
  and user/client data;
- exact release, registry and bootstrapper pins plus detached-signature
  verification;
- a package-parity test derived from the canonical release manifest;
- seeded guidance that requires confirmation before internal activation and
  never exposes a user-run command;
- failure injection for platform mismatch, tampering, conflicting managed
  roots, interrupted activation and failed readiness with bounded rollback;
- successful clean extraction and attended activation on real Windows amd64
  and macOS arm64 devices; and
- a second Claude session that observes Session Start, managed skills, native
  agents, continuity state and contextual next actions.

Build validation, local tests, attended device activation, native signing,
publication and pilot readiness remain separate evidence classes. A portable
local-beta ZIP is never called production-signed or corporate-approved without
those independent gates.
