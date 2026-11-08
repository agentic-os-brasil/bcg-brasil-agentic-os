"""Closed qualification inputs and actual launcher fragments without accounts."""
import hashlib
import importlib.util
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

    def test_product_workflow_windows_blocks_parse(self):
        lines = (ROOT / ".github/workflows/product-mcp.yml").read_text().splitlines()
        current, blocks = [], []
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
        self.assertEqual(len(blocks), 4)
        parser = self.root / "parse-product.ps1"
        parser.write_text("$e=$null; [Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$e) | Out-Null; if($e.Count){throw ($e | Out-String)}", encoding="utf-8")
        for index, body in enumerate(blocks):
            script = self.root / f"product-{index}.ps1"
            script.write_text(body, encoding="utf-8")
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

    def producer_fixture(self):
        source, sdk = self.root / "source", self.root / "sdk"
        output = self.root / "compatible-producer"
        output.mkdir()
        sdk.mkdir()
        go = sdk / "go.exe"
        go.write_bytes(b"unexecuted SDK fixture")
        compiler = source / "internal/privatewiki/compiler.go"
        compiler.parent.mkdir(parents=True)
        original = 'commonFrontmatter(m, "Memory Index", "Memory navigation", in.CommittedAt)'
        compiler.write_text(original, encoding="utf-8")
        overlay = output / "compiler.go"
        overlay.write_text(original.replace('"Memory navigation"', '"Compatible producer-only description"'), encoding="utf-8")
        mapping = output / "overlay.json"
        mapping.write_text(json.dumps({"Replace": {str(compiler): str(overlay)}}), encoding="utf-8")
        helper = source / "dev/pilot/private-wiki-producer"
        helper.mkdir(parents=True)
        (helper / "main.go").write_bytes(b"unexecuted helper fixture")
        (helper / "build.py").write_bytes(b"unexecuted builder fixture")
        executable = output / "private-wiki-compatible-producer.exe"
        executable.write_bytes(b"unexecuted PE custody fixture")
        archive = self.root / "canonical-source.tar"
        archive.write_bytes(b"sealed archive fixture")
        digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
        build = dict(schema_version=1, kind="compatible-description-variant-not-historical-release",
                     source_revision="a" * 40, source_state="caller_sealed_source_archive", target_os="windows", target_arch="amd64",
                     compiler_path="internal/privatewiki/compiler.go", original_compiler_sha256=digest(compiler),
                     overlay_compiler_sha256=digest(overlay), overlay_json_sha256=digest(mapping),
                     helper_main_sha256=digest(helper / "main.go"), builder_sha256=digest(helper / "build.py"),
                     executable=str(executable), executable_sha256=digest(executable), native_execution="not_run_by_builder",
                     build_command=[str(go), "build", "-trimpath", "-buildvcs=false", "-overlay", str(mapping), "-o", str(executable), "./dev/pilot/private-wiki-producer"])
        report = output / "producer-build.json"
        report.write_text(json.dumps(build), encoding="utf-8")
        selected = dict(build_manifest=str(report), build_manifest_sha256=digest(report),
                        source_archive_sha256=digest(archive), sdk_go_sha256=digest(go),
                        executable=str(executable), executable_sha256=digest(executable))
        packages = [dict(name=name, tests=["TestPrivateWikiCompatibleProducerAcrossActualBinaries"] if name == "memory" else ["TestFixture"])
                    for name in ["execution", "datahome", "ownerctx", "memory", "privatewiki", "processidentity"]]
        return dict(packages=packages, source_root=str(source), source_revision=build["source_revision"], source_archive=str(archive),
                    source_archive_sha256=digest(archive), sdk_go=str(go), sdk_go_sha256=digest(go), compatible_producer=selected)

    def producer_guard_check(self, document, expected, inherited=None):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        script = self.root / "producer-guard.ps1"
        script.write_text(r"""$ErrorActionPreference='Stop'
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$null)
$guard=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$withCanonical -and $canonical.compatible_producer' -and $node.Extent.Text -match 'source/archive/SDK seal'},$true))
if ($guard.Count -ne 1) { throw 'Expected the actual sealed producer guard' }
$withCanonical=$true
$canonical=Get-Content -LiteralPath $args[1] -Raw | ConvertFrom-Json
$environmentGuard=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Extent.Text -match 'Canonical qualification refuses inherited producer helpers'},$true))
if ($environmentGuard.Count -ne 1) { throw 'Expected actual canonical inherited-environment refusal' }
if ($args[2] -ceq 'ALLOW') { Invoke-Expression $environmentGuard[0].Extent.Text; Invoke-Expression $guard[0].Extent.Text; exit 0 }
$refused=$false
try { Invoke-Expression $environmentGuard[0].Extent.Text; Invoke-Expression $guard[0].Extent.Text } catch { if ($_.Exception.Message -notmatch $args[2]) { throw }; $refused=$true }
if (-not $refused) { throw 'Producer substitution was accepted' }
""", encoding="utf-8")
        env = dict(os.environ)
        if inherited:
            env[inherited] = "synthetic-inherited-substitute"
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER), str(self.manifest), expected],
                                capture_output=True, text=True, timeout=30, env=env)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_producer_refuses_inherited_compatible_or_old_writer(self):
        document = self.producer_fixture()
        for variable in ["MAESTRO_PRIVATE_WIKI_COMPAT_PRODUCER", "MAESTRO_PRIVATE_WIKI_OLD_WRITER"]:
            with self.subTest(variable=variable):
                self.producer_guard_check(document, "refuses inherited producer helpers", variable)

    def test_producer_refuses_archive_sdk_report_and_pe_substitution(self):
        document = self.producer_fixture()
        self.producer_guard_check(document, "ALLOW")
        for field in ["source_archive_sha256", "sdk_go_sha256", "build_manifest_sha256", "executable_sha256"]:
            with self.subTest(field=field):
                original = document["compatible_producer"][field]
                document["compatible_producer"][field] = "0" * 64
                self.producer_guard_check(document, "Compatible producer")
                document["compatible_producer"][field] = original

    def test_producer_refuses_a_resealed_non_description_overlay(self):
        document = self.producer_fixture()
        report = Path(document["compatible_producer"]["build_manifest"])
        build = json.loads(report.read_text())
        overlay = report.with_name("compiler.go")
        overlay.write_text("unapproved compiler replacement", encoding="utf-8")
        build["overlay_compiler_sha256"] = hashlib.sha256(overlay.read_bytes()).hexdigest()
        report.write_text(json.dumps(build), encoding="utf-8")
        document["compatible_producer"]["build_manifest_sha256"] = hashlib.sha256(report.read_bytes()).hexdigest()
        self.producer_guard_check(document, "single approved description-only replacement")

    def test_producer_env_is_selected_only_for_memory_and_cleaned(self):
        document = self.producer_fixture()
        self.launcher_fragment_check(r"""
$selection=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$selectProducer'},$true))
$binding=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$selectProducer' -and $node.Extent.Text -match '\$env:MAESTRO_PRIVATE_WIKI_COMPAT_PRODUCER'},$true))
$cleanup=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$producerSelected'},$true))
if ($selection.Count -ne 1 -or $binding.Count -ne 1 -or $cleanup.Count -ne 1) { throw 'Expected actual producer selection/binding/cleanup blocks' }
$producerPath=Join-Path $PSScriptRoot 'compatible-producer/private-wiki-compatible-producer.exe'
$manifest=[pscustomobject]@{compatible_producer=[pscustomobject]@{executable=$producerPath; executable_sha256=(Get-FileHash -LiteralPath $producerPath -Algorithm SHA256).Hash.ToLowerInvariant()}}
foreach ($case in @(@{name='execution'; inspection=$false; selected=$false},@{name='memory'; inspection=$false; selected=$true},@{name='memory'; inspection=$true; selected=$false})) {
 $Inspection=$case.inspection; $entry=[pscustomobject]@{name=$case.name}; $producerSelected=$false; $producerCustodyValid=$true
 Invoke-Expression $selection[0].Extent.Text
 Invoke-Expression $binding[0].Extent.Text
 if ($producerSelected -ne $case.selected -or (Test-Path Env:MAESTRO_PRIVATE_WIKI_COMPAT_PRODUCER) -ne $case.selected) { throw 'Producer escaped memory-only scope' }
 Invoke-Expression $cleanup[0].Extent.Text
 if (Test-Path Env:MAESTRO_PRIVATE_WIKI_COMPAT_PRODUCER) { throw 'Producer remained inherited after memory' }
 if (-not $producerCustodyValid) { throw 'Unchanged producer custody was lost' }
}
""")

    def test_actual_compatibility_acceptance_requires_pass_without_skip_and_custody(self):
        self.launcher_fragment_check(r"""
$checks=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$compatibilityPassed'},$true))
if ($checks.Count -ne 1) { throw 'Expected actual compatibility acceptance predicate' }
foreach ($case in @(@{outcome='PASS'; custody=$true; skips=@(); expected=$true},@{outcome='SKIP'; custody=$true; skips=@(); expected=$false},@{outcome='FAIL'; custody=$true; skips=@(); expected=$false},@{outcome='PASS'; custody=$false; skips=@(); expected=$false},@{outcome='PASS'; custody=$true; skips=@('TestPrivateWikiCompatibleProducerAcrossActualBinaries/nested'); expected=$false})) {
 $producerSelected=$true; $producerCustodyValid=$case.custody; $compatOutcomes=@(@{outcome=$case.outcome}); $allSkips=$case.skips
 Invoke-Expression $checks[0].Extent.Text
 if ($compatibilityPassed -ne $case.expected) { throw 'Unqualified compatible producer was accepted' }
}
""")

    def test_historical_acceptance_requires_real_pass_without_nested_skip_and_custody(self):
        self.launcher_fragment_check(r"""
$checks=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$historicalPassed'},$true))
if ($checks.Count -ne 1) { throw 'Expected actual historical acceptance predicate' }
foreach ($case in @(@{outcome='PASS'; custody=$true; skips=@(); expected=$true},@{outcome='SKIP'; custody=$true; skips=@(); expected=$false},@{outcome='FAIL'; custody=$true; skips=@(); expected=$false},@{outcome='PASS'; custody=$false; skips=@(); expected=$false},@{outcome='PASS'; custody=$true; skips=@('TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete/nested'); expected=$false})) {
 $historicalSelected=$true; $historicalCustodyValid=$case.custody; $historicalOutcomes=@(@{outcome=$case.outcome}); $allSkips=$case.skips
 Invoke-Expression $checks[0].Extent.Text
 if ($historicalPassed -ne $case.expected) { throw 'Unqualified historical witness was accepted' }
}
""")

    def historical_fixture(self):
        document = self.producer_fixture()
        output = self.root / "historical-witness"
        output.mkdir()
        source = output / "source"
        source.mkdir()
        builder_path = ROOT / "dev/pilot/private-wiki-old-cohort/build.py"
        spec = importlib.util.spec_from_file_location("historical_builder", builder_path)
        builder = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(builder)
        archive = subprocess.check_output(["git", "archive", "--format=tar", builder.REVISION], cwd=ROOT)
        (output / "historical-source.tar").write_bytes(archive)
        records = builder.extract_pinned_archive(archive, source)
        harness = {}
        for name in ["main.go", "main_test.go"]:
            relative = "dev/pilot/private-wiki-old-cohort/" + name
            for root in [source, Path(document["source_root"])]:
                path = root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(b"unexecuted synthetic harness fixture")
            harness[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
        helper = Path(document["source_root"]) / "dev/pilot/private-wiki-old-cohort"
        (helper / "build.py").write_bytes(b"unexecuted synthetic builder fixture")
        executable = output / "private-wiki-old-cohort-harness.exe"
        executable.write_bytes(b"unexecuted synthetic PE custody fixture")
        digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
        build = dict(schema_version=1, artifact_kind="new_harness_unmodified_historical_library",
                     historical_release_artifact=False, library_revision=builder.REVISION,
                     original_archive_sha256=hashlib.sha256(archive).hexdigest(), original_library_files=records,
                     harness_files=harness, builder_sha256=digest(helper / "build.py"),
                     sdk_executable_sha256=document["sdk_go_sha256"], target_os="windows", target_arch="amd64",
                     executable=str(executable), executable_sha256=digest(executable), library_unchanged_after_build=True,
                     native_execution="not_run_by_builder", build_command=[document["sdk_go"], "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags",
                     "-X=main.linkedLibraryRevision=" + builder.REVISION, "-o", str(executable), "./dev/pilot/private-wiki-old-cohort"])
        report = output / "historical-build.json"
        report.write_text(json.dumps(build), encoding="utf-8")
        document["historical_witness"] = dict(build_manifest=str(report), build_manifest_sha256=digest(report),
                  original_archive_sha256=build["original_archive_sha256"], sdk_go_sha256=document["sdk_go_sha256"],
                  original_library_inventory_sha256=hashlib.sha256(json.dumps(records, separators=(",", ":")).encode()).hexdigest(),
                  executable=str(executable), executable_sha256=digest(executable))
        document["packages"][3]["tests"].append("TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete")
        return document

    def historical_guard_check(self, document, expected):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        script = self.root / "historical-guard.ps1"
        script.write_text(r"""$ErrorActionPreference='Stop'
$ast=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$null)
$guard=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$withCanonical -and $canonical.historical_witness' -and $node.Extent.Text -match 'archive/SDK seal'},$true))
if ($guard.Count -ne 1) { throw 'Expected actual sealed historical witness guard' }
$withCanonical=$true; $canonical=Get-Content -LiteralPath $args[1] -Raw | ConvertFrom-Json
if ($args[2] -ceq 'ALLOW') { Invoke-Expression $guard[0].Extent.Text; exit 0 }
$refused=$false
try { Invoke-Expression $guard[0].Extent.Text } catch { if ($_.Exception.Message -notmatch $args[2]) { throw }; $refused=$true }
if (-not $refused) { throw 'Historical substitution was accepted' }
""", encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(script), str(DRIVER), str(self.manifest), expected],
                                capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_historical_refuses_archive_sdk_report_inventory_and_pe_substitution(self):
        document = self.historical_fixture()
        self.historical_guard_check(document, "ALLOW")
        for field in ["original_archive_sha256", "sdk_go_sha256", "build_manifest_sha256", "original_library_inventory_sha256", "executable_sha256"]:
            with self.subTest(field=field):
                original = document["historical_witness"][field]
                document["historical_witness"][field] = "0" * 64
                self.historical_guard_check(document, "Historical witness")
                document["historical_witness"][field] = original

    def test_historical_refuses_changed_library_and_missing_owning_test(self):
        document = self.historical_fixture()
        document["packages"][3]["tests"].remove("TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete")
        self.historical_guard_check(document, "actual owning memory historical test")
        document["packages"][3]["tests"].append("TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete")
        original = self.root / "historical-witness/source/go.mod"
        original.write_bytes(original.read_bytes() + b"\n// altered old library\n")
        self.historical_guard_check(document, "input hash or ordinary-file custody")

    def test_historical_refuses_resealed_release_claim_or_altered_build_command(self):
        document = self.historical_fixture()
        report = Path(document["historical_witness"]["build_manifest"])
        original = json.loads(report.read_text())
        for field, replacement in [("historical_release_artifact", True), ("build_command", ["unapproved build"]),
                                   ("library_unchanged_after_build", False)]:
            with self.subTest(field=field):
                altered = dict(original, **{field: replacement})
                report.write_text(json.dumps(altered), encoding="utf-8")
                document["historical_witness"]["build_manifest_sha256"] = hashlib.sha256(report.read_bytes()).hexdigest()
                self.historical_guard_check(document, "identity, immutable library or exact build command")

    def test_historical_env_is_selected_only_for_memory_and_cleaned(self):
        document = self.historical_fixture()
        self.launcher_fragment_check(r"""
$selection=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -ceq '$selectHistorical'},$true))
$binding=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$selectHistorical' -and $node.Extent.Text -match '\$env:MAESTRO_PRIVATE_WIKI_OLD_WRITER'},$true))
$cleanup=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$historicalSelected'},$true))
if ($selection.Count -ne 1 -or $binding.Count -ne 1 -or $cleanup.Count -ne 1) { throw 'Expected actual historical selection/binding/cleanup blocks' }
$historicalPath=Join-Path $PSScriptRoot 'historical-witness/private-wiki-old-cohort-harness.exe'
$manifest=[pscustomobject]@{historical_witness=[pscustomobject]@{executable=$historicalPath; executable_sha256=(Get-FileHash -LiteralPath $historicalPath -Algorithm SHA256).Hash.ToLowerInvariant()}}
foreach ($case in @(@{name='execution'; inspection=$false; selected=$false},@{name='memory'; inspection=$false; selected=$true},@{name='memory'; inspection=$true; selected=$false})) {
 $Inspection=$case.inspection; $entry=[pscustomobject]@{name=$case.name}; $historicalSelected=$false; $historicalCustodyValid=$true
 Invoke-Expression $selection[0].Extent.Text
 Invoke-Expression $binding[0].Extent.Text
 if ($historicalSelected -ne $case.selected -or (Test-Path Env:MAESTRO_PRIVATE_WIKI_OLD_WRITER) -ne $case.selected) { throw 'Historical witness escaped memory-only scope' }
 Invoke-Expression $cleanup[0].Extent.Text
 if ((Test-Path Env:MAESTRO_PRIVATE_WIKI_OLD_WRITER) -or -not $historicalCustodyValid) { throw 'Historical cleanup or custody failed' }
}
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
