# Explicit local-browser owner review

A local operator may select `serve --owner-review-channel local-browser` at server startup. The default remains native form review or `confirmation_unavailable`. MCP tool arguments cannot enable the channel. An explicit `local-browser` startup selection chooses the browser even when the current request advertises native forms. With no selected channel, native forms remain first. Neither channel silently switches after an error. Native `InputResponses` are rejected whenever the browser is selected, including with native form capabilities still advertised.

The ordinary participant-local Desktop pilot selects the same channel explicitly in both its typed inventory and canonical manifest. Its preparation screen and README disclose the selection before `PREPARAR`. Builders may opt into it only with `mcpb --desktop-pilot --owner-review-channel local-browser`; generic and stable/source packet profiles stay unchanged. An explicit experimental Code pilot may carry the same selection as described below. Rebuild a pilot to adopt the selection. Existing pilots are preserved.

When a live foreground MCP tool request needs exact-record confirmation uses the explicitly selected browser channel, Maestro opens one temporary page in the computer's browser through the operating system opener. The page shows the complete current review presentation and approval schema. The owner reads the full text, selects the initially unchecked exact-record checkbox and consciously approves, or declines. Chat yes, tool permission and Always Allow confer no record approval. Closing a tab, declining, cancellation, expiry or an unavailable browser grants no authority. An unanswered review expires after at most 150 seconds; an MCP cancellation ends it earlier.

This common confirmation seam covers the existing protected owner, onboarding, identity, memory/lifetime, knowledge, authored-brain, navigation, atlas, case-setup and maintenance operations. It does not introduce domain approval endpoints or change their apply rules. After approval, the existing canonical owners still reconstruct and compare the exact reviewed digest/revision/base before applying. Changed state requires a new review. Scheduled maintenance and lifecycle work never acquire interactive authority.

Each request uses only `127.0.0.1` on a random ephemeral port, an unpredictable 256-bit URL nonce and independent 256-bit CSRF secret. Exact Host, loopback peer, URL and POST Origin checks reject foreign origins and DNS rebinding. The page escapes review text, permits no scripts or external assets, denies framing and caching, requires a bounded form body, and consumes one decision only. The listener closes with the request; no daemon, service, standing token or queued approval remains. URLs, secrets and private review contents never enter MCP responses, ordinary diagnostics or telemetry. Local processes and the owner's trusted browser/OS remain within the local operator trust boundary.

This is a local operator review channel, not MCP URL elicitation, native form parity, native lifecycle qualification or proof that a real client has loaded it. Source tests stub the browser opener and exercise real loopback HTTP; actual Mac/Windows client presentation and owner readback remain separately observed evidence.

Native Desktop form qualification remains `unavailable` until an actual client advertises and renders the form. Local-browser review is a separate supported channel and does not close a native-form qualification gate.


## Experimental Code pilot

Use `plugin --code-pilot --owner-review-channel local-browser` only while
explicitly building a participant-local experimental packet. The same flags
are accepted by `native-review` and `native-qualify-and-build`; the qualification
plan discloses the exact Code pilot selection and binds it into its review digest.
The existing native owner/workspace and hook fingerprints remain unchanged.
Native qualification continues to require its independent actual host evidence.

The closed inventory binds the exact owner, workspace, canonical selection digest
and browser channel to the canonical MCP startup arguments. Server startup rejects
an omitted/changed channel or changed binding. Hooks retain their original argument
vectors and cannot open a browser. Native packets keep their existing userConfig
for those vectors; the launcher must provide its already bound selections. Passive
lifecycle packets retain their existing fixed hook selections.

Versioned, independently registry-pinned experiments reuse the existing prepared
engineering runtime identity and expiry. Their exact scripts/ server image,
constructor version, registry and compatible runtime metadata are revalidated.
This does not turn a stable/enrolled/source packet into an experimental pilot or
allow arbitrary registries. Unseeded packets acquire no runtime authority.

Keep this generated packet local: it contains participant paths. Distribute the
generic reviewed kit, then perform local preparation and confirmation. The packet
README and participant preparation review disclose browser selection. Full Code
browser readability, decline/accept/stale behavior and independent saved readback
must still be observed on actual final client bytes before declaring UX qualified.


## Experimental installed Code and Desktop

On a fresh experimental installed carrier, the normal participant manager asks
whether protected foreground MCP changes should use native-first review or the
complete local-browser review. Pressing Enter keeps native-first. The choice is
shown again in the full setup review and applies to both Code and Desktop using
that carrier. It approves no individual write. Choose local-browser when the
client cannot present the complete exact record through native forms.

The original private current record retains this selection through reopening
and supported source updates. The installed launcher reads that authenticated
record; no extension configuration edit, standalone pilot replacement or extra
server flag is needed. Existing carriers retain their selected mode; this flow
does not silently convert an old installation or offer an in-place mode change.

Each protected request still opens its own complete review and needs conscious
approval. Decline, closing, cancellation, expiry or a changed record leaves the
change unapplied. The receiving server independently checks the selected record
before and after review and before publication. If source or carrier state
changes, reconnect through the current supported launcher and obtain a fresh
review. No lock is held throughout the browser wait. Native-first diagnostic
sessions without management carrier metadata preserve their existing behavior;
a raw installed browser flag without an authenticated carrier is refused.

This selection is restricted to explicitly reviewed experimental installed
carriers. It changes neither production platform trust nor standalone pilot
packaging restrictions, native hooks or qualification. Passing source tests or
the installed checker does not establish actual Code/Desktop approval UX.
