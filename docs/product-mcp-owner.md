# Product MCP: owner onboarding and methods

This engineering guide covers canonical owner onboarding in Code and Desktop.
Use a dedicated profile and synthetic content for qualification. Local source
tests do not establish native confirmation privacy or a signed team release.

## Prepare the binding

Build the server as described in [its module guide](../adapters/mcp/README.md).
Select the installed product, profile and workspace locally, then run explicit
`init`. Reuse the same three flags for `serve`, packets and operator commands.

```sh
maestro-mcp init --product-root /installed/product --user-home /owner --workspace /work/project
```

Initialization preserves admitted migrated state. Copied old registry,
interview, refinement and observation controls remain inactive history until
their owning workflows create fresh authority. Ordinary reads do not initialize
or enroll a producer. Keep owner content outside managed product directories.

## Complete the interview

Query `owner.interview` with `track=quick` or `complete` to obtain the canonical
questions. Apply `owner.track` with that track to request a trusted human form.
Decline or missing form support leaves the choice unchanged.

For each answer, apply `owner.propose` with `facet`, an `evidence` summary and
`proposed_body`. This creates a proposal without automatic producer authority.
Query `owner.review` with `proposal_id` for metadata, then apply `owner.apply`
with that ID for trusted full review. The form shows all current/proposed text,
evidence and access policy. Approve only the exact intended content; the tool
returns a metadata receipt.

After the required answers, query `owner.onboarding_review` and apply
`owner.confirm_onboarding` to review and confirm the full profile. Query
`owner.status` for body-free progress. Restart with the same binding and query
status again to verify continuity. Operations are values of `operation` in
`maestro_query` or `maestro_apply`; their schemas reject additional fields.

The host must support form elicitation. Modern MCP uses official SDK multi-round
input requests; older supported sessions use elicitation. Missing approval,
cancellation, an invalid response or transport failure grants no authority.
A changed profile, proposal or track requires fresh review. Concurrent track
selections, including a repeated current value, invalidate an earlier approval.

## Explicit local fallback

For clients without forms, a local operator can perform the same review.
Append the three binding flags to every command below. Full review JSON is
private; keep it local and out of ordinary chat/tool output.

```sh
maestro-mcp owner-track --track quick --confirm
maestro-mcp owner-review --proposal-id PROPOSAL_ID
maestro-mcp owner-apply --proposal-id PROPOSAL_ID --review-digest REVIEW_SHA256 --confirm
maestro-mcp owner-onboarding-review
maestro-mcp owner-confirm-onboarding --review-digest REVIEW_SHA256 --confirm
```

Use the digest returned by the immediately preceding full review. On refusal,
inspect the binding and current review; do not remove authority files or weaken
permissions. An older strict reader may refuse the optional track revision
field; upgrade it while preserving the registry. Embedded NUL in malformed
legacy profile text requires explicit cleanup before review. Valid-text digests
remain compatible.

## Discover methods

Code packets include 92 canonical methods and two operator/resume aliases.
Desktop discovers those methods through 92 MCP prompts and 247 immutable
method/policy/provenance/license resources. The catalog records canonical source
hashes; Code's separate `methods-projection.json` records projection hashes.
Method resources cannot fetch owner-local files. Check
available operations before following instructions: packaging a method does
not establish that every native hook, agent or provider is available.

Generate a new plugin directory or `.mcpb` for each version; both reuse external
owner state. MCPB installs a server and manifest, not Code Skills. Native loading,
human-form privacy, corporate signing, team publication and pilot acceptance
remain separate qualification steps.
