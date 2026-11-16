"""Authenticated NTXT model-free document bridge. Closed routes, bounded bytes on stdio."""
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
assert os.name == "nt"
sys.path[:] = [str(root / "python/Lib"), str(root / "python/DLLs"),
               str(root / "site-packages")]
# Authenticated wheel extension DLLs stay inside this execution snapshot.
_dll_directories = [os.add_dll_directory(str(root / "python")),
                    os.add_dll_directory(str(root / "python/DLLs"))]
# Pin the app-local C++ runtime before any wheel DLL can use an ambient redist.
import ctypes
from hashlib import sha256
native_runtime = root / "python/msvcp140.dll"
assert native_runtime.is_file()
assert sha256(native_runtime.read_bytes()).hexdigest() == "a4c2229bdc2a2a630acdc095b4d86008e5c3e3bc7773174354f3da4f5beb9cde"
kernel = ctypes.WinDLL("kernel32", use_last_error=True, winmode=0x800)
assert kernel.SetDefaultDllDirectories(0x1000)  # Excludes PATH and current directory.
_msvcp = ctypes.WinDLL(str(native_runtime), winmode=0x100 | 0x800)
kernel.GetModuleFileNameW.argtypes = [ctypes.c_void_p, ctypes.POINTER(ctypes.c_wchar), ctypes.c_uint]
kernel.GetModuleFileNameW.restype = ctypes.c_uint
_loaded_path = ctypes.create_unicode_buffer(32768)
_loaded_length = kernel.GetModuleFileNameW(_msvcp._handle, _loaded_path, len(_loaded_path))
assert 0 < _loaded_length < len(_loaded_path)
assert Path(_loaded_path.value).samefile(native_runtime)

def denied(error):
    return error.errno in (errno.EPERM, errno.EACCES) or getattr(error, "winerror", None) == 10013

# These are kernel denial observations, not a self-reported offline flag.
for family, address in [(socket.AF_INET, ("127.0.0.1", 9)),
                        (socket.AF_INET6, ("::1", 9)),
                        (socket.AF_INET, ("198.51.100.1", 9)),
                        (socket.AF_INET6, ("2001:db8::1", 9))]:
    for kind in (socket.SOCK_STREAM, socket.SOCK_DGRAM):
        try:
            with socket.socket(family, kind) as connection:
                connection.settimeout(1)
                if kind == socket.SOCK_STREAM:
                    connection.connect(address)
                else:
                    connection.sendto(b"synthetic-confinement-probe", address)
        except OSError as error:
            assert denied(error)
        else:
            raise RuntimeError("network_not_denied")
for name, mode in [(os.environ["MAESTRO_CONFINEMENT_SENTINEL"], "rb"),
                   (os.environ["MAESTRO_CONFINEMENT_SENTINEL"], "ab"),
                   (__file__, "ab")]:
    try:
        with open(name, mode):
            pass
    except OSError as error:
        assert denied(error)
    else:
        raise RuntimeError("filesystem_not_confined")
scratch = Path(os.environ["TMP"]) / "synthetic-confinement-probe"
scratch.write_text("synthetic scratch", encoding="utf-8")
scratch.unlink()
import subprocess
try:
    subprocess.run([sys.executable, "-I", "-S", "-c", "raise SystemExit(73)"], check=False)
except OSError as error:
    assert getattr(error, "winerror", None) in (5, 367, 1260)
else:
    raise RuntimeError("child_process_not_denied")

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
        HTMLBackendOptions, MarkdownBackendOptions, MsExcelBackendOptions, MsWordBackendOptions, MsPowerpointBackendOptions, PdfBackendOptions)
    from docling.datamodel.pipeline_options import ConvertPipelineOptions, NativePdfPipelineOptions
    from docling.document_converter import (DocumentConverter, CsvFormatOption,
        DoclingJSONFormatOption, EpubFormatOption, ExcelFormatOption, HTMLFormatOption,
        MarkdownFormatOption, WordFormatOption, PowerpointFormatOption, NativePdfFormatOption)
    from docling.pipeline.simple_pipeline import SimplePipeline
    options = ConvertPipelineOptions(document_timeout=30, enable_remote_services=False,
        allow_external_plugins=False, artifacts_path=models, do_picture_classification=False,
        do_picture_description=False, do_chart_extraction=False)
    safe = dict(enable_remote_fetch=False, enable_local_fetch=False)
    archive = dict(max_total_bytes=request["max_expanded"],
        max_file_bytes=request["max_expanded"], max_member_count=4096)
    if extension == ".pdf":
        import pypdfium2 as pdfium
        from docling.backend.pypdfium2_backend import PyPdfiumDocumentBackend
        try:
            with pdfium.PdfDocument(source) as pdf:
                if pdfium.raw.FPDF_GetSecurityHandlerRevision(pdf.raw) != -1:
                    raise DocumentRefusal("pdf_protected_unavailable")
                if len(pdf) == 0 or len(pdf) > 4096:
                    raise DocumentRefusal("pdf_native_text_unavailable")
                page_count = len(pdf)
                characters = 0
                for index in range(len(pdf)):
                    page = pdf[index]
                    try:
                        # The first profile admits encoded text only. Refuse
                        # image/form-bearing pages instead of silently dropping
                        # scanned regions or nested visual content.
                        if next(page.get_objects(filter=[pdfium.raw.FPDF_PAGEOBJ_IMAGE,
                                pdfium.raw.FPDF_PAGEOBJ_FORM], max_depth=1), None) is not None:
                            raise DocumentRefusal("pdf_native_text_unavailable")
                        text = page.get_textpage()
                        try:
                            count = text.count_chars()
                            if count <= 0:
                                raise DocumentRefusal("pdf_native_text_unavailable")
                            characters += count
                            if characters > request["max_output"]:
                                raise DocumentRefusal("output_limit")
                        finally:
                            text.close()
                    finally:
                        page.close()
        except pdfium.PdfiumError as error:
            if error.err_code == pdfium.raw.FPDF_ERR_PASSWORD:
                raise DocumentRefusal("pdf_protected_unavailable") from error
            raise ValueError("pdf_invalid") from error
        native = NativePdfPipelineOptions(document_timeout=30, enable_remote_services=False,
            allow_external_plugins=False, artifacts_path=models, parser_threads=1,
            generate_page_images=False, generate_picture_images=False,
            do_picture_classification=False, do_picture_description=False, do_chart_extraction=False)
        converter = DocumentConverter(allowed_formats=[InputFormat.PDF],
            format_options={InputFormat.PDF: NativePdfFormatOption(
                backend=PyPdfiumDocumentBackend, pipeline_options=native,
                backend_options=PdfBackendOptions(**safe))}, show_progress=False)
        result = converter.convert(DocumentStream(name="source.pdf", stream=io.BytesIO(source)),
            max_file_size=request["max_input"], raises_on_error=True)
        if result.status != ConversionStatus.SUCCESS or result.errors:
            raise DocumentRefusal("pdf_native_text_unavailable")
        represented = {prov.page_no for item in result.document.texts if item.text.strip() for prov in item.prov}
        if set(result.document.pages) != represented or len(represented) != page_count:
            raise DocumentRefusal("pdf_native_text_unavailable")
        return result.document
    routes = {
        ".pptx": (InputFormat.PPTX, PowerpointFormatOption,
            MsPowerpointBackendOptions(render_chart_images=False, **safe, **archive)),
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

class DocumentRefusal(ValueError):
    def __init__(self, reason):
        self.reason = reason

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
        assert selected["route"] == "docling_local" and selected["fidelity"] == ("textual" if extension == ".pdf" else "structured")
        document = native_document()
        if extension == ".pdf":
            assert selected["parser"] == "docling.pdf_native_text"
            warnings.append("pdf_native_text_only")
        if extension == ".xlsx":
            warnings.append("xlsx_cached_values_only")
    markdown = document.export_to_markdown().encode("utf-8")
    structured = json.dumps(document.export_to_dict(), ensure_ascii=False, sort_keys=True).encode("utf-8")
except DocumentRefusal as error:
    print(json.dumps({"refusal": error.reason}))
    sys.exit(0)
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
