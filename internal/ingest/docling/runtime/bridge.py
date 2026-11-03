"""Authenticated PFMT model-free bridge. Closed routes, bounded bytes on stdio."""
import base64
import errno
import io
import json
import math
import os
from pathlib import Path
import socket
import sys

root = Path(__file__).parent
assert sys.version_info[:3] == (3, 12, 16)
assert sys.flags.isolated and sys.flags.no_site and sys.flags.dont_write_bytecode
assert all(Path(p).is_relative_to(root) for p in sys.path)
sys.path[:] = [str(root / "python/lib/python3.12"),
               str(root / "python/lib/python3.12/lib-dynload"), str(root / "site-packages")]
for family, address in [(socket.AF_INET, ("127.0.0.1", 9)), (socket.AF_INET6, ("::1", 9))]:
    try:
        with socket.socket(family, socket.SOCK_STREAM) as connection:
            connection.settimeout(1)
            connection.connect(address)
    except OSError as error:
        assert error.errno in (errno.EPERM, errno.EACCES)
    else:
        raise RuntimeError("network_not_denied")

def audit(event, args):
    # Office helpers, browser renderers and arbitrary external commands cannot
    # introduce another unauthenticated executable closure through a backend.
    if event in ("subprocess.Popen", "os.system", "os.posix_spawn", "os.exec", "os.spawn"):
        raise PermissionError("external_process_unavailable")
    if event == "urllib.Request":
        raise PermissionError("resource_fetch_unavailable")

sys.addaudithook(audit)
request = json.loads(sys.stdin.buffer.read((70 << 20) + 1))
source = base64.b64decode(request["source"], validate=True)
assert len(source) <= request["max_input"] <= 50 << 20
assert 0 < request["max_output"] <= 100 << 20
extension = request["format"]
selected = request["selected_route"]
cache = Path(os.environ["XDG_CACHE_HOME"])
models = Path(os.environ["HF_HOME"])
assert not any(cache.rglob("*")) and not any(models.rglob("*"))
from docling_core.types.doc import DoclingDocument, TableCell, TableData

if request["phase"] == "classify_json":
    assert extension == ".json"
    try:
        DoclingDocument.model_validate_json(source)
        is_export = True
    except ValueError:
        is_export = False
    assert not any(cache.rglob("*")) and not any(models.rglob("*"))
    print(json.dumps({"docling_export": is_export, "network_denied": True}))
    sys.exit(0)
assert request["phase"] == "convert"

def literal_document():
    text = source.decode("utf-8", errors="strict")
    if extension == ".json":
        json.loads(text, parse_constant=lambda value: (_ for _ in ()).throw(ValueError("json_constant")))
    elif extension == ".xml":
        from defusedxml.ElementTree import fromstring
        fromstring(text, forbid_dtd=True, forbid_entities=True, forbid_external=True)
    elif extension != ".txt":
        raise ValueError("literal_format_invalid")
    document = DoclingDocument(name="source" + extension)
    document.add_code(text=text, orig=text)
    return document

def biff_document():
    import xlrd
    document = DoclingDocument(name="source.xls")
    # Read stored BIFF caches, never evaluate formulas or macros. Render dates
    # as ISO local date/time using the workbook epoch; keep cell coordinates and
    # empty values. Numeric caches use a finite round-trip representation.
    with xlrd.open_workbook(file_contents=source, on_demand=True,
                            formatting_info=False, ragged_rows=True) as workbook:
        if workbook.nsheets > 256:
            raise ValueError("sheet_limit")
        count = 0
        for sheet_index in range(workbook.nsheets):
            sheet = workbook.sheet_by_index(sheet_index)
            count += sheet.nrows * sheet.ncols
            if count > 1000000:
                raise ValueError("cell_limit")
            document.add_heading(text=sheet.name, level=1)
            cells = []
            for row in range(sheet.nrows):
                for column in range(sheet.ncols):
                    cell = sheet.cell(row, column) if column < sheet.row_len(row) else xlrd.sheet.Cell(xlrd.XL_CELL_EMPTY, "")
                    if cell.ctype in (xlrd.XL_CELL_EMPTY, xlrd.XL_CELL_BLANK):
                        text = ""
                    elif cell.ctype == xlrd.XL_CELL_DATE:
                        text = xlrd.xldate_as_datetime(cell.value, workbook.datemode).isoformat()
                    elif cell.ctype == xlrd.XL_CELL_BOOLEAN:
                        text = "true" if cell.value else "false"
                    elif cell.ctype == xlrd.XL_CELL_ERROR:
                        text = xlrd.error_text_from_code[cell.value]
                    elif cell.ctype == xlrd.XL_CELL_NUMBER:
                        if not math.isfinite(cell.value):
                            raise ValueError("nonfinite_cell")
                        text = str(int(cell.value)) if cell.value.is_integer() else repr(cell.value)
                    else:
                        text = str(cell.value)
                    cells.append(TableCell(text=text, start_row_offset_idx=row,
                        end_row_offset_idx=row+1, start_col_offset_idx=column,
                        end_col_offset_idx=column+1, column_header=(row == 0)))
            if sheet.nrows and sheet.ncols:
                document.add_table(data=TableData(num_rows=sheet.nrows,
                    num_cols=sheet.ncols, table_cells=cells))
    return document

def native_document():
    from docling.datamodel.base_models import ConversionStatus, DocumentStream, InputFormat
    from docling.datamodel.backend_options import (CsvBackendOptions, EpubBackendOptions,
        HTMLBackendOptions, MarkdownBackendOptions, MsExcelBackendOptions, MsWordBackendOptions)
    from docling.datamodel.pipeline_options import ConvertPipelineOptions
    from docling.document_converter import (DocumentConverter, CsvFormatOption,
        DoclingJSONFormatOption, EpubFormatOption, ExcelFormatOption, HTMLFormatOption,
        MarkdownFormatOption, WordFormatOption)
    from docling.pipeline.simple_pipeline import SimplePipeline
    options = ConvertPipelineOptions(document_timeout=30, enable_remote_services=False,
        allow_external_plugins=False, artifacts_path=models, do_picture_classification=False,
        do_picture_description=False, do_chart_extraction=False)
    safe = dict(enable_remote_fetch=False, enable_local_fetch=False)
    archive = dict(max_total_bytes=request["max_expanded"],
        max_file_bytes=request["max_expanded"], max_member_count=4096)
    routes = {
        ".docx": (InputFormat.DOCX, WordFormatOption,
            MsWordBackendOptions(render_chart_images=False, **safe, **archive)),
        ".csv": (InputFormat.CSV, CsvFormatOption, CsvBackendOptions(encoding="utf-8", **safe)),
        ".html": (InputFormat.HTML, HTMLFormatOption,
            HTMLBackendOptions(render_page=False, fetch_images=False, **safe)),
        ".htm": (InputFormat.HTML, HTMLFormatOption,
            HTMLBackendOptions(render_page=False, fetch_images=False, **safe)),
        ".epub": (InputFormat.EPUB, EpubFormatOption,
            EpubBackendOptions(fetch_images=False, **safe, **archive)),
        ".md": (InputFormat.MD, MarkdownFormatOption,
            MarkdownBackendOptions(encoding="utf-8", fetch_images=False, **safe)),
        ".xlsx": (InputFormat.XLSX, ExcelFormatOption,
            MsExcelBackendOptions(render_chart_images=False, parse_charts=False, **safe, **archive)),
        ".json": (InputFormat.JSON_DOCLING, DoclingJSONFormatOption, None),
    }
    format_id, option_type, backend = routes[extension]
    if extension == ".json":
        DoclingDocument.model_validate_json(source)
    args = dict(pipeline_cls=SimplePipeline, pipeline_options=options)
    if backend is not None:
        args["backend_options"] = backend
    converter = DocumentConverter(allowed_formats=[format_id],
        format_options={format_id: option_type(**args)}, show_progress=False)
    result = converter.convert(DocumentStream(name="source"+extension, stream=io.BytesIO(source)),
        max_file_size=request["max_input"], raises_on_error=True)
    if result.status != ConversionStatus.SUCCESS or result.errors:
        raise ValueError("conversion_incomplete")
    if extension == ".xlsx":
        from docling_core.types.doc import GroupLabel
        # Native Excel parsing preserves sheet names in groups, but its default
        # Markdown serializer omits those names. Add real heading items inside
        # those same groups, before their existing children; retain native cell,
        # merge and page provenance without rereading or reinterpreting Excel.
        for group in result.document.groups:
            if group.label == GroupLabel.SHEET:
                result.document.add_heading(text=group.name, level=1, parent=group)
                group.children.insert(0, group.children.pop())
    return result.document

try:
    warnings = []
    if selected["route"] == "maestro_literal_v1":
        assert selected["parser"] == "maestro.literal_utf8.v1" and selected["fidelity"] == "textual"
        document = literal_document()
    elif selected["route"] == "maestro_biff_v1":
        assert extension == ".xls" and selected["parser"] == "xlrd.biff" and selected["fidelity"] == "textual"
        document = biff_document()
        warnings.append("xls_cached_values_only")
    else:
        assert selected["route"] == "docling_local" and selected["fidelity"] == "structured"
        document = native_document()
        if extension == ".xlsx":
            warnings.append("xlsx_cached_values_only")
    markdown = document.export_to_markdown().encode("utf-8")
    structured = json.dumps(document.export_to_dict(), ensure_ascii=False, sort_keys=True).encode("utf-8")
except (ValueError, KeyError, UnicodeError):
    print(json.dumps({"refusal": "source_format_invalid"}))
    sys.exit(0)
if len(markdown) + len(structured) > request["max_output"]:
    print(json.dumps({"refusal": "output_limit"}))
    sys.exit(0)
assert not any(cache.rglob("*")) and not any(models.rglob("*"))
print(json.dumps({"markdown": base64.b64encode(markdown).decode("ascii"),
    "document": base64.b64encode(structured).decode("ascii"), "network_denied": True,
    "selected_route": selected, "warnings": warnings}))
