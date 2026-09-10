#!/usr/bin/env bash
# block-cross-case-writes.sh — Case isolation PreToolUse hook
# Blocks file-writing tool calls that would write into a case workspace
# other than the currently active case.
#
# Scope: the file-writing tools the runtime exposes — Edit, MultiEdit, Write
# and NotebookEdit. Bash is intentionally excluded — Bash detection via regex
# is structurally unreliable (bypass vectors: python scripts, variable-stored
# commands, heredocs). If Bash enforcement is needed, use a PostToolUse
# file-watcher on `git diff --name-only` instead.
#
# ---------------------------------------------------------------------------
# Fail-closed, narrowly.
#
# The previous contract was fail-open everywhere: a missing python3, a missing
# `.active`, or any parse error allowed the write. For the only technical
# boundary between one client's material and another's, that is the wrong
# default — and the python3 case disabled the control entirely and silently on
# any machine without it, which is the default state of a stock Windows host.
#
# The policy now splits:
#
#   - Outside brain/accounts/ -> always allow. Errors there are not this hook's
#     business and must never block ordinary work.
#   - Inside a case dir    -> allowed only when it can be POSITIVELY shown to
#     target the active (or pending) case. If the marker is missing, empty or
#     unreadable, or the payload cannot be parsed, the write is refused with a
#     message saying how to recover.
#
# Bypasses closed here, each because a payload got through when tried, not
# because it seemed prudent:
#
#   1. `..` traversal. Segments were compared literally, so
#      accounts/alfa/cases/x/../../../beta/cases/y/leak.md matched the ACTIVE
#      pair on its leading segments and was allowed, while the OS resolved the
#      `..` and wrote into another client's case.
#      Paths are now canonicalized lexically (canon_path) before any segment
#      is read.
#   2. Case. Only the drive letter was lowercased, but Windows is
#      case-insensitive: C:\USERS\... is the same file and failed the prefix
#      test, falling through to allow. Comparison happens on a lowercased copy.
#   3. Double separator. accounts//alfa left an empty segment that never
#      matched, so the identity came out wrong. canon_path collapses them.
#   4/5. MultiEdit and NotebookEdit reached the hook and were waved through by
#      a `case` naming only Edit and Write. NotebookEdit also carries its
#      target in `notebook_path`, a field the hook never read.
#   6. The block reason was printed as a JSON object on stdout. The runtime
#      reads stdout only on exit 0, so on the exit-2 path the reason was
#      discarded and the model saw a bare refusal. It goes to stderr now.
#
# Portability: written for bash 3.2, which is what stock macOS ships. No `${x,,}`,
# no negative array indices. That matters more than it looks: bash exits 2 on a
# syntax error, and 2 is this hook's "block" signal — a 4.0-only construct here
# would turn every unparsed write into a refusal on an un-upgraded Mac.
#
# Performance: the raw payload is string-tested for "accounts" before anything is
# spawned, so ordinary writes cost zero subprocesses. The parse extracts both
# fields in a single python call rather than two.
#
# Input: JSON on stdin matching the Claude Code PreToolUse hook contract.
# Output: exit 0 = allow; exit 2 + reason on stderr = block the tool call.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
BRAIN_DIR="$PROJECT_DIR/brain"
ACCOUNTS_DIR="$BRAIN_DIR/accounts"
ACTIVE_FILE="$ACCOUNTS_DIR/.active"
PENDING_FILE="$ACCOUNTS_DIR/.pending"

HOOK_INPUT=$(cat 2>/dev/null)
[ -z "$HOOK_INPUT" ] && exit 0

block() {
  printf 'Cross-case write blocked. %s\n' "$1" >&2
  exit 2
}

# ---------------------------------------------------------------------------
# Fast path: if the payload never mentions the accounts tree, no write it
# describes can land inside a case. Any path reaching brain/accounts/ — including
# one that gets there through `..` — contains the literal segment.
#
# Matched with bracket expressions rather than a lowercased copy so this stays
# free of subprocesses and free of bash 4. This test only DECIDES WHETHER TO
# LOOK; it is not the defence.
# ---------------------------------------------------------------------------
case "$HOOK_INPUT" in
  *[Aa][Cc][Cc][Oo][Uu][Nn][Tt][Ss]*|*[Aa][Cc][Cc][Oo][Uu][Nn]~*) ;;
  *) exit 0 ;;
esac

# ---------------------------------------------------------------------------
# Parse tool name and target path in ONE spawn.
#
# The parser body is a QUOTED heredoc used through plain variable expansion.
# Inside `python3 -c "..."` bash reinterprets the contents, and a comment
# carrying a backtick or a quote has broken this hook before — once into
# `command not found` noise on every write, once by closing the string early so
# the parser returned nothing.
# ---------------------------------------------------------------------------
read -r -d '' PARSE_PY <<'PY'
import sys, json
try:
    d = json.load(sys.stdin)
    inp = d.get('tool_input') or {}
    tool = d.get('tool_name', '')
    # The field is chosen by TOOL, not by first-non-empty: with
    # file_path OR notebook_path, a NotebookEdit carrying a decoy file_path
    # pointed at the active case would pass while its real target
    # (notebook_path) was another case.
    key = 'notebook_path' if tool == 'NotebookEdit' else 'file_path'
    p = inp.get(key)
    print(tool)
    print(p if isinstance(p, str) else '')
except Exception:
    pass
PY

PARSED=""
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null
if maestro_python >/dev/null 2>&1; then
  PARSED=$(printf '%s' "$HOOK_INPUT" | PYTHONIOENCODING=utf-8 maestro_py -c "$PARSE_PY" 2>/dev/null)
fi

# Python's text mode on Windows writes CRLF and command substitution strips only
# the trailing newline, so the CR between the two values survives and turns
# "Write" into "Write\r" — a tool name that matches nothing and silently allows.
PARSED=$(printf '%s' "$PARSED" | tr -d '\r')

if [ -z "$PARSED" ]; then
  # No python3, or the payload did not parse. Narrow before refusing: a call
  # carrying neither path field cannot be a file write, and refusing it would
  # block legitimate work — a TodoWrite whose list mentions `brain/accounts/` is
  # ordinary in a Portuguese-language workspace, and the settings matcher is an
  # unanchored regex, so `TodoWrite` does reach this hook.
  case "$HOOK_INPUT" in
    *file_path*|*notebook_path*)
      block "The guard could not read this tool call (no Python 3 interpreter on this machine, or an unparsable payload) and the request references the accounts tree, so isolation cannot be verified. Ask the owner to make this write themselves, or to have Python 3 installed on this machine."
      ;;
    *)
      exit 0
      ;;
  esac
fi

# ORDER MATTERS: the tool name is filtered BEFORE a path is required.
#
# Inverted, this produced a live regression. The settings matcher is an
# unanchored regex, so `TodoWrite` matches `Write` and arrives here. With no
# file_path the parser prints one line, command substitution eats the trailing
# newline, and a path-first check would refuse a TodoWrite in the name of an
# isolation that was never at risk.
TOOL_NAME="${PARSED%%$'\n'*}"
case "$TOOL_NAME" in
  Edit|MultiEdit|Write|NotebookEdit) ;;
  *) exit 0 ;;
esac

TARGET_PATH="${PARSED#*$'\n'}"
TARGET_PATH="${TARGET_PATH%%$'\n'*}"

if [ -z "$TARGET_PATH" ]; then
  block "A file-writing call referencing the accounts tree arrived without a readable target path, so isolation cannot be verified. Ask the owner to make this write themselves."
fi

# ---------------------------------------------------------------------------
# canon_path — one canonical shape for both sides of every comparison.
#
# Windows hands this hook drive-letter paths ("C:\...\data\cases\x") while
# CLAUDE_PROJECT_DIR and Git Bash may use "/c/..." or forward slashes. An
# earlier version tested only for a leading "/", so a drive-letter path was
# classified as relative, got PROJECT_DIR prepended, and matched no case
# directory — the guard was inactive on Windows while passing on macOS.
#
# Resolution is lexical, never filesystem-backed: the target of a write does
# not exist yet, so realpath has nothing to resolve, and `..` must be collapsed
# on the string. Nothing can climb above the root.
#
# The result is lowercased. On a case-sensitive filesystem that treats two case
# ids differing only in letter case as one; case ids are lowercase slugs by
# construction, and the alternative reopens bypass 2, which was observed rather
# than theorised.
# ---------------------------------------------------------------------------
canon_path() {
  local p="$1" root="" out="" seg oldIFS
  p="${p//\\//}"                      # C:\a\b -> C:/a/b
  case "$p" in
    /[A-Za-z]/*)                      # /c/a/b (MSYS) -> c:/a/b
      root="$(printf '%s' "${p#/}" | cut -c1):"
      p="${p#/?}"
      ;;
    [A-Za-z]:/*)                      # C:/a/b -> c:/a/b
      root="$(printf '%s' "$p" | cut -c1):"
      p="${p#??}"
      ;;
  esac
  oldIFS="$IFS"
  IFS='/'
  set -f                              # a segment holding * or ? must not glob
  set -- $p
  set +f
  IFS="$oldIFS"
  for seg in "$@"; do
    case "$seg" in
      ''|.) ;;                        # collapses // and /./
      ..)   out="${out%/*}" ;;        # cannot climb above the root
      *)    out="$out/$seg" ;;
    esac
  done
  printf '%s%s' "$root" "$out" | tr '[:upper:]' '[:lower:]'
}

TARGET_CANON=$(canon_path "$TARGET_PATH")
case "$TARGET_CANON" in
  /*|[A-Za-z]:/*) ;;                                     # already absolute
  *) TARGET_CANON=$(canon_path "$PROJECT_DIR/$TARGET_PATH") ;;
esac

ACCOUNTS_CANON=$(canon_path "$ACCOUNTS_DIR")

# Scope bound: anything outside brain/accounts/ is none of this hook's business.
case "$TARGET_CANON" in
  "$ACCOUNTS_CANON"/*) ;;
  *) exit 0 ;;
esac

# Under the accounts tree a case lives at <account>/cases/<case>/..., so the
# identity being guarded is the PAIR, not the case id alone. Two clients may
# reasonably name a case the same thing ("diagnostico", "tmo"), and comparing
# only the last segment would let a write into the other client's identically
# named case look like a write into the active one.
REL="${TARGET_CANON#"$ACCOUNTS_CANON/"}"
TARGET_ACCOUNT="${REL%%/*}"

# Sentinels (.active, .pending, ...) sit directly under accounts/, not inside an
# account. Writing them is how the owner switches case.
case "$TARGET_ACCOUNT" in
  .*) exit 0 ;;
esac

REST="${REL#*/}"
# One segment only — the write targets the accounts root, not a case.
[ "$REST" = "$REL" ] && exit 0

# Account-level material (the account brief, for instance) is not case-scoped
# and is not this hook's business. Only <account>/cases/... continues.
case "$REST" in
  cases/*) ;;
  *) exit 0 ;;
esac

CASE_PART="${REST#cases/}"
TARGET_CASE="${CASE_PART%%/*}"

# Sentinels under an account's own cases/ dir.
case "$TARGET_CASE" in
  .*|"") exit 0 ;;
esac

TARGET_ID="$TARGET_ACCOUNT/$TARGET_CASE"

# From here the write is inside a case directory, so it must be shown to be the
# right one. Every remaining exit that is not a positive match is a refusal.
if [ ! -f "$ACTIVE_FILE" ]; then
  block "No active case is recorded ($ACTIVE_FILE is missing), and this write targets '$TARGET_ID'. Isolation cannot be verified. Ask the owner which case is active."
fi

# The marker holds <account>/<case>. Whitespace is stripped rather than trimmed
# so a trailing newline, a stray space or a CRLF from an editor on Windows all
# read the same.
ACTIVE_ID=$(tr -d '[:space:]' < "$ACTIVE_FILE" 2>/dev/null | tr '[:upper:]' '[:lower:]')
if [ -z "$ACTIVE_ID" ]; then
  block "The active-case marker ($ACTIVE_FILE) is empty or unreadable, and this write targets '$TARGET_ID'. Isolation cannot be verified. Ask the owner which case is active."
fi

# A marker carrying a bare case id is the pre-accounts format. It cannot name
# an account, so it cannot authorise a write under one — refuse rather than
# guess which client it meant.
case "$ACTIVE_ID" in
  */*) ;;
  *) block "The active-case marker ($ACTIVE_FILE) still holds the old single-case format '$ACTIVE_ID', which does not name an account, and this write targets '$TARGET_ID'. Isolation cannot be verified. Ask the owner to set the active case as <account>/<case>." ;;
esac

[ "$TARGET_ID" = "$ACTIVE_ID" ] && exit 0

# Bootstrap path: a case being created is named in .pending before it becomes
# active.
if [ -f "$PENDING_FILE" ]; then
  PENDING_ID=$(tr -d '[:space:]' < "$PENDING_FILE" 2>/dev/null | tr '[:upper:]' '[:lower:]')
  [ -n "$PENDING_ID" ] && [ "$TARGET_ID" = "$PENDING_ID" ] && exit 0
fi

block "Active case: $ACTIVE_ID. This write targets: $TARGET_ID. To switch cases, ask the owner to confirm the switch, then set brain/accounts/.active to the target case."
