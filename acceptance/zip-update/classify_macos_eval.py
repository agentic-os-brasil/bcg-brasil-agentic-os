#!/usr/bin/env python3
"""Fail-closed classification of the complete macOS release evaluator transcript.

This classifies local evidence only. It cannot qualify Windows or a release.
"""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import sys

FOREIGN_SKIP = "native Windows drive-letter/junction parity requires dedicated PowerShell CI"
FOREIGN_CHECK_ID = "windows-native-path-parity"
REQUIRED_NATIVE_PASSES = {
    "cross-case write blocked through an in-case filesystem alias",
    "cross-case write blocked through an alias outside the cases tree",
    "same-case write allowed (POSIX paths)",
}
FOOTER = "All local evaluator checks green. Native qualification and distribution remain separate."
ANSI_SGR = re.compile(r"\x1b\[[0-9;]*m")
MAX_LOG_BYTES = 16 * 1024 * 1024


def classify_log(text, eval_exit_code):
    if eval_exit_code != 0:
        raise ValueError("evaluator process did not exit zero")
    lines = ANSI_SGR.sub("", text).splitlines()
    if not lines or lines[0] != "Maestro release eval":
        raise ValueError("missing evaluator header")
    summary_indices = [i for i, line in enumerate(lines) if line.startswith("Summary:")]
    if len(summary_indices) != 1:
        raise ValueError("expected exactly one summary")
    summary_index = summary_indices[0]
    match = re.fullmatch(r"Summary:\s+(\d+) pass\s+(\d+) fail\s+(\d+) skip", lines[summary_index])
    if not match:
        raise ValueError("malformed summary")
    expected = tuple(int(value) for value in match.groups())
    phases = []
    records = []
    for index, line in enumerate(lines):
        phase = re.fullmatch(r"Phase (\d+) — .+", line)
        if phase:
            if index >= summary_index:
                raise ValueError("phase appears after summary")
            phases.append(int(phase.group(1)))
        record = re.fullmatch(r"  (PASS|FAIL|SKIP)  (.+)", line)
        if record:
            if index >= summary_index:
                raise ValueError("check appears after summary")
            records.append(record.groups())
        elif re.match(r"\s*(PASS|FAIL|SKIP)\b", line):
            raise ValueError("malformed check record")
    if phases != list(range(1, 26)):
        raise ValueError("missing, duplicated or out-of-order evaluator phase")
    counts = Counter(status for status, _ in records)
    actual = tuple(counts[status] for status in ("PASS", "FAIL", "SKIP"))
    if expected != actual or actual[0] == 0:
        raise ValueError("summary does not match check records")
    if actual[1] != 0:
        raise ValueError("evaluator contains failures")
    skips = [message for status, message in records if status == "SKIP"]
    if skips != [FOREIGN_SKIP]:
        raise ValueError("only the exact deferred Windows-native check is permitted")
    passed_messages = [message for status, message in records if status == "PASS"]
    if any(passed_messages.count(required) != 1 for required in REQUIRED_NATIVE_PASSES):
        raise ValueError("required native Mac alias/positive-control checks did not pass exactly once")
    footers = [i for i, line in enumerate(lines) if line == FOOTER]
    if len(footers) != 1 or footers[0] <= summary_index:
        raise ValueError("missing or invalid terminal completion marker")
    # The native wrapper supplies the process exit status separately. External
    # diagnostic harnesses may append one explicit marker; contradictions fail.
    terminal = [line for line in lines[summary_index + 1:] if line.strip()]
    if not terminal or terminal[0] != FOOTER:
        raise ValueError("incomplete or inconsistent summary trailer")
    extras = terminal[1:]
    if extras and extras[0].startswith("scratch kept: "):
        extras = extras[1:]
    if extras not in ([], ["EVALUATOR_EXIT_CODE=0"]):
        raise ValueError("unexpected or contradictory completion trailer")
    return {
        "qualification_scope": "macos-only",
        "release_qualified": False,
        "verdict": "PASS_MACOS_ONLY",
        "checks": {
            "passed": actual[0], "failed": actual[1], "skipped": actual[2],
            "platform_native_path_exercised": True,
        },
        "deferred_checks": [{
            "id": FOREIGN_CHECK_ID, "reason": FOREIGN_SKIP,
            "required_on": "windows", "status": "deferred",
        }],
        "evaluator_log_sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", type=Path, required=True)
    parser.add_argument("--eval-exit-code", type=int, required=True)
    args = parser.parse_args()
    try:
        with args.log.open("rb") as stream:
            raw = stream.read(MAX_LOG_BYTES + 1)
        if len(raw) > MAX_LOG_BYTES:
            raise ValueError("evaluator log exceeds bounded input size")
        result = classify_log(raw.decode("utf-8"), args.eval_exit_code)
    except (OSError, UnicodeError, ValueError) as error:
        print("macOS qualification refused: " + str(error), file=sys.stderr)
        return 1
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
