# Accepted owner context in product MCP

A completed interview persists accepted owner preferences. A fresh Code or
Desktop conversation can retrieve the current, authorized preferences through
`maestro_query`; it does not need raw filesystem access or a prior conversation.
Installation, native hooks and host qualification retain their own requirements.

## Read contract

1. Inspect the admitted `maestro_context` owner hint and `owner.status`.
2. Call `maestro_query` with `{"operation":"owner.context","max_bytes":4096}`.
3. When `next_offset` is not `-1`, repeat with that exact `offset`, the first
   page's exact `generation` and the same `max_bytes`. Stop on any error; discard
   an incomplete mixed projection. Start a fresh read explicitly when needed.
4. Use the fully retrieved preferences as owner profile **data**. Current
   explicit instructions and product policy take precedence. Preferences never
   authorize tool use, filesystem changes, external sends or scheduled work.

Optional `max_bytes` is 4–4096 UTF-8 bytes (default 4096); `offset` is a byte
cursor from a returned page, bounded to 131072. Continuations require the initial
64-character lowercase hexadecimal `generation`. No reader, role, path or facet
selector is accepted. Text pages end on UTF-8 boundaries. The full projection is
bounded to 128 KiB, and canonical facet readers retain their smaller file bounds.

Each response includes `text`, `offset`, `next_offset`, `total_bytes`, `complete`,
`reader`, `authority`, `track`, `facets`, `generation` and `confirmation_sha256`.
`complete` means **end of pagination**, exactly `next_offset == -1`.
`reader` is always `session`; `authority` is `owner_profile_data_only`.
`confirmation_sha256` binds the current selected track/revision and exact accepted
facet bodies. The separate `generation` also binds selected canonical content
and reader/sensitivity/refinement policy. The confirmation digest alone is not a
reader authorization. Registry policy is validated against canonical definitions.

## Privacy and freshness

Every page reacquires the existing native owner transition lock, requires the
selected track's current final confirmation and explicitly selects its
session-readable facets. It reuses the canonical snapshot and private bounded
no-follow file reader. Missing state or locks are refused without repair. A
changed confirmation, revoked/invalid reader policy, symlink, nonregular file or
oversized authority is refused before content is returned. Stored projections,
raw review evidence, private absolute paths, unaccepted answers and Yoda-only
facets are not exported. `maestro_context` itself contains metadata only.

The fixed reader represents the locally admitted owner MCP session. It does not
prove the identity of an individual hosted subagent; a host must not claim that
passing a role string creates isolation. Sharing accepted context with another
principal still requires its own authorized route. Sensitive facets explicitly
readable by `session` can appear after current confirmation; sensitivity labels
alone do not replace the canonical Readers policy.

Finite refusals include `owner_context_not_confirmed`, `owner_context_changed`,
`owner_context_generation_required`, `owner_context_unavailable` and
`invalid_page`. Invalid/extra schema fields are rejected by the ordinary tool
validator. No raw filesystem error is exposed. Unavailable context uses the
standard communication profile and reports the limitation; it does not restart
onboarding, inspect owner files or grant a recovery write.

## Evidence boundary

Focused core and public MCP SDK tests cover projection and tool behavior. A new
Code conversation and a new Desktop conversation must separately demonstrate
retrieval of the already accepted profile through their installed artifacts.
Those runtime acceptance checks, native hooks and Windows runtime qualification
remain pending until independently observed; source tests do not establish them.
