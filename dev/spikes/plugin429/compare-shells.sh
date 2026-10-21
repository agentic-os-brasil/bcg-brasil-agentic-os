#!/bin/bash
# Attended Windows comparison only; stdin and native output pass through.
case "$1" in session-start|guard|delegation) ;; *) exit 2 ;; esac
printf 'MAESTRO_SPIKE_HANDLER=bash/%s\n' "$1" >&2
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || exit 2
exec "$script_dir/maestro-spike-hook.exe" "$1"
