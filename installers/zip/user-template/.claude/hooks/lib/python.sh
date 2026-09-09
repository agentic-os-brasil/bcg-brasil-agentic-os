# shellcheck shell=bash
# Resolve a Python 3 interpreter for the Maestro hooks.
#
# Every hook that needs to parse JSON used to test `command -v python3` and
# nothing else. On Windows that is the wrong name: the python.org installer
# ships `python.exe` and the `py` launcher and creates no `python3` at all, so
# a machine with Python installed was indistinguishable from a machine with
# none — and each hook exited 0 in silence, taking memory injection, routing
# and case isolation with it.
#
# Usage:
#
#   . "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh"
#   if MAESTRO_PY=$(maestro_python); then
#     out=$(printf '%s' "$payload" | $MAESTRO_PY -c "$script")
#   fi
#
# The variable is used UNQUOTED on purpose: the resolved value may be `py -3`,
# two words. It is always one of three fixed literals — never a path — so word
# splitting is safe here and quoting would break the launcher form.
#
# Prints the interpreter command and returns 0, or prints nothing and returns 1
# when no Python 3 is available. The answer is memoised in MAESTRO_PY so a hook
# that resolves once does not probe again.
#
# bash 3.2 compatible: stock macOS ships 3.2, and several hooks signal "block"
# with exit 2, which is also what bash returns on a syntax error.

maestro_python() {
  if [ -n "${MAESTRO_PY:-}" ]; then
    printf '%s' "$MAESTRO_PY"
    return 0
  fi

  # python3 first. The name asserts the major version, so it needs no probe —
  # and this is the only branch that costs nothing on a healthy Linux or macOS
  # box, which is where the hooks run most often.
  if command -v python3 >/dev/null 2>&1; then
    MAESTRO_PY="python3"
    printf '%s' "$MAESTRO_PY"
    return 0
  fi

  # `python` has to be probed: on an older macOS it is Python 2, which would
  # fail on the f-strings and encoding arguments the hook scripts use — and
  # fail in a way 2>/dev/null hides, which is the failure mode this whole file
  # exists to end.
  if command -v python >/dev/null 2>&1 \
     && python -c 'import sys; sys.exit(0 if sys.version_info[0] == 3 else 1)' >/dev/null 2>&1; then
    MAESTRO_PY="python"
    printf '%s' "$MAESTRO_PY"
    return 0
  fi

  # The Windows launcher picks the version itself. Probed the same way, because
  # `py` can be present with no 3.x installed.
  if command -v py >/dev/null 2>&1 \
     && py -3 -c 'import sys; sys.exit(0)' >/dev/null 2>&1; then
    MAESTRO_PY="py -3"
    printf '%s' "$MAESTRO_PY"
    return 0
  fi

  return 1
}
