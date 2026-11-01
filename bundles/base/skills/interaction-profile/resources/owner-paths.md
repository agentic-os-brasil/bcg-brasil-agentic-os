# Admitted owner-state paths

Before owner-state file operations, use the current canonical runtime-path
response or invoke the verified managed helper:

```text
maestro-runtime home paths --product-root <managed-product-root>
```

The local installer may bind `--user-home`; do not invent another owner root.
The response must be admitted (`writable: true`) and current. Use its `brain_root`,
`metadata_root` and `owner_scope_root`; product assets remain under `product_root`.
An external HOME uses HOME/brain for the owner brain and HOME/state for canonical
DataRoot APIs. Metadata under HOME/brain/.maestro is a distinct surface. Do not
prefix every DataRoot or workspace-local path with the owner brain.

In these methods, owner paths such as `brain/owner/identity.json`,
`brain/accounts/.active`, `brain/tasks/` and `brain/memory/` are logical paths.
Replace that owner `brain/` prefix with the admitted `brain_root` before calling
Read/Edit/Write or a shell command. It is not relative to cwd or the managed
product. A case/workspace-local `brain/` explicitly selected by that method stays
relative to its confirmed case/workspace; never redirect it to the owner's root.
Do not assume a variable exported by a hook persists in the client process.

Use the tools' public `--product-root`/`--user-home` binding when needed; they
consume the same canonical helper. Never parse locator/origin JSON yourself or
turn a read into bootstrap, migration or repair. Unknown, pending, revoked,
conflicting or unverified authority stops state effects before logging/scaffold.
A missing context marker is not evidence of a healthy or initialized state.

`home init` is an explicit local setup action after the owner chooses a new
installation. It uses fresh-only initialization and does not import an old ZIP.
Existing state is preserved. If legacy content is present or a migrated origin
is unverified, use the diagnostic/setup flow and keep migration qualification
open; never create a replacement brain in the product to make the check pass.

If this client adapter has no canonical path capability or verified helper,
report the state-dependent operation unavailable and continue useful conversation
without those effects. Use the standard interaction profile until its admitted
profile files can be read. A method's presence is not runtime qualification.
