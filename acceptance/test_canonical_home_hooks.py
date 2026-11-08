"""Packaged-hook public contracts; pass a locally built MAESTRO_TEST_RUNTIME.

Run with TMPDIR under Developer. No native-client or host-profile writes.
"""
import hashlib
import datetime
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import unittest


HOOKS = Path(__file__).resolve().parents[1] / "installers/zip/user-template/.claude/hooks"


class HomeBinding(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="maestro-hook-home-")
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.product = self.base / "managed product"
        shutil.copytree(HOOKS, self.product / ".claude/hooks")
        tools = self.product / "bundles/base/tools"
        tools.mkdir(parents=True)
        shutil.copy2(HOOKS.parents[4] / "bundles/base/tools/maestro_paths.py", tools)
        self.profile = self.base / "isolated profile"
        self.profile.mkdir(mode=0o700)
        self.product.joinpath("VERSION").write_text("fixture\n")
        machine = {"x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[platform.machine()]
        host = platform.system().lower()
        relative = f"runtime/{host}-{machine}/maestro-runtime"
        helper = self.product / relative
        helper.parent.mkdir(parents=True)
        shutil.copy2(os.environ["MAESTRO_TEST_RUNTIME"], helper)
        helper.chmod(0o700)
        self.product.joinpath("runtime/manifest.json").write_text(json.dumps({
            "schema_version": 1, "version": "fixture", "artifacts": [{
                "os": host, "arch": machine, "path": relative,
                "sha256": hashlib.sha256(helper.read_bytes()).hexdigest(),
            }],
        }))
        self.env = dict(os.environ, HOME=str(self.profile), USERPROFILE=str(self.profile),
                        CLAUDE_PROJECT_DIR=str(self.product), CLAUDE_SESSION_ID="fixture")
        self.env.pop("MAESTRO_HOME", None)
        self.env.pop("MAESTRO_PY_RECORD", None)
        self.cwd = self.base / "unrelated nested working directory"
        self.cwd.mkdir()

    def run_hook(self, name, shell="bash", payload=None):
        extension = "ps1" if shell == "pwsh" else "sh"
        args = [shell]
        if shell == "pwsh":
            args += ["-NoProfile", "-File"]
        args += [str(self.product / f".claude/hooks/{name}.{extension}")]
        return subprocess.run(args, input=json.dumps(payload or {"session_id": "fixture", "prompt": "hello"}),
                              text=True, capture_output=True, env=self.env, cwd=self.cwd, timeout=90)

    def snapshot(self, root):
        return {str(p.relative_to(root)): ("symlink", os.readlink(p)) if p.is_symlink()
                else hashlib.sha256(p.read_bytes()).hexdigest()
                for p in root.rglob("*") if p.is_symlink() or p.is_file()}

    def test_hook_state_directory_alias_cannot_redirect_prompt_effects(self):
        brain = self.initialize()
        outside = self.base / "outside target"
        outside.mkdir()
        (outside / "keep.md").write_text("private outside sentinel")
        (brain / ".maestro/context-state").symlink_to(self.product, target_is_directory=True)
        roots = (self.product, self.profile, outside)
        before = [self.snapshot(root) for root in roots]
        shells = ["bash"] + (["pwsh"] if shutil.which("pwsh") else [])
        for shell in shells:
          for name in sorted(p.stem for p in HOOKS.glob("*.sh")):
            with self.subTest(hook=name, shell=shell):
                result = self.run_hook(name, shell)
                self.assertEqual(result.returncode, 2 if name == "block-cross-case-writes" else 0,
                                 result.stderr)
                self.assertEqual([self.snapshot(root) for root in roots], before, result.stdout + result.stderr)
                self.assertEqual(result.stdout.count("<!-- maestro:state-unavailable -->"), 1,
                                 result.stdout + result.stderr)

    def test_eod_reader_omits_aliased_daily_body_and_keeps_safe_excerpt(self):
        brain = self.initialize()
        daily = brain / "daily"
        daily.mkdir()
        (brain / "owner").mkdir()
        outside = self.base / "outside daily.md"
        outside.write_text("### 10:00 — briefing\nOUTSIDE_PRIVATE_DAILY_BODY\n")
        today = datetime.date.today()
        aliased = daily / f"{today - datetime.timedelta(days=1)}.md"
        aliased.symlink_to(outside)
        (daily / f"{today - datetime.timedelta(days=2)}.md").write_text(
            "### 10:00 — briefing\nSAFE_DAILY_BODY\n")
        core_before = self.snapshot(self.product)
        outside_before = outside.read_bytes()
        result = self.run_hook("session-stop-eod-check")
        self.assertEqual(result.returncode, 0, result.stderr)
        brief = (brain / ".maestro/day-brief.json").read_text()
        self.assertNotIn("OUTSIDE_PRIVATE_DAILY_BODY", brief, brief)
        self.assertIn("SAFE_DAILY_BODY", brief)
        self.assertEqual(outside.read_bytes(), outside_before)
        self.assertTrue(aliased.is_symlink())
        self.assertEqual(self.snapshot(self.product), core_before)

    def test_eod_import_uses_managed_helper_instead_of_cwd_module(self):
        brain = self.initialize()
        (brain / "daily").mkdir()
        (brain / "owner").mkdir()
        yesterday = datetime.date.today() - datetime.timedelta(days=1)
        (brain / "daily" / f"{yesterday}.md").write_text("### 10:00 — briefing\nSAFE_DAILY_BODY\n")
        marker = self.product / "untrusted-cwd-import-effect"
        (self.cwd / "maestro_paths.py").write_text(
            "from pathlib import Path\n"
            f"Path({str(marker)!r}).write_text('untrusted code executed')\n"
            "def read_regular_text(path): return Path(path).read_text()\n")
        dependency_marker = self.product / "untrusted-cwd-dependency-effect"
        (self.cwd / "json.py").write_text(
            "from pathlib import Path\n"
            f"Path({str(dependency_marker)!r}).write_text('untrusted dependency executed')\n")
        core_before = self.snapshot(self.product)
        result = self.run_hook("session-stop-eod-check")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.snapshot(self.product), core_before, result.stdout + result.stderr)
        self.assertFalse(marker.exists())
        self.assertFalse(dependency_marker.exists())
        self.assertIn("SAFE_DAILY_BODY", (brain / ".maestro/day-brief.json").read_text())

    def test_session_memory_omits_aliased_pages_and_keeps_safe_memory(self):
        self.env.pop("PYTHONDONTWRITEBYTECODE", None)
        brain = self.initialize()
        self.assertEqual(self.run_hook("first-run-scaffold").returncode, 0)
        tools = self.product / "bundles/base/tools"
        shutil.copy2(HOOKS.parents[4] / "bundles/base/tools/session-memory-emit.py", tools)
        outside = self.base / "outside memory"
        outside.mkdir()
        secret = outside / "private.md"
        secret.write_text("---\ntitle: OUTSIDE_PRIVATE_TITLE\n---\nOUTSIDE_PRIVATE_MEMORY_BODY\n")
        lifetime = brain / "memory/lifetime"
        (lifetime / "safe.md").write_text("SAFE_LIFETIME_BODY\n")
        (lifetime / "private_alias.md").symlink_to(secret)
        os.link(secret, lifetime / "private_hardlink.md")
        (brain / "memory/weekly/2026-W35.md").write_text("SAFE_WEEKLY_BODY\n")
        (brain / "memory/weekly/9999-W99.md").symlink_to(secret)
        (brain / "memory/medium-term/g1-safe.md").write_text("---\ntitle: SAFE_L3_TITLE\n---\nSafe\n")
        (brain / "memory/medium-term/g999-private.md").symlink_to(secret)
        (brain / "learnings/outside_alias").symlink_to(outside, target_is_directory=True)
        (brain / "learnings/safe.md").write_text("---\ntitle: SAFE_LEARNING_TITLE\n---\nSafe\n")
        profile = outside / "profile.json"
        profile.write_text(json.dumps({"private": "OUTSIDE_PRIVATE_PROFILE"}))
        (brain / "owner/style.json").symlink_to(profile)
        core_before, outside_before = self.snapshot(self.product), self.snapshot(outside)
        result = self.run_hook("session-start-memory-inject")
        self.assertEqual(result.returncode, 0, result.stderr)
        for text in ("OUTSIDE_PRIVATE", "private_alias", "private_hardlink", "g999-private", "outside_alias"):
            self.assertNotIn(text, result.stdout, result.stdout + result.stderr)
        for text in ("SAFE_LIFETIME_BODY", "SAFE_WEEKLY_BODY", "SAFE_L3_TITLE", "SAFE_LEARNING_TITLE"):
            self.assertIn(text, result.stdout, result.stdout + result.stderr)
        self.assertEqual(self.snapshot(self.product), core_before)
        self.assertEqual(self.snapshot(outside), outside_before)

    @unittest.skipUnless(shutil.which("pwsh"), "PowerShell unavailable")
    def test_powershell_daily_reader_refuses_hardlink_body(self):
        brain = self.initialize()
        daily = brain / "daily"
        daily.mkdir()
        (brain / "owner").mkdir()
        (brain / ".initialized").write_text("fixture\n")
        outside = self.base / "outside daily.md"
        outside.write_text("### 10:00 — briefing\nOUTSIDE_PRIVATE_HARDLINK_BODY\n")
        yesterday = datetime.date.today() - datetime.timedelta(days=1)
        os.link(outside, daily / f"{yesterday}.md")
        core_before, outside_before = self.snapshot(self.product), outside.read_bytes()
        result = self.run_hook("session-stop-eod-check", "pwsh")
        self.assertEqual(result.returncode, 0, result.stderr)
        brief = brain / ".maestro/day-brief.json"
        self.assertNotIn("OUTSIDE_PRIVATE_HARDLINK_BODY", brief.read_text() if brief.exists() else "")
        self.assertIn("aliased", result.stderr.lower())
        self.assertEqual(outside.read_bytes(), outside_before)
        self.assertEqual(self.snapshot(self.product), core_before)

    def test_uninitialized_profile_has_no_hook_effects(self):
        before = self.snapshot(self.product)
        profile_before = self.snapshot(self.profile)
        shells = ["bash"] + (["pwsh"] if shutil.which("pwsh") else [])
        for shell in shells:
          for name in sorted(p.stem for p in HOOKS.glob("*.sh")):
            with self.subTest(hook=name, shell=shell):
                result = self.run_hook(name, shell)
                self.assertEqual(result.returncode, 2 if name == "block-cross-case-writes" else 0,
                                 result.stderr)
                self.assertEqual(self.snapshot(self.product), before, name)
                self.assertEqual(self.snapshot(self.profile), profile_before, name)
                self.assertEqual(result.stdout.count("<!-- maestro:state-unavailable -->"), 1,
                                 result.stdout + result.stderr)

    def test_python_library_requires_admission_even_with_cached_interpreter(self):
        marker = self.profile / "unadmitted-python-effect"
        library = self.product / ".claude/hooks/lib/python.sh"
        code = "from pathlib import Path; Path(" + repr(str(marker)) + ").write_text('effect')"
        env = dict(self.env, MAESTRO_PY_CMD=sys.executable)
        result = subprocess.run(["bash", "-c", '. "$1"; maestro_py -c "$2"', "fixture", str(library), code],
                                env=env, text=True, capture_output=True, cwd=self.cwd, timeout=30)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(marker.exists(), result.stderr)

    def initialize(self):
        result = subprocess.run([os.environ["MAESTRO_TEST_RUNTIME"], "home", "init",
                                 "--product-root", str(self.product), "--user-home", str(self.profile)],
                                env=self.env, capture_output=True, text=True, check=True)
        return Path(json.loads(result.stdout)["brain_root"])

    def test_external_owner_hooks_preserve_managed_product_and_use_owner_markers(self):
        brain = self.initialize()
        policy = self.product / "bundles/base/agents/activation-policy.json"
        policy.parent.mkdir(parents=True)
        shutil.copyfile(HOOKS.parents[4] / "bundles/base/agents/activation-policy.json", policy)
        before = self.snapshot(self.product)
        result = self.run_hook("first-run-scaffold")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((brain / ".initialized").is_file(), result.stdout + result.stderr)
        result = self.run_hook("context-inject-userprompt")
        self.assertIn(str(brain / "memory"), result.stdout)
        self.assertTrue((brain / ".maestro/context-state/context-inject-fixture.marker").exists())
        result = self.run_hook("session-stop-agent-check")
        self.assertTrue((brain / ".maestro/.darwin-requested").exists(), result.stderr)
        result = self.run_hook("session-start-memory-inject")
        self.assertIn(str(brain), result.stdout)
        self.assertFalse((brain / ".maestro/.darwin-requested").exists(), result.stderr)
        for name in ("session-stop-eod-check", "session-stop-brain-index", "session-stop-dream", "announce-agent-dispatch"):
            self.assertEqual(self.run_hook(name).returncode, 0, name)
        cross_case = brain / "accounts/b/cases/two"
        cross_case.mkdir(parents=True)
        active_case = brain / "accounts/a/cases/one"
        active_case.mkdir(parents=True)
        (brain / "accounts/.active").write_text("a/one\n")
        result = self.run_hook("block-cross-case-writes", payload={
            "cwd": str(active_case), "tool_name": "Write", "tool_input": {"file_path": "ok.md"}})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        result = self.run_hook("block-cross-case-writes", payload={
            "cwd": str(cross_case), "tool_name": "Write", "tool_input": {"file_path": "secret.md"}})
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertEqual(self.snapshot(self.product), before)

    def test_prompt_stub_preserves_unicode_within_character_budget(self):
        self.env['MAESTRO_HOME'] = str(self.profile / 'mémoire-界')
        brain = self.initialize()
        scaffold = self.run_hook('first-run-scaffold')
        self.assertEqual(scaffold.returncode, 0, scaffold.stderr)
        first = self.run_hook('context-inject-userprompt')
        self.assertEqual(first.returncode, 0, first.stderr)
        stub = self.run_hook('context-inject-userprompt')
        self.assertEqual(stub.returncode, 0, stub.stderr)
        expected = ('<!-- maestro:context-inject:stub -->\n'
                    f'Memory: {brain}/memory/ · Load specific tiers on demand.\n')
        self.assertEqual(stub.stdout, expected[:160])
        self.assertLessEqual(len(stub.stdout), 160)
        self.assertIn('mémoire-界', stub.stdout)

    def test_same_owner_marker_across_read_only_product_versions(self):
        brain = self.initialize()
        self.assertEqual(self.run_hook("first-run-scaffold").returncode, 0)
        self.assertIn("context-inject:first", self.run_hook("context-inject-userprompt").stdout)
        original = self.product
        second = self.base / "managed product v2"
        shutil.copytree(original, second)
        (second / "VERSION").write_text("fixture-v2\n")
        manifest = second / "runtime/manifest.json"
        second_manifest = json.loads(manifest.read_text())
        second_manifest["version"] = "fixture-v2"
        manifest.write_text(json.dumps(second_manifest))
        self.product = second
        self.env["CLAUDE_PROJECT_DIR"] = str(second)
        before = {str(root): self.snapshot(root) for root in (original, second)}

        def make_writable():
            for root in (original, second):
                root.chmod(0o700)
                for path in root.rglob("*"):
                    path.chmod(0o700 if path.is_dir() else 0o600)

        self.addCleanup(make_writable)
        for root in (original, second):
            for path in root.rglob("*"):
                path.chmod(0o555 if path.is_dir() or path.name == "maestro-runtime" else 0o444)
            root.chmod(0o555)
        result = self.run_hook("context-inject-userprompt")
        self.assertIn("context-inject:stub", result.stdout, result.stderr)
        self.assertIn(str(brain / "memory"), result.stdout)
        for root in (original, second):
            self.assertEqual(self.snapshot(root), before[str(root)])

    def test_unavailable_origin_preserves_both_trees_for_every_hook(self):
        brain = self.initialize()
        origin = brain / ".maestro/origin.json"
        valid = json.loads(origin.read_text())
        shells = ["bash"] + (["pwsh"] if shutil.which("pwsh") else [])
        for origin_state in ("pending", "revoked", "migrated", "malformed"):
            record = dict(valid, origin=origin_state)
            origin.write_text("{" if origin_state == "malformed" else json.dumps(record))
            before = self.snapshot(self.product)
            profile_before = self.snapshot(self.profile)
            for shell in shells:
              for name in sorted(p.stem for p in HOOKS.glob("*.sh")):
                with self.subTest(state=origin_state, shell=shell, hook=name):
                    result = self.run_hook(name, shell)
                    self.assertEqual(result.returncode, 2 if name == "block-cross-case-writes" else 0,
                                     result.stderr)
                    self.assertEqual(self.snapshot(self.product), before, name)
                    self.assertEqual(self.snapshot(self.profile), profile_before, name)
                    self.assertEqual(result.stdout.count("<!-- maestro:state-unavailable -->"), 1,
                                     result.stdout + result.stderr)

    @unittest.skipUnless(shutil.which("pwsh"), "PowerShell unavailable")
    def test_powershell_ready_owner_markers_and_nested_guard(self):
        brain = self.initialize()
        policy = self.product / "bundles/base/agents/activation-policy.json"
        policy.parent.mkdir(parents=True)
        shutil.copyfile(HOOKS.parents[4] / "bundles/base/agents/activation-policy.json", policy)
        before = self.snapshot(self.product)
        result = self.run_hook("first-run-scaffold", "pwsh")
        self.assertTrue((brain / ".initialized").is_file(), result.stdout + result.stderr)
        result = self.run_hook("context-inject-userprompt", "pwsh")
        self.assertIn(str(brain / "memory"), result.stdout, result.stderr)
        self.assertTrue((brain / ".maestro/context-state/context-inject-fixture.marker").exists())
        result = self.run_hook("session-stop-eod-check", "pwsh")
        self.assertTrue((brain / ".maestro/day-brief.json").exists(), result.stderr)
        result = self.run_hook("session-stop-agent-check", "pwsh")
        self.assertTrue((brain / ".maestro/.darwin-requested").exists(), result.stderr)
        result = self.run_hook("session-start-memory-inject", "pwsh")
        self.assertIn(str(brain), result.stdout, result.stderr)
        for name in ("session-stop-brain-index", "session-stop-dream", "announce-agent-dispatch"):
            self.assertEqual(self.run_hook(name, "pwsh").returncode, 0, name)
        self.assertTrue((brain / ".maestro/index-capability.json").exists())
        cross_case = brain / "accounts/b/cases/two"
        cross_case.mkdir(parents=True)
        active_case = brain / "accounts/a/cases/one"
        active_case.mkdir(parents=True)
        (brain / "accounts/.active").write_text("a/one\n")
        result = self.run_hook("block-cross-case-writes", "pwsh", {
            "cwd": str(active_case), "tool_name": "Write", "tool_input": {"file_path": "ok.md"}})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        result = self.run_hook("block-cross-case-writes", "pwsh", {
            "cwd": str(cross_case), "tool_name": "Write", "tool_input": {"file_path": "secret.md"}})
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertEqual(self.snapshot(self.product), before)


if __name__ == "__main__":
    unittest.main()
