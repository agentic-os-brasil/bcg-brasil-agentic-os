#!/usr/bin/env bash
# Maestro SessionEnd dream marker — writes a timestamped marker to
# data/memory/.dream-requested so the next session can detect that a
# dreaming cycle is due. Fail-open: never blocks Claude.
#
# The marker is consumed by the dream-memory skill on the next SessionStart
# when adapters are available. It is idempotent: multiple stops in the
# same session update the timestamp without creating duplicate requests.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
MEMORY_DIR="$DATA_DIR/memory"
MARKER="$MEMORY_DIR/.dream-requested"
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# If memory dir does not exist, nothing to do.
[ -d "$MEMORY_DIR" ] || exit 0

printf '%s\n' "$TS" > "$MARKER" 2>/dev/null

exit 0
