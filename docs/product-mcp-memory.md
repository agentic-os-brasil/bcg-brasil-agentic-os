# Product MCP: reviewed memory and lifecycle

The product uses the canonical memory engine for the locally admitted workspace.
Code and Desktop share that state across restarts. Memory stores reviewed
operational text; client documents, credentials and secrets are not authorized
by a capture. Local source tests and native-client qualification are separate.

## Explicit setup and discovery

Use the product server's explicit `init` with the selected product, profile and
workspace. It prepares the fixed capture authority without replacing owner state.
Ordinary `serve`, context and queries never create this authority. Use the same
three binding flags for every local command; see the
[module guide](../adapters/mcp/README.md).

The following `operation` values belong to `maestro_query`:

| Operation | Input and result |
| --- | --- |
| `memory.list` | Optional `offset` and `limit`; metadata, capture identity/revision, eligibility and visibility revision. Limit1–32; `next_offset=-1` ends discovery. |
| `memory.search` | `query` plus optional pagination; at most256 UTF-8 bytes, searches currently eligible text only. |
| `memory.read` | `capture_id`, optional `expected_revision`, `offset` and `max_bytes`; eligible UTF-8 text only. Carry the returned revision across pages. |
| `memory.context` | Optional `offset` and `max_bytes`; bounded canonical generated context. |

Text slices are at most4096 bytes. Use returned offsets rather than splitting
Unicode yourself. Removed text is absent from search, read and generated context.
Legacy manual text remains visible as metadata for cleanup, but cannot enter
ordinary model readback before trusted correction. If legacy ineligible input
could contaminate generated context, the context request refuses explicitly.

## Capture, consolidate and correct

Apply `memory.capture` with `text`. The trusted client form displays the exact
text and authorized use; explicit acceptance permits sealing and persistence.
The model cannot choose timestamps, producer identity, keys or an approval flag.
The maximum new/corrected text is8192 UTF-8 bytes. Receipts contain identity,
revision, source digest and visibility metadata, not private files or keys.

Apply `memory.consolidate` with `cycle=daily` or `weekly` and a `date` in
`YYYY-MM-DD` form. The bundled deterministic synthesizers perform local
consolidation; no remote model call is implied. Daily produces L1; weekly can
produce L2/L3. Lifetime promotion requires evidence of the canonical minimum
L3 generations and trusted review of the exact candidate. Missing prerequisites
do not become approval. A changed candidate/source state requires renewed review.

Apply `memory.correct` with `capture_id`, current `expected_revision` and `text`.
Its form includes the complete current/proposed record. Stale identity, revision,
review or binding refuses without overwriting newer work. After correction,
rebuild relevant consolidation and query context again; the superseded text
cannot return from historical fallback or derived carry-forward.

## Delete and retain

Apply `memory.delete` with `capture_id` and `expected_revision` for exact trusted
record review. Apply `memory.retention` with an RFC3339 cutoff and the current
`expected_visibility_revision` from discovery. The cutoff is exclusive: only
records before it are selected. A zero revision must still be present explicitly.

The retention form shows the cutoff, workspace, affected count and date range.
Its hidden proof binds the exact selected identities/revisions/source digests
and visibility state. Individual bodies can be inspected separately; bulk
retention does not dump every body into a giant form. Changed selection requires
new review. Revocation precedes cleanup, and affected generated memory is
invalidated conservatively across the workspace before reconstruction.

Deletion of the final capture leaves a valid empty view. Interrupted operations
preserve revocation or report unavailable; do not delete control files or copy
old manifests back to make the state appear ready.

## Local operator fallback

Clients without forms use `memory-review` followed by `memory-apply`. Supply the
same closed JSON request through stdin, keep full private review output local,
and use its exact digest. For a synthetic capture, a private request file contains:

```json
{"operation":"memory.capture","text":"Use a short decision log for reviewed operational notes."}
```

Append the normal three binding flags to both commands:

```sh
maestro-mcp memory-review < /private/local/request.json
maestro-mcp memory-apply --review-digest REVIEW_SHA256 --confirm < /private/local/request.json
```

Use the same mutation fields for correction, deletion and retention. The file is
an operator-selected local input, not a new product database. A modified request,
stale state or missing confirmation refuses. Never paste full private review
output into ordinary model chat. Local review commands do not turn an unsupported
native host into qualified evidence.

## Capacity and recovery

The canonical limit is4096 lifetime capture identities, including permanent
denial tombstones. `memory_capacity` is nonretryable for new identities; removal
does not silently reclaim them. Current captures remain readable, correctable
and removable at capacity. Serialized records/journals are checked before
replacement. Visibility uses a required atomic checkpoint; missing or corrupted
authority fails closed rather than selecting older forbidden content.

Other outcomes distinguish revision conflicts, busy consolidation, ineligible
legacy content, unavailable authority/confinement and declined or unsupported
confirmation. Inspect the selected binding and current metadata before retrying.
Hosted CI, actual host forms, Windows admission, provider/model behavior, signing,
publication and pilot acceptance remain separate gates.
