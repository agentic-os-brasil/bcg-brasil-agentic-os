"""Check the one permitted overlay without invoking Go or modifying production."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest import mock
from types import SimpleNamespace

PATH = Path(__file__).with_name("build.py")
SPEC = importlib.util.spec_from_file_location("producer_build", PATH)
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)


class DescriptionOnlyOverlay(unittest.TestCase):
    def test_exact_current_source_changes_only_index_description(self):
        source = (builder.ROOT / builder.COMPILER).read_bytes()
        before = hashlib.sha256(source).hexdigest()
        variant = builder.overlay_source(source)
        self.assertEqual(variant.replace(builder.VARIANT, builder.ORIGINAL, 1), source)
        self.assertEqual(variant.count(builder.VARIANT), 1)
        self.assertEqual(hashlib.sha256((builder.ROOT / builder.COMPILER).read_bytes()).hexdigest(), before)

    def test_missing_duplicate_and_already_modified_seams_refuse(self):
        for source in [b"missing", builder.ORIGINAL + builder.ORIGINAL, builder.VARIANT]:
            with self.subTest(source=source):
                with self.assertRaises(ValueError):
                    builder.overlay_source(source)

    def test_declared_revision_must_match_checkout_before_build_effects(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".git").write_text("synthetic checkout marker", encoding="utf-8")
            output = root / "proof"
            with mock.patch.object(builder, "ROOT", root), mock.patch.object(
                    builder.subprocess, "run", return_value=SimpleNamespace(stdout="b" * 40)):
                with self.assertRaisesRegex(ValueError, "differs from the checkout"):
                    builder.build(output, "a" * 40, "not-executed")
            self.assertFalse(output.exists())

    def test_existing_output_refuses_before_creating_files_or_running_go(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(ValueError):
                builder.build(root, "a" * 40, "not-executed")
            self.assertEqual(list(root.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
