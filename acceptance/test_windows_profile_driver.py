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
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DRIVER = ROOT / "acceptance/windows-core-pilot.ps1"

# Canonical repository text is UTF-8, independently of the Windows ANSI default.


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
$inner=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$coreCalls=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.CommandElements.Count -gt 0 -and $n.CommandElements[0].Extent.Text -ceq '$TestBinary'},$true) | Sort-Object {$_.Extent.StartOffset})
if ($coreCalls.Count -eq 0) { throw 'Expected actual first native core test command' }
foreach($name in @('$env:TEMP','$env:TMP','$env:GOTMPDIR')) {
 $early=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $name -and $n.Right.Extent.Text -ceq '$temporary' -and $n.Extent.StartOffset -lt $coreCalls[0].Extent.StartOffset},$true))
 if($early.Count -ne 1){throw ('Ordinary child must select its own '+$name+' before the first native core test')}
}
"""
        script = self.root / "parse.ps1"
        script.write_text(check, encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_workflow_windows_blocks_parse(self):
        for workflow_path, expected_blocks in (
                (".github/workflows/data-home-inspection.yml", 3),
                (".github/workflows/windows-native-engine.yml", 4)):
            workflow = (ROOT / workflow_path).read_text(encoding="utf-8")
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
            self.assertEqual(len(blocks), expected_blocks, workflow_path)
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
        workflow = (ROOT / ".github/workflows/data-home-inspection.yml").read_text(encoding="utf-8")
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




class WindowsDoclingRuntimeProfile(unittest.TestCase):
    """The new converter profile is closed before the shared account mechanism."""

    def setUp(self):
        self.pwsh = shutil.which("pwsh")
        self.assertIsNotNone(self.pwsh)
        self.temp = tempfile.TemporaryDirectory(dir=os.environ.get("MAESTRO_TEST_TMP"))
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.manifest = self.root / "docling.json"

    def invoke(self, document, *extra):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        return subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(DRIVER),
                               "-DoclingManifest", str(self.manifest), "-OutputRoot", str(self.root / "proof"),
                               *extra], text=True, capture_output=True, timeout=30)

    def document(self):
        public = "C:\\MaestroNativeEngine-" + "a" * 32
        return dict(schema_version=1, qualification_profile="windows_docling_registry_read_r17", runtime_profile="model-free-documents-v1", runtime_lock_sha256="38b1a0257c3feec8a5b4b97856e7f2096cdb72ed2bd1734d8e61567bf4757643",
                    source_revision="a" * 40, source_tree="b" * 40, public_root=public, source_root=public + "\\source",
                    sdk_go=public + "\\sdk\\bin\\go.exe", module_cache=public + "\\modules",
                    raw_inputs=public + "\\raw", test_binary=public + "\\bin\\docling.test.exe",
                    inventory_sha256="a" * 64,
                    required_tests=["TestWindowsNativeConfinement", "TestWindowsNativeConfinementDeadline",
                                    "TestWindowsSharedImplementationIdentityAcrossEntryModules",
                                    "TestWindowsRealAuthenticatedDocling"])

    def assert_preaccount_refusal(self, document):
        result = self.invoke(document)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Docling requires the exact closed native runtime profile", result.stdout + result.stderr)
        self.assertFalse((self.root / "proof").exists())
        self.assertNotIn("New-LocalUser", result.stdout + result.stderr)

    def test_docling_rejects_legacy_profile_or_lock_before_account(self):
        for field, value in (("qualification_profile", "windows_docling_documents_v1"),
                             ("qualification_profile", "windows_docling_runtime_v1"),
                             ("runtime_profile", "policy-formats-v1"),
                             ("runtime_lock_sha256", "728aab0b59dd5011070dfc4e9159e3ed6180b1668539f84275f1b88e81f5be58")):
            document = self.document()
            document[field] = value
            self.assert_preaccount_refusal(document)

    def test_docling_rejects_unknown_profile_before_account(self):
        document = self.document()
        document["qualification_profile"] = "arbitrary_commands"
        self.assert_preaccount_refusal(document)

    def test_docling_requires_exact_source_tree_before_account(self):
        document = self.document()
        document.pop("source_tree")
        self.assert_preaccount_refusal(document)
        document = self.document()
        document["source_tree"] = "b" * 39 + "Z"
        self.assert_preaccount_refusal(document)

    def test_docling_failure_stops_without_replaying_converter(self):
        # Exercise the actual failure branch with an inert trace stub. No native
        # proof, account, registry fixture, debugger or converter is invoked.
        helper = ROOT / "acceptance/windows-docling-profile.ps1"
        script = self.root / "placement-trace-condition.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$e=$null
$tree=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$e)
if($e.Count){throw 'Actual Docling helper must parse'}
$dispatch=@($tree.FindAll({param($n)
 $n -is [Management.Automation.Language.IfStatementAst] -and
 $n.Clauses[0].Item1.Extent.Text.Contains('$exit -ne 0 -or $body -match')
},$true))
if($dispatch.Count -ne 1){throw 'Expected original native failure boundary'}
function Invoke-DoclingFailureTrace { $script:replays++ }
foreach($case in @(
 @{exit=1;body='    native_windows_test.go:121: MAESTRO_NATIVE_RUNTIME_FAILURE operation=place stage=launch process_status=0xc0000022'},
 @{exit=1;body='--- FAIL: synthetic'},
 @{exit=0;body='--- SKIP: synthetic'}
)) {
 $exit=$case.exit;$body=$case.body;$script:replays=0;$failed=$false
 try {Invoke-Expression $dispatch[0].Extent.Text} catch {$failed=$true}
 if(-not $failed -or $script:replays -ne 0){throw 'Failed or skipped coverage must stop without a native replay'}
}
Write-Output 'failure_stops_without_replay=true'
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File",
                                 str(script), str(helper)],
                                text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("failure_stops_without_replay=true", result.stdout)

    def test_docling_bootstrap_environment_is_closed_before_creation_and_restored(self):
        # Execute the real launch barrier around a harmless stub. A real .NET
        # ProcessStartInfo observes its inherited environment; no process starts.
        script = self.root / "docling-launch-environment.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$tokens=$null;$errors=$null
$tree=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Original driver must parse'}
$calls=@($tree.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -ceq 'Start-Process' -and $n.Extent.Text.Contains('-Credential $credential')},$true))
if($calls.Count -ne 1){throw 'Expected the original credentialed launch'}
$barrier=$calls[0].Parent
while($barrier -and $barrier -isnot [Management.Automation.Language.TryStatementAst]){$barrier=$barrier.Parent}
if(-not $barrier){throw 'Expected original launch restoration boundary'}
$builder=@($tree.FindAll({param($n)
 $n -is [Management.Automation.Language.IfStatementAst] -and $n.Clauses[0].Item1.Extent.Text -ceq '$withDocling' -and
 @($n.Clauses[0].Item2.Statements | Where-Object {$_ -is [Management.Automation.Language.AssignmentStatementAst] -and $_.Left.Extent.Text -ceq '$doclingLaunchEnvironment'}).Count -eq 1
},$true))
$withDocling=$true;$OutputRoot=$PSScriptRoot
$doclingSystemRoot=Join-Path $PSScriptRoot 'windows';$doclingSystemDirectory=Join-Path $doclingSystemRoot 'System32'
$doclingRuntime=@{sdk_go=(Join-Path $PSScriptRoot 'sdk/bin/go.exe');module_cache=(Join-Path $PSScriptRoot 'modules')}
$powershell=$PSHOME+'/pwsh';$arguments=@('synthetic');$credential=$null;$stdout='unused';$stderr='unused'
$env:GITHUB_ACTIONS='true';$env:RUNNER_ENVIRONMENT='github-hosted'
$env:GH_TOKEN='synthetic-gh';$env:GITHUB_TOKEN='synthetic-github';$env:ACTIONS_RUNTIME_TOKEN='synthetic-actions'
$env:UNRELATED_SYNTHETIC_SECRET='synthetic-unlisted';$env:USERPROFILE='synthetic-runner-home';$env:HOME='synthetic-runner-home'
function Start-Process {
 param($FilePath,$ArgumentList,$Credential,[switch]$LoadUserProfile,$WorkingDirectory,$RedirectStandardOutput,$RedirectStandardError,[switch]$Wait,[switch]$PassThru)
 $script:launches++
 $info=[Diagnostics.ProcessStartInfo]::new()
 $visible=$info.Environment
 foreach($key in @('GH_TOKEN','GITHUB_TOKEN','ACTIONS_RUNTIME_TOKEN','UNRELATED_SYNTHETIC_SECRET')) {
  if($visible.ContainsKey($key)){throw 'Inherited credential canary reached process creation'}
 }
 $expected=@('SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','PSModulePath','TEMP','TMP','GOTMPDIR','USERPROFILE','HOME','APPDATA','LOCALAPPDATA','GOROOT','GOMODCACHE','GOCACHE','GOPATH','GOTOOLCHAIN','GOPROXY','GOSUMDB','GOFLAGS','CGO_ENABLED','GITHUB_ACTIONS','RUNNER_ENVIRONMENT')
 if(@(Compare-Object ($expected|Sort-Object) (@($visible.Keys)|Sort-Object)).Count -ne 0){throw 'Bootstrap environment is not the closed key set'}
 $subjectHome=Join-Path $OutputRoot 'docling-user-home'
 if($visible['HOME'] -cne $subjectHome -or $visible['USERPROFILE'] -cne $subjectHome -or $visible['GOMODCACHE'] -cne $doclingRuntime.module_cache -or
  $visible['GOCACHE'] -cne (Join-Path $OutputRoot 'go-cache') -or $visible['GOPATH'] -cne (Join-Path $OutputRoot 'go-path') -or
  $visible['PATH'] -notlike ('*'+(Split-Path -Parent $doclingRuntime.sdk_go)+'*') -or $visible['PSModulePath'] -cne (Join-Path $PSHOME 'Modules') -or
  $visible['GITHUB_ACTIONS'] -cne 'true' -or $visible['RUNNER_ENVIRONMENT'] -cne 'github-hosted') {throw 'Required resolved nonsecret bootstrap binding was lost'}
 if($script:failLaunch){throw 'synthetic launch failed'}
 return @{ExitCode=0}
}
foreach($fail in @($false,$true)) {
 $script:failLaunch=$fail;$script:launches=0;$doclingParentEnvironment=$null
 $hadModulePath=Test-Path Env:PSModulePath;$priorModulePath=$env:PSModulePath
 $before=[Environment]::GetEnvironmentVariables('Process')
 if($builder.Count -eq 1){Invoke-Expression $builder[0].Extent.Text}
 $caught=$null
 try {Invoke-Expression $barrier.Extent.Text} catch {$caught=$_.Exception.Message}
 $after=[Environment]::GetEnvironmentVariables('Process')
 if($script:launches -ne 1 -or ($fail -and $caught -cne 'synthetic launch failed') -or (-not $fail -and $null -ne $caught)){
  $category=switch -Exact ($caught) {
   'Inherited credential canary reached process creation' {'inherited_canary'}
   'Bootstrap environment is not the closed key set' {'unexpected_keys'}
   'Required resolved nonsecret bootstrap binding was lost' {'required_binding'}
   'Docling bootstrap environment could not be closed.' {'barrier_keys'}
   default {'unexpected_launch_error'}
  }
  throw ('Actual bootstrap environment contract failed: '+$category)
 }
 if($before.Count -ne $after.Count){throw 'Parent environment was not restored'}
 foreach($key in $before.Keys){if(-not $after.Contains($key) -or $before[$key] -cne $after[$key]){throw 'Parent environment was not restored'}}
}
Write-Output 'closed_creation_and_parent_restore=true'
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File",
                                 str(script), str(DRIVER)], text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("closed_creation_and_parent_restore=true", result.stdout)

    def test_docling_account_removal_failure_is_fatal_and_clears_credentials(self):
        # Run only the existing teardown block with a synthetic removal stub.
        script = self.root / "docling-account-cleanup.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$errors=$null
$tree=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$errors)
if($errors.Count){throw 'Original driver must parse'}
$cleanup=@($tree.FindAll({param($n)
 $n -is [Management.Automation.Language.TryStatementAst] -and $n.Finally -and
 @($n.Finally.FindAll({param($c) $c -is [Management.Automation.Language.CommandAst] -and $c.GetCommandName() -ceq 'Remove-LocalUser'},$true)).Count -gt 0
},$true))
if($cleanup.Count -ne 1){throw 'Expected original credential cleanup owner'}
function Remove-LocalUser {
 param($SID,$ErrorAction)
 $script:removals++
 if($ErrorAction -cne 'Stop'){throw 'Account removal must fail closed'}
 if($script:failRemoval){throw 'synthetic removal failed'}
}
foreach($fail in @($false,$true)) {
 $withDocling=$true;$account=@{SID='synthetic-account'};$script:removals=0;$script:failRemoval=$fail
 $password='synthetic';$secure='synthetic';$credential='synthetic';$caught=$null
 try {$output=@(Invoke-Expression ($cleanup[0].Finally.Statements.Extent.Text -join "`n"))} catch {$caught=$_.Exception.Message}
 if($script:removals -ne 1 -or $null -ne $password -or $null -ne $secure -or $null -ne $credential){throw 'Account cleanup did not clear credential references'}
 if($fail) {
  if($caught -cne 'Docling disposable account removal failed; trial incomplete.'){throw 'Removal failure was not a finite fatal outcome'}
 } elseif($null -ne $caught -or $output -cnotcontains 'docling_subject_cleanup account_removal_completed=true'){throw 'Successful account removal was not observed'}
}
Write-Output 'account_cleanup_fails_closed=true'
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File",
                                 str(script), str(DRIVER)], text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("account_cleanup_fails_closed=true", result.stdout)

    def test_docling_checkout_does_not_persist_credentials_or_add_automatic_triggers(self):
        workflow = (ROOT / ".github/workflows/windows-docling-runtime.yml").read_text(encoding="utf-8")
        self.assertRegex(workflow, r"uses: actions/checkout@[^\n]+\n        with:\n          persist-credentials: false\n")
        for trigger in ("pull_request:", "push:", "schedule:", "workflow_run:", "workflow_call:"):
            self.assertNotIn(trigger, workflow)

    def test_registry_assessment_decodes_complete_matrix_and_rejects_invalid_rows(self):
        # Invoke only the pure decoder in the existing helper. No ordinary-user
        # driver, Windows API, token, registry fixture or test executable runs.
        helper = ROOT / "acceptance/windows-docling-profile.ps1"
        script = self.root / "decode-r17.ps1"
        script.write_text(
            "$ErrorActionPreference='Stop'\n. $args[0]\n"
            "ConvertFrom-R17RegistryAssessmentLog ([IO.File]::ReadAllText($args[1])) | ConvertTo-Json -Depth 8\n",
            encoding="utf-8")
        log = self.root / "assessment.log"
        fixtures = ("owner_only", "capability_read", "capability_broad", "foreign_capability", "integrity_pair")
        rights = ("query_value", "enumerate_sub_keys", "set_value", "create_sub_key", "delete", "write_dac")
        definition = ("R17_REGISTRY_DEFINITION version=maestro-r17-registry-assessment-v1 "
                      "sha256=024fcb4fe0a6d92eaff4a933c5d25dc7c9fb7a63a68b778839c79458fd50be8e")
        rows = [f"R17_REGISTRY_OBSERVATION phase={phase} fixture_enum={fixture} right_enum={right} win32_status=5 sentinel_match=false"
                for phase in ("first", "second") for fixture in fixtures for right in rights]
        complete = "\n".join(["R17_REGISTRY_CLEANUP state=complete", definition, *rows]) + "\n"

        def decode(body):
            log.write_text(body, encoding="utf-8")
            return subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(script), str(helper), str(log)],
                                  text=True, capture_output=True, timeout=30)

        denied = decode(complete)
        self.assertEqual(denied.returncode, 0, denied.stdout + denied.stderr)
        receipt = json.loads(denied.stdout)
        self.assertTrue(receipt["complete"])
        self.assertTrue(receipt["all_denied"])
        self.assertFalse(receipt["discriminating"])
        self.assertEqual(len(receipt["observations"]), 60)
        self.assertEqual(receipt["phase_ordinals"], [{"phase": "first", "ordinal": 1}, {"phase": "second", "ordinal": 2}])
        readable = complete.replace("fixture_enum=capability_read right_enum=query_value win32_status=5 sentinel_match=false",
                                    "fixture_enum=capability_read right_enum=query_value win32_status=0 sentinel_match=true")
        measured = decode(readable)
        self.assertEqual(measured.returncode, 0, measured.stdout + measured.stderr)
        self.assertTrue(json.loads(measured.stdout)["discriminating"])
        self.assertFalse(json.loads(measured.stdout)["all_denied"])
        invalid = {
            "missing": complete.replace(rows[0] + "\n", "", 1),
            "duplicate": complete + rows[0] + "\n",
            "unknown_fixture": complete.replace("fixture_enum=owner_only", "fixture_enum=foreign", 1),
            "unknown_phase": complete.replace("phase=first", "phase=third", 1),
            "overflow": complete.replace("win32_status=5", "win32_status=4294967296", 1),
            "negative_status": complete.replace("win32_status=5", "win32_status=-1", 1),
            "noncanonical_status": complete.replace("win32_status=5", "win32_status=05", 1),
            "unexpected_field": complete.replace(rows[0], rows[0] + " extra=1", 1),
            "sentinel_without_read": complete.replace("sentinel_match=false", "sentinel_match=true", 1),
            "successful_wrong_sentinel": complete.replace("win32_status=5", "win32_status=0", 1),
            "bad_definition": complete.replace("sha256=024f", "sha256=124f", 1),
            "duplicate_definition": complete + definition + "\n",
            "missing_cleanup": complete.replace("R17_REGISTRY_CLEANUP state=complete\n", "", 1),
            "diagnostic": complete + "R17_REGISTRY_DIAGNOSTIC stage=internal win32_status=13\n",
            "raw_child_row": complete + "R17_REGISTRY_ROW fixture_enum=owner_only right_enum=query_value win32_status=5 sentinel_match=false\n",
            "truncated": complete[:-20],
            "extra_carriage_return": complete.replace(rows[0] + "\n", rows[0] + "\r\r\n", 1),
        }
        for field in ("subkeys", "max_subkey", "values", "max_value_name", "max_value_bytes",
                      "security_bytes", "last_write", "multiple"):
            invalid["metadata_" + field] = complete + f"R17_REGISTRY_DIAGNOSTIC stage=leaf_metadata_{field}_changed win32_status=13\n"
        for name, body in invalid.items():
            with self.subTest(name=name):
                self.assertNotEqual(decode(body).returncode, 0)

    def test_registry_assessment_requires_existing_hosted_qualification_markers(self):
        helper = ROOT / "acceptance/windows-docling-profile.ps1"
        script = self.root / "r17-hosted-guard.ps1"
        script.write_text(
            "$ErrorActionPreference='Stop'\n. $args[0]\n"
            "Test-R17HostedAssessment | ConvertTo-Json\n", encoding="utf-8")
        for qualification in ("", "1"):
            for actions in ("", "true"):
                for runner in ("", "github-hosted"):
                    with self.subTest(qualification=qualification, actions=actions, runner=runner):
                        env = os.environ.copy()
                        env.update(MAESTRO_WINDOWS_DOCLING_QUALIFY=qualification,
                                   GITHUB_ACTIONS=actions, RUNNER_ENVIRONMENT=runner)
                        result = subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(script), str(helper)],
                                                env=env, text=True, capture_output=True, timeout=30)
                        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                        self.assertEqual(json.loads(result.stdout),
                                         qualification == "1" and actions == "true" and runner == "github-hosted")

    def test_docling_rejects_open_command_field_before_account(self):
        document = self.document()
        document["command"] = "anything"
        self.assert_preaccount_refusal(document)

    def test_docling_requires_all_native_tests_without_substitution(self):
        document = self.document()
        document["required_tests"] = document["required_tests"][:-1]
        self.assert_preaccount_refusal(document)
        document = self.document()
        document["required_tests"][-1] = "TestDummyConverter"
        self.assert_preaccount_refusal(document)

    def test_docling_and_old_profiles_are_exclusive(self):
        result = self.invoke(self.document(), "-InspectionManifest", str(self.manifest))
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "proof").exists())

    def test_docling_helper_and_manual_workflow_parse(self):
        helper = ROOT / "acceptance/windows-docling-profile.ps1"
        script = self.root / "parse.ps1"
        script.write_text("$errors=$null; [Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$errors)|Out-Null; if($errors.Count){throw ($errors|Out-String)}", encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-File", str(script), str(helper)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        workflow = (ROOT / ".github/workflows/windows-docling-runtime.yml").read_text(encoding="utf-8")
        self.assertIn("workflow_dispatch:", workflow)
        self.assertNotIn("pull_request:", workflow)
        self.assertIn("./acceptance/windows-core-pilot.ps1 -DoclingManifest", workflow)

    def test_registry_read_experiment_rejects_export_before_staging(self):
        import textwrap
        workflow = (ROOT / ".github/workflows/windows-docling-runtime.yml").read_text(encoding="utf-8")
        step = workflow.split("      - name: Validate closed dispatch and create fresh Developer staging\n", 1)[1].split("      - name:", 1)[0]
        script = self.root / "closed-dispatch.ps1"
        script.write_text(textwrap.dedent(step.split("        run: |\n", 1)[1]), encoding="utf-8")
        # An existing profile is legitimate; the workflow owns only its staging.
        profile = self.root / "synthetic-host"
        profile.mkdir()
        developer = profile / "Developer"
        staging = developer / "maestro-windows-kit-1729-1"
        for mode in ("export", "assemble", "unknown"):
            env = os.environ.copy()
            for name in ("PILOT_REGISTRY", "PILOT_N", "PILOT_N1", "PILOT_INSTALLER", "PILOT_UNSIGNED_RUN", "PILOT_EXPORT_SHA", "PILOT_METADATA"):
                env[name] = ""
            env.update(PILOT_MODE=mode, USERPROFILE=str(profile), GITHUB_RUN_ID="1729", GITHUB_RUN_ATTEMPT="1")
            result = subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(script)], env=env, text=True, capture_output=True, timeout=30)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("R17 registryRead experiment is proof-only", result.stdout + result.stderr)
            self.assertFalse(developer.exists())
            self.assertFalse(staging.exists())

    def test_docling_public_acquisition_python_parses_without_execution(self):
        import ast
        source = (ROOT / "acceptance/windows-engine-build.ps1").read_text(encoding="utf-8")
        acquisition = source.split("@'\nimport hashlib,json,os,pathlib,urllib.request\n", 1)[1].split("\n'@ | python -", 1)[0]
        ast.parse("import hashlib,json,os,pathlib,urllib.request\n" + acquisition)


if __name__ == "__main__":
    unittest.main()

class WindowsEngineProfile(unittest.TestCase):
    setUp = WindowsInspectionDriver.setUp
    closed_manifest = WindowsInspectionDriver.closed_manifest
    assert_preaccount_refusal = WindowsInspectionDriver.assert_preaccount_refusal

    def invoke_engine(self, document, *extra):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        binary = self.root / "public.exe"
        binary.write_bytes(b"unexecuted public executable fixture")
        return subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(DRIVER),
                               "-TestBinary", str(binary), "-Server", str(binary),
                               "-CanonicalManifest", str(self.manifest), "-OutputRoot", str(self.root / "proof"), *extra],
                              capture_output=True, text=True, timeout=30)

    def engine_document(self):
        document = self.closed_manifest()
        adapter_vendor = Path(document["source_root"]) / "adapters/mcp/vendor/modules.txt"
        adapter_vendor.parent.mkdir(parents=True)
        adapter_vendor.write_text("sealed fixture", encoding="utf-8")
        for entry in document["packages"]:
            entry.pop("skip_policy")
        digest = hashlib.sha256(b"unexecuted public executable fixture").hexdigest()
        document.update(qualification_profile="windows_private_engine_v1",
                        public_test_binary_sha256=digest, public_server_sha256=digest,
                        adapter_profile="windows_private_engine_adapters_v1", adapter_packages=[], adapter_vendor_files=document["vendor_files"], authored_package={})
        return document

    def test_engine_unknown_profile_refuses_before_account(self):
        document = self.engine_document()
        document["qualification_profile"] = "windows_private_engine_v999"
        self.assert_preaccount_refusal(self.invoke_engine(document), "Unknown canonical qualification profile")

    def test_engine_package_subset_refuses_before_account(self):
        self.assert_preaccount_refusal(self.invoke_engine(self.engine_document()), "Engine qualification requires exactly the nine owning packages")

    def test_engine_public_executable_hash_is_bound(self):
        document = self.engine_document()
        document["public_server_sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke_engine(document), "Engine public executables differ from the sealed source manifest")

    def exact_kit_fixture(self, inventory_newline=None):
        kit = self.root / "Maestro-Pilot-Artifact"
        (kit / "bin").mkdir(parents=True)
        revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
        tree = subprocess.check_output(["git", "rev-parse", "HEAD^{tree}"], cwd=ROOT, text=True).strip()
        server = b"unexecuted public executable fixture"
        (kit / "bin/maestro-mcp-windows-amd64.exe").write_bytes(server)
        (kit / "ARTIFACT.txt").write_text(f"binary_source_base_commit={revision}\nsource_tree={tree}\ntrust=unsigned_engineering_candidate\n", encoding="utf-8")
        rows = "".join(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  ./{path.relative_to(kit).as_posix()}\n"
                       for path in sorted(kit.rglob("*")) if path.is_file())
        (kit / "SHA256SUMS").write_text(rows, encoding="utf-8", newline=inventory_newline)
        # The identity binds stored bytes; text writes use CRLF on Windows.
        kit_id = hashlib.sha256((kit / "SHA256SUMS").read_bytes()).hexdigest()
        (kit / "KIT-ID.txt").write_text(kit_id + "\n", encoding="utf-8")
        archive = Path(str(kit) + ".zip")
        with zipfile.ZipFile(archive, "w") as zipped:
            for path in sorted(kit.rglob("*")):
                if path.is_file():
                    zipped.write(path, kit.name + "/" + path.relative_to(kit).as_posix())
        zip_hash = hashlib.sha256(archive.read_bytes()).hexdigest()
        Path(str(archive) + ".sha256").write_text(zip_hash + "  " + archive.name + "\n", encoding="utf-8")
        identity = dict(schema_version=1, source_revision=revision, source_tree=tree, producer_run_id=os.environ.get("GITHUB_RUN_ID", "123456"),
                        generic_zip_sha256=zip_hash, kit_id=kit_id, shipping_server_sha256=hashlib.sha256(server).hexdigest())
        identity_path = self.root / "pilot-artifact-input.json"
        identity_path.write_text(json.dumps(identity), encoding="utf-8")
        return kit, archive, identity_path, identity

    def test_exact_kit_refuses_changed_inputs_before_account(self):
        document = self.engine_document()
        kit, archive, identity_path, identity = self.exact_kit_fixture(inventory_newline="\r\n")
        original_zip = archive.read_bytes()
        server = kit / "bin/maestro-mcp-windows-amd64.exe"
        original_server = server.read_bytes()
        fixture = ROOT / "adapters/mcp/packaging/testdata/method-ids.txt"
        cases = [("zip", "Exact pilot ZIP checksum differs"),
                 ("server", "Exact pilot PE checksum differs"),
                 ("source", "Exact pilot source differs"),
                 ("missing", "Exact pilot identity is missing")]
        for kind, message in cases:
            with self.subTest(kind=kind):
                archive.write_bytes(original_zip)
                server.write_bytes(original_server)
                current = dict(identity)
                selected_identity = identity_path
                if kind == "zip": archive.write_bytes(original_zip + b"changed")
                if kind == "server": server.write_bytes(original_server + b"changed")
                if kind == "source": current["source_revision"] = "0" * 40
                if kind == "missing": selected_identity = self.root / "missing-identity.json"
                identity_path.write_text(json.dumps(current), encoding="utf-8")
                result = self.invoke_engine(document, "-Kit", str(kit), "-KitIdentity", str(selected_identity), "-MethodFixture", str(fixture))
                self.assert_preaccount_refusal(result, message)

        archive.write_bytes(original_zip)
        server.write_bytes(original_server)
        identity_path.write_text(json.dumps(identity), encoding="utf-8")
        result = self.invoke_engine(document, "-Kit", str(kit), "-KitIdentity", str(identity_path), "-MethodFixture", str(fixture))
        self.assert_preaccount_refusal(result, "Engine qualification requires exactly the nine owning packages")

    def test_exact_kit_requires_complete_matched_evidence(self):
        _, _, identity_path, identity = self.exact_kit_fixture()
        proof = dict(identity, passed=True, powershell_edition="Desktop", powershell_version="5.1.26100.1", command_test_image_sha256="c" * 64, prepared_server_sha256=identity["shipping_server_sha256"],
                     packet_server_sha256=identity["shipping_server_sha256"], desktop_packet_server_sha256=identity["shipping_server_sha256"], workspace_id="a" * 32,
                     emitted_review_confirmed=True, private_and_managed_unchanged=True, sdk_context_verified=True,
                     canonical_methods=92, skill_folders=94, reports_count=3, native_Code_Desktop="not_run")
        proof_path = self.root / "native-kit-proof.json"
        proof_path.write_text(json.dumps(proof), encoding="utf-8")
        script = self.root / "artifact-proof-check.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$null)
$functions=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Assert-PilotArtifactProof'},$true))
if($functions.Count -ne 1){throw 'Expected the existing driver to own exact artifact evidence validation'}
Invoke-Expression $functions[0].Extent.Text
$identity=Get-Content -LiteralPath $args[1] -Raw | ConvertFrom-Json
$proofPath=$args[2]
$null=Assert-PilotArtifactProof $identity $proofPath
foreach($kind in @('missing','zip','prepared','workspace','preservation','sdk','malformed-boolean','preparer-version','preparer-edition')){
 $copy=Get-Content -LiteralPath $proofPath -Raw | ConvertFrom-Json
 switch($kind){
  zip {$copy.generic_zip_sha256='0'*64}
  prepared {$copy.prepared_server_sha256='0'*64}
  workspace {$copy.workspace_id=''}
  preservation {$copy.private_and_managed_unchanged=$false}
  sdk {$copy.sdk_context_verified=$false}
  malformed-boolean {$copy.passed='true'}
  preparer-version {$copy.powershell_version='7.5.0'}
  preparer-edition {$copy.powershell_edition='Core'}
 }
 $selected=$proofPath+'.invalid';$copy|ConvertTo-Json|Set-Content -LiteralPath $selected
 if($kind -ceq 'missing'){$selected=$proofPath+'.absent'}
 $refused=$false
 try{$null=Assert-PilotArtifactProof $identity $selected}catch{$refused=$true}
 if(-not $refused){throw ('Missing or mismatched exact artifact evidence accepted: '+$kind)}
}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER), str(identity_path), str(proof_path)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_exact_kit_retains_engine_host(self):
        script = self.root / "artifact-engine-shell.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$body=Get-Content -LiteralPath $args[0] -Raw
$ast=[Management.Automation.Language.Parser]::ParseInput($body,[ref]$null,[ref]$null)
$assignments=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq '$powershell'},$true)|Sort-Object {$_.Extent.StartOffset})
$start=$assignments[0].Extent.StartOffset
$finish=$body.IndexOf('    $hadModulePath =',$start)
if($start -lt 0 -or $finish -le $start){throw 'Expected actual credentialed engine shell selection'}
$env:SystemRoot=Join-Path $args[1] 'synthetic-windows'
$withInspection=$false;$withManifest=$false;$withKit=$true
$AtlasTestBinary=$null;$DoclingTestBinary=$null;$AuthoredBrain=$false;$PrivateWiki=$false
$launcher='synthetic-launcher';$OutputRoot='synthetic-output';$nativeTest='synthetic-test';$nativeKit='synthetic-kit'
Invoke-Expression $body.Substring($start,$finish-$start)
if($powershell -cne (Join-Path $PSHOME 'pwsh.exe')){throw 'Exact artifact mode replaced the engine qualifier host; strict JSON case-pair classification requires PowerShell 7'}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER), str(self.root)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_engine_builder_parses(self):
        parser = self.root / "builder-parser.ps1"
        parser.write_text("$e=$null; [Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$e) | Out-Null; if($e.Count){throw ($e | Out-String)}", encoding="utf-8")
        for name in ["windows-engine-build.ps1", "windows-engine-check.ps1", "windows-engine-evidence.ps1"]:
            result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(parser), str(ROOT / "acceptance" / name)], capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_engine_proof_rejects_unknown_skip_duplicate_and_tampered_case_identity(self):
        script = self.root / "engine-proof.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$tokens=$null; $errors=$null
$driverAST=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors)
if($errors.Count){throw ($errors | Out-String)}
$launcher=@($tokens | Where-Object {$_.Kind -eq 'HereStringLiteral'})
if($launcher.Count -ne 1){throw 'Expected the actual shared launcher'}
$ast=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$errors)
foreach($name in @('Get-CanonicalLogRecords','Test-CanonicalObservedPass','Test-CanonicalCasePair','Get-CanonicalPackageProof','Get-EnginePublicProof')) {
 $found=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
 if($found.Count -ne 1){throw ('Expected actual profile helper '+$name)}
 Invoke-Expression $found[0].Extent.Text
}
$entry=@{tests=@('TestOriginal','TestNative'); canonical_required_tests=@('TestNative'); canonical_original_assertions=@('TestOriginal'); canonical_skip_policy=@(@{test='TestOriginal';reason='exact unavailable capability';kind='platform-counterpart';paired='TestNative'})}
$body="=== RUN   TestOriginal`n    fixture_test.go:1: exact unavailable capability`n--- SKIP: TestOriginal (0.00s)`n=== RUN   TestNative`n--- PASS: TestNative (0.00s)`n"
$valid=Get-CanonicalPackageProof $entry $body 0 $true
if(-not $valid.qualified -or $valid.classified_skips.Count -ne 1 -or $valid.original_assertions[0].original_outcome -cne 'SKIP'){throw ('Exact native pair must preserve the original SKIP: '+($valid | ConvertTo-Json -Depth 6 -Compress)+' records='+(@(Get-CanonicalLogRecords $body)|ConvertTo-Json -Compress))}
foreach($invalid in @(($body.Replace('exact unavailable capability','unknown capability')),($body+"--- PASS: TestNative (0.00s)`n"),($body.Replace('--- PASS: TestNative','--- SKIP: TestNative')),($body.Replace('TestNative','TestUnknown')))) {
 if((Get-CanonicalPackageProof $entry $invalid 0 $true).qualified){throw ('Unknown, duplicate, missing or skipped native evidence qualified: '+($invalid | ConvertTo-Json -Compress))}
}

# Exercise the exact shipped Darwin records through the original proof owner.
foreach($name in @('$canonicalRequired','$canonicalPolicies')) {
 $found=@($driverAST.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $name},$true))
 if($found.Count -ne 1){throw 'Expected one actual canonical source policy'}
 Invoke-Expression $found[0].Extent.Text
}
$expectedDarwin=@{
 TestEvolutionRecoveryRejectsSymlinkedNamespace='TestNativeEvolutionPersistenceIsExplicitlyUnavailable'
 TestEvolutionStoreRejectsSymlinkedRoot='TestNativeEvolutionPersistenceIsExplicitlyUnavailable'
 TestEvolutionStoreRejectsSymlinkedAncestorAboveRoot='TestNativeEvolutionPersistenceIsExplicitlyUnavailable'
 TestProposalStoreRejectsSymlinkedArtifact='TestProductDarwinWindowsRetainedTransaction/reparse_artifact'
 TestProposalStoreRejectsSymlinkedProposalDirectory='TestProductDarwinWindowsRetainedTransaction/reparse_proposal_directory'
 TestProposalStoreRejectsSymlinkedAncestorBeforeCreatingStore='TestProductDarwinWindowsRetainedTransaction/reparse_ancestor'
}
$darwin=@($canonicalPolicies['darwin'])
if($darwin.Count -ne 6 -or @((Compare-Object (@($expectedDarwin.Keys)|Sort-Object) (@($darwin.test)|Sort-Object) -CaseSensitive)).Count -ne 0){throw 'Darwin legacy classification must remain the exact six named cases'}
foreach($rule in $darwin) {
 $expectedReason=if($rule.test.StartsWith('TestEvolution')){'symlink creation requires platform-specific privileges on Windows'}else{'legacy symlink fixture lacks Windows creation privilege'}
 if($rule.reason -cne $expectedReason -or $rule.paired -cne $expectedDarwin[$rule.test] -or $rule.paired -cnotin $canonicalRequired['darwin']){throw 'Darwin legacy reason/counterpart/mandatory selection changed'}
 $parent=($rule.paired -split '/')[0]
 $entry=@{tests=@($rule.test,$parent);canonical_required_tests=@($rule.paired);canonical_original_assertions=@($rule.test);canonical_skip_policy=@($rule)}
 $pairedPass="--- PASS: $($rule.paired) (0.00s)`n"
 $actual="=== RUN   $($rule.test)`n    fixture_test.go:1: $($rule.reason)`n--- SKIP: $($rule.test) (0.00s)`n=== RUN   $parent`n"
 if($parent -cne $rule.paired){$actual+="=== RUN   $($rule.paired)`n"}
 $actual+=$pairedPass
 if($parent -cne $rule.paired){$actual+="--- PASS: $parent (0.00s)`n"}
 $observed=Get-CanonicalPackageProof $entry $actual 0 $true
 if(-not $observed.qualified -or $observed.classified_skips.Count -ne 1 -or $observed.original_assertions[0].original_outcome -cne 'SKIP'){throw 'Exact Darwin counterpart must retain its observed legacy SKIP'}
 foreach($invalid in @(($actual.Replace($rule.reason,'unknown arbitrary refusal')),($actual.Replace($pairedPass,'')),($actual.Replace('PASS: '+$rule.paired,'SKIP: '+$rule.paired)),($actual+$pairedPass),($actual+"=== RUN   TestUnknownSkip`n    fixture_test.go:1: unknown`n--- SKIP: TestUnknownSkip (0.00s)`n"))) {
  if((Get-CanonicalPackageProof $entry $invalid 0 $true).qualified){throw 'Darwin unknown reason/skip or absent/skipped/duplicate counterpart qualified'}
 }
}

$policy=@{paired='TestOriginal/same-object-case-contract'}
$identity=@{kind='selected-directory-case-pair-v1';scope=$policy.paired;selected='C:\owner\state';counterpart='C:\owner\STATE';selected_identity=@(1,2,3);counterpart_identity=@(1,2,3);same_object=$true}
# Absolute Windows paths are evaluated by the actual Windows parser in CI.
# Use native rooted case spellings locally without pretending to observe NTFS.
if(-not [IO.Path]::IsPathRooted($identity.selected)){$identity.selected='/owner/state';$identity.counterpart='/owner/STATE'}
$records=@([pscustomobject]@{test=$policy.paired;text=('case_pair_evidence='+($identity|ConvertTo-Json -Compress))})
if(-not (Test-CanonicalCasePair $policy $records)){throw 'Well-typed same-object evidence should parse'}
$identity.counterpart_identity=@(1,2,4)
$records[0].text='case_pair_evidence='+($identity|ConvertTo-Json -Compress)
if(Test-CanonicalCasePair $policy $records){throw 'Different native identities were accepted'}
$identity.counterpart_identity=@(1,2,3);$identity.unexpected=$true
$records[0].text='case_pair_evidence='+($identity|ConvertTo-Json -Compress)
if(Test-CanonicalCasePair $policy $records){throw 'Unknown evidence field was accepted'}
$public=@{required_tests=@('TestPublic');required_assertions=@('TestPublic','TestPublic/child')}
$publicBody="--- PASS: TestPublic (0.00s)`n    --- PASS: TestPublic/child (0.00s)`n"
if(-not (Get-EnginePublicProof $public $publicBody 0).qualified){throw 'Exact complete public outcomes refused'}
foreach($invalid in @(($publicBody+"    --- PASS: TestPublic/unknown (0.00s)`n"),($publicBody.Replace('PASS: TestPublic/child','SKIP: TestPublic/child')),($publicBody+"    --- PASS: TestPublic/child (0.00s)`n"))) {
 if((Get-EnginePublicProof $public $invalid 0).qualified){throw 'Unknown, skipped or duplicate public child qualified'}
}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_host_exclusions_refuse_unqualified_or_wrong_source_evidence(self):
        public = self.root / "public"
        proof = self.root / "native-engine-proof"
        public.mkdir()
        proof.mkdir()
        revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
        manifest = dict(source_revision=revision, sdk_go_sha256="a" * 64, public_server_sha256="b" * 64)
        report = dict(source_revision=revision, qualification_profile="windows_private_engine_v1",
                      actual_standard_user=True, private_current_user_temp=True, public_source_and_sdk_read_only=True,
                      qualification_complete=True, sdk_go_sha256="a" * 64, public_families=dict(qualified=True), packages=[])
        core = dict(server_sha256="b" * 64, elevated=False, admin_member=False)
        (public / "engine-build-manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
        (proof / "native-core-proof.json").write_text(json.dumps(core), encoding="utf-8")
        environment = dict(os.environ, MAESTRO_ENGINE_PUBLIC_ROOT=str(public), RUNNER_TEMP=str(self.root),
                           GITHUB_OUTPUT=str(self.root / "host-exclusions.txt"))
        for source, message in [(revision, "Closed native owner/public assertion coverage is incomplete"),
                                ("0" * 40, "Exact-source ordinary-account engine and shipping proof did not qualify")]:
            report["source_revision"] = source
            (proof / "engine-contract-results.json").write_text(json.dumps(report), encoding="utf-8")
            result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(ROOT / "acceptance/windows-engine-check.ps1")],
                                    cwd=ROOT, env=environment, capture_output=True, text=True, timeout=30)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(message, result.stdout + result.stderr)
            self.assertFalse((self.root / "host-exclusions.txt").exists())

    def test_current_companion_literals_are_closed_and_bound_to_source(self):
        parser = self.root / "ledger.ps1"
        parser.write_text(r'''$ErrorActionPreference='Stop'
$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$errors)
if($errors.Count){throw ($errors | Out-String)}
$helperAST=[Management.Automation.Language.Parser]::ParseFile($args[1],[ref]$null,[ref]$errors)
$helper=@($helperAST.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Read-EngineLiteralArray'},$true))
if($helper.Count -ne 1){throw 'Expected actual source literal reader'}
Invoke-Expression $helper[0].Extent.Text
$values=@{}
foreach($name in @('$adapterParents','$adapterAssertions','$adapterCommandParents','$adapterSetupParents','$adapterSetupAssertions','$adapterPackagingParents','$adapterPackagingAssertions')) {
 $assignment=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $name},$true))
 if($assignment.Count -ne 1){throw 'Expected one closed actual source declaration'}
 $values[$name]=@(Read-EngineLiteralArray $assignment[0])
 if($values[$name].Count -eq 0 -or @($values[$name]|Sort-Object -CaseSensitive -Unique).Count -ne $values[$name].Count){throw 'Source inventory is empty or duplicated'}
}
foreach($child in $values['$adapterAssertions']) {
 if(($child -split '/')[0] -cnotin $values['$adapterParents']){throw 'Assertion is outside the exact selected source parents'}
}
foreach($parent in $values['$adapterParents']) {
 if($parent -cnotin $values['$adapterAssertions']){throw 'Selected source parent is missing from outcome requirements'}
}
$values|ConvertTo-Json -Depth 4 -Compress
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(parser), str(DRIVER), str(ROOT / "acceptance/windows-engine-check.ps1")], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        ledger = json.loads(result.stdout)
        self.assertEqual(len(ledger["$adapterParents"]), 63)
        self.assertEqual(len(ledger["$adapterAssertions"]), 185)
        self.assertIn("TestMaintenancePublicJourney/owned-background", ledger["$adapterAssertions"])
        darwin_parents = ['TestProductDarwinSDKAndShippingStdioLifecycle', 'TestProductDarwinReviewProjectionR1']
        darwin_outcomes = ['TestProductDarwinSDKAndShippingStdioLifecycle', 'TestProductDarwinSDKAndShippingStdioLifecycle/false', 'TestProductDarwinSDKAndShippingStdioLifecycle/true', 'TestProductDarwinReviewProjectionR1', 'TestProductDarwinReviewProjectionR1/initialization', 'TestProductDarwinReviewProjectionR1/refresh']
        self.assertEqual([name for name in ledger["$adapterParents"] if name in darwin_parents], darwin_parents)
        self.assertEqual([name for name in ledger["$adapterAssertions"] if name.split("/")[0] in darwin_parents], darwin_outcomes)
        for case in ("refusal-diagnostic", "request-capability-withdrawal", "request-url-only",
                     "request-empty-elicitation", "request-explicit-form", "request-form-added"):
            self.assertIn(f"TestOwnerConfirmationRefusals/{case}", ledger["$adapterAssertions"])
        lifecycle = ['TestProductHookGeneratedMemoryReadOnly', 'TestPassiveLifecycleMissingMemoryDoesNotInitialize', 'TestPassiveLifecycleRefusalsPreserveState', 'TestPassiveStopOnlyRecordsIdempotentMetadata', 'TestLifecycleMemoryCorrectionAndDeletionHideOldView']
        self.assertEqual(ledger["$adapterParents"][-5:], lifecycle)
        self.assertEqual(ledger["$adapterAssertions"][-5:], lifecycle)
        for version in ("0_1_0", "0_1_1"):
            for format in ("plugin", "mcpb"):
                self.assertIn(f"TestCompiledStdioWorkflowAndPackets/portable_{format}_{version}", ledger["$adapterAssertions"])
        self.assertEqual(len(ledger["$adapterCommandParents"]), 1)
        self.assertEqual(len(ledger["$adapterAssertions"]) + len(ledger["$adapterCommandParents"]), 186)
        self.assertEqual(ledger["$adapterSetupParents"], ["TestModernFactoryFirstInstallAndRestart", "TestDirectInnerInstallRequiresProductionAuthority", "TestModernSyncedSourceLifecycle", "TestSyncedPublicationInterleavingAndReadOnlyReviews"])
        self.assertEqual(len(ledger["$adapterSetupAssertions"]), 22)
        self.assertEqual(len(set(ledger["$adapterSetupAssertions"])), 22)
        for assertion in ledger["$adapterSetupAssertions"]:
            self.assertIn(assertion.split("/")[0], ledger["$adapterSetupParents"])
        setup_source = "\n".join(p.read_text(encoding="utf-8") for p in (ROOT / "adapters/mcp/setup").glob("*_test.go"))
        for parent in ledger["$adapterSetupParents"]:
            self.assertIn(f"func {parent}(", setup_source)
        self.assertEqual(len(ledger["$adapterAssertions"]) + len(ledger["$adapterCommandParents"]) + len(ledger["$adapterSetupAssertions"]), 208)
        self.assertEqual(ledger["$adapterPackagingParents"], ["TestNativeWindowsOwnedProcessTree"])
        self.assertEqual(ledger["$adapterPackagingAssertions"], ["TestNativeWindowsOwnedProcessTree"] + [
            "TestNativeWindowsOwnedProcessTree/" + child for child in
            ("private-scratch", "cancel", "parent-exit", "output-limit", "nonzero-exit", "pre-cancel")])
        self.assertEqual(len(ledger["$adapterAssertions"]) + len(ledger["$adapterCommandParents"]) +
                         len(ledger["$adapterSetupAssertions"]) + len(ledger["$adapterPackagingAssertions"]), 215)

        for name in ("TestMethodPlansResolveMaintenanceKeepWikiUnavailable", "TestMethodPlansResolveRegisteredMaintenanceAndWiki", "TestAgentResourceCallbacksRevalidateBinding"):
            self.assertIn(name, ledger["$adapterParents"])
            self.assertIn(name, ledger["$adapterAssertions"])
        source = "\n".join(p.read_text(encoding="utf-8") for p in (ROOT / "adapters/mcp").glob("*_test.go"))
        for name in ledger["$adapterParents"]:
            self.assertIn(f"func {name}(", source)
        # These are compiled adapter contracts under the existing ordinary-user
        # source proof, not native-client or host-enforcement qualification.
        self.assertIn("TestCompiledClaudeHook", ledger["$adapterParents"])
        hook_outcomes = {
            'TestCompiledClaudeHook',
            'TestCompiledClaudeHook/root_removal_and_allowed_control',
            'TestCompiledClaudeHook/startup_read_only_and_current_context',
            'TestCompiledClaudeHook/malformed_event_and_size_fail_closed',
            'TestCompiledClaudeHook/paths_glob_regex_and_cross_case',
            'TestCompiledClaudeHook/repository_review_pair',
            'TestCompiledClaudeHook/namespace_and_predispatch',
            'TestCompiledClaudeHook/observations_retries_and_body_privacy',
            'TestCompiledClaudeHook/optional_prompt_identity_never_resets',
            'TestCompiledClaudeHook/concurrent_reservations_choose_one',
            'TestCompiledClaudeHook/private_alias_and_changed_binding',
            'TestCompiledClaudeHook/failed_observation_grants_no_child_authority',
            'TestCompiledClaudeHook/truncated_state_never_reinitializes',
            'TestCompiledClaudeHook/stop_recursion_has_no_completion_effect',
        }
        self.assertEqual({name for name in ledger["$adapterAssertions"]
                          if name == "TestCompiledClaudeHook" or name.startswith("TestCompiledClaudeHook/")},
                         hook_outcomes)
        self.assertNotIn("TestContentDesktopPacketOracle", ledger["$adapterParents"])
        command = "\n".join(p.read_text(encoding="utf-8") for p in (ROOT / "adapters/mcp/cmd/maestro-mcp").glob("*_test.go"))
        for name in ledger["$adapterCommandParents"]:
            self.assertIn(f"func {name}(", command)

    def test_required_engine_parents_exist_in_actual_windows_build_files(self):
        # Ask Go's build selector, not a filename-only scan: a !windows file
        # can declare the exact required name while omitting it from the PE.
        go = shutil.which("go")
        self.assertIsNotNone(go, "The source inventory check requires the pinned Go tool")
        parser = self.root / "required-parents.ps1"
        parser.write_text(r'''$ErrorActionPreference='Stop'
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$null)
$assignments=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -cin @('$canonicalRequired','$originalAssertions','$adapterParents','$adapterCommandParents','$authoredAdapterParents','$authoredAtlasParents','$adapterSetupParents','$adapterPackagingParents')},$true))
if($assignments.Count -ne 8){throw 'Expected actual owning and closed companion source contracts'}
foreach($assignment in $assignments){Invoke-Expression $assignment.Extent.Text}
foreach($name in $originalAssertions.Keys){$canonicalRequired[$name]=@($canonicalRequired[$name]) + @($originalAssertions[$name])}
$canonicalRequired['mcp']=@($adapterParents)+@($authoredAdapterParents)
$canonicalRequired['atlasops']=$authoredAtlasParents
$canonicalRequired['mcp-command']=$adapterCommandParents
$canonicalRequired['setup']=$adapterSetupParents
$canonicalRequired['packaging']=$adapterPackagingParents
$canonicalRequired|ConvertTo-Json -Depth 4 -Compress
''', encoding="utf-8")
        required = subprocess.run([self.pwsh,"-NoProfile","-File",str(parser),str(DRIVER)],capture_output=True,text=True,timeout=30)
        self.assertEqual(required.returncode,0,required.stdout+required.stderr)
        required = json.loads(required.stdout)
        temporary = self.root / "go-temp"
        temporary.mkdir()
        environment = dict(os.environ, GOOS="windows", GOARCH="amd64", GOTOOLCHAIN="local", GOENV="off", GOWORK="off",
                           GOPROXY="off", GOSUMDB="off", GOFLAGS="-mod=readonly -buildvcs=false",
                           GOCACHE=str(self.root / "go-cache"), GOPATH=str(self.root / "go-path"),
                           GOMODCACHE=str(self.root / "go-path/pkg/mod"), GOTMPDIR=str(temporary), TMPDIR=str(temporary))
        owning = sorted(set(required)-{"mcp","mcp-command","setup","packaging"})
        selected = {}
        decoder = json.JSONDecoder()
        for working, packages in ((ROOT,["./internal/"+name for name in owning]),
                                  (ROOT/"adapters/mcp",[".","./cmd/maestro-mcp","./setup","./packaging"])):
            result = subprocess.run([go,"list","-e","-find","-json",*packages],cwd=working,env=environment,
                                    capture_output=True,text=True,timeout=60)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            remaining = result.stdout
            while remaining.strip():
                package, consumed = decoder.raw_decode(remaining.lstrip())
                remaining = remaining.lstrip()[consumed:]
                name = "mcp-command" if package["ImportPath"].endswith("/adapters/mcp/cmd/maestro-mcp") else package["ImportPath"].rsplit("/",1)[-1]
                self.assertNotIn(name,selected)
                selected[name] = package
        self.assertEqual(set(selected),set(required))
        import re
        for name, assertions in required.items():
            package = selected[name]
            self.assertNotIn("Error",package,package.get("Error"))
            files = package.get("TestGoFiles",[])+package.get("XTestGoFiles",[])
            declarations = []
            for filename in files:
                declarations.extend(re.findall(r"^func (Test[A-Za-z0-9_]+)\(", (Path(package["Dir"])/filename).read_text(encoding="utf-8"),re.M))
            for parent in {assertion.split("/")[0] for assertion in assertions}:
                self.assertEqual(declarations.count(parent),1,f"{name}/{parent} is missing or duplicated in actual Windows-selected source")

    def test_authored_profile_requires_partial_and_legacy_custody_outcomes(self):
        script = self.root / "authored-proof.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$tokens=$null; $errors=$null
$outer=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors)
if($errors.Count){throw ($errors | Out-String)}
$launcher=@($tokens | Where-Object {$_.Kind -eq 'HereStringLiteral'})
if($launcher.Count -ne 1){throw 'Expected the actual shared launcher'}
$inner=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$errors)
$helper=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Get-EnginePublicProof'},$true))
if($helper.Count -ne 1){throw 'Expected actual public evidence parser'}
Invoke-Expression $helper[0].Extent.Text
$assignment=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq '$publicProof'},$true))
if($assignment.Count -ne 1){throw 'Expected one actual closed public family contract'}
$literal=@($assignment[0].Right.FindAll({param($n) $n -is [Management.Automation.Language.HashtableAst]},$true))
if($literal.Count -ne 1){throw 'Public contract must remain one literal hashtable'}
$public=$literal[0].SafeGetValue()
if($public.required_tests.Count -ne 7 -or $public.required_assertions.Count -ne 39 -or
 'TestWindowsAuthoredBrainPilot' -cnotin $public.required_tests -or
 'TestWindowsAuthoredBrainPartialPreparationPilot' -cnotin $public.required_tests -or
 'TestWindowsPrivateWikiPilot' -cnotin $public.required_tests){throw 'Authored and private wiki families must preserve the original public contract'}
$checker=[Management.Automation.Language.Parser]::ParseFile($args[1],[ref]$null,[ref]$errors)
$reader=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Read-EngineLiteralArray'},$true))
Invoke-Expression $reader[0].Extent.Text
$atlas=@{}
foreach($binding in @(@{variable='$authoredAtlasParents'; field='required_tests'},@{variable='$authoredAtlasAssertions'; field='required_assertions'})){
 $variable=$binding.variable
 $found=@($outer.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $variable},$true))
 if($found.Count -ne 1){throw 'Expected one closed actual authored Atlas declaration'}
 $atlas[$binding.field]=@(Read-EngineLiteralArray $found[0])
}
if($atlas.required_tests.Count -ne 2 -or $atlas.required_assertions.Count -ne 8){throw 'Exact Atlas publication and journal assertions required'}
foreach($case in @(@{entry=$public; missing='TestWindowsAuthoredBrainPartialPreparationPilot'},
                  @{entry=$atlas; missing='TestWindowsAtlasPublicationKernel/legacy_lock_bytes_and_custody_survive_refusal'})){
 $body=($case.entry.required_assertions | ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
 $body+="`n"
 if(-not (Get-EnginePublicProof $case.entry $body 0).qualified){throw 'Exact complete authored evidence refused'}
 $missing=($case.entry.required_assertions | Where-Object {$_ -cne $case.missing} | ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
 foreach($bad in @($missing, $body.Replace('PASS: '+$case.missing,'SKIP: '+$case.missing), $body.Replace('PASS: '+$case.missing,'FAIL: '+$case.missing), ($body+'--- PASS: '+$case.missing+" (0.00s)`n"), ($body+"--- PASS: TestUnknown (0.00s)`n"))){
  if((Get-EnginePublicProof $case.entry $bad 0).qualified){throw 'Missing/skipped/failed/duplicated/unknown authored evidence qualified'}
 }
 if((Get-EnginePublicProof $case.entry $body 1).qualified){throw 'Nonzero authored process qualified'}
}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER),
                                 str(ROOT / "acceptance/windows-engine-check.ps1")], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_all_sealed_engine_workflow_callers_supply_authored_inputs(self):
        parser = self.root / "sealed-callers.ps1"
        parser.write_text(r'''$e=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$e)
if($e.Count){throw ($e | Out-String)}
$calls=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and
 $n.GetCommandName() -like '*windows-core-pilot.ps1'},$true))
$sealed=0
foreach($call in $calls){
 $parameters=@($call.CommandElements | Where-Object {$_ -is [Management.Automation.Language.CommandParameterAst]} | ForEach-Object {$_.ParameterName})
 if('CanonicalManifest' -cnotin $parameters){continue}
 $sealed++
 if('AuthoredBrain' -cnotin $parameters -or 'AtlasTestBinary' -cnotin $parameters){throw 'Sealed engine caller omits mandatory authored family or Atlas companion'}
}
if($sealed -eq 0){throw 'Expected an actual sealed-engine caller'}
''', encoding="utf-8")
        selected = 0
        for workflow in sorted((ROOT / ".github/workflows").glob("*.yml")):
            lines = workflow.read_text(encoding="utf-8").splitlines()
            for index, line in enumerate(lines):
                if line.strip() != "run: |":
                    continue
                indent = len(line) - len(line.lstrip()) + 2
                body = []
                for child in lines[index + 1:]:
                    if child and len(child) - len(child.lstrip()) < indent:
                        break
                    body.append(child)
                text = textwrap.dedent("\n".join(body))
                if "windows-core-pilot.ps1" not in text or "CanonicalManifest" not in text:
                    continue
                selected += 1
                source = self.root / f"sealed-caller-{selected}.ps1"
                source.write_text(text, encoding="utf-8")
                result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(parser), str(source)],
                                        capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0, f"{workflow.name}: {result.stdout}{result.stderr}")
        self.assertGreater(selected, 0, "No actual sealed-engine workflow callers were checked")

    def test_darwin_companion_requires_all_actual_outcomes(self):
        script = self.root / "darwin-companion-proof.ps1"
        script.write_text(r"""$ErrorActionPreference='Stop'
$e=$null; $tokens=$null
$outer=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$e)
if($e.Count){throw ($e | Out-String)}
$checker=[Management.Automation.Language.Parser]::ParseFile($args[1],[ref]$null,[ref]$e)
$reader=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Read-EngineLiteralArray'},$true))
Invoke-Expression $reader[0].Extent.Text
$entry=@{}; $closed=@{}
foreach($binding in @(@{name='$adapterParents';field='required_tests'},@{name='$adapterAssertions';field='required_assertions'})){
 $name=$binding.name
 $assignment=@($outer.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $name},$true))
 if($assignment.Count -ne 1){throw 'Expected one actual closed MCP source ledger'}
 $entry[$binding.field]=@(Read-EngineLiteralArray $assignment[0])
 $closed[$name]=$entry[$binding.field]
}
$darwin=@('TestProductDarwinSDKAndShippingStdioLifecycle','TestProductDarwinSDKAndShippingStdioLifecycle/false','TestProductDarwinSDKAndShippingStdioLifecycle/true','TestProductDarwinReviewProjectionR1','TestProductDarwinReviewProjectionR1/initialization','TestProductDarwinReviewProjectionR1/refresh')
foreach($test in $darwin){if($test -cnotin $entry.required_assertions -or $test.Split('/')[0] -cnotin $entry.required_tests){throw 'Both actual Darwin MCP families and their closed outcomes must be selected'}}
$launcher=@($tokens | Where-Object {$_.Kind -eq 'HereStringLiteral'})
$inner=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$e)
$parser=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Get-EnginePublicProof'},$true))
if($parser.Count -ne 1){throw 'Expected original ordinary-account public proof parser'}
Invoke-Expression $parser[0].Extent.Text
$body=($entry.required_assertions|ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
$body+="`n"
if(-not (Get-EnginePublicProof $entry $body 0).qualified){throw 'Exact MCP cohort including Darwin outcomes refused'}
foreach($test in $darwin){
 $partial=($entry.required_assertions|Where-Object {$_ -cne $test}|ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
 foreach($bad in @($partial,$body.Replace('PASS: '+$test+' (','SKIP: '+$test+' ('),$body.Replace('PASS: '+$test+' (','FAIL: '+$test+' ('),($body+'--- PASS: '+$test+" (0.00s)`n"))){
  if((Get-EnginePublicProof $entry $bad 0).qualified){throw 'Missing/skipped/failed/duplicated Darwin outcome qualified'}
 }
}
if((Get-EnginePublicProof $entry ($body+"--- PASS: TestProductDarwinReviewProjectionR1/unknown (0.00s)`n") 0).qualified -or
   (Get-EnginePublicProof $entry $body 1).qualified){throw 'Unknown outcome or nonzero Darwin cohort qualified'}
$loops=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.ForEachStatementAst] -and $n.Extent.Text.StartsWith('foreach ($entry in $report.adapter_packages)')},$true))
if($loops.Count -ne 1){throw 'Expected actual host exclusion adapter verification loop'}
$declaredAssertions=@($entry.required_assertions)
$proof=Get-EnginePublicProof $entry $body 0
$report=@{adapter_packages=@(@{name='mcp';qualified=$true;copied_binary_custody=$true;proof=$proof})}
Invoke-Expression $loops[0].Extent.Text
foreach($test in $darwin){
 $proof.required_assertions=@($declaredAssertions|Where-Object {$_ -cne $test})
 $refused=$false
 try{Invoke-Expression $loops[0].Extent.Text}catch{$refused=$true}
 if(-not $refused){throw 'Host exclusion accepted a missing declared Darwin assertion'}
}
$proof.required_assertions=@($declaredAssertions)+@('TestProductDarwinReviewProjectionR1/unknown')
$refused=$false
try{Invoke-Expression $loops[0].Extent.Text}catch{$refused=$true}
if(-not $refused){throw 'Host exclusion accepted an unknown declared Darwin assertion'}
""", encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER),
                                 str(ROOT / "acceptance/windows-engine-check.ps1")],
                                capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_owned_process_companion_requires_all_actual_outcomes(self):
        script = self.root / "owned-process-proof.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$e=$null; $tokens=$null
$outer=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$e)
if($e.Count){throw ($e | Out-String)}
$checker=[Management.Automation.Language.Parser]::ParseFile($args[1],[ref]$null,[ref]$e)
$reader=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Read-EngineLiteralArray'},$true))
Invoke-Expression $reader[0].Extent.Text
$entry=@{}
foreach($binding in @(@{name='$adapterPackagingParents';field='required_tests'},@{name='$adapterPackagingAssertions';field='required_assertions'})){
 $name=$binding.name
 $assignment=@($outer.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $name},$true))
 if($assignment.Count -ne 1){throw 'Expected one actual closed native process source ledger'}
 $entry[$binding.field]=@(Read-EngineLiteralArray $assignment[0])
}
$launcher=@($tokens | Where-Object {$_.Kind -eq 'HereStringLiteral'})
$inner=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$e)
$parser=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Get-EnginePublicProof'},$true))
Invoke-Expression $parser[0].Extent.Text
$body=($entry.required_assertions|ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
$body+="`n"
if(-not (Get-EnginePublicProof $entry $body 0).qualified){throw 'Exact owned-process outcomes refused'}
foreach($test in $entry.required_assertions){
 $partial=($entry.required_assertions|Where-Object {$_ -cne $test}|ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
 foreach($bad in @($partial,$body.Replace('PASS: '+$test+' (','SKIP: '+$test+' ('),$body.Replace('PASS: '+$test+' (','FAIL: '+$test+' ('),($body+'--- PASS: '+$test+" (0.00s)`n"))){
  if((Get-EnginePublicProof $entry $bad 0).qualified){throw 'Missing/skipped/failed/duplicated native process outcome qualified'}
 }
}
foreach($bad in @('native_windows_owned_process ordinary_account=true atomic_job_assignment=true',($body+"--- PASS: TestNativeWindowsOwnedProcessTree/unknown (0.00s)`n"))){
 if((Get-EnginePublicProof $entry $bad 0).qualified){throw 'Marker-only or unowned native process outcome qualified'}
}
if((Get-EnginePublicProof $entry $body 1).qualified){throw 'Nonzero native process family qualified'}
$aggregate=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and
 $n.Left.Extent.Text -ceq '$qualificationComplete' -and $n.Right.Extent.Text.Contains('$adapterResults.Count')},$true))
if($aggregate.Count -ne 1){throw 'Expected actual source aggregate qualification gate'}
$publicProof=@{qualified=$true}; $authoredResult=@{qualified=$true}; $authoredAdapterResult=@{qualified=$true}
$results=@(@{canonical_contract_proof=@{qualified=$true}})
$adapterResults=@(@{qualified=$true},@{qualified=$true},@{qualified=$true},@{qualified=$true})
Invoke-Expression $aggregate[0].Extent.Text
if(-not $qualificationComplete){throw 'Four fully qualified closed adapter cohorts must qualify the actual aggregate'}
for($i=0;$i -lt 4;$i++){
 $adapterResults[$i].qualified=$false
 Invoke-Expression $aggregate[0].Extent.Text
 if($qualificationComplete){throw 'An unqualified adapter cohort passed the aggregate'}
 $adapterResults[$i].qualified=$true
}
foreach($count in @(3,5)){
 $adapterResults=@(1..$count | ForEach-Object {@{qualified=$true}})
 Invoke-Expression $aggregate[0].Extent.Text
 if($qualificationComplete){throw 'Missing or extra adapter cohort passed the aggregate'}
}

$exclusion=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq '$exclusions'},$true))
if($exclusion.Count -ne 1 -or -not $exclusion[0].Right.Extent.Text.Contains('$adapterPackagingParents')){throw 'Verified owned-process family is missing from exact host exclusions'}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER),
                                 str(ROOT / "acceptance/windows-engine-check.ps1")], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_authored_same_binary_companion_is_closed_before_host_exclusion(self):
        script = self.root / "authored-adapter-proof.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$tokens=$null; $e=$null
$outer=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$e)
if($e.Count){throw ($e | Out-String)}
$checker=[Management.Automation.Language.Parser]::ParseFile($args[1],[ref]$null,[ref]$e)
$reader=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Read-EngineLiteralArray'},$true))
Invoke-Expression $reader[0].Extent.Text
$entry=@{}
foreach($binding in @(@{name='$authoredAdapterParents';field='required_tests'},@{name='$authoredAdapterAssertions';field='required_assertions'})){
 $name=$binding.name
 $assignment=@($outer.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq $name},$true))
 if($assignment.Count -ne 1){throw 'Expected one closed authored adapter ledger'}
 $entry[$binding.field]=@(Read-EngineLiteralArray $assignment[0])
}
if($entry.required_tests.Count -ne 13 -or $entry.required_assertions.Count -ne 41 -or
 @($entry.required_assertions|Sort-Object -CaseSensitive -Unique).Count -ne 41){throw 'Exact13-parent/41-outcome authored cohort required'}
foreach($test in $entry.required_assertions){if($test.Split('/')[0] -cnotin $entry.required_tests){throw 'Unowned authored assertion'}}
$launcher=@($tokens | Where-Object {$_.Kind -eq 'HereStringLiteral'})
$inner=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$e)
$parser=@($inner.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Get-EnginePublicProof'},$true))
Invoke-Expression $parser[0].Extent.Text
$body=($entry.required_assertions|ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
$body+="`n"
if(-not (Get-EnginePublicProof $entry $body 0).qualified){throw 'Exact authored adapter outcomes refused'}
$missing='TestAuthoredBrainSDKCreatesPageWithoutConversionArtifact'
$partial=($entry.required_assertions|Where-Object {$_ -cne $missing}|ForEach-Object {'--- PASS: '+$_+' (0.00s)'}) -join "`n"
foreach($bad in @($partial,$body.Replace('PASS: '+$missing,'SKIP: '+$missing),$body.Replace('PASS: '+$missing,'FAIL: '+$missing),($body+'--- PASS: '+$missing+" (0.00s)`n"),($body+"--- PASS: TestUnowned (0.00s)`n"))){
 if((Get-EnginePublicProof $entry $bad 0).qualified){throw 'Missing/skipped/failed/duplicated/unknown authored companion qualified'}
}
if((Get-EnginePublicProof $entry $body 1).qualified){throw 'Failed authored companion process qualified'}
$guard=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and
 $n.Extent.Text.Contains("Every authored positive adapter assertion must pass under the ordinary account before its host exclusion.")},$true))
$exclusion=@($checker.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -ceq '$exclusions'},$true))
if($guard.Count -ne 1 -or $exclusion.Count -ne 1 -or $guard[0].Extent.EndOffset -ge $exclusion[0].Extent.StartOffset -or
 -not $exclusion[0].Right.Extent.Text.Contains('$authoredAdapterParents')){throw 'Authored host exclusion must follow its exact ordinary-account evidence guard'}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER),
                                 str(ROOT / "acceptance/windows-engine-check.ps1")], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_remaining_protocol_preserves_exact_partition_and_unqualified_skips(self):
        script = self.root / "remaining-proof.ps1"
        script.write_text(r'''$ErrorActionPreference='Stop'
$tokens=$null;$errors=$null
[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors)|Out-Null
$launcher=@($tokens|Where-Object {$_.Kind -eq 'HereStringLiteral'})
if($errors.Count -or $launcher.Count -ne 1){throw 'Expected original credentialed launcher'}
$ast=[Management.Automation.Language.Parser]::ParseInput($launcher[0].Value,[ref]$null,[ref]$errors)
$reader=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Get-RemainingProtocolProof'},$true))
if($errors.Count -or $reader.Count -ne 1){throw 'Expected original remaining classifier'}
Invoke-Expression $reader[0].Extent.Text
$module='github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp'
$inventory=@{$module=@('TestRemaining','TestProved','TestWindowsWorkspaceInitializationMatchesElevation')}
$excluded=@('TestProved','TestWindowsWorkspaceInitializationMatchesElevation')
$valid=@([pscustomobject]@{Package=$module;Test='TestRemaining';Action='pass'})
$proof=Get-RemainingProtocolProof $inventory $valid $excluded
if(-not $proof.complete -or $proof.inventory.Count -ne 3 -or $proof.outcomes.Count -ne 1 -or $proof.skipped_coverage_qualified -ne $false){throw 'Exact parent union refused'}
if(@($proof.inventory|Where-Object {$_.execution -ceq 'elevated_refusal'}).Count -ne 1){throw 'Admin negative disappeared'}
function Assert-RemainingRefused($Events) {
 if((Get-RemainingProtocolProof $inventory $Events $excluded).complete){throw 'Unknown, missing, duplicate, excluded or failed evidence qualified'}
}
Assert-RemainingRefused @()
Assert-RemainingRefused @($valid + $valid)
Assert-RemainingRefused @([pscustomobject]@{Package=$module;Test='TestUnknown';Action='pass'})
Assert-RemainingRefused @([pscustomobject]@{Package=$module;Test='TestRemaining';Action='fail'})
Assert-RemainingRefused @([pscustomobject]@{Package=$module;Test='TestRemaining';Action='skip'})
Assert-RemainingRefused @($valid + [pscustomobject]@{Package=$module;Test='TestProved';Action='pass'})
$optional=@{$module=@('TestContentRealJourneys')}
$message='fixture_test.go:1: explicit final retained authenticated engineering fixture and independent registry seed required' + [Environment]::NewLine
$events=@([pscustomobject]@{Package=$module;Test='TestContentRealJourneys';Action='output';Output=$message},[pscustomobject]@{Package=$module;Test='TestContentRealJourneys';Action='skip'})
$proof=Get-RemainingProtocolProof $optional $events @()
if(-not $proof.complete -or $proof.classified_skips.Count -ne 1 -or $proof.skipped_coverage_qualified -ne $false -or $proof.outcomes[0].outcome -cne 'skip'){throw 'Existing explicit fixture SKIP lost its missing-coverage meaning'}
$events[0].Output='fixture_test.go:1: unknown reason' + [Environment]::NewLine
if((Get-RemainingProtocolProof $optional $events @()).complete){throw 'Unknown reason was silently classified'}
$duplicate=@{$module=@('TestRemaining','TestRemaining')}
$refused=$false
try {Get-RemainingProtocolProof $duplicate $valid @()|Out-Null} catch {$refused=$true}
if(-not $refused){throw 'Duplicate compiled inventory accepted'}
$names=@('TestFoxtrot','TestDelta','TestAlpha','TestEcho','TestCharlie','TestBravo')
$orderedInventory=@{$module=@($names + $excluded)}
$orderedEvents=@($names|ForEach-Object {[pscustomobject]@{Package=$module;Test=$_;Action='pass'}})
$orderedProof=Get-RemainingProtocolProof $orderedInventory $orderedEvents $excluded
$expectedNames=@('TestAlpha','TestBravo','TestCharlie','TestDelta','TestEcho','TestFoxtrot')
$expectedInventory=@(($expectedNames + 'TestProved' + 'TestWindowsWorkspaceInitializationMatchesElevation')|ForEach-Object {$module+'/'+$_})
$expectedOutcomes=@($expectedNames|ForEach-Object {$module+'/'+$_})
if(-not $orderedProof.complete -or ($orderedProof.inventory.test -join '|') -cne ($expectedInventory -join '|') -or
 ($orderedProof.outcomes.test -join '|') -cne ($expectedOutcomes -join '|')){throw 'Proof inventory and outcomes must have canonical test-key order'}
[array]::Reverse($names)
[array]::Reverse($orderedEvents)
$reversedProof=Get-RemainingProtocolProof @{$module=@($excluded + $names)} $orderedEvents $excluded
if(($orderedProof|ConvertTo-Json -Depth 10 -Compress) -cne ($reversedProof|ConvertTo-Json -Depth 10 -Compress)){throw 'Proof changed with compiled inventory or event insertion order'}
''', encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER)], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
