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
        for workflow_path in (".github/workflows/data-home-inspection.yml", ".github/workflows/windows-native-engine.yml"):
            workflow = (ROOT / workflow_path).read_text()
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

    def test_workflow_keeps_migration_skips_exact_and_explicitly_unqualified(self):
        workflow = (ROOT / ".github/workflows/data-home-inspection.yml").read_text()
        stage = workflow.split("      - name: Stage closed Windows inspection inputs\n", 1)[1]
        body = stage.split("      - name:", 1)[0].split("        run: |\n", 1)[1]
        source = self.root / "stage.ps1"
        source.write_text(textwrap.dedent(body), encoding="utf-8")
        script = self.root / "migration-policy.ps1"
        script.write_text(r"""$ErrorActionPreference='Stop'
$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$assignment=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$skipPolicy'},$true))
if ($assignment.Count -ne 1) { throw 'Expected the actual inspection skip policy' }
Invoke-Expression $assignment[0].Extent.Text
$required=@{
 datahome=@('TestResolveCommittedMigrationUsesSealedSourceIdentity')
 zipruntime=@('TestMigratedPathsAcceptPhysicalSourceCaseIdentity','TestMigratedPathsRetainSourceAndContinueAcrossProducts','TestMigrationRollbackRequiresActualHomeSelection','TestNewCustomHomeStillRequiresExplicitInitialization','TestUnverifiedMigrationPathsHaveNoRuntimeEffects/pending','TestUnverifiedMigrationPathsHaveNoRuntimeEffects/plan-tamper','TestUnverifiedMigrationPathsHaveNoRuntimeEffects/commit-tamper','TestUnverifiedMigrationPathsHaveNoRuntimeEffects/revoked')
 'maestro-runtime'=@('TestExternalCLIApplyStatusAndRollbackKeepBindingExplicit')
}
foreach ($package in $required.Keys) {
 foreach ($test in $required[$package]) {
  $entry=@($skipPolicy[$package] | Where-Object { $_.test -ceq $test })
  if ($entry.Count -ne 1 -or $entry[0].reason -notmatch 'native migration coverage is missing') { throw ('Migration skip must remain explicitly unavailable: '+$test) }
 }
}
$children=@($skipPolicy.zipruntime | Where-Object { $_.test -like 'TestUnverifiedMigrationPathsHaveNoRuntimeEffects*' })
if ($children.Count -ne 4 -or @($children | Where-Object { $_.test -ceq 'TestUnverifiedMigrationPathsHaveNoRuntimeEffects' }).Count -ne 0) { throw 'Migration skips cannot blanket-authorize unknown child tests' }
foreach ($test in @('TestCodexLifecycleRequiresExplicitMigration','TestHookPreflightPreservesCommittedLegacyAndCaseAliases')) {
 if (@($skipPolicy.zipruntime | Where-Object { ($_.test -split '/')[0] -ceq $test }).Count) { throw 'Mandatory guarded contracts cannot be skipped' }
}
""", encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(source)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_guarded_migration_contracts_cannot_be_classified_as_skips(self):
        document = self.closed_manifest()
        for name in ["TestCodexLifecycleRequiresExplicitMigration", "TestHookPreflightPreservesCommittedLegacyAndCaseAliases/nested"]:
            with self.subTest(name=name):
                document["packages"][2]["skip_policy"] = [dict(test=name, reason="must not bypass")]
                self.assert_preaccount_refusal(self.invoke(document), "Both guarded migration contracts must run without skips")

    def test_changed_binary_refuses_before_account_creation(self):
        document = self.closed_manifest()
        document["packages"][0]["sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke(document), "Canonical test executable differs from its manifest")

    def test_changed_vendor_input_refuses_before_account_creation(self):
        document = self.closed_manifest()
        document["vendor_files"][0]["sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke(document), "Inspection vendored input differs from its sealed ordinary file")

    def launcher_fragment_check(self, assertions):
        script = self.root / "launcher-fragment.ps1"
        script.write_text(r"""$ErrorActionPreference='Stop'
$tokens=$null; $errors=$null
[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors) | Out-Null
if ($errors.Count) { throw ($errors | Out-String) }
$launcher=@($tokens | Where-Object { $_.Kind -eq 'HereStringLiteral' })
$ast=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$classifier=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$unclassified' -and $node.Right.Extent.Text -match '\$allSkips'},$true))
$acceptance=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$qualificationComplete' -and $node.Right.Extent.Text -match 'unclassified_skips'},$true))
if ($classifier.Count -ne 1 -or $acceptance.Count -ne 1) { throw 'Expected the actual classifier and acceptance assignments' }
""" + assertions, encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_actual_classifier_accepts_only_explicit_exact_skip_names(self):
        self.launcher_fragment_check(r"""
$entry=[pscustomobject]@{skip_policy=@(@{test='TestParent'},@{test='TestParent/known'})}
$allSkips=@('TestParent','TestParent/known')
Invoke-Expression $classifier[0].Extent.Text
if ($unclassified.Count -ne 0) { throw 'Explicit exact skip names were refused' }
""")

    def test_actual_classifier_refuses_unknown_child_beneath_allowed_parent(self):
        self.launcher_fragment_check(r"""
$entry=[pscustomobject]@{skip_policy=@(@{test='TestParent'})}
$allSkips=@('TestParent/new-child')
Invoke-Expression $classifier[0].Extent.Text
if ($unclassified.Count -ne 1 -or $unclassified[0] -cne 'TestParent/new-child') { throw 'Unknown nested skip inherited its parent policy' }
$results=@(@{exit_code=0; inventory_complete=$true; unclassified_skips=$unclassified; required_migration_tests_passed=$true})
Invoke-Expression $acceptance[0].Extent.Text
if ($qualificationComplete) { throw 'Unknown nested skip received execution acceptance' }
""")

    def test_nested_policy_requires_an_existing_parent_test(self):
        document = self.closed_manifest()
        document["packages"][0]["skip_policy"] = [dict(test="TestMissing/known", reason="orphan fixture")]
        self.assert_preaccount_refusal(self.invoke(document), "Inspection skips require unique existing tests")

    def test_explicit_nested_policy_reaches_the_next_custody_check(self):
        document = self.closed_manifest()
        document["packages"][0]["skip_policy"] = [dict(test="TestFixture/known", reason="explicit fixture subtest")]
        document["vendor_files"][0]["sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke(document), "Inspection vendored input differs from its sealed ordinary file")

    def test_actual_inspection_report_distinguishes_execution_and_coverage(self):
        self.launcher_fragment_check(r"""
$coverage=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$Inspection' -and $node.Extent.Text -match '\$report.all_contracts_qualified'},$true))
if ($coverage.Count -ne 1) { throw 'Expected the actual Inspection coverage report block' }
$Inspection=$true
foreach ($case in @(@{skips=@('TestParent','TestParent/known'); unknown=@(); accepted=$true; classified=2; qualified=$false},@{skips=@('TestParent/new'); unknown=@('TestParent/new'); accepted=$false; classified=0; qualified=$false},@{skips=@(); unknown=@(); accepted=$true; classified=0; qualified=$true})) {
 $qualificationComplete=$case.accepted
 $report=[ordered]@{qualification_complete=$qualificationComplete}
 $results=@(@{all_skipped_tests=$case.skips; unclassified_skips=$case.unknown})
 Invoke-Expression $coverage[0].Extent.Text
 if ($report.qualification_complete -ne $case.accepted -or $report.all_contracts_qualified -ne $case.qualified -or $report.observed_skips_count -ne $case.skips.Count -or $report.classified_skips_count -ne $case.classified -or $report.unclassified_skips_count -ne $case.unknown.Count -or -not $report.native_coverage) { throw 'Inspection execution acceptance hid missing native coverage' }
}
$Inspection=$false
$report=[ordered]@{qualification_complete=$true}
Invoke-Expression $coverage[0].Extent.Text
if ($report.Count -ne 1) { throw 'Inspection fields changed the default canonical report' }
""")

    def test_actual_private_fixture_guard_rejects_foreign_owner_and_acl(self):
        self.launcher_fragment_check(r"""
$guard=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -match '\$fixtureEntry'},$true))
if ($guard.Count -ne 1) { throw 'Expected the actual private fixture attestation guard' }
$fixtureEntry=Get-Item -LiteralPath $PSScriptRoot
$ExpectedUserSID='synthetic-current-account'
foreach ($case in @(@{owner='synthetic-foreign-account'; grant=$ExpectedUserSID},@{owner=$ExpectedUserSID; grant='synthetic-foreign-account'})) {
 $fixtureAcl=[pscustomobject]@{OwnerValue=$case.owner; RuleSID=$case.grant}
 $fixtureAcl | Add-Member ScriptMethod GetOwner { param($kind) [pscustomobject]@{Value=$this.OwnerValue} }
 $fixtureAcl | Add-Member ScriptMethod GetAccessRules { param($explicit,$inherited,$kind) @([pscustomobject]@{IdentityReference=[pscustomobject]@{Value=$this.RuleSID}; AccessControlType='Allow'}) }
 $refused=$false
 try { Invoke-Expression $guard[0].Extent.Text } catch { if ($_.Exception.Message -notmatch 'Inspection private fixtures must') { throw }; $refused=$true }
 if (-not $refused) { throw 'Foreign owner or effective ACL principal was accepted' }
}
""")

    def test_actual_fixture_bindings_replace_inherited_environment(self):
        self.launcher_fragment_check(r"""
$guard=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -match '\$fixtureEntry'},$true))
$bindings=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -cin @('$previousUserProfile','$previousHome','$env:USERPROFILE','$env:HOME','$inspectionProfile')},$true) | Sort-Object { $_.Extent.StartOffset })
if ($guard.Count -ne 1 -or $bindings.Count -ne 5) { throw 'Expected actual fixture guard and environment/provenance assignments' }
$userHomeFixture=$PSScriptRoot
$fixtureEntry=Get-Item -LiteralPath $userHomeFixture
$ExpectedUserSID='synthetic-current-account'
$fixtureAcl=[pscustomobject]@{OwnerValue=$ExpectedUserSID; RuleSID=$ExpectedUserSID}
$fixtureAcl | Add-Member ScriptMethod GetOwner { param($kind) [pscustomobject]@{Value=$this.OwnerValue} }
$fixtureAcl | Add-Member ScriptMethod GetAccessRules { param($explicit,$inherited,$kind) @([pscustomobject]@{IdentityReference=[pscustomobject]@{Value=$this.RuleSID}; AccessControlType='Allow'}) }
Invoke-Expression $guard[0].Extent.Text
$env:USERPROFILE='synthetic-inherited-runner-profile'; $env:HOME='synthetic-inherited-home'
foreach ($assignment in $bindings) { Invoke-Expression $assignment.Extent.Text }
if ($env:USERPROFILE -cne $userHomeFixture -or $env:HOME -cne $userHomeFixture -or $inspectionProfile.inherited_userprofile_matches_fixture -or $inspectionProfile.inherited_home_matches_fixture -or -not $inspectionProfile.private_fixture_roots_admitted -or $inspectionProfile.profile_resolution -cne 'inspection_private_fixture') { throw 'Inherited environment survived the admitted fixture binding' }
""")

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
