"""Authenticated DOCX-only bridge. Input/output are bounded bytes on stdio."""
import base64
import errno
import io
import json
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
request = json.loads(sys.stdin.buffer.read((70 << 20) + 1))
source = base64.b64decode(request["source"], validate=True)
assert len(source) <= request["max_input"] <= 50 << 20
cache = Path(os.environ["XDG_CACHE_HOME"])
models = Path(os.environ["HF_HOME"])
assert not any(cache.rglob("*")) and not any(models.rglob("*"))
from docling.datamodel.base_models import ConversionStatus, DocumentStream, InputFormat
from docling.datamodel.backend_options import MsWordBackendOptions
from docling.datamodel.pipeline_options import ConvertPipelineOptions
from docling.document_converter import DocumentConverter, WordFormatOption
from docling.pipeline.simple_pipeline import SimplePipeline
options = ConvertPipelineOptions(document_timeout=30, enable_remote_services=False,
    allow_external_plugins=False, artifacts_path=models, do_picture_classification=False,
    do_picture_description=False, do_chart_extraction=False)
backend = MsWordBackendOptions(render_chart_images=False, max_total_bytes=request["max_expanded"],
    max_file_bytes=request["max_expanded"], max_member_count=4096)
converter = DocumentConverter(allowed_formats=[InputFormat.DOCX], format_options={
    InputFormat.DOCX: WordFormatOption(pipeline_cls=SimplePipeline,
        pipeline_options=options, backend_options=backend)}, show_progress=False)
result = converter.convert(DocumentStream(name="source.docx", stream=io.BytesIO(source)),
    max_file_size=request["max_input"], raises_on_error=True)
if result.status != ConversionStatus.SUCCESS or result.errors:
    raise RuntimeError("conversion_incomplete")
markdown = result.document.export_to_markdown().encode("utf-8")
document = json.dumps(result.document.export_to_dict(), ensure_ascii=False, sort_keys=True).encode("utf-8")
if len(markdown) + len(document) > request["max_output"]:
    print(json.dumps({"refusal": "output_limit"}))
    sys.exit(0)
assert not any(cache.rglob("*")) and not any(models.rglob("*"))
print(json.dumps({"markdown": base64.b64encode(markdown).decode("ascii"),
    "document": base64.b64encode(document).decode("ascii"), "network_denied": True}))
