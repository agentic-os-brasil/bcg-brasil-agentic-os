"""Executable regression tests: actual stdout must fit the cumulative byte cap."""
import importlib.util
import json
import os
import shutil
import zipfile
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
EMITTER = ROOT / "bundles/base/tools/session-memory-emit.py"
spec = importlib.util.spec_from_file_location("session_memory_emit", EMITTER)
emit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(emit)


class SessionContextBudget(unittest.TestCase):
    def test_whole_hook_includes_large_profile_in_total(self):
        with tempfile.TemporaryDirectory() as td:
            base = Path(td)
            with zipfile.ZipFile(ROOT / "dist/Maestro-v0.2.0-macos.zip") as archive:
                archive.extractall(base)
            project = base / "Maestro"
            for binary in (project / "runtime").glob("*/maestro-runtime"):
                binary.chmod(0o700)
            for name in ["session-start-memory-inject.sh", "lib/maestro-runtime.sh"]:
                shutil.copy2(ROOT / "installers/zip/user-template/.claude/hooks" / name, project / ".claude/hooks" / name)
            owner = project / "brain/owner"
            owner.mkdir(parents=True)
            (owner / "identity.json").write_text(json.dumps({"name": "ação" * 20000}), encoding="utf-8")
            result = subprocess.run(["/bin/bash", str(project / ".claude/hooks/session-start-memory-inject.sh")],
                env={**os.environ, "CLAUDE_PROJECT_DIR": str(project)}, capture_output=True, check=True)
            self.assertGreater(len(result.stdout), 0)
            self.assertLessEqual(len(result.stdout), 30000)
            result.stdout.decode("utf-8", errors="strict")

    def test_positive_byte_caps_include_multibyte_truncation_notice(self):
        for budget in [1, 80, 100, 180, 500]:
            output, source, measured = emit.cap("ação útil\n" * 100, budget)
            self.assertLessEqual(len(output.encode("utf-8")), budget)
            self.assertEqual(measured, len(output.encode("utf-8")))
            self.assertGreater(source, measured)

    def test_cumulative_cap_applies_before_stdout_across_invocations(self):
        with tempfile.TemporaryDirectory(prefix="Maestro orçamento ") as td:
            base = Path(td)
            brain = base / "brain"
            (brain / "owner/self").mkdir(parents=True)
            (brain / "memory/recent").mkdir(parents=True)
            (brain / "owner/self/identity.md").write_text("Regra importante.\n" * 100, encoding="utf-8")
            (brain / "memory/recent/2026-09-29.md").write_text("Memória diária.\n" * 100, encoding="utf-8")
            ledger = base / "blocks.txt"
            receipt = base / "context.json"
            common = [sys.executable, str(EMITTER), "--brain", str(brain),
                      "--envelope", str(ledger), "--budgets", "self=900,l1=900",
                      "--cap-total", "1200"]
            first = subprocess.run(common + ["--blocks", "self"], capture_output=True, check=True)
            last = subprocess.run(common + ["--blocks", "l1", "--finalize", "--out", str(receipt)],
                                  capture_output=True, check=True)
            actual = len(first.stdout + last.stdout)
            self.assertLessEqual(actual, 1200)
            evidence = json.loads(receipt.read_text(encoding="utf-8"))
            self.assertEqual(evidence["memory_inject_bytes"], actual)
            self.assertFalse(evidence["total_over_budget"])
            self.assertEqual(evidence["injection_order"], ["self", "l1"])

    def test_exhausted_cap_emits_no_extra_newline(self):
        with tempfile.TemporaryDirectory() as td:
            base = Path(td)
            (base / "brain/owner/self").mkdir(parents=True)
            (base / "brain/owner/self/a.md").write_text("value\n" * 50, encoding="utf-8")
            ledger = base / "blocks.txt"
            ledger.write_text("previous 100 100 100\n", encoding="utf-8")
            result = subprocess.run([sys.executable, str(EMITTER), "--brain", str(base / "brain"),
                                     "--envelope", str(ledger), "--blocks", "self", "--cap-total", "100"],
                                    capture_output=True, check=True)
            self.assertEqual(result.stdout, b"")


if __name__ == "__main__":
    unittest.main()
