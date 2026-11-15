---
name: find-prior-work
description: Recover explicitly requested prior professional material (past decks, documents, project artifacts) that already lives inside the Maestro workspace. Use only when the user explicitly asks to find a past deliverable or artifact. Not for general research, browsing or speculative context gathering.
---

# Find prior work

Resolve [interaction-profile](../interaction-profile/SKILL.md) for tone and depth, without changing scope or authority.

Recover only material explicitly requested by the owner, within the admitted current workspace. No general research, cross-case expansion or speculative collection. Respond briefly and impersonally, with sources and evidence limits.

## Read-only MCP workflow

1. Confirm the selected workspace through `maestro_context`. Extract the named project, account, period and artifact from the request. Ask one question if scope is ambiguous; never guess another workspace or account.
2. Read `maestro_query` with `operation=brain.navigation_status`. A ready generation is an explicitly admitted source cohort. `page_count` counts its source pages. Its `reference` is a **virtual navigation entry**, not an authored page path. `brain.status.prepared=false` only means scaffold pages/templates are missing; it does not mean saved knowledge is empty.
3. Search the ready generation with `brain.search`: supply returned `generation`, a short distinctive `query`, and `purpose`; use `limit=5`, `max_bytes=4096`. Search is a case/accent-insensitive **literal substring**, not natural-language question answering. Start with the project/account name, not the entire question. On zero hits, try at most two shorter terms already present in the request, keeping its named scope.
4. If search still misses, follow the returned navigation entry with `brain.navigate(generation, reference, purpose)`. Follow only links actually returned inside that virtual tree. Resolve relative links within it, preserving percent-encoding. Read at most eight relevant navigation pages and inspect at most five source candidates. This fallback is read-only and stays in the same generation/cohort. Do not invent source paths or treat zero hits as proof that no material exists.
5. For a matching result, use its canonical `page` with `brain.read(page, purpose)` or `knowledge.collect(pages, purpose)`. A virtual reference such as `index.md` belongs to `brain.navigate`; it is **not** a name to guess for `brain.read`. Navigation summaries are pointers, not the full evidence. Read the canonical source before asserting its decision, result or limitations. For paged reads use at most 4096 bytes and follow `next_offset` until `-1`; disclose any unread portion. For every `brain.read` slice, require the same selected connector, confirmed `workspace_id`, `page` and `revision` as the first slice; on drift or an unconfirmed binding, discard the assembled text and report a stale read without compiling or repairing.
6. When relevant to the explicit request, use `memory.search(query, limit=5)` for eligible captures in this workspace. Open only relevant returned `capture_id` values with `memory.read`, bind a returned search revision through `expected_revision` if present, and page at most 4096 bytes. Every continuation MUST send the first read's returned capture revision as `expected_revision` using the same selected connector/workspace and capture; on conflict or drift, discard the assembled text and report a stale read. Do not substitute owner files, historical hidden captures or another case for unavailable eligible memory.
7. Return up to five relevant pointers, with source page/capture, revision, provenance and the facts actually read. Distinguish observation from inference and preserve sample limits. Report the searched cohort and any scope/budget/stale/unavailable limit. Never invent dates, recency, content or absence; ask for a narrower term if the bounded candidates remain ambiguous.

## Boundaries

Use only the admitted Maestro MCP for knowledge and memory reads. Raw filesystem search, `Read` of owner/workspace data, shell/Python and direct installer commands are not this method's retrieval route. Packaged method instructions may be read through the client's supported method mechanism.

Never prepare, compile, ingest, repair, capture memory or change state just to answer a read request. An absent/stale generation is a reported prerequisite, not permission to publish one. No remote SharePoint, OneDrive, Teams, email, browser or alternate connector fallback. A file outside the admitted scope was not searched; acquisition/ingestion is a separate explicitly authorized task through its own method and prerequisites. Do not request credentials or technical setup from the owner.
