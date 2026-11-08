"""Public packaged Python commands consume the real admitted HOME decision."""
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
TEMP = Path(os.environ.get('MAESTRO_TEST_TMP', str(ROOT.parent / 'maestro-python-e2a-tmp')))
TOOLS = ('brain-index.py', 'brain-route.py', 'skill-route.py', 'paths-check.py')


@unittest.skipIf(os.name == "nt", "External HOME mutation is unavailable on Windows")
class CanonicalHomeTools(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        TEMP.mkdir(parents=True, exist_ok=True)
        cls.build = tempfile.TemporaryDirectory(dir=TEMP)
        cls.binary = Path(cls.build.name) / 'maestro-runtime'
        env = dict(os.environ, GOCACHE=os.environ.get('GOCACHE', str(TEMP / 'go-cache')))
        subprocess.run(['go', 'build', '-o', str(cls.binary), './cmd/maestro-runtime'],
                       cwd=ROOT, env=env, check=True, capture_output=True)

    @classmethod
    def tearDownClass(cls):
        cls.build.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=TEMP)
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.product = self.base / 'product version one'
        self.product.mkdir()
        shutil.copytree(ROOT / 'bundles/base/tools', self.product / 'bundles/base/tools',
                        ignore=shutil.ignore_patterns('__pycache__'))
        native_os = {'Darwin': 'darwin', 'Linux': 'linux', 'Windows': 'windows'}[platform.system()]
        native_arch = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64', 'AMD64': 'amd64'}[platform.machine()]
        rel = f'runtime/{native_os}-{native_arch}/maestro-runtime'
        helper = self.product / rel
        helper.parent.mkdir(parents=True)
        shutil.copy2(self.binary, helper)
        (self.product / 'VERSION').write_text('0.2.0\n')
        (self.product / 'runtime/manifest.json').write_text(json.dumps({
            'schema_version': 1, 'version': '0.2.0', 'artifacts': [{
                'os': native_os, 'arch': native_arch, 'path': rel,
                'sha256': hashlib.sha256(helper.read_bytes()).hexdigest()}]}))
        self.user = self.base / 'user home'
        self.user.mkdir(mode=0o700)
        self.home = self.user / 'private Maestro'
        self.env = dict(os.environ, MAESTRO_HOME=str(self.home), PYTHONDONTWRITEBYTECODE='1')
        self.cwd = self.base / 'outside product/nested'
        self.cwd.mkdir(parents=True)
        self.helper = helper

    def tool(self, name, *args, product=None):
        product = product or self.product
        return subprocess.run([sys.executable, str(product / 'bundles/base/tools' / name),
                               '--user-home', str(self.user), *args], cwd=self.cwd,
                              env=self.env, text=True, capture_output=True)

    def initialize(self):
        result = subprocess.run([str(self.helper), 'home', 'init', '--product-root', str(self.product),
                                 '--user-home', str(self.user)], env=self.env, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)

    def test_uninitialized_public_commands_fail_before_output_or_write(self):
        before = sorted(str(p.relative_to(self.base)) for p in self.base.rglob('*'))
        for tool in TOOLS:
            with self.subTest(tool=tool):
                result = self.tool(tool, '--check' if tool == 'brain-index.py' else '--names')
                self.assertEqual(result.returncode, 2, result.stderr + result.stdout)
                self.assertEqual(result.stdout, '')
        self.assertFalse(self.home.exists())
        self.assertEqual(sorted(str(p.relative_to(self.base)) for p in self.base.rglob('*')), before)


    def test_admitted_home_indexes_and_routes_from_readonly_product_and_nested_cwd(self):
        self.initialize()
        brain = self.home / 'brain'
        (brain / 'daily').mkdir()
        page = brain / 'daily/2026-10-31.md'
        page.write_text('---\nid: daily/2026-10-31\ntitle: Capacity planning\n'
                        'summary: Capacity planning evidence\ntype: daily\nscope: owner\n'
                        'status: active\nsensitivity: owner-private\nupdated: 2026-10-31\n'
                        '---\n# Capacity planning\n\nCapacity planning evidence.\n')
        (brain / 'daily/2026-10-30.md').write_text(page.read_text().replace('2026-10-31', '2026-10-30').replace('Capacity planning', 'Unrelated operations').replace('evidence', 'record'))
        skill = self.product / 'bundles/base/skills/sample-tool/SKILL.md'
        skill.parent.mkdir(parents=True)
        skill.write_text('---\nname: sample-tool\ndescription: Capacity planning method\n'
                         '---\n# Sample Tool\n\nCapacity planning method.\n')
        (self.product / 'CLAUDE.md').write_text('Read `brain/daily/` and '
            '`bundles/base/skills/sample-tool/SKILL.md`.\n')
        product_before = {p.relative_to(self.product): p.read_bytes()
                          for p in self.product.rglob('*') if p.is_file()}
        for p in self.product.rglob('*'):
            p.chmod(0o555 if p.is_dir() or p == self.helper else 0o444)
        self.product.chmod(0o555)
        def restore():
            self.product.chmod(0o755)
            for p in self.product.rglob('*'):
                p.chmod(0o755 if p.is_dir() or p == self.helper else 0o644)
        self.addCleanup(restore)
        before = {p.relative_to(self.home): p.read_bytes()
                  for p in self.home.rglob('*') if p.is_file()}
        check = self.tool('brain-index.py', '--check')
        self.assertEqual(check.returncode, 0, check.stderr)
        self.assertEqual({p.relative_to(self.home): p.read_bytes()
                          for p in self.home.rglob('*') if p.is_file()}, before)
        indexed = self.tool('brain-index.py')
        self.assertEqual(indexed.returncode, 0, indexed.stderr)
        self.assertIn('[Capacity planning](2026-10-31.md)',
                      (brain / 'daily/daily_index.md').read_text())
        self.assertTrue((brain / '.maestro/route-index.json').is_file())
        self.assertEqual(stat.S_IMODE(brain.stat().st_mode), 0o700)
        self.assertEqual(list((self.home / 'state').glob('brain-index-*')), [])
        routed = self.tool('brain-route.py', 'capacity planning')
        self.assertEqual(routed.returncode, 0, routed.stderr)
        self.assertIn('`brain/daily/2026-10-31.md`', routed.stdout)
        names = self.tool('skill-route.py', '--names')
        self.assertEqual(names.returncode, 0, names.stderr)
        self.assertIn('- **base**: `sample-tool`', names.stdout)
        routed_skill = self.tool('skill-route.py', 'sample-tool')
        self.assertIn('`bundles/base/skills/sample-tool/SKILL.md`', routed_skill.stdout)
        checked = self.tool('paths-check.py', '--all')
        self.assertEqual(checked.returncode, 0, checked.stderr + checked.stdout)
        self.assertIn('2 caminhos citados', checked.stdout)
        self.assertEqual({p.relative_to(self.product): p.read_bytes()
                          for p in self.product.rglob('*') if p.is_file()}, product_before)
        self.assertEqual(list(self.cwd.iterdir()), [])


    def test_supported_legacy_binding_remains_usable_without_initializing_home(self):
        self.env.pop('MAESTRO_HOME')
        brain = self.product / 'brain'
        brain.mkdir()
        (brain / 'notes.md').write_text('# Existing authored legacy note\n')
        result = self.tool('brain-index.py', '--check')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.user / '.maestro').exists())
        self.assertFalse((self.user / '.maestro-home.json').exists())
        result = self.tool('brain-index.py')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((brain / '.maestro/route-index.json').exists())
        self.assertEqual(list((brain / '.maestro').glob('brain-index-*')), [])


    def test_changed_product_version_refreshes_skills_and_preserves_owner_state(self):
        self.initialize()
        skill = self.product / 'bundles/base/skills/version-one/SKILL.md'
        skill.parent.mkdir(parents=True)
        skill.write_text('---\nname: version-one\ndescription: First version\n---\n# First\n')
        pending = self.home / 'state/pending-continuation.json'
        pending.parent.mkdir(mode=0o700)
        pending.write_text('owner continuation retained')
        first = self.tool('skill-route.py', '--names')
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertIn('`version-one`', first.stdout)
        second_product = self.base / 'product version two'
        shutil.copytree(self.product, second_product)
        old = second_product / 'bundles/base/skills/version-one'
        old.rename(old.with_name('version-two'))
        (second_product / 'VERSION').write_text('0.2.1\n')
        manifest_file = second_product / 'runtime/manifest.json'
        manifest = json.loads(manifest_file.read_text())
        manifest['version'] = '0.2.1'
        manifest_file.write_text(json.dumps(manifest))
        second = self.tool('skill-route.py', '--names', product=second_product)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertIn('`version-two`', second.stdout)
        self.assertNotIn('`version-one`', second.stdout)
        routed = self.tool('skill-route.py', 'version-two', product=second_product)
        self.assertIn('`bundles/base/skills/version-two/SKILL.md`', routed.stdout)
        self.assertEqual(pending.read_text(), 'owner continuation retained')
        self.assertFalse((second_product / 'brain').exists())

    def test_rejected_origin_or_unverified_helper_cannot_use_an_existing_cache(self):
        self.initialize()
        cache = self.home / 'brain/.maestro/skill-route-index.json'
        cache.write_text('existing private index')
        origin = self.home / 'brain/.maestro/origin.json'
        origin.write_text('{malformed authority')
        before = {p.relative_to(self.base): p.read_bytes()
                  for p in self.base.rglob('*') if p.is_file()}
        for tool in TOOLS:
            result = self.tool(tool, '--check' if tool == 'brain-index.py' else '--names')
            self.assertEqual(result.returncode, 2, result.stderr + result.stdout)
            self.assertEqual(result.stdout, '')
        self.assertEqual({p.relative_to(self.base): p.read_bytes()
                          for p in self.base.rglob('*') if p.is_file()}, before)
        self.helper.write_bytes(self.helper.read_bytes() + b'changed helper')
        for tool in TOOLS:
            result = self.tool(tool, '--names')
            self.assertEqual(result.returncode, 2, result.stderr + result.stdout)
            self.assertIn('checksum mismatch', result.stderr)
            self.assertEqual(result.stdout, '')
        self.assertEqual(cache.read_text(), 'existing private index')


    def test_managed_agent_policy_routes_from_outside_product_without_owner_state(self):
        policy = self.product / 'bundles/base/agents/activation-policy.json'
        policy.parent.mkdir(parents=True)
        shutil.copy2(ROOT / 'bundles/base/agents/activation-policy.json', policy)
        before = {p.relative_to(self.product): p.read_bytes()
                  for p in self.product.rglob('*') if p.is_file()}
        result = subprocess.run([sys.executable, str(self.product / 'bundles/base/tools/agent-route.py'),
                                 '--list'], cwd=self.cwd, text=True, capture_output=True,
                                env={k: v for k, v in self.env.items() if k != 'PYTHONDONTWRITEBYTECODE'})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('yoda', result.stdout)
        self.assertEqual({p.relative_to(self.product): p.read_bytes()
                          for p in self.product.rglob('*') if p.is_file()}, before)
        self.assertFalse(self.home.exists())


    def test_index_rejects_directory_alias_before_any_publication_or_cleanup(self):
        self.initialize()
        brain = self.home / 'brain'
        outside = self.base / 'unrelated sibling'
        outside.mkdir()
        for day in ('2026-10-30', '2026-10-31'):
            (outside / f'{day}.md').write_text(
                f'---\nid: daily/{day}\ntitle: Capacity planning {day}\n'
                'summary: Capacity planning evidence\ntype: daily\nscope: owner\n'
                f'status: active\nsensitivity: owner-private\nupdated: {day}\n'
                '---\n# Capacity planning\n\nCapacity planning evidence.\n')
        (outside / 'sentinel.txt').write_text('unrelated data must remain unchanged')
        (outside / 'obsolete').mkdir()
        (outside / 'obsolete/obsolete_index.md').write_text('unrelated index must not be deleted')
        (brain / 'brain_index.md').write_text('last known good owner index')
        (brain / '.maestro/route-index.json').write_text('last known good route index')
        (self.product / 'sentinel.txt').write_text('managed product unchanged')
        (brain / 'daily').symlink_to(outside, target_is_directory=True)
        before = {p.relative_to(self.base): p.read_bytes()
                  for p in self.base.rglob('*') if p.is_file()}
        for args in ((), ('--check',)):
            with self.subTest(args=args):
                result = self.tool('brain-index.py', *args)
                self.assertEqual(result.returncode, 2, result.stderr + result.stdout)
                self.assertEqual(result.stdout, '')
                self.assertFalse((outside / 'daily_index.md').exists())
                self.assertEqual({p.relative_to(self.base): p.read_bytes()
                                  for p in self.base.rglob('*') if p.is_file()}, before)


    def test_index_rejects_source_and_publication_leaf_aliases_before_effects(self):
        self.initialize()
        brain = self.home / 'brain'
        (brain / 'daily').mkdir()
        for day in ('2026-10-30', '2026-10-31'):
            (brain / 'daily' / f'{day}.md').write_text(
                f'---\nid: daily/{day}\ntitle: Capacity planning {day}\n'
                'summary: Capacity planning evidence\ntype: daily\nscope: owner\n'
                f'status: active\nsensitivity: owner-private\nupdated: {day}\n'
                '---\n# Capacity planning\n\nCapacity planning evidence.\n')
        outside = self.base / 'unrelated sibling'
        outside.mkdir()
        (brain / 'brain_index.md').write_text('last known good owner index')
        (self.product / 'sentinel.txt').write_text('managed product unchanged')
        for rel in ('daily/source.md', 'daily/daily_index.md', '.maestro/tasks-seen.json',
                    '.maestro/route-index.json.tmp', '.maestro/log.md'):
            with self.subTest(path=rel):
                target = outside / rel.replace('/', '-')
                target.write_text('unrelated leaf must remain unchanged')
                alias = brain / rel
                alias.unlink(missing_ok=True)
                alias.symlink_to(target)
                try:
                    before = {p.relative_to(self.base): p.read_bytes()
                              for p in self.base.rglob('*') if p.is_file()}
                    result = self.tool('brain-index.py')
                    self.assertEqual(result.returncode, 2, result.stderr + result.stdout)
                    self.assertEqual(result.stdout, '')
                    self.assertEqual({p.relative_to(self.base): p.read_bytes()
                                      for p in self.base.rglob('*') if p.is_file()}, before)
                finally:
                    alias.unlink(missing_ok=True)


    def assert_hardlinked_output_is_preserved(self, relative, tool, *args):
        self.initialize()
        brain = self.home / 'brain'
        (brain / 'daily').mkdir()
        for day in ('2026-10-30', '2026-10-31'):
            (brain / 'daily' / f'{day}.md').write_text(
                f'---\nid: daily/{day}\ntitle: Capacity planning {day}\n'
                'summary: Capacity planning evidence\ntype: daily\nscope: owner\n'
                f'status: active\nsensitivity: owner-private\nupdated: {day}\n'
                '---\n# Capacity planning\n\nCapacity planning evidence.\n')
        skill = self.product / 'bundles/base/skills/cache-guard/SKILL.md'
        skill.parent.mkdir(parents=True)
        skill.write_text('---\nname: cache-guard\ndescription: Guard cache paths\n---\n# Guard\n')
        outside = self.base / 'outside sentinel.txt'
        outside.write_bytes(b'Outside owner bytes must survive output publication.\n')
        outside.chmod(0o640)
        alias = brain / relative
        os.link(outside, alias)

        def snapshot():
            values = {}
            for path in self.base.rglob('*'):
                info = path.lstat()
                values[str(path.relative_to(self.base))] = (
                    stat.S_IMODE(info.st_mode), info.st_dev, info.st_ino, info.st_nlink,
                    hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else None)
            return values

        before = snapshot()
        outside_key = str(outside.relative_to(self.base))
        result = self.tool(tool, *args)
        after = snapshot()
        changed = sorted(key for key in set(before) | set(after)
                         if before.get(key) != after.get(key))
        self.assertEqual({
            'exit': result.returncode,
            'stdout': result.stdout,
            'outside': after.get(outside_key),
            'alias_preserved': alias.exists() and os.path.samefile(outside, alias),
            'changed_paths': changed,
        }, {
            'exit': 2,
            'stdout': '',
            'outside': before[outside_key],
            'alias_preserved': True,
            'changed_paths': [],
        }, result.stderr)

    def test_index_refuses_hardlinked_daily_index_without_effects(self):
        self.assert_hardlinked_output_is_preserved('daily/daily_index.md', 'brain-index.py')

    def test_index_refuses_hardlinked_backlinks_temporary_without_effects(self):
        self.assert_hardlinked_output_is_preserved('.maestro/backlinks.json.tmp', 'brain-index.py')

    def test_skill_rebuild_refuses_hardlinked_temporary_without_effects(self):
        self.assert_hardlinked_output_is_preserved('.maestro/skill-route-index.json.tmp',
                                                 'skill-route.py', '--names', '--rebuild')


    def test_skill_cache_rejects_cache_and_temp_aliases_before_read_or_build(self):
        self.initialize()
        skill = self.product / 'bundles/base/skills/cache-guard/SKILL.md'
        skill.parent.mkdir(parents=True)
        skill.write_text('---\nname: cache-guard\ndescription: Guard cache paths\n---\n# Guard\n')
        cache = self.home / 'brain/.maestro/skill-route-index.json'
        temporary = cache.with_name(cache.name + '.tmp')
        for args in (('--names',), ('--names', '--rebuild'), ('--names',)):
            normal = self.tool('skill-route.py', *args)
            self.assertEqual(normal.returncode, 0, normal.stderr)
            self.assertIn('`cache-guard`', normal.stdout)
        safe_cache = cache.read_bytes()
        outside = self.base / 'unrelated sibling'
        outside.mkdir()
        for target in (self.product / 'sentinel.txt', outside / 'sentinel.txt'):
            for leaf, retain_cache in (('cache', False), ('temporary', False), ('temporary', True)):
                with self.subTest(target=target.parent.name, leaf=leaf, retain_cache=retain_cache):
                    cache.unlink(missing_ok=True)
                    temporary.unlink(missing_ok=True)
                    if retain_cache:
                        cache.write_bytes(safe_cache)
                    target.write_text('unrelated bytes must remain unchanged')
                    alias = cache if leaf == 'cache' else temporary
                    alias.symlink_to(target)
                    try:
                        before = {p.relative_to(self.base): p.read_bytes()
                                  for p in self.base.rglob('*') if p.is_file()}
                        result = self.tool('skill-route.py', '--names')
                        self.assertEqual(result.returncode, 2, result.stderr + result.stdout)
                        self.assertEqual(result.stdout, '')
                        self.assertEqual({p.relative_to(self.base): p.read_bytes()
                                          for p in self.base.rglob('*') if p.is_file()}, before)
                    finally:
                        alias.unlink(missing_ok=True)


if __name__ == '__main__':
    unittest.main()
