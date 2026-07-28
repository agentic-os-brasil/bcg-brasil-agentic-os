#!/usr/bin/env bash
# Compact, non-blocking session close reminder.
set -uo pipefail
cat >/dev/null 2>&1 || true
printf '%s\n' 'Close-out: record a durable decision if one was made; otherwise leave a checkpoint and next step before ending the session.'
