"""Real-package upgrade acceptance; author content is entirely synthetic.

Run on macOS:
  python3 -m unittest discover -s acceptance -p test_upgrade_v020.py -v

Configure MAESTRO_UPGRADE_BASELINE_011_ZIP, MAESTRO_UPGRADE_BASELINE_012_ZIP,
MAESTRO_UPGRADE_CANDIDATE_ZIP and optionally MAESTRO_UPGRADE_REPORT_DIR.
The default 0.1.11 artifact is a reconstructed historical snapshot, not a field
installation. These tests never modify either input ZIP or an existing install.
"""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import shutil
import stat
import subprocess
import tempfile
import unittest
import zipfile


REPO = Path(__file__).resolve().parents[1]
DEFAULT_BASELINES = {
    "0.1.11": (
        Path("/private/tmp/maestro-v011-baseline-o4F3QB/dist/Maestro-v0.1.11.zip"),
        "reconstructed historical snapshot; not a field install",
    ),
    "0.1.12": (
        Path("/Users/scardinidaniel/Downloads/Maestro-0.1.12-audit-2026-09-28/Maestro-v0.1.12-macos.zip"),
        "archived 0.1.12 macOS package; extracted test install, not user data",
    ),
}
ENV_BASELINES = {
    "0.1.11": "MAESTRO_UPGRADE_BASELINE_011_ZIP",
    "0.1.12": "MAESTRO_UPGRADE_BASELINE_012_ZIP",
}


def sha256(file):
    digest = hashlib.sha256()
    with file.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def inventory(root):
    """Every original directory and file, including dotfiles and empty dirs."""
    result = {}
    for item in sorted(root.rglob("*")):
        relative = item.relative_to(root).as_posix()
        if item.is_symlink():
            raise AssertionError(f"fixture contains unexpected symlink: {relative}")
        result[relative] = {"kind": "directory"} if item.is_dir() else {
            "kind": "file", "sha256": sha256(item), "size": item.stat().st_size}
    return result


def extract_package(archive, destination):
    """Reject unsafe members and retain shipped executable permission bits."""
    destination.mkdir(parents=True)
    with zipfile.ZipFile(archive) as package:
        members = package.infolist()
        seen = set()
        for member in members:
            relative = PurePosixPath(member.filename)
            if relative.is_absolute() or ".." in relative.parts or "\\" in member.filename:
                raise AssertionError("unsafe package member")
            if member.filename in seen:
                raise AssertionError("duplicate package member")
            seen.add(member.filename)
            mode = member.external_attr >> 16
            if stat.S_ISLNK(mode):
                raise AssertionError("package symlink requires separate review")
        for member in members:
            package.extract(member, destination)
            target = destination.joinpath(*PurePosixPath(member.filename).parts)
            mode = (member.external_attr >> 16) & 0o777
            if mode:
                target.chmod(mode)
    roots = [item.parent for item in destination.glob("*/VERSION")]
    if len(roots) != 1:
        raise AssertionError("expected one packaged Maestro root")
    return roots[0]


def write_fixture(root, relative, content):
    target = root / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(content if isinstance(content, bytes) else content.encode("utf-8"))
    return target


def scaffold(root):
    return subprocess.run(
        ["bash", str(root / ".claude/hooks/first-run-scaffold.sh")],
        cwd=root, env={**os.environ, "CLAUDE_PROJECT_DIR": str(root)},
        capture_output=True, text=True, timeout=120,
    )


def migration(root, *args):
    script = (
        '. "$1/.claude/hooks/lib/maestro-runtime.sh"; '
        'project="$1"; shift; '
        'maestro_runtime "$project" migration --project "$project" "$@"'
    )
    result = subprocess.run(
        ["bash", "-c", script, "upgrade-verifier", str(root), *args],
        cwd=root, capture_output=True, text=True, timeout=120,
    )
    try:
        value = json.loads(result.stdout)
    except json.JSONDecodeError as error:
        raise AssertionError(f"managed migration JSON missing: {result.stderr}") from error
    return result, value


def reconcile_host_file(old_root, new_root, relative):
    """Explicit fixture operator action; never silently overwrite a host file."""
    source, target = old_root / relative, new_root / relative
    source_hash = sha256(source)
    if target.exists():
        return {
            "path": relative, "action": "conflict_requires_explicit_reconciliation",
            "source_sha256": source_hash, "candidate_sha256": sha256(target),
            "overwritten": False,
        }
    target.parent.mkdir(parents=True, exist_ok=True)
    with source.open("rb") as original, target.open("xb") as copy:
        shutil.copyfileobj(original, copy)
    if sha256(target) != source_hash:
        raise AssertionError("owner-local host file copy was not preserved")
    return {
        "path": relative, "action": "explicit_copy_to_absent_owner_local_path",
        "source_sha256": source_hash, "candidate_sha256": sha256(target),
        "overwritten": False,
    }


def merge_authorized_fixture_env(old_root, new_root):
    """A specific fixture authorization, not a general automatic installer merge."""
    relative = ".claude/settings.json"
    key = "MAESTRO_UPGRADE_FIXTURE"
    source = json.loads((old_root / relative).read_text())
    target = new_root / relative
    original = json.loads(target.read_text())
    merged = json.loads(json.dumps(original))
    value = source["env"][key]
    if value != "synthetic managed-file conflict":
        raise AssertionError("fixture merge authorization does not cover this value")
    if key in original.get("env", {}):
        raise AssertionError("fixture merge does not authorize replacing an existing env key")
    before_hash = sha256(target)
    merged.setdefault("env", {})[key] = value
    if merged.get("hooks") != original.get("hooks"):
        raise AssertionError("fixture reconciliation altered candidate hooks")
    for field, expected in original.items():
        if field != "env" and merged[field] != expected:
            raise AssertionError("fixture reconciliation altered another managed field")
    write_fixture(new_root, relative, json.dumps(merged, indent=2) + "\n")
    return {
        "path": relative, "action": "authorized_fixture_only_env_key_merge",
        "selected_key": key, "candidate_before_sha256": before_hash,
        "candidate_after_sha256": sha256(target),
        "candidate_hooks_preserved": merged.get("hooks") == original.get("hooks"),
        "general_automatic_installer_merge": False,
    }


@unittest.skipUnless(platform.system() == "Darwin", "actual macOS ZIP/scaffold acceptance requires macOS")
class PackagedUpgradeV020(unittest.TestCase):
    def test_packaged_011_to_020(self):
        self.exercise("0.1.11")

    def test_packaged_012_to_020(self):
        self.exercise("0.1.12")

    def exercise(self, version):
        default, provenance = DEFAULT_BASELINES[version]
        baseline = Path(os.environ.get(ENV_BASELINES[version], str(default))).expanduser()
        candidate = Path(os.environ.get(
            "MAESTRO_UPGRADE_CANDIDATE_ZIP",
            str(REPO / "dist/Maestro-v0.2.0-macos.zip"),
        )).expanduser()
        if not baseline.is_file():
            self.skipTest(f"baseline unavailable: {baseline}; set {ENV_BASELINES[version]}")
        if not candidate.is_file():
            self.skipTest(f"candidate unavailable: {candidate}; set MAESTRO_UPGRADE_CANDIDATE_ZIP")
        if ENV_BASELINES[version] in os.environ:
            provenance = os.environ.get(
                "MAESTRO_UPGRADE_BASELINE_PROVENANCE",
                "operator-supplied package; provenance requires independent review",
            )
        artifact_hashes = {"baseline": sha256(baseline), "candidate": sha256(candidate)}
        evidence = {
            "schema_version": 1, "baseline_version": version, "target_version": "0.2.0",
            "baseline": {"path": str(baseline), "sha256": artifact_hashes["baseline"], "provenance": provenance},
            "candidate": {"path": str(candidate), "sha256": artifact_hashes["candidate"]},
            "content_provenance": "synthetic author content created by this test; no real user installation",
            "runtime_evidence": "direct native macOS Bash scaffold and verified managed helper; not an attended Claude/Codex session",
            "result": "running",
        }
        report_dir = Path(os.environ.get(
            "MAESTRO_UPGRADE_REPORT_DIR", str(Path(tempfile.gettempdir()) / "maestro-upgrade-v020-reports"),
        ))
        report_dir.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix=f"maestro upgrade {version} ação ") as temporary:
            work = Path(temporary)
            try:
                old = extract_package(baseline, work / "baseline extracted")
                self.assertEqual((old / "VERSION").read_text().strip(), version)
                old_start = scaffold(old)
                self.assertEqual(old_start.returncode, 0, old_start.stderr)
                self.assertTrue((old / "data/.initialized").is_file(), "actual old scaffold did not initialize data")
                evidence["old_scaffold"] = {"exit_code": old_start.returncode, "initialized": True}

                # Authored material is synthetic; preserve every old scaffold file as well.
                fixtures = {
                    "craft/index.md": "# Authored craft index\n- methods/fixture.md — synthetic retained index\n",
                    "learnings/index.md": "# Authored learning index\n- fixture.md — synthetic retained index\n",
                    "owner/fixture-author-note.md": "Synthetic owner note; no real person or client.\n",
                    "cases/fixture-case/brain/canon/facts.md": "Synthetic case canon for upgrade acceptance.\n",
                    "cases/.active": b"\xef\xbb\xbffixture-case\r\n",
                    "cases/.pending": b"\xef\xbb\xbffixture-case\r\n",
                    "agents/custom/agent.md": "Synthetic retained agent customization.\n",
                    "workspaces/project-a/context.md": "Synthetic retained workspace context.\n",
                    "canary/fixture-state.json": '{"fixture":true,"contains_user_data":false}\n',
                    "customizações/ação 1.md": "Synthetic Unicode customization.\n",
                    ".fixture-hidden": b"synthetic hidden bytes\x00\x01\n",
                }
                for relative, content in fixtures.items():
                    write_fixture(old / "data", relative, content)
                (old / "data/customizações/empty").mkdir()
                write_fixture(old, ".claude/settings.local.json",
                              '{"env":{"MAESTRO_UPGRADE_FIXTURE":"synthetic-author-content"}}\n')
                host_settings = json.loads((old / ".claude/settings.json").read_text())
                host_settings.setdefault("env", {})["MAESTRO_UPGRADE_FIXTURE"] = "synthetic managed-file conflict"
                write_fixture(old, ".claude/settings.json", json.dumps(host_settings))
                before = inventory(old / "data")
                evidence["original_data_inventory"] = before
                evidence["original_data_entry_count"] = len(before)
                host_before = {p: sha256(old / p) for p in (".claude/settings.local.json", ".claude/settings.json")}

                new = extract_package(candidate, work / "candidate extracted")
                self.assertEqual((new / "VERSION").read_text().strip(), "0.2.0")
                self.assertFalse((new / "data").exists(), "candidate package contains a data workspace")
                self.assertFalse((new / "brain").exists(), "candidate package contains a brain workspace")
                shutil.copytree(old / "data", new / "data", copy_function=shutil.copy2)
                self.assertEqual(inventory(new / "data"), before)
                self.assertTrue((new / ".claude/settings.json").is_file(), "managed candidate hook configuration missing")
                managed_settings_hash = sha256(new / ".claude/settings.json")
                candidate_hooks = json.loads((new / ".claude/settings.json").read_text())["hooks"]
                self.assertFalse((new / ".claude/settings.local.json").exists(), "candidate shipped owner-local host settings")
                reconciliation = [
                    reconcile_host_file(old, new, ".claude/settings.local.json"),
                    reconcile_host_file(old, new, ".claude/settings.json"),
                ]
                self.assertEqual(reconciliation[0]["action"], "explicit_copy_to_absent_owner_local_path")
                self.assertEqual(reconciliation[1]["action"], "conflict_requires_explicit_reconciliation")
                self.assertEqual(sha256(new / ".claude/settings.json"), managed_settings_hash)
                reconciliation.append(merge_authorized_fixture_env(old, new))
                self.assertTrue(reconciliation[2]["candidate_hooks_preserved"])
                self.assertFalse(reconciliation[2]["general_automatic_installer_merge"])
                reconciled_settings_hash = sha256(new / ".claude/settings.json")
                evidence["host_file_reconciliation"] = reconciliation

                new_start = scaffold(new)
                self.assertEqual(new_start.returncode, 0, new_start.stderr)
                self.assertNotIn("maestro:migration-blocked", new_start.stdout, new_start.stdout)
                self.assertTrue((new / "brain/.initialized").is_file(), "new scaffold did not initialize migrated workspace")
                status_call, status = migration(new, "--status")
                self.assertEqual(status_call.returncode, 0, status)
                self.assertEqual(status["state"], "committed", status)
                evidence["migration"] = status
                self.assertEqual(inventory(old / "data"), before)
                self.assertEqual(inventory(new / "data"), before)
                self.assertEqual((new / "brain/accounts/.active").read_text(), "_sem-conta/fixture-case\n")
                self.assertEqual((new / "brain/accounts/.pending").read_text(), "_sem-conta/fixture-case\n")
                for relative in (
                    "owner/fixture-author-note.md",
                    "accounts/_sem-conta/cases/fixture-case/canon/facts.md",
                    "customizações/ação 1.md", ".fixture-hidden",
                ):
                    self.assertTrue((new / "brain" / relative).is_file(), f"missing migrated fixture: {relative}")
                self.assertTrue((new / "brain/customizações/empty").is_dir())
                for old_index, new_index in (("craft/index.md", "craft/craft.md"), ("learnings/index.md", "learnings/learnings.md")):
                    self.assertEqual((new / "brain" / new_index).read_bytes(), (old / "data" / old_index).read_bytes(), "author index must remain readable at canonical path")
                    self.assertFalse((new / "brain" / old_index).exists(), "scaffold must not create a competing legacy index")
                for namespace, file in (
                    ("agents", "custom/agent.md"),
                    ("workspaces", "project-a/context.md"),
                    ("canary", "fixture-state.json"),
                ):
                    resolved_call, resolved = migration(new, "--resolve-legacy", namespace)
                    self.assertEqual(resolved_call.returncode, 0, resolved)
                    self.assertEqual(resolved["verification"], "legacy_current")
                    self.assertEqual((Path(resolved["path"]) / file).read_bytes(),
                                     (old / "data" / namespace / file).read_bytes())

                state_file = new / "brain/.maestro/migration/state.json"
                receipt_file = state_file.parent / "attempts" / status["attempt_id"] / "receipt.json"
                authority_before = (state_file.read_bytes(), receipt_file.read_bytes())
                repeat = scaffold(new)
                self.assertEqual(repeat.returncode, 0, repeat.stderr)
                self.assertNotIn("maestro:migration-blocked", repeat.stdout)
                authored = write_fixture(new, "brain/owner/fixture-author-note.md", "Synthetic newer authored work.\n")
                post_edit = scaffold(new)
                self.assertEqual(post_edit.returncode, 0, post_edit.stderr)
                self.assertNotIn("maestro:migration-blocked", post_edit.stdout)
                for old_index, new_index in (("craft/index.md", "craft/craft.md"), ("learnings/index.md", "learnings/learnings.md")):
                    self.assertEqual((new / "brain" / new_index).read_bytes(), (old / "data" / old_index).read_bytes(), "repeated startup overwrote authored index")
                _, evolved = migration(new, "--status")
                self.assertEqual(evolved["state"], "committed", evolved)
                self.assertEqual(evolved["verification"], "target_evolved", evolved)
                self.assertFalse(evolved["current"])
                self.assertEqual(authored.read_text(), "Synthetic newer authored work.\n")
                self.assertEqual((state_file.read_bytes(), receipt_file.read_bytes()), authority_before)
                evidence["post_edit_verification"] = evolved

                brain_before_rollback = inventory(new / "brain")
                rollback_call, rollback = migration(new, "--rollback", status["attempt_id"])
                self.assertEqual(rollback_call.returncode, 0, rollback)
                self.assertEqual(rollback["state"], "rolled_back")
                self.assertFalse(rollback["restored_runtime"])
                brain_after_rollback = inventory(new / "brain")
                for relative, entry in brain_before_rollback.items():
                    if relative != ".maestro/migration/state.json":
                        self.assertEqual(brain_after_rollback[relative], entry, f"rollback altered {relative}")
                self.assertEqual(set(brain_after_rollback), set(brain_before_rollback))
                self.assertEqual(inventory(old / "data"), before)
                self.assertEqual(inventory(new / "data"), before)
                self.assertEqual(sha256(new / ".claude/settings.json"), reconciled_settings_hash)
                final_settings = json.loads((new / ".claude/settings.json").read_text())
                self.assertEqual(final_settings["hooks"], candidate_hooks)
                self.assertEqual(final_settings["env"]["MAESTRO_UPGRADE_FIXTURE"], "synthetic managed-file conflict")
                for relative, expected in host_before.items():
                    self.assertEqual(sha256(old / relative), expected)
                self.assertEqual(sha256(new / ".claude/settings.local.json"), host_before[".claude/settings.local.json"])
                self.assertEqual(sha256(baseline), artifact_hashes["baseline"])
                self.assertEqual(sha256(candidate), artifact_hashes["candidate"], "candidate ZIP changed during acceptance")
                evidence["rollback"] = rollback
                evidence["original_data_preserved"] = True
                evidence["copied_data_preserved"] = True
                evidence["historical_receipt_preserved"] = True
                evidence["managed_host_hook_wiring_preserved"] = True
                evidence["authorized_fixture_customization_reconciled"] = True
                evidence["result"] = "passed"
            except Exception as error:
                evidence["result"] = "failed"
                evidence["failure"] = f"{type(error).__name__}: {error}"
                raise
            finally:
                report = report_dir / f"upgrade-{version}-to-0.2.0-{artifact_hashes['candidate'][:12]}.json"
                report.write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
                print(f"upgrade evidence: {report}", flush=True)


if __name__ == "__main__":
    unittest.main()
