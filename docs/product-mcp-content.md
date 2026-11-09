# Authenticated local format matrix

Use this guide when selecting a local document or diagnosing per-format availability.
The existing content/knowledge operations, exact review and operator fallback are
in the [document knowledge guide](product-document-knowledge.md).

## Closed policy and truthful coverage

Query `content.status` before conversion. Each entry reports its extension,
intended route/parser/fidelity and actual verified availability. A policy name or
library import alone proves no installed capability. Only the exact authenticated
current bridge, 51 pinned inputs, parser notices, lock and complete installed
inventory can make a route ready. Old, unknown, altered or unseeded profiles keep
all routes unavailable without blocking unrelated work. Status installs nothing.

| Extensions | Owning route | Fidelity and limits |
| --- | --- | --- |
| DOCX, CSV, HTML/HTM, Markdown, EPUB, XLSX | Native local Docling parsers | Structured output, with closed source validation and no external-resource fetch. |
| TXT, generic JSON, generic XML | `maestro_literal_v1` | Textual literal UTF-8; preserve whitespace, attributes and order. XML refuses DTD/entities; no domain-schema inference. |
| XLS | `maestro_biff_v1` / pinned xlrd | Textual cells, stored formula caches, workbook-epoch ISO dates and explicit empty cells. |

Generic JSON stays literal by default. Only an actual DoclingDocument validated
inside the authenticated runtime uses native `docling.json`; a claimed schema
header is insufficient. XLSX keeps cached values and declares that formulas are
not evaluated. Empty workbooks or absent formula caches can be partial and publish
no usable artifact. Macros, chart/image rendering, Office helpers, model downloads
and remote fallback remain unavailable. PDF/OCR/images, PPTX and other platforms
need separate admission and evidence.

## Source and runtime boundaries

Core chooses the parser; the model supplies only the existing relative source
path. Source no-follow/single-link admission, actual registered workspace binding,
compressed/expanded budgets and fixed output limits apply before publication.
XLSX preflight validates the supported OPC workbook/relationship selection and
sparse actual coordinates, merged/hyperlink expansion and workbook-wide geometry
before authentication; a clipped or partially interpreted table is not complete
conversion. HTML/Markdown/EPUB cannot fetch local or remote resources.

The assembler consumes already acquired exact inputs; ordinary status/convert
never downloads, installs, executes an ambient Python or modifies the source.
The six additional parser/transitive inputs and their notices are authenticated
alongside the existing pack. The retained xlrd CLI is inert, non-executable
inventory outside PATH. Pinned fixture-generation tooling is test-only and does
not become a production dependency.

## Proving the matrix

Independent committed sources and literal expected facts cover quoting/newlines,
leading zeros, DOCX tables, HTML/Markdown no-fetch bait, EPUB spine order, spreadsheet
coordinates/merged spans/date/cache values and literal TXT/JSON/XML. The product
matrix converts each source through SDK and the rebuilt compiled server, checks
actual Markdown and structured output, performs exact reviewed create/append,
restarts and reads the same scoped page without duplication. Validated JSON export
comes from each transport's own fresh public DOCX conversion.

The existing authenticated runtime CI job acquires the exact 51 official inputs,
then runs the original document journey and all eleven-format journeys against
one retained authenticated profile, followed serially by the owning empty/cache
workbook and XLSX native-limit proofs. Exact parent/transport/case PASS and zero
skip/fail are mandatory. It retains pure bounded stdout JSONL with diagnostics
separate; this is not an installation or client-profile qualification.

Source reuse and static allowlists are not fresh native evidence. Record current
source, fixture/input/authority hashes, actual case outcomes and artifacts before
claiming matrix delivery. Four source reviews, affected hosted full/product gates
and actual native matrix execution precede merge. Installed client, production
trust and colleague rollout remain separate gates.
