"""Checks the shipped continuation contract shared by both native hosts."""
import json
from pathlib import Path
import unittest

TEMPLATE = Path(__file__).resolve().parents[1] / "installers/zip/user-template"


class UpdateContract(unittest.TestCase):
    def test_contract_is_host_neutral_and_binds_the_migration(self):
        contract = json.loads((TEMPLATE / "UPDATE-CONTRACT.json").read_text(encoding="utf-8"))
        self.assertEqual(contract["schema_version"], 2)
        self.assertEqual(set(contract["hosts"]), {"claude-code", "codex"})
        self.assertEqual(contract["source_versions"], ["0.1.11", "0.1.12"])
        self.assertEqual(contract["receipt"], "brain/.maestro/updates/update-0.2.0.json")
        for binding in ["migration_receipt_sha256", "target_core_sha256", "installation_root_sha256", "runtime_host"]:
            self.assertIn(binding, contract["receipt_bindings"])
        self.assertIn("legacy_namespaces_readable", contract["required_checks"])
        self.assertIn("native_agent_return", contract["required_checks"])
        self.assertEqual(contract["resume"], "first_non_pass_check_after_binding_validation")
        self.assertFalse(contract["caseos_required_for_local_update"])
        self.assertFalse(contract["may_bypass_host_limits"])

    def test_runbook_uses_current_contract_and_no_false_restoration_claim(self):
        text = (TEMPLATE / "UPDATE-RUNBOOK.md").read_text(encoding="utf-8")
        self.assertIn("UPDATE-CONTRACT.json", text)
        self.assertIn("restored_runtime: false", text)
        self.assertNotIn("progress_receipt: data/canary", text)


if __name__ == "__main__":
    unittest.main()
