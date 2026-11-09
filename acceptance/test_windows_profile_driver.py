"""Closed qualification inputs and actual launcher fragments without accounts."""
import hashlib
import importlib.util
import json
import os
import re
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

    def migration_manifest(self):
        suites = {}
        for name, files in [("winprivate", ["internal/winprivate/migration_windows_test.go"]),
                            ("zipmigration", ["internal/zipmigration/external_fs_windows_test.go",
                                              "internal/zipmigration/external_durability_windows_test.go"])]:
            suites[name] = [test for file in files for test in re.findall(
                r"^func (TestWindows(?:Migration|ExternalAdapter)\w+)\(", (ROOT / file).read_text(), re.M)]
        self.assertEqual([len(suites[name]) for name in ["winprivate", "zipmigration"]], [12, 13])
        names = ["execution", "datahome", "ownerctx", "memory", "privatewiki", "processidentity", "winprivate", "zipmigration"]
        return dict(schema_version=1, qualification_profile="quarantined_windows_migration_primitives_v2",
                    packages=[dict(name=name, tests=suites.get(name, ["TestFixture"])) for name in names])

    def canonical_admission(self, document, expected):
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        result = subprocess.run([self.pwsh, "-NoProfile", "-File", str(DRIVER), "-CanonicalManifest", str(self.manifest),
                                 "-TestBinary", "unused", "-Server", "unused", "-OutputRoot", str(self.root / "proof")],
                                capture_output=True, text=True, timeout=30)
        self.assert_preaccount_refusal(result, expected)

    def test_migration_profile_requires_exact_eight_packages_and_preserves_default_six(self):
        document = self.migration_manifest()
        self.canonical_admission(document, "Canonical public source and SDK inputs")
        original = document["packages"][-1]["name"]
        for name in ["ZIPMIGRATION", "arbitrary", "winprivate"]:
            document["packages"][-1]["name"] = name
            self.canonical_admission(document, "Migration primitive qualification requires exactly")
        document["packages"][-1]["name"] = original
        document["packages"].pop()
        self.canonical_admission(document, "Migration primitive qualification requires exactly")
        document["packages"].pop()
        del document["qualification_profile"]
        self.canonical_admission(document, "Canonical public source and SDK inputs")
        document["qualification_profile"] = "arbitrary-profile"
        self.canonical_admission(document, "Unknown canonical qualification profile")

    def test_migration_profile_requires_both_native_durability_cases(self):
        document = self.migration_manifest()
        self.canonical_admission(document, "Canonical public source and SDK inputs")
        for test in ("TestWindowsExternalAdapterDurabilityRetriesHeldObjectsWithoutSourceEffects",
                     "TestWindowsExternalAdapterDurabilityRetainsParentCustodyAcrossRollback"):
            with self.subTest(test=test):
                changed = json.loads(json.dumps(document))
                changed["packages"][-1]["tests"].remove(test)
                self.canonical_admission(changed, "Migration primitive inventory differs")

    def test_migration_profile_refuses_missing_extra_or_duplicate_native_cases(self):
        document = self.migration_manifest()
        for index in [-2, -1]:
            original = list(document["packages"][index]["tests"])
            prefix = "TestWindowsMigration" if index == -2 else "TestWindowsExternalAdapter"
            for tests in [original[:-1], original + [prefix + "Unexpected"], original + [original[0]]]:
                with self.subTest(package=document["packages"][index]["name"], tests=len(tests)):
                    document["packages"][index]["tests"] = tests
                    self.canonical_admission(document, "Migration primitive inventory differs from the fixed required cases")
            document["packages"][index]["tests"] = original

    def test_migration_native_acceptance_requires_actual_pass_complete_inventory_and_no_nested_skip(self):
        self.launcher_fragment_check(r"""
$check=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$migrationProfile -and $entry.name -cin @(''winprivate'', ''zipmigration'')'},$true))
if ($check.Count -ne 1) { throw 'Expected actual migration primitive acceptance block' }
$migrationProfile=$true
foreach ($case in @(@{outcomes=@('PASS'); code=0; complete=$true; skips=@(); expected=$true},@{outcomes=@(); code=0; complete=$true; skips=@(); expected=$false},@{outcomes=@('PASS','PASS'); code=0; complete=$true; skips=@(); expected=$false},@{outcomes=@('FAIL'); code=1; complete=$true; skips=@(); expected=$false},@{outcomes=@('SKIP'); code=0; complete=$true; skips=@(); expected=$false},@{outcomes=@('PASS'); code=0; complete=$true; skips=@('TestRequired/nested'); expected=$false},@{outcomes=@('PASS'); code=1; complete=$true; skips=@(); expected=$false},@{outcomes=@('PASS'); code=0; complete=$false; skips=@(); expected=$false})) {
 $entry=[pscustomobject]@{name='winprivate'; migration_required_tests=@('TestRequired')}; $code=$case.code; $complete=$case.complete; $allSkips=$case.skips
 $outcomes=@($case.outcomes | ForEach-Object { @{test='TestRequired'; outcome=$_} }); $results=@([ordered]@{})
 Invoke-Expression $check[0].Extent.Text
 if ($results[-1].migration_primitives_proof.actual_required_tests_passed -ne $case.expected -or $results[-1].migration_primitives_proof.public_transaction_qualified -ne $false) { throw 'Migration primitive proof accepted absent, failing, skipped or incomplete execution' }
}
""")

    def test_migration_report_does_not_upgrade_missing_coverage_or_public_transaction(self):
        self.launcher_fragment_check(r"""
$gate=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$migrationProfile' -and $node.Extent.Text -match '\$migrationPrimitivesPassed\s*='},$true))
$reportBlock=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$migrationProfile' -and $node.Extent.Text -match '\$report.quarantined_migration_primitives'},$true))
if ($gate.Count -ne 1 -or $reportBlock.Count -ne 1) { throw 'Expected actual migration aggregate gate and report' }
$migrationProfile=$true
foreach ($case in @(@{count=2; passed=$true; prior=$true; expected=$true},@{count=1; passed=$true; prior=$true; expected=$false},@{count=2; passed=$false; prior=$true; expected=$false},@{count=2; passed=$true; prior=$false; expected=$false},@{count=2; passed=$true; prior=$true; duplicate=$true; expected=$false})) {
 $results=@(@{name='winprivate'; migration_primitives_proof=@{actual_required_tests_passed=$case.passed}},@{name='zipmigration'; migration_primitives_proof=@{actual_required_tests_passed=$case.passed}}) | Select-Object -First $case.count
 if ($case.duplicate) { $results[1].name='winprivate' }
 $qualificationComplete=$case.prior; $report=[ordered]@{}
 Invoke-Expression $gate[0].Extent.Text
 Invoke-Expression $reportBlock[0].Extent.Text
 if ($qualificationComplete -ne $case.expected -or $report.quarantined_migration_primitives.public_transaction_qualified -ne $false -or $report.quarantined_migration_primitives.mutation_gate -cne 'unavailable') { throw 'Primitive acceptance upgraded missing coverage or public mutation authority' }
}
$migrationProfile=$false; $report=[ordered]@{}
Invoke-Expression $reportBlock[0].Extent.Text
if ($report.Count -ne 0) { throw 'Migration profile changed the default report' }
""")

    def public_migration_manifest(self):
        document = self.migration_manifest()
        document["qualification_profile"] = "windows_public_external_migration_v1"
        public = re.findall(r"^func (TestWindowsExternalPublic\w+)\(",
                            (ROOT / "internal/zipmigration/external_public_windows_test.go").read_text(), re.M)
        self.assertEqual(len(public), 3)
        document["packages"][-1]["tests"] += public + ["TestExternalWindowsUnreviewedDigestRefusesBeforeEffects"]
        document["packages"][1]["tests"].append("TestResolveCommittedMigrationUsesSealedSourceIdentity")
        return document

    def canonical_contract_manifest(self):
        document = self.public_migration_manifest()
        document["qualification_profile"] = "windows_canonical_contracts_v1"
        required = {
            "execution": ["TestStoreRejectsSymlinkParent", "TestWindowsConfinedStoreRejectsJunctionAndPrivateHardlink"],
            "processidentity": ["TestPinnedFileNeverAdoptsAReplacement", "TestPinnedWindowsFileBlocksReplacement"],
            "memory": ["TestPrivateWikiCompatibleProducerAcrossActualBinaries", "TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete"],
            "datahome": ["TestResolveRejectsUnsafeSelectedPathsAndConflictingLocator", "TestResolveCaseDistinctNondefaultCannotBypassLocator", "TestResolveCaseDistinctLocatorCannotAuthorizeDifferentHome", "TestWindowsResolveCaseAliasesRespectLocatorAuthority"],
            "zipmigration": ["TestCommittedTargetSafetyAndRecovery", "TestCopyFailureRetriesAfterPermissionRecovery", "TestReadOnlyDestinationReportsFailureWithoutSuccessMarker", "TestWindowsExternalCaseCapabilityPreservesOverlapGuards",
                "TestExternalOwnerAuthorityRequiresFreshEnrollment", "TestExternalOwnerControlsPreserveAuthoredNeighbors", "TestExternalOwnerObservationsRequireFreshAttestation", "TestExternalCurrentBrainSurvivesLegacyEvolution", "TestExternalReviewedPlanCannotOmitOrRedirectSource", "TestExternalControlStagingNeverWritesInsideSource", "TestExternalPhysicalWriteSetAliasesRefuseBeforeEffects", "TestExternalPrivateControlDirectoriesRefuseWithoutRepair", "TestExternalHomeCannotOverlapProfilePublication", "TestExternalMissingReservedProfileAliasesRefuseBeforeEffects", "TestExternalDistinctCaseSensitiveRootRemainsSupported", "TestExternalRollbackRetainedPathsUseArchive", "TestExternalReservedAuthorityAliasesAreInactive", "TestExternalProofIsSelfContainedAndStrict", "TestExternalForeignLocatorDoesNotRevokeOrMoveHome", "TestExternalPreflightConflictsHaveNoProfileOrSourceEffects", "TestExternalConflictingPartialAndConcurrentAttemptStayOwned", "TestExternalRetirementIsFixedDenyOnlyAndInterruptedRollbackResumes", "TestExternalCaseAuthorityIsArchivedOutsideActiveReaders", "TestExternalInterruptionAndRollbackRetainNewWork", "TestExternalLegacyAuthoredStateAndInactiveAuthority"],
        }
        for entry in document["packages"]:
            entry["tests"] += required.get(entry["name"], [])
        return document

    def test_canonical_profile_requires_closed_original_and_native_inventory(self):
        document = self.canonical_contract_manifest()
        self.canonical_admission(document, "Canonical public source and SDK inputs")
        for package, name in [("execution", "TestStoreRejectsSymlinkParent"), ("execution", "TestWindowsConfinedStoreRejectsJunctionAndPrivateHardlink"),
                              ("processidentity", "TestPinnedWindowsFileBlocksReplacement"), ("datahome", "TestResolveCaseDistinctLocatorCannotAuthorizeDifferentHome"),
                              ("datahome", "TestWindowsResolveCaseAliasesRespectLocatorAuthority"), ("zipmigration", "TestCommittedTargetSafetyAndRecovery"),
                              ("zipmigration", "TestExternalReviewedPlanCannotOmitOrRedirectSource"), ("memory", "TestPrivateWikiOldCohortProcessCannotReadCaptureOrDelete")]:
            with self.subTest(package=package, missing=name):
                changed = json.loads(json.dumps(document))
                entry = next(item for item in changed["packages"] if item["name"] == package)
                entry["tests"].remove(name)
                self.canonical_admission(changed, "Canonical Windows inventory differs from its closed contract")

    def canonical_fragment_check(self, assertions):
        document = self.canonical_contract_manifest()
        self.manifest.write_text(json.dumps(document), encoding="utf-8")
        self.launcher_fragment_check(r"""
$outer=[Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$null,[ref]$null)
$declaration=@($outer.FindAll({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Clauses[0].Item1.Extent.Text -ceq '$withCanonicalContracts'},$true))
if($declaration.Count -ne 1){throw 'Expected closed canonical declarations'}
$canonical=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'inspection.json') -Raw | ConvertFrom-Json
$withCanonicalContracts=$true
Invoke-Expression $declaration[0].Extent.Text
foreach($name in @('Get-CanonicalLogRecords','Test-CanonicalObservedPass','Test-CanonicalCasePair','Get-CanonicalPackageProof')) {
 $fn=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true))
 if($fn.Count -ne 1){throw ('Expected actual canonical proof function: '+$name)}
 Invoke-Expression $fn[0].Extent.Text
}
function New-CanonicalObservedBody($entry) {
 $lines=@()
 $policyNames=@($entry.canonical_skip_policy | ForEach-Object {$_.test})
 foreach($name in @(@($entry.tests)+@($entry.canonical_required_tests)+@($entry.canonical_original_assertions) | Sort-Object -Unique)) {
  if($name -cnotin $policyNames){$lines+='--- PASS: '+$name+' (0.01s)'}
 }
 foreach($policy in @($entry.canonical_skip_policy)) {
  $lines+='=== RUN   '+$policy.test
  if($policy.kind -ceq 'selected-directory-case-pair') {
   $lines+='=== RUN   '+$policy.paired
   $evidence=[ordered]@{kind='selected-directory-case-pair-v1';scope=$policy.paired;selected=(Join-Path $PSScriptRoot 'fixture/.maestro');counterpart=(Join-Path $PSScriptRoot 'fixture/.MAESTRO');selected_identity=@(123,4,5);counterpart_identity=@(123,4,5);same_object=$true}
   $lines+='    fixture_test.go:12: case_pair_evidence='+($evidence | ConvertTo-Json -Depth 4 -Compress)
   $lines+='--- PASS: '+$policy.paired+' (0.01s)'
   $lines+='=== NAME  '+$policy.test
  }
  $lines+='    fixture_test.go:13: '+$policy.reason
  $lines+='--- SKIP: '+$policy.test+' (0.01s)'
 }
 return ($lines -join "`n")
}
""" + assertions)

    def test_canonical_package_requires_actual_ports_and_exact_counterparts(self):
        self.canonical_fragment_check(r"""
foreach($name in @('execution','processidentity','zipmigration','memory')) {
 $entry=@($canonical.packages | Where-Object {$_.name -ceq $name})[0]
 $body=New-CanonicalObservedBody $entry
 $proof=Get-CanonicalPackageProof $entry $body 0 $true
 if(-not $proof.qualified -or $proof.original_assertions.Count -ne $entry.canonical_original_assertions.Count){throw ('Actual original/native coverage refused: '+$name)}
 foreach($mode in @('missing','duplicate','nested-duplicate','fail','skip','nested-skip','unknown-skip','exit','partial','wrong-reason')) {
  if($mode -ceq 'wrong-reason' -and $entry.canonical_skip_policy.Count -eq 0){continue}
  $altered=$body; $code=0; $complete=$true; $required=$entry.canonical_required_tests[0]
  switch($mode) {
   'missing' {$altered=$altered.Replace(('--- PASS: '+$required+' (0.01s)'), '')}
   'duplicate' {$altered+="`n--- PASS: "+$required+' (0.01s)'}
   'nested-duplicate' {$altered+="`n    --- PASS: "+$required+'/hidden (0.01s)'+"`n    --- PASS: "+$required+'/hidden (0.01s)'}
   'fail' {$altered=$altered.Replace(('--- PASS: '+$required+' (0.01s)'), ('--- FAIL: '+$required+' (0.01s)'))}
   'skip' {$altered=$altered.Replace(('--- PASS: '+$required+' (0.01s)'), ('--- SKIP: '+$required+' (0.01s)'))}
   'nested-skip' {$altered+="`n    --- SKIP: "+$required+'/hidden (0.01s)'}
   'unknown-skip' {$altered+="`n--- SKIP: TestUnknown/hidden (0.01s)"}
   'exit' {$code=1}
   'partial' {$complete=$false}
   'wrong-reason' {$altered=$altered.Replace($entry.canonical_skip_policy[0].reason, 'arbitrary unavailable')}
  }
  if((Get-CanonicalPackageProof $entry $altered $code $complete).qualified){throw ('Unobserved native coverage accepted: '+$name+'/'+$mode)}
 }
}
""")

    def test_canonical_case_classification_binds_original_directory_and_child(self):
        self.canonical_fragment_check(r"""
$entry=@($canonical.packages | Where-Object {$_.name -ceq 'datahome'})[0]
$original=New-CanonicalObservedBody $entry
if(-not (Get-CanonicalPackageProof $entry $original 0 $true).qualified){throw 'Actual paired directory proof refused'}
$policy=$entry.canonical_skip_policy[0]
$maxBody=$original.Replace('[123,4,5]', '[4294967295,4294967295,4294967294]')
if(-not (Get-CanonicalPackageProof $entry $maxBody 0 $true).qualified){throw 'Exact maximum uint32 identity words were lost'}
$highMismatch=$maxBody.Replace('"counterpart_identity":[4294967295,4294967295,4294967294]', '"counterpart_identity":[4294967295,4294967294,4294967294]')
if((Get-CanonicalPackageProof $entry $highMismatch 0 $true).qualified){throw 'High file index identity mismatch was rounded or accepted'}
foreach($mode in @('missing-marker','wrong-scope','unrelated-emitter','duplicate-marker','unknown-key','duplicate-key','wrong-kind','string-boolean','false-boolean','zero-id','different-id','overflow-id','fraction-id','quoted-id','exponent-id','large-uint64-id','wrong-path','missing-child','child-skip','wrong-reason','unknown-child')) {
 $lines=@($original -split "`n"); $index=0
 for($i=0;$i -lt $lines.Count;$i++){if($lines[$i] -match 'case_pair_evidence='){$index=$i;break}}
 $line=$lines[$index]; $json=$line.Substring($line.IndexOf('case_pair_evidence=')+'case_pair_evidence='.Length)
 $record=$json | ConvertFrom-Json -AsHashtable
 switch($mode) {
  'missing-marker' {$lines[$index]=''}
  'wrong-scope' {$record.scope='TestOther/same-object-case-contract'}
  'unrelated-emitter' {$lines[$index-1]='=== RUN   TestOther/same-object-case-contract'}
  'duplicate-marker' {$lines[$index]+="`n"+$line}
  'unknown-key' {$record.unapproved=$true}
  'duplicate-key' {$lines[$index]=$line.Replace('{','{"same_object":true,')}
  'wrong-kind' {$record.kind='volume-name-only'}
  'string-boolean' {$record.same_object='true'}
  'false-boolean' {$record.same_object=$false}
  'zero-id' {$record.selected_identity=@(123,0,0);$record.counterpart_identity=@(123,0,0)}
  'different-id' {$record.counterpart_identity=@(123,4,6)}
  'overflow-id' {$record.selected_identity=@(123,4294967296,5);$record.counterpart_identity=@(123,4294967296,5)}
  'fraction-id' {$record.selected_identity=@(123,4.5,5);$record.counterpart_identity=@(123,4.5,5)}
  'quoted-id' {$record.selected_identity=@(123,'4',5);$record.counterpart_identity=@(123,'4',5)}
  'exponent-id' {$lines[$index]=$line.Replace('[123,4,5]', '[123,4e0,5]')}
  'large-uint64-id' {$lines[$index]=$line.Replace('[123,4,5]', '[123,18446744073709551615,5]')}
  'wrong-path' {$record.counterpart=Join-Path $PSScriptRoot 'unrelated/other'}
  'missing-child' {$lines=@($lines | Where-Object {$_ -cne ('--- PASS: '+$policy.paired+' (0.01s)')})}
  'child-skip' {$lines+=('    --- SKIP: '+$policy.paired+'/hidden (0.01s)')}
  'wrong-reason' {$lines=@($lines | ForEach-Object {$_.Replace($policy.reason,'unrelated skip reason')})}
  'unknown-child' {$lines+=('    --- SKIP: '+$policy.test+'/unknown (0.01s)')}
 }
 if($mode -cin @('wrong-scope','unknown-key','wrong-kind','string-boolean','false-boolean','zero-id','different-id','overflow-id','fraction-id','quoted-id','wrong-path')){$lines[$index]='    fixture_test.go:12: case_pair_evidence='+($record | ConvertTo-Json -Depth 4 -Compress)}
 if((Get-CanonicalPackageProof $entry ($lines -join "`n") 0 $true).qualified){throw ('Invalid original-directory classification accepted: '+$mode)}
}
""")

    def test_canonical_aggregate_keeps_witnesses_prerequisites_and_every_package(self):
        self.canonical_fragment_check(r"""
$gate=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Clauses[0].Item1.Extent.Text -ceq '$canonicalProfile' -and $n.Extent.Text -match '\$canonicalQualified\s*='},$true))
$wire=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Clauses[0].Item1.Extent.Text -ceq '$canonicalProfile' -and $n.Extent.Text -match '\$results\[-1\].canonical_contract_proof'},$true))
if($gate.Count -ne 1 -or $wire.Count -ne 1){throw 'Expected actual canonical aggregate and package wiring'}
$canonicalProfile=$true
foreach($mode in @('pass','missing','duplicate','bad-package','exit','partial','backend','public','resolver','producer','history','producer-absent','history-absent','original-count')) {
 $manifest=[pscustomobject]@{compatible_producer=@{sealed=$true};historical_witness=@{sealed=$true}}
 $results=@()
 foreach($entry in $canonical.packages){
  $body=New-CanonicalObservedBody $entry;$code=0;$complete=$true;$results+=@{name=$entry.name;exit_code=0;inventory_complete=$true}
  Invoke-Expression $wire[0].Extent.Text
  $results[-1].compatible_producer_proof=@{actual_compatibility_test_passed=$true;copied_pe_custody=$true}
  $results[-1].historical_witness_proof=@{actual_historical_test_passed=$true;copied_pe_custody=$true}
 }
 $migrationPrimitivesPassed=$true;$publicMigrationPassed=$true;$canonicalResolutionQualified=$true;$qualificationComplete=$false
 switch($mode){
  'missing' {$results=@($results | Select-Object -Skip 1)}
  'duplicate' {$results[1].name=$results[0].name}
  'bad-package' {$results[0].canonical_contract_proof.qualified=$false}
  'exit' {$results[0].exit_code=1}
  'partial' {$results[0].inventory_complete=$false}
  'backend' {$migrationPrimitivesPassed=$false}
  'public' {$publicMigrationPassed=$false}
  'resolver' {$canonicalResolutionQualified=$false}
  'producer' {$results[3].compatible_producer_proof.actual_compatibility_test_passed=$false}
  'history' {$results[3].historical_witness_proof.copied_pe_custody=$false}
  'producer-absent' {$manifest.compatible_producer=$null}
  'history-absent' {$manifest.historical_witness=$null}
  'original-count' {$results[0].canonical_contract_proof.original_assertions=@()}
 }
 Invoke-Expression $gate[0].Extent.Text
 if($qualificationComplete -ne ($mode -ceq 'pass')){throw ('Canonical aggregate ignored prerequisite: '+$mode)}
}
""")

    def test_canonical_report_preserves_actual_source_skips_and_workflow_profile(self):
        self.assertIn("qualification_profile='windows_canonical_contracts_v1'", (ROOT / ".github/workflows/product-mcp.yml").read_text())
        self.canonical_fragment_check(r"""
$block=@($ast.FindAll({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and $n.Clauses[0].Item1.Extent.Text -ceq '$canonicalProfile' -and $n.Extent.Text -match '\$report.canonical_contracts'},$true))
if($block.Count -ne 1){throw 'Expected actual final canonical report'}
$canonicalProfile=$true;$canonicalQualified=$true;$results=@()
foreach($entry in $canonical.packages){$body=New-CanonicalObservedBody $entry;$results+=@{canonical_contract_proof=(Get-CanonicalPackageProof $entry $body 0 $true)}}
$report=[ordered]@{public_external_migration=@{}}
Invoke-Expression $block[0].Extent.Text
if($report.qualification_profile -cne 'windows_canonical_contracts_v1' -or $report.canonical_contracts.original_assertions_count -ne 31 -or
 $report.canonical_contracts.executed_original_assertions_count -ne 26 -or $report.canonical_contracts.classified_original_assertions_count -ne 5 -or
 $report.canonical_contracts.all_original_source_assertions_executed -ne $false -or $report.canonical_contracts.classified_skips.Count -ne 5 -or
 @($report.canonical_contracts.classified_skips | Where-Object {$_.original_outcome -cne 'SKIP' -or $_.paired_outcome -cne 'PASS'}).Count -ne 0 -or
 $report.canonical_contracts.all_admitted_windows_contracts_qualified -ne $true){throw 'Canonical report lost actual skip or observed counterpart provenance'}
""")

    def test_public_migration_profile_requires_exact_public_and_resolver_inventory(self):
        document = self.public_migration_manifest()
        self.canonical_admission(document, "Canonical public source and SDK inputs")
        for name in ["TestWindowsExternalPublicReviewedPlanRefusesBeforeEffects",
                     "TestWindowsExternalPublicCommitPreservesOriginalAndOfflineProof",
                     "TestWindowsExternalPublicInterruptionResumesAndRollbackRetainsWork",
                     "TestExternalWindowsUnreviewedDigestRefusesBeforeEffects",
                     "TestResolveCommittedMigrationUsesSealedSourceIdentity"]:
            with self.subTest(missing=name):
                changed = json.loads(json.dumps(document))
                entry = changed["packages"][1 if name.startswith("TestResolve") else -1]
                entry["tests"].remove(name)
                self.canonical_admission(changed, "Public migration inventory differs from the fixed required cases")
        for name in ["TestWindowsExternalPublicUnexpected", "TestWindowsExternalPublicReviewedPlanRefusesBeforeEffects"]:
            with self.subTest(extra=name):
                changed = json.loads(json.dumps(document))
                changed["packages"][-1]["tests"].append(name)
                self.canonical_admission(changed, "Public migration inventory differs from the fixed required cases")
        document["qualification_profile"] = "quarantined_windows_migration_primitives_v1"
        self.canonical_admission(document, "Unknown canonical qualification profile")

    def test_public_native_acceptance_requires_observed_pass_and_no_nested_skip(self):
        self.launcher_fragment_check(r"""
$check=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$publicMigrationProfile -and $entry.name -cin @(''datahome'', ''zipmigration'')'},$true))
if ($check.Count -ne 1) { throw 'Expected actual public package outcome gate' }
$publicMigrationProfile=$true
foreach ($package in @('zipmigration','datahome')) {
 $required=if($package -ceq 'zipmigration') {@('TestWindowsExternalPublicReviewedPlanRefusesBeforeEffects','TestWindowsExternalPublicCommitPreservesOriginalAndOfflineProof','TestWindowsExternalPublicInterruptionResumesAndRollbackRetainsWork','TestExternalWindowsUnreviewedDigestRefusesBeforeEffects')}else{@('TestResolveCommittedMigrationUsesSealedSourceIdentity')}
 foreach ($mode in @('pass','absent','duplicate','fail','skip','nested-skip','lowercase-pass','exit','partial','missing-required')) {
  $entry=[pscustomobject]@{name=$package; public_migration_required_tests=$required}; $code=0; $complete=$true; $allSkips=@()
  $outcomes=@($required | ForEach-Object { @{test=$_;outcome='PASS'} })
  switch ($mode) {
   'absent' {$outcomes=@($outcomes | Select-Object -Skip 1)}
   'duplicate' {$outcomes+=@{test=$required[0];outcome='PASS'}}
   'fail' {$outcomes[0].outcome='FAIL'}
   'skip' {$outcomes[0].outcome='SKIP'}
   'nested-skip' {$allSkips=@($required[0]+'/unavailable')}
   'lowercase-pass' {$outcomes[0].outcome='pass'}
   'exit' {$code=1}
   'partial' {$complete=$false}
   'missing-required' {$entry.public_migration_required_tests=@()}
  }
  $results=@([ordered]@{})
  Invoke-Expression $check[0].Extent.Text
  if ($results[-1].public_migration_proof.actual_required_tests_passed -ne ($mode -ceq 'pass')) {throw ('Unobserved public/resolver coverage was accepted: '+$package+'/'+$mode)}
 }
}
""")

    def test_public_report_preserves_legacy_missing_coverage_and_prerequisites(self):
        self.launcher_fragment_check(r"""
$gate=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$publicMigrationProfile' -and $node.Extent.Text -match '\$publicTransactionQualified\s*='},$true))
$reportBlock=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '$publicMigrationProfile' -and $node.Extent.Text -match '\$report.public_external_migration'},$true))
if ($gate.Count -ne 1 -or $reportBlock.Count -ne 1) {throw 'Expected actual public aggregate and report blocks'}
$publicMigrationProfile=$true
foreach ($case in @(@{backend=$true; public=$true; resolver=$true; prior=$true; skips=@(); publicExpected=$true; resolverExpected=$true; overall=$true},@{backend=$true; public=$true; resolver=$true; prior=$true; skips=@('TestLegacy/known'); publicExpected=$true; resolverExpected=$true; overall=$false},@{backend=$true; public=$true; resolver=$true; prior=$false; skips=@(); publicExpected=$true; resolverExpected=$true; overall=$false},@{backend=$false; public=$true; resolver=$true; prior=$true; skips=@(); publicExpected=$false; resolverExpected=$false; overall=$false},@{backend=$true; public=$false; resolver=$true; prior=$true; skips=@(); publicExpected=$false; resolverExpected=$false; overall=$false},@{backend=$true; public=$true; resolver=$false; prior=$true; skips=@(); publicExpected=$true; resolverExpected=$false; overall=$false},@{backend=$true; public=$true; resolver=$true; prior=$true; missing=$true; skips=@(); publicExpected=$true; resolverExpected=$false; overall=$false},@{backend=$true; public=$true; resolver=$true; prior=$true; duplicate=$true; skips=@(); publicExpected=$false; resolverExpected=$false; overall=$false})) {
 $migrationPrimitivesPassed=$case.backend; $qualificationComplete=$case.prior
 $results=@(@{name='zipmigration'; exit_code=0; inventory_complete=$true; all_skipped_tests=@(); public_migration_proof=@{actual_required_tests_passed=$case.public}},@{name='datahome'; exit_code=0; inventory_complete=$true; all_skipped_tests=@(); public_migration_proof=@{actual_required_tests_passed=$case.resolver}},@{name='memory'; exit_code=0; inventory_complete=$true; all_skipped_tests=$case.skips})
 if($case.missing){$results=@($results | Where-Object {$_.name -cne 'datahome'})}
 if($case.duplicate){$results+=@{name='zipmigration';exit_code=0;inventory_complete=$true;all_skipped_tests=@();public_migration_proof=@{actual_required_tests_passed=$true}}}
 Invoke-Expression $gate[0].Extent.Text
 $report=[ordered]@{qualification_complete=$qualificationComplete}
 Invoke-Expression $reportBlock[0].Extent.Text
 if($report.qualification_complete -ne $case.overall -or $report.public_external_migration.public_transaction_qualified -ne $case.publicExpected -or $report.public_external_migration.canonical_resolution_qualified -ne $case.resolverExpected -or $report.public_external_migration.observed_skips_count -ne $case.skips.Count){throw 'Public subset proof upgraded missing prerequisites or legacy coverage'}
}
$publicMigrationProfile=$false; $report=[ordered]@{}
Invoke-Expression $reportBlock[0].Extent.Text
if($report.Count -ne 0){throw 'Public qualification fields changed a historical profile'}
""")

    def test_public_profile_keeps_incomplete_coverage_failure(self):
        self.launcher_fragment_check(r"""
$failure=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -ceq '-not $qualificationComplete' -and $node.Extent.Text -match 'Canonical public migration coverage remains incomplete'},$true))
if($failure.Count -ne 1){throw 'Expected preserved final incomplete coverage failure'}
$qualificationComplete=$false; $Inspection=$false; $migrationProfile=$true; $publicMigrationProfile=$true
$refused=$false
try {Invoke-Expression $failure[0].Extent.Text}catch{if($_.Exception.Message -notmatch 'Canonical public migration coverage remains incomplete'){throw};$refused=$true}
if(-not $refused){throw 'Public subset received a successful exit despite unresolved coverage'}
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
