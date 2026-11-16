# Product method discovery

The product packages 92 canonical methods, including the 27 pinned engineering methods. They remain instructions for the agent and user. Selecting a method grants no tool access, review approval, provider credentials or installation authority.

Use the installed product's ordinary `maestro_query` tool:

1. Query `methods.list` with `limit` from 1 through 32 (default 16). Continue from the returned `next_offset` while `has_more` is true. Each method carries its exact ID, immutable resource inventory, hashes and explicit-user-only flag. Shared policy, catalogue, dependency declarations and upstream provenance appear in `shared_resources`.
2. Query `methods.plan` with the exact selected `method_id`. The plan reports registered operations, expected effects and output kinds. Conditional effects carry a named capability and an explicit condition; they are not part of the unconditional effect list. Required dependencies without admission evidence make the plan `requirements_unavailable`, with a concrete next action. `requirements_available` means the declared prerequisites are represented; execution remains `not_executed`. Explicit-user-only methods still require the actual user's selection.
3. Query `methods.read` with an exact returned `resource_uri`. Read at most 4096 bytes per page (minimum 4), advancing from `next_offset`. Pages preserve UTF-8 boundaries and carry the complete resource's byte count and SHA-256. Reconstruct the bytes before interpreting a long instruction. The server never resolves the URI as a filesystem path.

For example, select `execution-continuity`, inspect its plan and read `maestro://methods/execution-continuity/SKILL.md`. Its product route uses the actual `execution.*` operations to create, start, checkpoint, pause and resume work. The ledger binds existing workspace artifacts; selecting the method does not create or execute those artifacts. A new client reads the current revision and checkpoint, then resumes with a new attempt ID.

The `brain-index` plan uses the existing `brain.compile` owner over an explicit
cohort of 1–24 admitted workspace pages and named absences. Native review binds
the cohort, source revisions, workspace and expected generation. Publication
writes derived navigation into the existing private journal; canonical pages
stay unchanged. `brain.navigation_status`, `brain.search`, `brain.navigate`,
`brain.diagnostics` and `brain.read` provide the current-generation read route.
This is foreground workspace compilation, with `host.trusted_review` separately
required. It neither reads the owner corpus nor grants shell execution, a root
walk, private-memory compilation or standing maintenance authority.

Other recipes can depend on host commands, delegated agents, connectors, provider configuration or owning domain operations. A packaged recipe and its discovered schema do not establish that those capabilities are admitted in the current client. Keep the requested effect pending and follow its named prerequisite. Missing capability never authorizes direct writes into private state or silently selects a legacy filesystem route.

The canonical declaration is [`method-contracts.json`](../adapters/mcp/method-contracts.json). Its loader validates complete, unique method coverage and operation names against the registered schemas. Groups share declarations without creating a second method catalogue. Changes to a recipe's effects or prerequisites must update that declaration and the focused public discovery tests together.

`required_operations` names only operations registered by this source. A named domain dependency can instead declare bounded `fulfillment_operations` for an owning route that has not landed yet. Plans compare those names with the actual registered schemas under the admitted binding and report the exact missing operations. Once all are registered, only that domain dependency becomes available; input review and host, connector, runtime or provider admission remain separate. Content conversion and workspace knowledge operations are registered by the document-knowledge adapter. Their domain requirements can therefore be available while runtime, host review, owner-knowledge or provider requirements remain unavailable until independently admitted. Maintenance lifecycle and wiki navigation still report their exact missing operations. Planning executes none of those capabilities.

Construction contract: [specification 065](../specs/065-product-method-discovery.md). Implementation tracking: [issue 463](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/463). SDK and compiled packet checks establish local protocol behaviour; native-client method invocation remains separate evidence.

## Existing owners and remaining prerequisites

Case-bound methods query `case.setup.status` for the current admitted binding;
selection files and remembered paths are not inputs. `case-canon-ingest` and
`case-decision-log-entry` use actual reviewed authored-brain creation/append or
replacement, with complete readback at the returned revision. Named sources use
`knowledge.collect`/`brain.read`; an actual converted source uses
`content.read` and `knowledge.create`/`knowledge.append` to retain its owning
source and conversion provenance. Ordinary authored synthesis never fabricates
an artifact ID. Interrupted writes retry the exact original request/revision;
a conflict preserves newer work and requires fresh review.

`account-case-setup` exposes its existing first-case registration/brief/optional
checkpoint bridge through `case.setup.*`. Its broader account narrative,
relationship history and multi-case recall/promotion still lack the separate
`domain.account_case_scaffold` owner. Keep that required dependency unavailable;
a saved first brief does not fulfill the broader account promise.

`case-sharepoint-map` can persist its reviewed map through `brain.create` or
`brain.update`, but remote reading and `domain.sharepoint_enrollment` remain
independent prerequisites. The recipe neither implements enrollment nor infers
it from connector tool names. `caseos-connect` resolves its local binding through
`case.setup.status`; authorized native configuration, OAuth and scope-verified
remote reads remain external prerequisites.

caseOS preparation returns a nonpersisted proposal by default. Explicit Save
uses the existing reviewed brain owner, valid scoped frontmatter, actual source
references/revisions and complete readback. Both the conversational and saved
proposal retain `remote_write: not_attempted`; local persistence does not queue
or authorize a later connector publication. `pocock-teach` likewise saves only
on request, using the actual admitted host/artifact teaching workspace for its
HTML lessons and supporting course files. A brain Markdown note cannot satisfy
that different artifact promise. A host without that capability leaves Save
unavailable.

Yoda's dispatch prerequisite is the existing Code `host.subagents` capability,
using the Agent tool with `subagent_type: "yoda"` and the sealed packet, plus
separate senior-review model authority. Desktop's ordinary MCP tools supply no
dispatcher. Plans still leave host/provider/runtime capabilities unattested and
execution `not_executed`; static registration, packaged instructions and these
recipes are not fresh host or model-execution evidence.
