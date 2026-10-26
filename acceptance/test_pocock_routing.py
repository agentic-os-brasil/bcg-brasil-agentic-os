"""Explicit-only upstream methods remain deliberate choices in the ZIP router."""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("skill_route", ROOT / "bundles/base/tools/skill-route.py")
router = importlib.util.module_from_spec(spec)
spec.loader.exec_module(router)


class PocockRouting(unittest.TestCase):
    def test_explicit_only_method_does_not_route_from_ordinary_words(self):
        idx = {"skills": {"pocock-implement": {"user_invoked_only": True}},
               "terms": {"imple": {"pocock-implement": 4}, "softw": {"pocock-implement": 4}}}
        self.assertEqual(router.route(idx, "implementar software"), [])
        for prompt in ("Read https://example.test/pocock-implement for context",
                       "Read docs/pocock-implement/SKILL.md", "Read /pocock-implement/SKILL.md",
                       "Read ./pocock-implement", "Read https://pocock-implement.example"):
            self.assertEqual(router.route(idx, prompt), [], prompt)
        for prompt in ("pocock-implement", "$pocock-implement", "/pocock-implement for this change"):
            self.assertEqual(router.route(idx, prompt)[0][0], "pocock-implement")

    def test_product_index_retains_upstream_invocation_policy(self):
        previous = router.BUNDLES
        try:
            router.BUNDLES = str(ROOT / "bundles")
            idx = router.build()
        finally:
            router.BUNDLES = previous
        self.assertTrue(idx["skills"]["pocock-implement"]["user_invoked_only"])
        self.assertFalse(idx["skills"]["pocock-tdd"]["user_invoked_only"])


if __name__ == "__main__":
    unittest.main()
