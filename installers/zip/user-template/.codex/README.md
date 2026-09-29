# Bind this installation before normal Codex work

The portable hook template blocks file edits and agent dispatch until attended
setup binds it to the confirmed installation. Shell tools remain subject to the
host's normal approvals so Maestro can perform this explicit setup; the template
is not a shell sandbox or complete isolation barrier.

After the owner confirms the exact installation location, Maestro verifies the
release manifest/helper through the platform setup mechanism and invokes that
specific packaged helper with bind-codex --root CONFIRMED_ABSOLUTE_ROOT. The root
must be canonical, without symlink components. The owner does not edit JSON.
If the host cannot run setup, present the same reviewed native command for an
attended terminal run; never disable hooks, approvals, or project trust to do it.

Binding validates the manifest, VERSION and helper SHA256, rejects symlink config
paths and writes absolute quoted hook commands to .codex/hooks.json. It preserves
unknown configuration and user hooks. Repeating the same binding is idempotent;
modified managed hooks are preserved and require attended reconciliation.

The owner must separately trust the project in Codex's normal trust flow. Binding
does not grant trust or prove host hook invocation. Reopen the project after setup
so the host loads the bound configuration. A nested .codex directory is never
used to locate the Maestro wrapper.

Moving the installation requires attended rebinding. Existing records bound to
another location fail closed in the binder and require review/reconciliation;
never infer the new root from the current working directory or a nearby folder.
Manifest hashes establish integrity only, not organization signing/authenticity.

## Required native hook review

Project trust and hook-definition trust are separate in Codex. After binding, open the native /hooks view, inspect the exact five managed commands and trust their current definitions through the normal host flow. A changed command/hash needs renewed review. Reopen the project and verify observed events before declaring hooks active. Do not write trust records by hand or use hook-trust bypass flags. Until definitions are trusted, Codex may skip them: installed deny handlers are not active enforcement. If attended review is unavailable, report hooks UNAVAILABLE and keep qualification open.
