"""Actual ZIP daily adapters on the current host; PS-on-Mac is not Windows proof."""
import datetime as dt
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[1]
ZIP = Path(os.environ.get('MAESTRO_ZIP', ROOT / 'dist/Maestro-v0.2.0-macos.zip'))


@unittest.skipUnless(platform.system() == 'Darwin', 'Mac adapter evidence only')
class DailyContinuity(unittest.TestCase):
    def test_packaged_bash_powershell_codex(self):
        self.assertIsNotNone(shutil.which('pwsh'), 'PowerShell required for parity')
        with tempfile.TemporaryDirectory(prefix='maestro-daily-') as tmp:
            with zipfile.ZipFile(ZIP) as archive:
                archive.extractall(tmp)
            root = (Path(tmp) / 'Maestro').resolve()
            arch = 'arm64' if platform.machine() == 'arm64' else 'amd64'
            helper = root / f'runtime/darwin-{arch}/maestro-runtime'
            helper.chmod(0o700)
            env = dict(os.environ, CLAUDE_PROJECT_DIR=str(root), MAESTRO_PROJECT_DIR=str(root))

            def run(args, payload=''):
                result = subprocess.run(args, input=payload, text=True, capture_output=True,
                                        cwd=root, env=env, timeout=120)
                self.assertEqual(result.returncode, 0, result.stderr)
                return result.stdout

            def put(path, text):
                target = root / path
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(text, encoding='utf-8')

            today = dt.datetime.now().astimezone()
            put('brain/accounts/.active', '\ufefffixture/case\r\n')
            scopes = ['brain', 'brain/accounts/fixture/cases/case']
            expected = []
            for scope in scopes:
                put(f'{scope}/memory/.dream-requested', '2026-09-28T23:40:00Z\n' if scope == 'brain' else 'a' * 64 + '\n')
                for layer in ('daily', 'memory/recent'):
                    for offset in range(-1, 4):
                        day = (today - dt.timedelta(days=offset)).date().isoformat()
                        marker = f'FIXTURE-{scope}-{layer}-{offset}'
                        put(f'{scope}/{layer}/{day}.md', marker)
                        if 0 <= offset <= 2:
                            expected.append(marker)
                    put(f'{scope}/{layer}/index.md', 'EXCLUDED-INDEX')
            # Fat unrelated layers must never evict the selected daily window.
            put('brain/owner/identity.json', json.dumps({'initialized': True, 'name': 'x' * 30000}))
            put('brain/memory/lifetime/fat.md', 'LIFETIME-FIXTURE\n' + 'x\n' * 20000)
            # The existing L3 contract selects only g<N>-* pages from the
            # newest generation; an arbitrary filename is not an L3 artifact.
            put('brain/memory/medium-term/g1-thread.md', '---\ntitle: L3-FIXTURE\n---\n' + 'x\n' * 20000)
            put('brain/memory/weekly/2026-09-28.md', 'L2-FIXTURE\n' + 'x\n' * 20000)
            put('brain/.maestro/day-brief.json', json.dumps({'last_daily_pages': [
                {'date': '2000-01-01', 'path': 'old', 'briefing_last': 'STALE-CACHE'}]}))
            put('brain/.maestro/daily-pending/invalid.json', 'PRIVATE-INVALID-CHECKPOINT')
            core = run([str(helper), 'daily-context', '--root', str(root)])
            bash = run(['bash', '.claude/hooks/session-start-memory-inject.sh'])
            ps = run(['pwsh', '-NoProfile', '-File', '.claude/hooks/session-start-memory-inject.ps1'])
            codex = json.loads(run([str(helper), 'codex-hook', 'SessionStart', '--root', str(root)],
                                   json.dumps({'session_id': 'fixture-session', 'cwd': str(root)})))
            codex = codex['hookSpecificOutput']['additionalContext']
            for output in (core, bash, ps, codex):
                self.assertIn('Dream pending: brain/accounts/fixture/cases/case/memory/.dream-requested', output)
                for marker in expected:
                    self.assertIn(marker, output)
                for forbidden in ('EXCLUDED-INDEX', 'STALE-CACHE', 'daily--1', 'daily-3', 'recent--1', 'recent-3'):
                    self.assertNotIn(forbidden, output)
            for output in (bash, ps, codex):
                self.assertIn('daily_recovery', output)
                self.assertNotIn('PRIVATE-INVALID-CHECKPOINT', output)
            for output, headings in (
                (bash, ('Memória permanente (lifetime)', 'Trilhas temáticas de médio prazo (L3)', 'Resumo semanal (L2)', '### memory/recent /')),
                (ps, ('Memory lifetime:', 'Memory medium-term:', 'Memory weekly:', '### memory/recent /')),
            ):
                positions = [output.find(heading) for heading in headings]
                for heading, position in zip(headings, positions):
                    self.assertGreaterEqual(position, 0, 'Canonical layer omitted: ' + heading)
                self.assertEqual(positions, sorted(positions), 'Canonical lifetime -> L3 -> L2 -> L1 order')
            self.assertLessEqual(len(bash.encode()), 18000)
            self.assertLessEqual(len(core.encode()), 5000)
            self.assertLessEqual(len(ps.encode()), 8192)
            self.assertLessEqual(len(codex.encode()), 8192)
            self.assertEqual((root / 'brain/memory/.dream-legacy-request').read_text(), '2026-09-28T23:40:00Z\n')

            # Both Stop adapters and Codex write the same scope-local journal.
            for adapter in ('bash', 'powershell', 'codex'):
                checkpoint = dict(schema_version=1, id=adapter, session_id='fixture-session',
                                  workspace=str(root), scope='owner', captured_at=today.isoformat(timespec='seconds'),
                                  local_date=today.date().isoformat(), summary='saved-' + adapter,
                                  decisions=[], next_actions=[], provenance='agent-authored')
                queued = f'brain/.maestro/daily-pending/{adapter}.json'
                put(queued, json.dumps(checkpoint))
                if adapter == 'bash':
                    run(['bash', '.claude/hooks/session-stop-dream.sh'])
                elif adapter == 'powershell':
                    run(['pwsh', '-NoProfile', '-File', '.claude/hooks/session-stop-dream.ps1'])
                else:
                    run([str(helper), 'codex-hook', 'Stop', '--root', str(root)],
                        json.dumps({'session_id': 'fixture-session', 'cwd': str(root)}))
                self.assertFalse((root / queued).exists())
                page = root / f'brain/daily/{today.date().isoformat()}.md'
                self.assertEqual(page.read_text().count('saved-' + adapter), 1)
            self.assertTrue((root / 'brain/memory/.dream-requested').exists())
            print('PASS: packaged Mac Bash, PowerShell-on-Mac and Codex helper daily adapter parity; native Windows UNATTESTED')


if __name__ == '__main__':
    unittest.main()
