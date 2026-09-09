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
#   - Outside data/cases/  -> always allow. Errors there are not this hook's
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
#      data/cases/alpha/../beta/leak.md matched the ACTIVE case on its leading
#      segment and was allowed, while the OS resolved `..` and wrote into beta.
#      Paths are now canonicalized lexically (canon_path) before any segment
#      is read.
#   2. Case. Only the drive letter was lowercased, but Windows is
#      case-insensitive: C:\USERS\... is the same file and failed the prefix
#      test, falling through to allow. Comparison happens on a lowercased copy.
#   3. Double separator. data/cases//alpha left an empty segment that never
#      matched, so the case id came out wrong. canon_path collapses them.
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
# Performance: the raw payload is string-tested for "cases" before anything is
# spawned, so ordinary writes cost zero subprocesses. The parse extracts both
# fields in a single python call rather than two.
#
# Input: JSON on stdin matching the Claude Code PreToolUse hook contract.
# Output: exit 0 = allow; exit 2 + reason on stderr = block the tool call.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
DATA_DIR="$PROJECT_DIR/data"
CASES_DIR="$DATA_DIR/cases"
ACTIVE_FILE="$CASES_DIR/.active"
PENDING_FILE="$CASES_DIR/.pending"

HOOK_INPUT=$(cat 2>/dev/null)
[ -z "$HOOK_INPUT" ] && exit 0

block() {
  printf 'Cross-case write blocked. %s\n' "$1" >&2
  exit 2
}

# ---------------------------------------------------------------------------
# Fast path: if the payload never mentions the cases tree, no write it
# describes can land inside a case. Any path reaching data/cases/ — including
# one that gets there through `..` — contains the literal segment.
#
# Matched with bracket expressions rather than a lowercased copy so this stays
# free of subprocesses and free of bash 4. This test only DECIDES WHETHER TO
# LOOK; it is not the defence.
# ---------------------------------------------------------------------------
case "$HOOK_INPUT" in
  *[Cc][Aa][Ss][Ee][Ss]*|*[Cc][Aa][Ss][Ee]~*) ;;
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
if command -v python3 >/dev/null 2>&1; then
  PARSED=$(printf '%s' "$HOOK_INPUT" | PYTHONIOENCODING=utf-8 python3 -c "$PARSE_PY" 2>/dev/null)
fi

# Python's text mode on Windows writes CRLF and command substitution strips only
# the trailing newline, so the CR between the two values survives and turns
# "Write" into "Write\r" — a tool name that matches nothing and silently allows.
PARSED=$(printf '%s' "$PARSED" | tr -d '\r')

if [ -z "$PARSED" ]; then
  # No python3, or the payload did not parse. Narrow before refusing: a call
  # carrying neither path field cannot be a file write, and refusing it would
  # block legitimate work — a TodoWrite whose list mentions `data/cases/` is
  # ordinary in a Portuguese-language workspace, and the settings matcher is an
  # unanchored regex, so `TodoWrite` does reach this hook.
  case "$HOOK_INPUT" in
    *file_path*|*notebook_path*)
      block "The guard could not read this tool call (no python3 on this machine, or an unparsable payload) and the request references the cases tree, so isolation cannot be verified. Ask the owner to make this write themselves, or to have python3 restored on this machine."
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
  block "A file-writing call referencing the cases tree arrived without a readable target path, so isolation cannot be verified. Ask the owner to make this write themselves."
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

CASES_CANON=$(canon_path "$CASES_DIR")

# Scope bound: anything outside data/cases/ is none of this hook's business.
case "$TARGET_CANON" in
  "$CASES_CANON"/*) ;;
  *) exit 0 ;;
esac

REL="${TARGET_CANON#"$CASES_CANON/"}"
TARGET_CASE="${REL%%/*}"

# Sentinels (.active, .pending, ...) live in the cases dir itself, not in a
# case. Writing them is how the owner switches case.
case "$TARGET_CASE" in
  .*) exit 0 ;;
esac

# From here the write is inside a case directory, so it must be shown to be the
# right one. Every remaining exit that is not a positive match is a refusal.
if [ ! -f "$ACTIVE_FILE" ]; then
  block "No active case is recorded ($ACTIVE_FILE is missing), and this write targets case '$TARGET_CASE'. Isolation cannot be verified. Ask the owner which case is active."
fi

ACTIVE_CASE=$(tr -d '[:space:]' < "$ACTIVE_FILE" 2>/dev/null | tr '[:upper:]' '[:lower:]')
if [ -z "$ACTIVE_CASE" ]; then
  block "The active-case marker ($ACTIVE_FILE) is empty or unreadable, and this write targets case '$TARGET_CASE'. Isolation cannot be verified. Ask the owner which case is active."
fi

[ "$TARGET_CASE" = "$ACTIVE_CASE" ] && exit 0

# Bootstrap path: a case being created is named in .pending before it becomes
# active.
if [ -f "$PENDING_FILE" ]; then
  PENDING_CASE=$(tr -d '[:space:]' < "$PENDING_FILE" 2>/dev/null | tr '[:upper:]' '[:lower:]')
  [ -n "$PENDING_CASE" ] && [ "$TARGET_CASE" = "$PENDING_CASE" ] && exit 0
fi

block "Active case: $ACTIVE_CASE. This write targets case: $TARGET_CASE. To switch cases, ask the owner to confirm the switch, then set data/cases/.active to the target case."
