"""Focused contracts for reconstructed shipped-configuration reader paths."""

import importlib.util
import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "readers_check", ROOT / "bundles/base/tools/readers-check.py")
readers = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(readers)


class ReconstructedReaderPathsTests(unittest.TestCase):
    target = "bundles/base/agents/activation-policy.json"

    def test_parent_join_resolves_the_live_policy_reader(self):
        source = (ROOT / "bundles/base/tools/agent-route.py").read_text(encoding="utf-8")
        self.assertTrue(readers.code_reads(self.target, readers.path_candidates(source)))

    def test_known_parent_join_uses_host_independent_separators(self):
        for directory in ["bundles/base/tools", "bundles\\base\\tools"]:
            with self.subTest(directory=directory):
                source = ('HERE = "' + directory + '"\n'
                          'open(os.path.join(HERE, "..", "agents", "activation-policy.json"))')
                self.assertTrue(readers.code_reads(self.target, readers.path_candidates(source)))

    def test_absolute_and_drive_components_do_not_keep_the_previous_root(self):
        for component, parents in [
            ("/unrelated", 2),
            ("\\unrelated", 2),
            ("C:\\unrelated", 3),
            ("C:unrelated", 2),
            ("\\\\server\\share\\unrelated", 4),
        ]:
            with self.subTest(component=component):
                arguments = [component, *([".."] * parents), "agents", "activation-policy.json"]
                source = ('HERE = "bundles/base/tools"\nopen(os.path.join(HERE, '
                          + ", ".join(json.dumps(value) for value in arguments) + '))')
                self.assertFalse(readers.code_reads(self.target, readers.path_candidates(source)))

    def test_bare_filename_wrong_sibling_and_escaping_join_do_not_count(self):
        for source in [
            'open("activation-policy.json")',
            'open(os.path.join(HERE, "..", "other", "activation-policy.json"))',
            'open(os.path.join(HERE, "..", "..", "agents", "activation-policy.json"))',
            'HERE = "bundles/base"\nopen(os.path.join(HERE, "..", "agents", "activation-policy.json"))',
        ]:
            with self.subTest(source=source):
                self.assertFalse(readers.code_reads(self.target, readers.path_candidates(source)))


if __name__ == "__main__":
    unittest.main()
