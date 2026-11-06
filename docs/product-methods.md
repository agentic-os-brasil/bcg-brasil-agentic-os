# Product method discovery

The product packages 92 canonical methods, including the 27 pinned engineering methods. They remain instructions for the agent and user. Selecting a method grants no tool access, review approval, provider credentials or installation authority.

Use the installed product's ordinary `maestro_query` tool:

1. Query `methods.list` with `limit` from 1 through 32 (default 16). Continue from the returned `next_offset` while `has_more` is true. Each method carries its exact ID, immutable resource inventory, hashes and explicit-user-only flag. Shared policy, catalogue, dependency declarations and upstream provenance appear in `shared_resources`.
2. Query `methods.plan` with the exact selected `method_id`. The plan reports registered operations, expected effects and output kinds. Conditional effects carry a named capability and an explicit condition; they are not part of the unconditional effect list. Required dependencies without admission evidence make the plan `requirements_unavailable`, with a concrete next action. `requirements_available` means the declared prerequisites are represented; execution remains `not_executed`. Explicit-user-only methods still require the actual user's selection.
3. Query `methods.read` with an exact returned `resource_uri`. Read at most 4096 bytes per page (minimum 4), advancing from `next_offset`. Pages preserve UTF-8 boundaries and carry the complete resource's byte count and SHA-256. Reconstruct the bytes before interpreting a long instruction. The server never resolves the URI as a filesystem path.

For example, select `execution-continuity`, inspect its plan and read `maestro://methods/execution-continuity/SKILL.md`. Its product route uses the actual `execution.*` operations to create, start, checkpoint, pause and resume work. The ledger binds existing workspace artifacts; selecting the method does not create or execute those artifacts. A new client reads the current revision and checkpoint, then resumes with a new attempt ID.

Other recipes can depend on host commands, delegated agents, connectors, provider configuration or owning domain operations. A packaged recipe and its discovered schema do not establish that those capabilities are admitted in the current client. Keep the requested effect pending and follow its named prerequisite. Missing capability never authorizes direct writes into private state or silently selects a legacy filesystem route.

The canonical declaration is [`method-contracts.json`](../adapters/mcp/method-contracts.json). Its loader validates complete, unique method coverage and operation names against the registered schemas. Groups share declarations without creating a second method catalogue. Changes to a recipe's effects or prerequisites must update that declaration and the focused public discovery tests together.

Construction contract: [specification 065](../specs/065-product-method-discovery.md). Implementation tracking: [issue 463](https://github.com/agentic-os-brasil/bcg-brasil-agentic-os/issues/463). SDK and compiled packet checks establish local protocol behaviour; native-client method invocation remains separate evidence.
