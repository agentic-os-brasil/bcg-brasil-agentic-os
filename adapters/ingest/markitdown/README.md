# MarkItDown adapter

This adapter is a source component of the managed local ingestion runtime
pack. It is not a user-facing Python installation path.

The Go core invokes it with a JSON request on stdin and the fixed argument
`--request-stdin`. The adapter calls only MarkItDown's local conversion API,
with plugins and remote/cloud routes disabled. It writes the derived Markdown
artifact to the path supplied by the core and returns a small JSON quality
receipt on stdout.

The runtime pack builder must pin and hash `requirements.txt`, validate the
Python runtime and package availability on Windows and macOS, and expose a
stable executable command to `bcgos`. The contributor environment must not be
used as the product runtime.
