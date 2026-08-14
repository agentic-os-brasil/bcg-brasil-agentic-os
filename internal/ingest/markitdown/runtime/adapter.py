#!/usr/bin/env python3
"""PYUV on-demand MarkItDown adapter.

Reads one JSON request object from stdin and writes one JSON response object
to stdout, matching the wire contract in
internal/ingest/markitdown/adapter.go. Network access and plugins are always
disabled regardless of the request, matching the local ingestion policy.
"""
import json
import os
import sys


def respond(status, fidelity, fmt, output_bytes, warnings):
    print(json.dumps({
        "status": status,
        "fidelity": fidelity,
        "format": fmt,
        "output_bytes": output_bytes,
        "warnings": warnings,
    }))
    return 0


def main():
    if "--request-stdin" not in sys.argv:
        return respond("blocked", "unknown", "", 0, ["missing --request-stdin flag"])

    try:
        request = json.load(sys.stdin)
    except Exception:
        return respond("blocked", "unknown", "", 0, ["invalid request"])

    source_path = request.get("source_path", "")
    output_path = request.get("output_path", "")
    max_output_bytes = int(request.get("max_output_bytes", 0) or 0)
    fmt = os.path.splitext(source_path)[1].lstrip(".").lower()

    try:
        from markitdown import MarkItDown
    except Exception:
        return respond("unavailable", "unknown", "", 0, ["markitdown is not installed in the on-demand environment"])

    try:
        converter = MarkItDown(enable_plugins=False)
        result = converter.convert(source_path)
        text = result.text_content or ""
    except Exception as exc:
        return respond("degraded", "unknown", fmt, 0, [("markitdown conversion failed: %s" % exc)[:200]])

    encoded = text.encode("utf-8")
    warnings = []
    if max_output_bytes and len(encoded) > max_output_bytes:
        encoded = encoded[:max_output_bytes]
        warnings.append("markitdown output was truncated to the configured limit")

    try:
        with open(output_path, "wb") as handle:
            handle.write(encoded)
    except Exception as exc:
        return respond("degraded", "unknown", fmt, 0, [("failed to write output: %s" % exc)[:200]])

    return respond("usable", "textual", fmt, len(encoded), warnings)


if __name__ == "__main__":
    sys.exit(main())
