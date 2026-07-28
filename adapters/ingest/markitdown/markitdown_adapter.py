#!/usr/bin/env python3
"""Bounded offline MarkItDown adapter used by the managed ingestion pack."""

from __future__ import annotations

import json
import sys
from pathlib import Path


def emit(payload: dict[str, object], code: int = 0) -> int:
    json.dump(payload, sys.stdout, ensure_ascii=False, separators=(",", ":"))
    sys.stdout.write("\n")
    return code


def main() -> int:
    if sys.argv[1:] != ["--request-stdin"]:
        return emit({"status": "blocked", "fidelity": "unknown", "format": "", "output_bytes": 0, "warnings": ["request must be provided through stdin"]}, 2)
    try:
        request = json.load(sys.stdin)
        raw_source = Path(str(request["source_path"]))
        raw_output = Path(str(request["output_path"]))
        if raw_source.is_symlink() or raw_output.is_symlink():
            raise ValueError("symlink input or output")
        source = raw_source.resolve(strict=True)
        output = raw_output.resolve()
        max_output_bytes = int(request["max_output_bytes"])
    except (KeyError, OSError, TypeError, ValueError, json.JSONDecodeError):
        return emit({"status": "blocked", "fidelity": "unknown", "format": "", "output_bytes": 0, "warnings": ["invalid local ingestion request"]}, 2)

    if not source.is_file():
        return emit({"status": "blocked", "fidelity": "unknown", "format": source.suffix.lstrip("."), "output_bytes": 0, "warnings": ["source or output is not a safe regular file"]}, 2)
    if bool(request.get("allow_network")) or bool(request.get("enable_plugins")):
        return emit({"status": "blocked", "fidelity": "unknown", "format": source.suffix.lstrip("."), "output_bytes": 0, "warnings": ["network and plugins are disabled"]}, 2)

    try:
        from markitdown import MarkItDown
    except Exception:
        return emit({"status": "unavailable", "fidelity": "unknown", "format": source.suffix.lstrip("."), "output_bytes": 0, "warnings": ["MarkItDown runtime is not installed"]}, 3)

    try:
        converter = MarkItDown(enable_plugins=False)
        convert_local = getattr(converter, "convert_local", None)
        if convert_local is None:
            return emit({"status": "unavailable", "fidelity": "unknown", "format": source.suffix.lstrip("."), "output_bytes": 0, "warnings": ["MarkItDown local conversion API is unavailable"]}, 3)
        converted = convert_local(str(source))
        text = getattr(converted, "markdown", None) or getattr(converted, "text_content", "")
        if not isinstance(text, str):
            text = str(text)
        encoded = text.encode("utf-8")
        if len(encoded) > max_output_bytes:
            return emit({"status": "degraded", "fidelity": "textual", "format": source.suffix.lstrip("."), "output_bytes": 0, "warnings": ["converted output exceeds the configured limit"]}, 0)
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_bytes(encoded)
        status = "usable" if len(text.strip()) >= 20 else "partial"
        warnings = [] if text.strip() else ["MarkItDown returned empty output"]
        return emit({"status": status, "fidelity": "textual", "format": source.suffix.lstrip("."), "output_bytes": len(encoded), "warnings": warnings})
    except Exception:
        return emit({"status": "unavailable", "fidelity": "unknown", "format": source.suffix.lstrip("."), "output_bytes": 0, "warnings": ["MarkItDown conversion failed"]}, 3)


if __name__ == "__main__":
    raise SystemExit(main())
