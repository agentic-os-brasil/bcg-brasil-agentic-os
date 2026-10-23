# Experimental team distribution contract

This extends the synthetic Code/Desktop experiment with a local synced-folder
delivery mechanism. It is an engineering pilot, not the complete Maestro
product or production release authority. The user authorized construction and
team testing through SharePoint; the concrete library is still required.

## Publisher and participant

The publisher builds a native installer and server per target, signs an exact
typed feed with an independent experimental Ed25519 key, and promotes the feed
after copying immutable artifacts. The installer pins the corresponding public
key at build time; a key from the mutable share is never trusted automatically.
Experimental keys cannot authorize production. Platform signing/notarization
and corporate admission remain separate evidence.

A participant opens the platform launcher from the synced folder. The native
installer verifies the feed and artifact bytes, copies them locally, creates a
fresh synthetic fixture only on first use and generates packages bound to that
participant's fixture. Package generations are immutable. A local marketplace
is promoted last; the updater preserves the previous generation and all state.
Incomplete sync, target mismatch, invalid signature, changed immutable release
or replay blocks promotion. Writes stay under the participant's Developer tree.

The Code registration uses supported CLI commands only when explicitly requested
by the install/update launcher. Opening the Desktop package requests normal
confirmation and is reported as pending, never as a completed host update.
Installed host versions must be read back separately. No profile internals,
corporate policy, quarantine or elevated permissions are changed.

## Evidence and limits

Exercise fresh installation, checkpoint/pause, N to N+1 update, unchanged state,
same-version no-op, damaged/partial payload, wrong signer/target and replay.
Actual SharePoint hydration, clean participant devices, Code tab and Windows
runtime remain unqualified until observed. There are no automatic model calls
or participant sends. Raw participant data stays local.

This materializer refines the earlier proposed direct shared-folder loading:
the share transports signed artifacts; clients execute verified local copies.
The existing production proposal and F0–F4 gates remain open. The binary is
experimental tooling and carries no contributor harness or repository files.
