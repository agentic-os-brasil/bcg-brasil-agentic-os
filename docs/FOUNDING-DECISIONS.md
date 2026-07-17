# Founding decisions

These decisions capture the current agreement. They are deliberately concise and may later be superseded by formal ADRs.

## Accepted

### FD-001 - Professional-only second brain

The OS serves work at BCG Brasil. Personal, family, financial, spiritual and unrelated private-life domains are out of scope.

### FD-002 - Broad audience, progressive capability

The long-term audience includes classic consultants, BCG X, data scientists and engineers. The repository records these personas now but will not build all their workflows before observing real needs.

### FD-003 - CLI-first experience

Installation, initialization, health checks and updates are product capabilities from the first pilot. The working command name is `bcgos`.

### FD-004 - Repository is the factory, releases are the product

Contributors clone the private source repository. Pilot users install versioned artifacts generated from private GitHub Releases and do not use `git pull` to update the OS.

### FD-005 - One user experience, separate update transactions

The user invokes `bcgos update`. Internally, CLI self-update and OS bundle update remain independent, validated and reversible transactions.

### FD-006 - Managed core does not live in client work

The versioned core is installed in a user-level application directory. A workspace receives only minimal metadata and runtime adapters. Local memory, credentials, logs and client content are never bundled or overwritten by updates.

### FD-007 - Go is the preferred CLI implementation

The initial technical direction is a thin Go binary to minimize prerequisites across Windows, macOS and Linux. This remains subject to a short build and corporate-device validation spike.

### FD-008 - Windows-first pilot

Windows is the primary pilot platform. macOS and Linux remain supported build targets. Installation should not require administrator access.

### FD-009 - Security is part of distribution

Release artifacts and manifests must be verified before execution. Checksums alone are not the final trust model. Tokens and secrets must use operating-system credential storage.

## Open

- Exact authentication mechanism for private GitHub Releases.
- Corporate signing, SmartScreen and Gatekeeper requirements.
- Initial runtime adapters and their parity contract.
- Location and governance of shared organizational knowledge.
- Final name and branding of the user-facing CLI.
- Distribution channel after the private pilot.
