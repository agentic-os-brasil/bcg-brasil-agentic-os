"""Offline synthetic ZIP fixtures, not executable runtime qualification."""
import hashlib
import io
import json
from pathlib import Path
import unittest
import zipfile

import update_package_contract as check

ROOT = Path(__file__).resolve().parents[2]


def fixture(platform='macos', mutate=None):
    template = ROOT / 'installers/zip/user-template'
    settings = 'settings.windows-powershell.json' if platform == 'windows-powershell' else 'settings.json'
    entries = {
        'VERSION': b'0.2.0\n',
        'UPDATE-CONTRACT.json': (template / 'UPDATE-CONTRACT.json').read_bytes(),
        '.claude/settings.json': (template / '.claude' / settings).read_bytes(),
        '.codex/hooks.json': (template / '.codex/hooks.json').read_bytes(),
    }
    os_name = 'windows' if platform == 'windows-powershell' else 'darwin'
    artifacts = []
    for arch in ('amd64', 'arm64'):
        path = 'runtime/' + os_name + '-' + arch + '/maestro-runtime' + ('.exe' if os_name == 'windows' else '')
        entries[path] = b'synthetic-not-an-executable'
        artifacts.append(dict(os=os_name, arch=arch, path=path, sha256=hashlib.sha256(entries[path]).hexdigest()))
    entries['runtime/manifest.json'] = json.dumps(dict(schema_version=1, version='0.2.0', artifacts=artifacts)).encode()
    if mutate:
        mutate(entries)
    out = io.BytesIO()
    with zipfile.ZipFile(out, 'w') as z:
        for name, data in entries.items():
            z.writestr('Maestro/' + name, data)
    return out.getvalue()


class Contracts(unittest.TestCase):
    def test_both_platform_contracts_accept_both_upgrade_sources(self):
        for platform in ('macos', 'windows-powershell'):
            for source in ('0.1.11', '0.1.12'):
                check.payload(fixture(platform), '0.2.0', source, platform)

    def test_wrong_version_or_upgrade_source_rejected(self):
        for target, source in (('0.2.1', '0.1.12'), ('0.2.0', '0.1.10')):
            with self.assertRaises(ValueError):
                check.payload(fixture(), target, source, 'macos')

    def test_old_six_handler_payload_rejected(self):
        def mutate(entries):
            settings = json.loads(entries['.claude/settings.json'])
            first = settings['hooks']['Stop'][0]['hooks'][0]
            settings['hooks']['Stop'] = [{'hooks': [first]}]
            entries['.claude/settings.json'] = json.dumps(settings).encode()
        with self.assertRaisesRegex(ValueError, 'nine'):
            check.payload(fixture(mutate=mutate), '0.2.0', '0.1.12', 'macos')

    def test_windows_cannot_wrap_mac_payload(self):
        with self.assertRaises(ValueError):
            check.payload(fixture(), '0.2.0', '0.1.12', 'windows-powershell')

    def test_runtime_hash_and_personal_state_rejected(self):
        for name in ('runtime/darwin-amd64/maestro-runtime', 'brain/owner/private.md'):
            with self.assertRaises(ValueError):
                check.payload(fixture(mutate=lambda entries: entries.update({name: b'changed'})), '0.2.0', '0.1.12', 'macos')

    def test_outer_digest_tampering_rejected(self):
        raw = fixture()
        with self.assertRaisesRegex(ValueError, 'mismatch'):
            check.checksum(raw, '0' * 64 + ' fixture.zip')

    def test_traversal_zip_rejected_without_extraction(self):
        raw = io.BytesIO()
        with zipfile.ZipFile(raw, 'w') as z:
            z.writestr('../outside', b'untrusted')
        with self.assertRaises(ValueError):
            check.archive(raw.getvalue())


if __name__ == '__main__':
    unittest.main()
