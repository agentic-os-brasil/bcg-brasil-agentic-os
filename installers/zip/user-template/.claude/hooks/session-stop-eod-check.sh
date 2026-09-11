#!/usr/bin/env bash
# Maestro Stop hook — checks whether today's owner daily page already has a
# closing ("fechamento") entry; if not, writes/refreshes a marker so the next
# SessionStart can offer to close the day. Fail-open: never blocks Claude.
#
# Unlike session-stop-dream.sh (silent, always-run daily light cycle), closing
# the day requires the owner's confirmation, so this hook never writes to the
# daily page itself — it only leaves a marker naming the date(s) that still
# need a fechamento entry. The eod skill deletes the marker once every open
# date has an actual closing entry; if the owner defers, the marker is left
# in place so the reminder is not silently lost.
#
# 2026-09-06 — also writes brain/.maestro/day-brief.json: the full list of
# open dates (not just today) plus a short excerpt of the two most recent
# daily pages and the active objectives. Before this, three different flows
# (SessionStart's own open-day check, start-day, eod's reconstruction) each
# re-read the same daily pages independently in the same session to answer
# overlapping questions — the concrete cost was the same 12 KB daily page
# read three times in one boot. day-brief.json is computed once here, at the
# only point that already knows the answer, and surfaced by
# session-start-memory-inject.sh instead of being rediscovered by rereading.
# It is derived, regenerated every Stop, and never hand-edited — same
# lifecycle as diagnostics.json and tasks.json in brain/.maestro/.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
BRAIN_DIR="$PROJECT_DIR/brain"
DAILY_DIR="$BRAIN_DIR/daily"
MARKER="$BRAIN_DIR/owner/.eod-requested"
MACHINE="$BRAIN_DIR/.maestro"
BRIEF="$MACHINE/day-brief.json"

# If the daily atlas dir does not exist, nothing to track yet.
[ -d "$DAILY_DIR" ] || exit 0
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true
maestro_python >/dev/null 2>&1 || exit 0

mkdir -p "$MACHINE" 2>/dev/null

PYTHONIOENCODING=utf-8 maestro_py - "$DAILY_DIR" "$MARKER" "$BRIEF" "$BRAIN_DIR" <<'PY' 2>/dev/null
import datetime, glob, io, json, os, re, sys

daily_dir, marker_p, brief_p, brain = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
today = datetime.date.today()

def read(path):
    try:
        return io.open(path, encoding="utf-8").read()
    except OSError:
        return None

def has_close(text):
    return bool(re.search(r"(?m)^### .*fechamento", text or ""))

def last_entry(text, label):
    if not text:
        return None
    blocks = re.findall(rf"(?m)^### \d{{2}}:\d{{2}} — {label}\n(.*?)(?=^### |\Z)", text or "", re.S)
    if not blocks:
        return None
    b = blocks[-1].strip()
    return b[:600] + ("..." if len(b) > 600 else "")

# --- open dates: scan backward from today until a closed page is found.
# A date with no page at all is not a gap (nothing happened that day) but
# does not stop the scan either — the eod skill's own reconstruction treats
# a session-less, file-less date the same way. Capped at 30 days so a long
# gap degrades to "advisory, incomplete" instead of an unbounded scan.
open_dates = []
d = today
scanned = 0
while scanned < 30:
    page = os.path.join(daily_dir, f"{d.isoformat()}.md")
    text = read(page)
    if text is not None and has_close(text):
        break
    if text is not None or d == today:
        open_dates.append(d.isoformat())
    d -= datetime.timedelta(days=1)
    scanned += 1
open_dates.reverse()

# --- last 2 daily pages strictly before today, with short excerpts ---
pages = sorted(glob.glob(os.path.join(daily_dir, "????-??-??.md")), reverse=True)
pages = [p for p in pages if os.path.basename(p) != f"{today.isoformat()}.md"][:2]
last_pages = []
for p in pages:
    text = read(p)
    date = os.path.basename(p)[:-3]
    last_pages.append({
        "date": date,
        "path": "brain/daily/" + os.path.basename(p),
        "briefing_last": last_entry(text, "briefing"),
        "fechamento_last": last_entry(text, "fechamento"),
    })

# --- active objectives (name only, from the heading line) ---
objectives = []
obj_text = read(os.path.join(brain, "development", "objectives.md"))
if obj_text:
    for m in re.finditer(r"(?m)^### \d+\. (.+)$", obj_text):
        name = m.group(1).strip()
        if name and name != "<objetivo>":
            objectives.append(name)

payload = {
    "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
    "today": today.isoformat(),
    "eod_open_dates": open_dates,
    "last_daily_pages": last_pages,
    "objectives_active": objectives,
}
with open(brief_p, "w", encoding="utf-8", newline="\n") as fh:
    json.dump(payload, fh, ensure_ascii=False, indent=2)
    fh.write("\n")

# Trigger marker consumed by session-start-memory-inject.sh: the real content
# lives in day-brief.json now, this only flips the block on/off. Comma-joined
# dates kept here too so a session-start running before this format existed
# (or without a resolvable interpreter at read time) still gets a usable, if plainer, message.
if open_dates:
    with open(marker_p, "w", encoding="utf-8", newline="\n") as fh:
        fh.write(",".join(open_dates) + "\n")
else:
    try:
        os.remove(marker_p)
    except OSError:
        pass
PY

exit 0
