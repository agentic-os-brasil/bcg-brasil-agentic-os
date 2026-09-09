# shellcheck shell=bash
# Resolve and run a Python 3 interpreter for the Maestro hooks.
#
# Every hook that parses JSON used to test `command -v python3` and nothing
# else. On Windows that is the wrong name: the python.org installer ships
# python.exe and the `py` launcher and creates no python3, so a machine with
# Python installed was indistinguishable from a machine with none — and each
# hook exited 0 in silence, taking memory injection, context routing and case
# isolation with it.
#
# Two entry points:
#
#   maestro_python   resolves an interpreter, memoises it, prints a label for
#                    diagnostics, returns 0 on success and 1 when there is none.
#   maestro_py       runs the resolved interpreter:
#                      out=$(printf '%s' "$payload" | maestro_py -c "$script")
#                      out=$(maestro_py - "$file" <<'PY' ... PY)
#
# Callers use maestro_py and never interpolate the interpreter themselves. The
# resolved value can be a two-word launcher (`py -3`) or an absolute path that
# contains spaces — `C:\Users\Firstname Lastname\...` is the normal shape on a
# Windows machine — and no single expansion is correct for both. The function
# holds the executable and its optional first argument separately, so quoting
# is right in every case and no call site has to know which form it got.
#
# bash 3.2 compatible: stock macOS ships 3.2, and several hooks signal "block"
# with exit 2, which is also what bash returns on a syntax error.

# Where a Maestro-provisioned interpreter is recorded. Plain text, one line,
# deliberately not JSON: this is the file that says where the JSON parser is,
# so needing the parser to read it would be circular — and the circularity
# would only surface on the machines that depend on it.
MAESTRO_PY_RECORD="${MAESTRO_PY_RECORD:-${CLAUDE_PROJECT_DIR:-.}/data/.maestro-python}"

# Set by maestro_python: the executable, and an optional first argument.
MAESTRO_PY_CMD="${MAESTRO_PY_CMD:-}"
MAESTRO_PY_ARG="${MAESTRO_PY_ARG:-}"

_maestro_py_is_three() {
  "$@" -c 'import sys; sys.exit(0 if sys.version_info[0] == 3 else 1)' >/dev/null 2>&1
}

maestro_python() {
  if [ -n "$MAESTRO_PY_CMD" ]; then
    printf '%s' "$MAESTRO_PY_CMD${MAESTRO_PY_ARG:+ $MAESTRO_PY_ARG}"
    return 0
  fi

  # python3 first. The name asserts the major version, so it needs no probe —
  # and this is the only branch that costs nothing on a healthy Linux or macOS
  # box, which is where the hooks run most often.
  if command -v python3 >/dev/null 2>&1; then
    MAESTRO_PY_CMD="python3"
    MAESTRO_PY_ARG=""
    printf '%s' "$MAESTRO_PY_CMD"
    return 0
  fi

  # `python` has to be probed: on an older macOS it is Python 2, which would
  # fail on the encoding arguments the hook scripts use — and fail in a way
  # 2>/dev/null hides, which is the failure mode this file exists to end.
  if command -v python >/dev/null 2>&1 && _maestro_py_is_three python; then
    MAESTRO_PY_CMD="python"
    MAESTRO_PY_ARG=""
    printf '%s' "$MAESTRO_PY_CMD"
    return 0
  fi

  # The Windows launcher picks the version itself. Probed the same way, because
  # `py` can be present with no 3.x installed.
  if command -v py >/dev/null 2>&1 && _maestro_py_is_three py -3; then
    MAESTRO_PY_CMD="py"
    MAESTRO_PY_ARG="-3"
    printf '%s' "py -3"
    return 0
  fi

  # Last: an interpreter Maestro provisioned for itself, recorded by
  # maestro-environment-setup. A uv-managed Python lives in uv's own directory
  # and is on PATH under none of the three names above, so without this record
  # provisioning would install an interpreter the hooks could never find.
  #
  # Tried last, not first: a system interpreter is cheaper and cannot go stale,
  # while a recorded path outlives the interpreter it points at — uninstalled,
  # moved, or carried along in a workspace copied to another machine. Verified
  # by running it, so a stale record is skipped rather than trusted.
  local recorded
  if [ -f "$MAESTRO_PY_RECORD" ]; then
    recorded=$(tr -d '\r\n' < "$MAESTRO_PY_RECORD" 2>/dev/null)
    if [ -n "$recorded" ] && [ -x "$recorded" ] && _maestro_py_is_three "$recorded"; then
      MAESTRO_PY_CMD="$recorded"
      MAESTRO_PY_ARG=""
      printf '%s' "$MAESTRO_PY_CMD"
      return 0
    fi
  fi

  return 1
}

maestro_py() {
  if [ -z "$MAESTRO_PY_CMD" ]; then
    maestro_python >/dev/null 2>&1 || return 127
  fi
  if [ -n "$MAESTRO_PY_ARG" ]; then
    "$MAESTRO_PY_CMD" "$MAESTRO_PY_ARG" "$@"
  else
    "$MAESTRO_PY_CMD" "$@"
  fi
}
