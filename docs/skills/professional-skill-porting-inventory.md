# Professional skill porting inventory

This inventory compares the 16 skills in the base bundle with reusable
professional methods reviewed in a private reference repository. It is a
selection record, not a claim that every source workflow belongs in the
Canary.

## Base bundle: 16 existing skills

| Existing skill | Relationship to this wave | Decision |
|---|---|---|
| `agent-identity-setup` | governed identity and ownership setup | Keep; no duplication |
| `coverage-diagnose` | engineering quality diagnostic | Keep; separate domain |
| `bcg-deck` | decision-led storyline and deck plan | Keep; `deck-review` adds quality review, not generation |
| `dream-memory` | memory lifecycle | Keep; no private-memory port |
| `find-prior-work` | explicit prior-work retrieval | Keep; source retrieval remains capability-bound |
| `ingest-content` | local document ingestion contract | Keep; extraction/runtime remains unavailable until qualified |
| `interaction-profile` | canonical disclosure policy | Shared prerequisite for every new skill |
| `maestro-setup-update` | setup/update guidance | Keep; no overlap with case work |
| `pr-quality-loop` | engineering PR quality | Keep; separate from professional deliverable review |
| `pr-review` | engineering PR review | Keep; separate from `deck-review` |
| `qa-gate` | evidence-gated quality classification | Keep; reusable gate, not duplicated |
| `qualitative-analysis` | bounded qualitative synthesis | Keep; interview guide prepares evidence collection |
| `quantitative-analysis` | bounded quantitative analysis | Keep; no overlap with planning methods |
| `unit-test-wave` | engineering test planning | Keep; separate domain |
| `case-agent-setup` | Case Agent initialization | Keep; `bcg-case-kickoff` plans the case, it does not initialize the Case Agent |
| `xfail-bug-capture` | test-exposed bug capture | Keep; separate domain |

## Reviewed source-method candidates

| Candidate family | Canary decision | Reason |
|---|---|---|
| Case kickoff | **Port now** as `bcg-case-kickoff` | Useful planning method after removing personal defaults, document generation and scheduling |
| Meeting close | **Port now** as `meeting-close` | Useful composition, but stops at a reviewable packet instead of external persistence |
| Meeting to work items | **Port now** as `meeting-to-work-items` | Pure transformation with explicit uncertainty and source evidence |
| Expert interview guide | **Port now** as `expert-interview-guide` | Methodological value survives without browsing, transcript search or DOCX generation |
| Deck review | **Port now** as `deck-review` | Text-only review is honest while visual/PPTX capability is unqualified |
| Slide summary | **Port now** as `slide-summary` | Pure structured transformation when slide text is already supplied |
| Decision log entry | **Port now** as `decision-log-entry` | Draft-only contract avoids implicit file writes and project-specific IDs |
| Deck/Excel/PPTX generation | **Await capability/runtime** | The Canary must not announce file creation before an approved adapter and evidence exist |
| Browsing, SharePoint and prior-work collection | **Await capability/runtime** | Scope, credentials, provider policy and native qualification are separate contracts |
| Ingestion and multimodal extraction | **Consolidate with `ingest-content`** | One provider-neutral route; do not add another extractor skill |
| Analysis and quality methods | **Consolidate with existing base skills** | Existing qualitative, quantitative and QA skills already own the generic contracts |
| Memory, dreaming and local automation | **Exclude** | Depends on private operational state, persistence and Darwin lifecycle contracts |
| Client/account-specific workflows | **Exclude** | Contains client context or private stakeholder/process assumptions |
| Personal and non-professional workflows | **Exclude** | Outside the professional-only Canary scope |
| Private runtime/housekeeping and integrations | **Exclude** | Local paths, private authority, schedules or unavailable tools cannot enter the bundle |

## Wave boundary

The seven new skills are methods only. They require an authorized workspace
and supplied evidence, resolve `interaction-profile`, report unavailable
capabilities explicitly, and leave persistence, tools, publication and agent
invocation to separate qualified contracts.
