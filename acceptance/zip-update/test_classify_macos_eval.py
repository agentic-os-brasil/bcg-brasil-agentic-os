"""Behavior tests: actual Mac evaluator transcript plus adversarial mutations."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
CLASSIFIER = HERE / "classify_macos_eval.py"
FOREIGN = "native Windows drive-letter/junction parity requires dedicated PowerShell CI"
ALIAS = "cross-case write blocked through an in-case filesystem alias"
OUTSIDE_ALIAS = "cross-case write blocked through an alias outside the cases tree"
FOOTER = "All local evaluator checks green. Native qualification and distribution remain separate."


class MacEvaluationClassificationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.log = (HERE / "fixtures/macos-eval-0.2.0.log").read_text(encoding="utf-8")

    def classify(self, log, rc=0):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "eval.log"
            path.write_text(log, encoding="utf-8")
            return subprocess.run(
                [sys.executable, str(CLASSIFIER), "--log", str(path),
                 "--eval-exit-code", str(rc)], capture_output=True, text=True,
            )

    def test_real_mac_log_qualifies_only_mac_with_exact_deferred_windows_check(self):
        result = self.classify(self.log)
        self.assertEqual(result.returncode, 0, result.stderr)
        receipt = json.loads(result.stdout)
        self.assertEqual(receipt["qualification_scope"], "macos-only")
        self.assertIs(receipt["release_qualified"], False)
        self.assertEqual(receipt["verdict"], "PASS_MACOS_ONLY")
        self.assertEqual(receipt["checks"], {
            "passed": 198, "failed": 0, "skipped": 1,
            "platform_native_path_exercised": True,
        })
        self.assertEqual(receipt["deferred_checks"], [{
            "id": "windows-native-path-parity", "reason": FOREIGN,
            "required_on": "windows", "status": "deferred",
        }])

    def test_ansi_and_crlf_are_supported_without_relaxing_contract(self):
        colored = self.log.replace("  PASS  ", "  \x1b[32mPASS\x1b[0m  ")
        colored = colored.replace("  SKIP  ", "  \x1b[33mSKIP\x1b[0m  ")
        result = self.classify(colored.replace("\n", "\r\n"))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_native_wrapper_log_without_external_exit_marker_is_valid(self):
        result = self.classify(self.log.replace("EVALUATOR_EXIT_CODE=0\n", ""))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_nonzero_process_exit_rejects_green_text(self):
        result = self.classify(self.log, 1)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")

    def test_mutated_logs_fail_closed_without_a_receipt(self):
        mutations = {
            "missing_summary": re.sub(r"^Summary:.*\n", "", self.log, flags=re.M),
            "duplicate_summary": self.log.replace("Summary:", "Summary: 198 pass 0 fail 1 skip\nSummary:", 1),
            "truncated_after_summary": self.log.split(FOOTER)[0],
            "missing_header": self.log.replace("Maestro release eval\n", ""),
            "missing_phase": self.log.replace("Phase 14 — Cross-case guard on the native host\n", ""),
            "missing_first_half": self.log[self.log.index("Phase 14"):],
            "missing_pass_record": self.log.replace("  PASS  ZIP extracted cleanly\n", ""),
            "wrong_pass_count": self.log.replace("198 pass", "199 pass"),
            "summary_hides_failure": self.log.replace("  PASS  ZIP extracted cleanly", "  FAIL  ZIP extracted cleanly"),
            "honest_failure": self.log.replace("  PASS  ZIP extracted cleanly", "  FAIL  ZIP extracted cleanly").replace("198 pass  0 fail", "197 pass  1 fail"),
            "unknown_skip": self.log.replace(FOREIGN, "optional unclassified check"),
            "native_alias_skipped": self.log.replace("  PASS  " + ALIAS, "  SKIP  " + ALIAS).replace("198 pass  0 fail  1 skip", "197 pass  0 fail  2 skip"),
            "additional_foreign_skip": self.log.replace("Summary:", "  SKIP  " + FOREIGN + "\nSummary:").replace("1 skip", "2 skip"),
            "additional_native_skip": self.log.replace("Summary:", "  SKIP  native Mac permissions unavailable\nSummary:").replace("1 skip", "2 skip"),
            "missing_foreign_skip": self.log.replace("  SKIP  " + FOREIGN + "\n", "").replace("1 skip", "0 skip"),
            "missing_alias_pass": self.log.replace("  PASS  " + ALIAS, "  PASS  unrelated alias text"),
            "missing_outside_alias_pass": self.log.replace("  PASS  " + OUTSIDE_ALIAS, "  PASS  unrelated outside alias text"),
            "skip_reason_prefix": self.log.replace(FOREIGN, "prefix " + FOREIGN),
            "skip_reason_suffix": self.log.replace(FOREIGN, FOREIGN + " suffix"),
            "exit_marker_disagrees": self.log.replace("EVALUATOR_EXIT_CODE=0", "EVALUATOR_EXIT_CODE=2"),
            "duplicate_exit_marker": self.log + "EVALUATOR_EXIT_CODE=0\n",
            "late_check": self.log + "  PASS  surprise\n",
            "duplicate_footer": self.log + FOOTER + "\n",
            "summary_only": "Summary: 198 pass 0 fail 1 skip\n" + FOOTER + "\n",
        }
        for name, log in mutations.items():
            with self.subTest(name=name):
                result = self.classify(log)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertEqual(result.stdout, "")



class NativeSmokeWrapperTests(unittest.TestCase):
    """Replay a real evaluator transcript at the slow subprocess boundary.

    The wrapper, classifier, JSON writer and ZIP inspection are real. This is
    test-fixture evidence, not a native release qualification receipt.
    """
    def run_wrapper(self, mutate=lambda value: value, eval_rc=0):
        import shutil
        import zipfile
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repo = root / "repo"
            target = repo / "acceptance/zip-update"
            target.mkdir(parents=True)
            for name in ("native-smoke.sh", "classify_macos_eval.py"):
                shutil.copy2(HERE / name, target / name)
            hooks = repo / "installers/zip/user-template/.claude/hooks/lib"
            hooks.mkdir(parents=True)
            shutil.copy2(HERE.parents[1] / "installers/zip/user-template/.claude/hooks/lib/python.sh", hooks / "python.sh")
            evaluator = repo / "installers/zip/eval-release.sh"
            evaluator.write_text('#!/bin/bash\ncat "$EVAL_FIXTURE_LOG"\nexit "$EVAL_FIXTURE_RC"\n', encoding="utf-8")
            subprocess.run(["git", "init", "-q", str(repo)], check=True, capture_output=True)
            subprocess.run(["git", "-C", str(repo), "add", "."], check=True, capture_output=True)
            subprocess.run(["git", "-C", str(repo), "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"], check=True, capture_output=True)
            log = root / "input.log"
            log.write_text(mutate((HERE / "fixtures/macos-eval-0.2.0.log").read_text(encoding="utf-8")), encoding="utf-8")
            archive = root / "Maestro.zip"
            with zipfile.ZipFile(archive, "w") as zip_file:
                zip_file.writestr("Maestro/VERSION", "0.2.0\n")
            # Stub only host discovery: this integration may itself run on Linux.
            # Do not invoke a locally authenticated Claude executable in tests.
            commands = root / "bin"
            commands.mkdir()
            (commands / "uname").write_text('#!/bin/sh\ncase "$1" in -s) echo Darwin;; -m) echo arm64;; *) exit 1;; esac\n', encoding="utf-8")
            (commands / "claude").write_text('#!/bin/sh\necho unavailable-test-fixture\n', encoding="utf-8")
            for command in commands.iterdir():
                command.chmod(0o755)
            environment = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ["PATH"], EVAL_FIXTURE_LOG=str(log), EVAL_FIXTURE_RC=str(eval_rc))
            environment.pop("CLAUDE_PROJECT_DIR", None)
            output = root / "receipt.json"
            completed = subprocess.run(["bash", str(target / "native-smoke.sh"), "--zip", str(archive), "--output", str(output)], cwd=repo, env=environment, capture_output=True, text=True)
            receipt = json.loads(output.read_text(encoding="utf-8")) if output.exists() else None
            return completed, receipt

    def test_real_wrapper_records_scoped_pass_with_windows_still_deferred(self):
        completed, receipt = self.run_wrapper()
        self.assertEqual(completed.returncode, 0, completed.stdout + completed.stderr)
        self.assertEqual(receipt["qualification_scope"], "macos-only")
        self.assertFalse(receipt["release_qualified"])
        self.assertEqual(receipt["verdict"], "PASS_MACOS_ONLY")
        self.assertEqual(receipt["checks"]["skipped"], 1)
        self.assertEqual(receipt["deferred_checks"][0]["id"], "windows-native-path-parity")
        self.assertRegex(receipt["release_sha256"], r"^[0-9a-f]{64}$")

    def test_wrapper_rejects_native_skip_without_writing_receipt(self):
        completed, receipt = self.run_wrapper(lambda log: log.replace(FOREIGN, "native Mac alias unavailable"))
        self.assertNotEqual(completed.returncode, 0)
        self.assertIsNone(receipt)

    def test_wrapper_rejects_nonzero_evaluator_without_writing_receipt(self):
        completed, receipt = self.run_wrapper(eval_rc=1)
        self.assertNotEqual(completed.returncode, 0)
        self.assertIsNone(receipt)


if __name__ == "__main__":
    unittest.main()
