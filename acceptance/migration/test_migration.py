"""Observable migration contract; run with python3 -m unittest discover -s acceptance/migration."""
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest
import shutil
import hashlib
import platform
import os

REPO = pathlib.Path(__file__).resolve().parents[2]


class MigrationContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory(prefix='maestro-helper-')
        cls.addClassCleanup(cls.build.cleanup)
        cls.helper = pathlib.Path(cls.build.name) / 'maestro-runtime'
        subprocess.run(['go', 'build', '-o', str(cls.helper), './cmd/maestro-runtime'], cwd=REPO, check=True)

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='maestro migração ')
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)

    def put(self, path, content='conteúdo'):
        p = self.root / path
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding='utf8')
        return p

    def invoke(self, *args):
        p = subprocess.run([str(self.helper), 'migration', '--project', str(self.root), *args],
                           capture_output=True, text=True)
        self.assertTrue(p.stdout.startswith('{'), p.stdout + p.stderr)
        return p.returncode, json.loads(p.stdout)

    def install_helper(self):
        architecture = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64'}[platform.machine()]
        relative = f'runtime/{platform.system().lower()}-{architecture}/maestro-runtime'
        binary = self.root / relative
        binary.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(self.helper, binary)
        self.put('VERSION', '0.2.0\n')
        manifest = {'schema_version': 1, 'version': '0.2.0', 'artifacts': [
            {'os': platform.system().lower(), 'arch': architecture, 'path': relative,
             'sha256': hashlib.sha256(binary.read_bytes()).hexdigest()}]}
        self.put('runtime/manifest.json', json.dumps(manifest))
        source = REPO/'installers/zip/user-template/.claude/hooks'
        for name in ['first-run-scaffold.sh', 'lib/maestro-runtime.sh', 'lib/python.sh']:
            target = self.root/'.claude/hooks'/name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source/name, target)
        return binary

    def adapter(self):
        return subprocess.run(['bash', '-c',
            '. "$1/.claude/hooks/lib/maestro-runtime.sh"; maestro_runtime "$1" migration --project "$1" --status',
            'adapter', str(self.root)], capture_output=True, text=True)

    @unittest.skipUnless(platform.system() == 'Darwin', 'native macOS manifest verifier')
    def test_native_manifest_verifier_and_tamper_rejection(self):
        binary = self.install_helper()
        self.assertEqual(self.adapter().returncode, 0)
        binary.write_bytes(binary.read_bytes() + b'tampered')
        result = self.adapter()
        self.assertEqual(result.returncode, 2)
        self.assertIn('checksum mismatch', result.stderr)
        self.assertFalse((self.root/'brain').exists())

    @unittest.skipUnless(platform.system() == 'Darwin', 'native macOS manifest verifier')
    def test_initialized_marker_does_not_bypass_blocked_migration(self):
        self.install_helper()
        self.put('data/owner/identity.json', 'old')
        self.put('brain/owner/identity.json', 'current')
        self.put('brain/.initialized', 'stale')
        result = subprocess.run(['bash', str(self.root/'.claude/hooks/first-run-scaffold.sh')],
            env={**os.environ, 'CLAUDE_PROJECT_DIR': str(self.root)}, capture_output=True, text=True)
        self.assertIn('maestro:migration-blocked', result.stdout)
        self.assertFalse((self.root/'brain/memory').exists(), 'blocked migration allowed backfill writes')
        self.assertEqual((self.root/'brain/owner/identity.json').read_text(), 'current')

    @unittest.skipUnless(platform.system() == 'Darwin', 'native macOS scaffold')
    def test_authored_work_after_commit_keeps_initialized_runtime_usable(self):
        self.install_helper()
        self.put('data/owner/note.md', 'original')
        self.assertEqual(self.invoke()[1]['state'], 'committed')
        self.put('brain/owner/note.md', 'new authored work')
        self.put('brain/.initialized', 'complete')
        result = subprocess.run(['bash', str(self.root/'.claude/hooks/first-run-scaffold.sh')],
            env={**os.environ, 'CLAUDE_PROJECT_DIR': str(self.root)}, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn('maestro:migration-blocked', result.stdout)
        self.assertEqual((self.root/'brain/owner/note.md').read_text(), 'new authored work')
        self.assertTrue((self.root/'brain/memory').exists(), 'normal initialized backfill did not run')
        verified = self.invoke('--status')[1]
        self.assertEqual(verified['state'], 'committed')
        self.assertFalse(verified['current'])

    @unittest.skipUnless(platform.system() == 'Darwin', 'native macOS scaffold')
    def test_unsafe_committed_target_blocks_then_sessionstart_recovers(self):
        self.install_helper()
        self.put('data/owner/a.md', 'original a')
        self.put('data/owner/z.md', 'original z')
        committed = self.invoke()[1]
        self.put('brain/owner/a.md', 'ordinary earlier edit')
        self.put('brain/.initialized', 'complete')
        state = self.root/'brain/.maestro/migration/state.json'
        receipt = state.parent/'attempts'/committed['attempt_id']/'receipt.json'
        before_state, before_receipt = state.read_bytes(), receipt.read_bytes()
        source_before = (self.root/'data/owner/z.md').read_bytes()
        target = self.root/'brain/owner/z.md'
        outside = self.put('external-note.md', 'outside content')
        target.unlink()
        target.symlink_to(outside)
        def session_start():
            return subprocess.run(['bash', str(self.root/'.claude/hooks/first-run-scaffold.sh')],
                env={**os.environ, 'CLAUDE_PROJECT_DIR': str(self.root)}, capture_output=True, text=True)
        for _ in range(2):
            blocked = session_start()
            self.assertIn('maestro:migration-blocked', blocked.stdout)
            self.assertEqual(state.read_bytes(), before_state)
            self.assertEqual(receipt.read_bytes(), before_receipt)
            self.assertFalse((self.root/'brain/memory').exists())
        target.unlink()
        target.write_text('original z')
        recovered = session_start()
        self.assertNotIn('maestro:migration-blocked', recovered.stdout)
        self.assertTrue((self.root/'brain/memory').exists())
        self.assertEqual(state.read_bytes(), before_state)
        self.assertEqual(receipt.read_bytes(), before_receipt)
        self.assertEqual((self.root/'data/owner/z.md').read_bytes(), source_before)
        self.assertEqual((self.root/'brain/owner/a.md').read_text(), 'ordinary earlier edit')

    def test_python_compatibility_launcher_requires_the_same_verified_helper(self):
        self.put('data/owner/note.md', 'original')
        wrapper = REPO/'bundles/base/tools/migrate-data-to-brain.py'
        blocked = subprocess.run([sys.executable, str(wrapper), '--project', str(self.root)], capture_output=True, text=True)
        self.assertEqual(blocked.returncode, 2)
        self.assertFalse((self.root/'brain').exists())
        self.install_helper()
        committed = subprocess.run([sys.executable, str(wrapper), '--project', str(self.root)], capture_output=True, text=True)
        self.assertEqual(committed.returncode, 0, committed.stderr)
        self.assertEqual(json.loads(committed.stdout)['state'], 'committed')

    @unittest.skipUnless(platform.system() == 'Darwin', 'native macOS verifier')
    def test_manifest_ambiguity_and_version_mismatch_never_execute(self):
        self.install_helper()
        file = self.root/'runtime/manifest.json'
        manifest = json.loads(file.read_text())
        manifest['artifacts'].append(manifest['artifacts'][0])
        file.write_text(json.dumps(manifest))
        self.assertEqual(self.adapter().returncode, 2)
        manifest['artifacts'].pop()
        manifest['version'] = '0.1.12'
        file.write_text(json.dumps(manifest))
        self.assertEqual(self.adapter().returncode, 2)
        self.assertFalse((self.root/'brain').exists())

    def test_all_namespaces_are_accounted_and_original_is_unchanged(self):
        for path in ['agents/custom/agent.md', 'workspaces/project-a/context.md',
                     'canary/state.json', 'owner/identity.json', 'custom/ação 1.md', '.private']:
            self.put('data/' + path)
        before = {str(p.relative_to(self.root/'data')): p.read_bytes()
                  for p in (self.root/'data').rglob('*') if p.is_file()}
        code, result = self.invoke()
        self.assertEqual((code, result['state']), (0, 'committed'))
        after = {str(p.relative_to(self.root/'data')): p.read_bytes()
                 for p in (self.root/'data').rglob('*') if p.is_file()}
        self.assertEqual(before, after)
        self.assertEqual((self.root/'brain/custom/ação 1.md').read_text(), 'conteúdo')
        self.assertEqual((self.root/'brain/.private').read_text(), 'conteúdo')
        _, resolved = self.invoke('--resolve-legacy', 'workspaces')
        self.assertEqual(resolved['path'], str(self.root/'data/workspaces'))

    def test_changed_source_and_tampered_target_are_not_green(self):
        source = self.put('data/owner/note.md', 'one')
        self.assertEqual(self.invoke()[1]['state'], 'committed')
        source.write_text('two')
        self.assertEqual(self.invoke('--status')[1]['state'], 'blocked')
        source.write_text('one')
        self.put('brain/owner/note.md', 'tampered')
        status = self.invoke('--status')[1]
        self.assertEqual(status['state'], 'committed')
        self.assertEqual(status['verification'], 'target_evolved')
        self.assertFalse(status['current'])

    def test_dual_tree_and_old_marker_do_not_authorize_overwrite(self):
        self.put('data/owner/note.md', 'old')
        self.put('data/.migrated-to-brain', 'stale')
        self.put('brain/owner/note.md', 'current')
        self.put('brain/.initialized', 'stale')
        code, result = self.invoke()
        self.assertEqual((code, result['state']), (2, 'blocked'))
        self.assertEqual((self.root/'brain/owner/note.md').read_text(), 'current')

    def test_mapping_collision_fails_before_copy(self):
        self.put('data/profile/identity.json', 'profile')
        self.put('data/owner/identity.json', 'owner')
        self.assertEqual(self.invoke()[1]['state'], 'blocked')
        self.assertFalse((self.root/'brain/owner/identity.json').exists())

    def test_case_insensitive_collision_fails_before_copy(self):
        self.put('data/custom/Note.md', 'one')
        self.put('data/custom/note.md', 'two')
        if len(list((self.root/'data/custom').iterdir())) < 2:
            self.skipTest('case insensitive host')
        self.assertEqual(self.invoke()[1]['state'], 'blocked')

    def test_symlink_escape_is_rejected(self):
        outside = self.put('outside/secret.md')
        self.put('data/owner/note.md')
        (self.root/'data/owner/link').symlink_to(outside)
        self.assertEqual(self.invoke()[1]['state'], 'blocked')
        self.assertFalse((self.root/'brain/owner/link').exists())

    def test_empty_directories_and_case_markers_are_preserved(self):
        (self.root/'data/custom/empty').mkdir(parents=True)
        self.put('data/cases/case-a/brain/canon/note.md')
        self.put('data/cases/.active', 'case-a\n')
        self.put('data/cases/.pending', 'case-a\n')
        self.assertEqual(self.invoke()[1]['state'], 'committed')
        self.assertTrue((self.root/'brain/custom/empty').is_dir())
        self.assertEqual((self.root/'brain/accounts/.active').read_text(), '_sem-conta/case-a\n')
        self.assertEqual((self.root/'brain/accounts/.pending').read_text(), '_sem-conta/case-a\n')

    def test_dry_run_and_status_do_not_write(self):
        self.put('data/owner/note.md')
        self.assertEqual(self.invoke('--dry-run')[1]['state'], 'planned')
        self.assertFalse((self.root/'brain').exists())
        self.assertEqual(self.invoke('--status')[1]['state'], 'planned')
        self.assertFalse((self.root/'brain').exists())

    def test_rollback_revokes_receipt_without_claiming_runtime_restoration(self):
        self.put('data/owner/note.md', 'original')
        result = self.invoke()[1]
        self.put('brain/owner/note.md', 'newer work')
        result = self.invoke('--rollback', result['attempt_id'])[1]
        self.assertEqual(result['state'], 'rolled_back')
        self.assertFalse(result['restored_runtime'])
        self.assertEqual((self.root/'data/owner/note.md').read_text(), 'original')
        self.assertEqual((self.root/'brain/owner/note.md').read_text(), 'newer work')



if __name__ == '__main__':
    unittest.main()
