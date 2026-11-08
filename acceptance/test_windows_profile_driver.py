"""Closed inspection mode refuses malformed inputs before creating an account."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[1]
DRIVER = ROOT / "acceptance/windows-core-pilot.ps1"


class WindowsInspectionDriver(unittest.TestCase):
    def setUp(self):
        self.pwsh = shutil.which("pwsh")
        self.assertIsNotNone(self.pwsh, "PowerShell parser and admission checks are required")
        self.temp = tempfile.TemporaryDirectory(dir=os.environ.get("MAESTRO_TEST_TMP"))
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.manifest = self.root / "inspection.json"

    def invoke(self, document, *extra):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        return subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(DRIVER),
                               "-InspectionManifest", str(self.manifest), "-OutputRoot", str(self.root / "proof"),
                               *extra], text=True, capture_output=True, timeout=30)

    def assert_preaccount_refusal(self, result, message):
        self.assertNotEqual(result.returncode, 0)
        output = result.stdout + result.stderr
        self.assertIn(message, output)
        self.assertNotIn("New-LocalUser", output)
        self.assertFalse((self.root / "proof").exists())

    def test_script_and_embedded_launcher_parse(self):
        check = r"""$tokens=$null; $errors=$null
[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors) | Out-Null
if ($errors.Count) { throw ($errors | Out-String) }
$launcher=@($tokens | Where-Object { $_.Kind -eq 'HereStringLiteral' })
if ($launcher.Count -ne 1) { throw 'Expected the single shared credentialed launcher' }
[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$errors) | Out-Null
if ($errors.Count) { throw ($errors | Out-String) }
"""
        script = self.root / "parse.ps1"
        script.write_text(check, encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_workflow_windows_blocks_parse(self):
        workflow = (ROOT / ".github/workflows/data-home-inspection.yml").read_text()
        current = []
        blocks = []
        lines = workflow.splitlines()
        for index, line in enumerate(lines):
            if line.startswith("      - "):
                current = []
            current.append(line)
            if line == "        run: |" and "        shell: pwsh" in current:
                body = []
                for child in lines[index + 1:]:
                    if child and not child.startswith("          "):
                        break
                    body.append(child)
                blocks.append(textwrap.dedent("\n".join(body)))
        self.assertEqual(len(blocks), 3)
        for index, body in enumerate(blocks):
            script = self.root / f"workflow-{index}.ps1"
            script.write_text(body, encoding="utf-8")
            parser = self.root / "parse-workflow.ps1"
            parser.write_text("$e=$null; [Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$e) | Out-Null; if($e.Count){throw ($e | Out-String)}", encoding="utf-8")
            result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(parser), str(script)], capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def closed_manifest(self):
        source = self.root / "source"
        sdk = self.root / "sdk"
        sdk.mkdir()
        go = sdk / "go.exe"
        go.write_bytes(b"unexecuted negative-test fixture")
        entries = []
        for name in ["datahome", "scheduler", "zipruntime", "maestro-runtime"]:
            relative = "cmd/maestro-runtime" if name == "maestro-runtime" else f"internal/{name}"
            directory = source / relative
            directory.mkdir(parents=True)
            binary = self.root / f"{name}.test.exe"
            binary.write_bytes(b"unexecuted negative-test fixture")
            tests = ["TestFixture"]
            if name == "zipruntime":
                tests += ["TestCodexLifecycleRequiresExplicitMigration", "TestHookPreflightPreservesCommittedLegacyAndCaseAliases"]
            entries.append(dict(name=name, package=f"./{relative}", working_directory=str(directory),
                                binary=str(binary), sha256=hashlib.sha256(binary.read_bytes()).hexdigest(), tests=tests, skip_policy=[]))
        vendor = source / "vendor/modules.txt"
        vendor.parent.mkdir()
        vendor.write_text("sealed fixture", encoding="utf-8")
        return dict(schema_version=1, source_revision="a" * 40, source_archive_sha256="b" * 64,
                    source_root=str(source), sdk_root=str(sdk), sdk_go=str(go), sdk_go_sha256=hashlib.sha256(go.read_bytes()).hexdigest(),
                    packages=entries, vendor_files=[dict(path="modules.txt", sha256=hashlib.sha256(vendor.read_bytes()).hexdigest())])

    def test_guarded_migration_contracts_cannot_be_classified_as_skips(self):
        document = self.closed_manifest()
        document["packages"][2]["skip_policy"] = [dict(test="TestCodexLifecycleRequiresExplicitMigration", reason="must not bypass")]
        self.assert_preaccount_refusal(self.invoke(document), "Both guarded migration contracts must run without skips")

    def test_changed_binary_refuses_before_account_creation(self):
        document = self.closed_manifest()
        document["packages"][0]["sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke(document), "Canonical test executable differs from its manifest")

    def test_changed_vendor_input_refuses_before_account_creation(self):
        document = self.closed_manifest()
        document["vendor_files"][0]["sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke(document), "Inspection vendored input differs from its sealed ordinary file")

    def test_inspection_and_pilot_parameters_are_exclusive(self):
        result = self.invoke({}, "-TestBinary", "unused", "-Server", "unused")
        self.assert_preaccount_refusal(result, "Parameter set cannot be resolved")

    def test_inspection_requires_the_closed_four_package_inventory(self):
        result = self.invoke({"schema_version": 1, "packages": []})
        self.assert_preaccount_refusal(result, "Inspection qualification requires exactly the four owning packages")

    def test_inspection_rejects_an_unlisted_package(self):
        names = ["datahome", "scheduler", "zipruntime", "arbitrary"]
        result = self.invoke({"schema_version": 1, "packages": [{"name": name} for name in names]})
        self.assert_preaccount_refusal(result, "Inspection qualification requires exactly the four owning packages")


if __name__ == "__main__":
    unittest.main()
