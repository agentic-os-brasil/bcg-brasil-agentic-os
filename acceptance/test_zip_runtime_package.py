"""Validate the built artifact, not just the factory's source text."""
import hashlib
import json
import os
from pathlib import Path
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[1]
ZIP = Path(os.environ.get("MAESTRO_ZIP", str(ROOT / "dist/Maestro-v0.2.0-macos.zip")))


class RuntimePackage(unittest.TestCase):
    def test_caseos_enrichment_preparation_is_discoverable_from_catalog(self):
        with zipfile.ZipFile(ZIP) as z:
            for surface in ("bundles/base/skills", ".agents/skills"):
                catalog = json.loads(z.read(f"Maestro/{surface}/catalog.json"))
                entries = {entry["id"]: entry for entry in catalog["skills"]}
                self.assertIn("caseos-prepare-enrichment", entries)
                pointer = entries["caseos-prepare-enrichment"]["relative_path"].removeprefix("skills/")
                self.assertIn(f"Maestro/{surface}/{pointer}", z.namelist())

    def test_caseos_ingest_preparation_is_discoverable_from_catalog(self):
        with zipfile.ZipFile(ZIP) as z:
            for surface in ("bundles/base/skills", ".agents/skills"):
                catalog = json.loads(z.read(f"Maestro/{surface}/catalog.json"))
                entries = {entry["id"]: entry for entry in catalog["skills"]}
                self.assertIn("caseos-prepare-ingest", entries)
                pointer = entries["caseos-prepare-ingest"]["relative_path"].removeprefix("skills/")
                self.assertIn(f"Maestro/{surface}/{pointer}", z.namelist())

    def test_caseos_guide_and_tutorial_resolve_in_both_skill_surfaces(self):
        # Catch a release that advertises caseOS but omits its offline learning material.
        with zipfile.ZipFile(ZIP) as z:
            self.assertIn("Maestro/CASEOS-GUIDE.md", z.namelist())
            for surface in ("bundles/base/skills", ".agents/skills"):
                self.assertIn(f"Maestro/{surface}/caseos-tutorial/SKILL.md", z.namelist())
                self.assertIn(f"Maestro/{surface}/caseos-tutorial/references/guide.md", z.namelist())
                self.assertEqual(z.read("Maestro/CASEOS-GUIDE.md"),
                                 z.read(f"Maestro/{surface}/caseos-tutorial/references/guide.md"))

    def test_no_personal_state_is_distributed(self):
        with zipfile.ZipFile(ZIP) as z:
            forbidden = [name for name in z.namelist()
                         if name.startswith(("Maestro/data/", "Maestro/brain/"))]
            self.assertEqual(forbidden, [])
            self.assertFalse(any(n.endswith(".go") for n in z.namelist()), "factory sources leaked")

    def test_codex_skills_are_native_discoverable_copies(self):
        with zipfile.ZipFile(ZIP) as z:
            canonical = [n for n in z.namelist() if n.startswith("Maestro/bundles/base/skills/") and n.endswith("/SKILL.md")]
            self.assertGreater(len(canonical), 20)
            for source in canonical:
                target = source.replace("bundles/base/skills/", ".agents/skills/")
                self.assertIn(target, z.namelist())
                self.assertEqual(z.read(source), z.read(target))
            self.assertIn("Maestro/.agents/skills/caseos-connect/SKILL.md", z.namelist())

    def test_managed_helper_manifest_matches_both_architectures(self):
        with zipfile.ZipFile(ZIP) as z:
            manifest = json.loads(z.read("Maestro/runtime/manifest.json"))
            self.assertEqual(manifest["schema_version"], 1)
            self.assertEqual(manifest["version"], z.read("Maestro/VERSION").decode().strip())
            target = "windows" if "windows" in ZIP.name else "darwin"
            self.assertEqual({(a["os"], a["arch"]) for a in manifest["artifacts"]},
                             {(target, "arm64"), (target, "amd64")})
            for artifact in manifest["artifacts"]:
                path = artifact["path"]
                self.assertTrue(path.startswith("runtime/"))
                self.assertNotIn("..", path.split("/"))
                binary = z.read("Maestro/" + path)
                self.assertEqual(hashlib.sha256(binary).hexdigest(), artifact["sha256"])
                self.assertGreater(len(binary), 100_000)
                if target == "windows":
                    self.assertTrue(binary.startswith(b"MZ"))
                else:
                    self.assertTrue(z.getinfo("Maestro/" + path).external_attr >> 16 & 0o111)


if __name__ == "__main__":
    unittest.main()
