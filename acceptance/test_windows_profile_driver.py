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

class WindowsEngineProfile(unittest.TestCase):
    setUp = WindowsInspectionDriver.setUp
    closed_manifest = WindowsInspectionDriver.closed_manifest
    assert_preaccount_refusal = WindowsInspectionDriver.assert_preaccount_refusal

    def invoke_engine(self, document):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        binary = self.root / "public.exe"
        binary.write_bytes(b"unexecuted public executable fixture")
        return subprocess.run([self.pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(DRIVER),
                               "-TestBinary", str(binary), "-Server", str(binary),
                               "-CanonicalManifest", str(self.manifest), "-OutputRoot", str(self.root / "proof")],
                              capture_output=True, text=True, timeout=30)

    def engine_document(self):
        document = self.closed_manifest()
        for entry in document["packages"]:
            entry.pop("skip_policy")
        digest = hashlib.sha256(b"unexecuted public executable fixture").hexdigest()
        document.update(qualification_profile="windows_private_engine_v1",
                        public_test_binary_sha256=digest, public_server_sha256=digest,
                        adapter_profile="windows_private_engine_adapters_v1", adapter_packages=[])
        return document

    def test_engine_unknown_profile_refuses_before_account(self):
        document = self.engine_document()
        document["qualification_profile"] = "windows_private_engine_v999"
        self.assert_preaccount_refusal(self.invoke_engine(document), "Unknown canonical qualification profile")

    def test_engine_package_subset_refuses_before_account(self):
        self.assert_preaccount_refusal(self.invoke_engine(self.engine_document()), "Engine qualification requires exactly the eight owning packages")

    def test_engine_public_executable_hash_is_bound(self):
        document = self.engine_document()
        document["public_server_sha256"] = "0" * 64
        self.assert_preaccount_refusal(self.invoke_engine(document), "Engine public executables differ from the sealed source manifest")

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
[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$tokens,[ref]$errors) | Out-Null
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
foreach($name in @('$adapterParents','$adapterAssertions','$adapterCommandParents')) {
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
        self.assertEqual(len(ledger["$adapterParents"]), 53)
        self.assertEqual(len(ledger["$adapterAssertions"]), 149)
        for version in ("0_1_0", "0_1_1"):
            for format in ("plugin", "mcpb"):
                self.assertIn(f"TestCompiledStdioWorkflowAndPackets/portable_{format}_{version}", ledger["$adapterAssertions"])
        self.assertEqual(len(ledger["$adapterCommandParents"]), 1)
        self.assertEqual(len(ledger["$adapterAssertions"]) + len(ledger["$adapterCommandParents"]), 150)
        for name in ("TestMethodPlansResolveMaintenanceKeepWikiUnavailable", "TestMethodPlansResolveRegisteredMaintenanceAndWiki", "TestAgentResourceCallbacksRevalidateBinding", "TestCanonicalAgentResourceDiscovery"):
            self.assertIn(name, ledger["$adapterParents"])
            self.assertIn(name, ledger["$adapterAssertions"])
        source = "\n".join(p.read_text() for p in (ROOT / "adapters/mcp").glob("*_test.go"))
        for name in ledger["$adapterParents"]:
            self.assertIn(f"func {name}(", source)
        self.assertNotIn("TestCompiledClaudeHook", ledger["$adapterParents"])
        self.assertNotIn("TestContentDesktopPacketOracle", ledger["$adapterParents"])
        command = "\n".join(p.read_text() for p in (ROOT / "adapters/mcp/cmd/maestro-mcp").glob("*_test.go"))
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
$assignments=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -cin @('$canonicalRequired','$originalAssertions','$adapterParents','$adapterCommandParents')},$true))
if($assignments.Count -ne 4){throw 'Expected actual owning and companion source contracts'}
foreach($assignment in $assignments){Invoke-Expression $assignment.Extent.Text}
foreach($name in $originalAssertions.Keys){$canonicalRequired[$name]=@($canonicalRequired[$name]) + @($originalAssertions[$name])}
$canonicalRequired['mcp']=$adapterParents
$canonicalRequired['mcp-command']=$adapterCommandParents
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
        owning = sorted(set(required)-{"mcp","mcp-command"})
        selected = {}
        decoder = json.JSONDecoder()
        for working, packages in ((ROOT,["./internal/"+name for name in owning]),
                                  (ROOT/"adapters/mcp",[".","./cmd/maestro-mcp"])):
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
                declarations.extend(re.findall(r"^func (Test[A-Za-z0-9_]+)\(", (Path(package["Dir"])/filename).read_text(),re.M))
            for parent in {assertion.split("/")[0] for assertion in assertions}:
                self.assertEqual(declarations.count(parent),1,f"{name}/{parent} is missing or duplicated in actual Windows-selected source")
