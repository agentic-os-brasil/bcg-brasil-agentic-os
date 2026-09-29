"""Host-local guard regressions; not native Claude or Windows qualification."""
import hashlib
import platform
import shutil
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HOOK = Path(__file__).parent / "user-template/.claude/hooks/block-cross-case-writes.sh"


class AccountGuardTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory(prefix="guard-runtime-")
        cls.addClassCleanup(cls.build.cleanup)
        cls.helper = Path(cls.build.name) / "maestro-runtime"
        subprocess.run([shutil.which("go") or "/opt/homebrew/bin/go", "build", "-o", str(cls.helper), "./cmd/maestro-runtime"], cwd=Path(__file__).resolve().parents[2], check=True)

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="maestro guard unicode ç ")
        self.root = Path(self.tmp.name)
        arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}[platform.machine()]
        relative = f"runtime/{platform.system().lower()}-{arch}/maestro-runtime"
        binary = self.root / relative
        binary.parent.mkdir(parents=True)
        shutil.copy2(self.helper, binary)
        (self.root / "VERSION").write_text("0.2.0")
        (self.root / "runtime/manifest.json").write_text(json.dumps({"schema_version": 1, "version": "0.2.0", "artifacts": [{"os": platform.system().lower(), "arch": arch, "path": relative, "sha256": hashlib.sha256(binary.read_bytes()).hexdigest()}]}))
        self.bin = self.root / "native-bin"
        self.bin.mkdir()
        for name in ("dirname", "uname", "tr", "shasum", "cut", "cat"):
            (self.bin / name).symlink_to(shutil.which(name))
        self.accounts = self.root / "brain/accounts"
        self.active = self.accounts / "alfa/cases/tmo"
        self.other = self.accounts / "beta/cases/tmo"
        self.active.mkdir(parents=True)
        self.other.mkdir(parents=True)
        (self.accounts / ".active").write_text("alfa/tmo\n")

    def tearDown(self):
        self.tmp.cleanup()

    def verdict(self, target, tool="Write", **extra):
        key = "notebook_path" if tool == "NotebookEdit" else "file_path"
        payload = {"tool_name": tool, "tool_input": {key: str(target), **extra}}
        return subprocess.run(
            ["/bin/bash", str(HOOK.resolve())], input=json.dumps(payload), text=True,
            capture_output=True, env={**os.environ, "CLAUDE_PROJECT_DIR": str(self.root), "PATH": str(self.bin)},
        )

    def test_pair_identity(self):
        self.assertEqual(self.verdict(self.active / "ok.md").returncode, 0)
        self.assertEqual(self.verdict(self.other / "no.md").returncode, 2)

    def test_traversal(self):
        self.assertEqual(self.verdict(self.active / "../../../beta/cases/tmo/no.md").returncode, 2)

    def test_inside_alias(self):
        (self.active / "link").symlink_to(self.other, target_is_directory=True)
        self.assertEqual(self.verdict(self.active / "link/no.md").returncode, 2)

    def test_outside_alias(self):
        (self.root / "innocent").symlink_to(self.other, target_is_directory=True)
        self.assertEqual(self.verdict(self.root / "innocent/no.md").returncode, 2)

    def test_alias_escape(self):
        outside = self.root / "outside"
        outside.mkdir()
        (self.active / "escape").symlink_to(outside, target_is_directory=True)
        self.assertEqual(self.verdict(self.active / "escape/no.md").returncode, 2)

    def test_notebook_decoy(self):
        self.assertEqual(self.verdict(self.other / "no.ipynb", "NotebookEdit",
                                     file_path=str(self.active / "ok.md")).returncode, 2)

    def test_all_write_tools(self):
        for tool in ("Edit", "MultiEdit", "Write", "NotebookEdit"):
            with self.subTest(tool=tool):
                self.assertEqual(self.verdict(self.other / "no.md", tool).returncode, 2)

    def test_missing_and_bare_marker(self):
        marker = self.accounts / ".active"
        marker.unlink()
        self.assertEqual(self.verdict(self.other / "no.md").returncode, 2)
        marker.write_text("tmo\n")
        self.assertEqual(self.verdict(self.active / "no.md").returncode, 2)

    def test_pending_pair(self):
        (self.accounts / ".pending").write_text("beta/tmo\n")
        self.assertEqual(self.verdict(self.other / "ok.md").returncode, 0)

    def test_unrelated_and_account_material(self):
        self.assertEqual(self.verdict(self.root / "notes.md").returncode, 0)
        self.assertEqual(self.verdict(self.accounts / "beta/brief.md").returncode, 0)
        self.assertEqual(self.verdict(self.other / "no.md", "TodoWrite").returncode, 0)


if __name__ == "__main__":
    unittest.main()
