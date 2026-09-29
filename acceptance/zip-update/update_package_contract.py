#!/usr/bin/env python3
"""Factory-only ZIP contract validation. Never executes packaged software."""
import argparse
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import re
import stat
import zipfile


def archive(raw):
    z = zipfile.ZipFile(io.BytesIO(raw))
    names = set()
    total = 0
    for item in z.infolist():
        name = item.filename
        path = PurePosixPath(name)
        if (name in names or name.startswith('/') or '\\' in name
                or '..' in path.parts or ':' in name
                or stat.S_ISLNK(item.external_attr >> 16)):
            raise ValueError('Unsafe or duplicate ZIP entry')
        names.add(name)
        total += item.file_size
        if total > 1024 * 1024 * 1024:
            raise ValueError('ZIP exceeds factory validation budget')
    return z


def checksum(raw, sidecar):
    parts = sidecar.strip().split()
    if not parts or not re.fullmatch(r'[a-fA-F0-9]{64}', parts[0]):
        raise ValueError('Invalid SHA-256 sidecar')
    if hashlib.sha256(raw).hexdigest() != parts[0].lower():
        raise ValueError('ZIP SHA-256 mismatch')


def payload(raw, version, source, platform):
    z = archive(raw)
    if any(not n.startswith('Maestro/') for n in z.namelist()):
        raise ValueError('Unexpected payload root')
    if z.read('Maestro/VERSION').decode('utf-8-sig').strip() != version:
        raise ValueError('Payload VERSION mismatch')
    contract_raw = z.read('Maestro/UPDATE-CONTRACT.json')
    contract = json.loads(contract_raw)
    if (contract.get('schema_version') != 2
            or contract.get('contract_id') != 'maestro-update-long-run-v2'
            or contract.get('target_version') != version
            or source not in contract.get('source_versions', [])):
        raise ValueError('Payload update contract/version/source mismatch')
    if contract.get('receipt') != 'brain/.maestro/updates/update-' + version + '.json':
        raise ValueError('Unexpected durable receipt namespace')
    if contract.get('may_bypass_host_limits') is not False or contract.get('caseos_required_for_local_update') is not False:
        raise ValueError('Contract must preserve host limits and optional caseOS')
    required = {'source_baseline_preserved', 'migration_committed', 'legacy_namespaces_readable',
                'customizations_reconciled', 'native_runtime_integrity', 'hooks_configured',
                'hooks_observed', 'native_agent_return', 'yoda_review'}
    if not required.issubset(contract.get('required_checks', [])):
        raise ValueError('Missing required update checks')
    bindings = {'attempt_id', 'from_version', 'to_version', 'platform', 'runtime_host',
                'target_release_sha256', 'target_core_sha256', 'baseline_manifest_sha256',
                'installation_root_sha256', 'migration_receipt_sha256'}
    if not bindings.issubset(contract.get('receipt_bindings', [])):
        raise ValueError('Missing receipt bindings')
    if not {'caseos_connected', 'python_optional_tools', 'host_goal_available'}.issubset(contract.get('optional_checks', [])):
        raise ValueError('Optional dependencies must remain explicitly optional')
    if contract.get('required_subagents') != ['yoda']:
        raise ValueError('Unexpected required review agents')
    if any(n.startswith(('Maestro/data/', 'Maestro/brain/')) for n in z.namelist()):
        raise ValueError('Payload ships personal state')
    hooks = json.loads(z.read('Maestro/.claude/settings.json'))['hooks']
    counts = {event: sum(len(group['hooks']) for group in groups) for event, groups in hooks.items()}
    if counts != {'SessionStart': 2, 'UserPromptSubmit': 1, 'PreToolUse': 2, 'Stop': 4}:
        raise ValueError('Expected nine Claude lifecycle handlers')
    suffix = '.ps1' if platform == 'windows-powershell' else '.sh'
    for groups in hooks.values():
        for group in groups:
            for hook in group['hooks']:
                if suffix not in hook['command']:
                    raise ValueError('Wrong platform hook implementation')
                if platform == 'windows-powershell' and (hook.get('shell') != 'powershell' or 'bash' in hook['command'].lower()):
                    raise ValueError('Windows critical hooks must use native PowerShell')
    matchers = {group.get('matcher') for group in hooks['PreToolUse']}
    if matchers != {'^(Edit|MultiEdit|Write|NotebookEdit)$', '^(Agent|Task)$'}:
        raise ValueError('Unanchored Claude tool matchers')
    codex = json.loads(z.read('Maestro/.codex/hooks.json'))['hooks']
    if set(codex) != {'SessionStart', 'UserPromptSubmit', 'PreToolUse', 'PostToolUse', 'Stop'}:
        raise ValueError('Codex native event set mismatch')
    manifest = json.loads(z.read('Maestro/runtime/manifest.json'))
    os_name = 'windows' if platform == 'windows-powershell' else 'darwin'
    if manifest.get('schema_version') != 1 or manifest.get('version') != version:
        raise ValueError('Runtime manifest identity mismatch')
    artifacts = manifest.get('artifacts', [])
    if {(a['os'], a['arch']) for a in artifacts} != {(os_name, 'amd64'), (os_name, 'arm64')} or len(artifacts) != 2:
        raise ValueError('Runtime platform artifacts mismatch')
    for entry in artifacts:
        path = 'runtime/' + os_name + '-' + entry['arch'] + '/maestro-runtime' + ('.exe' if os_name == 'windows' else '')
        if entry['path'] != path or hashlib.sha256(z.read('Maestro/' + path)).hexdigest() != entry['sha256']:
            raise ValueError('Runtime artifact integrity mismatch')
    return contract_raw


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['payload', 'kit'])
    parser.add_argument('--zip', required=True)
    parser.add_argument('--from-version', required=True)
    parser.add_argument('--to-version', required=True)
    parser.add_argument('--platform', choices=['both', 'macos', 'windows-powershell'], default='both')
    args = parser.parse_args()
    raw = Path(args.zip).read_bytes()
    checksum(raw, Path(args.zip).with_suffix('.sha256').read_text())
    platforms = ['macos', 'windows-powershell'] if args.platform == 'both' else [args.platform]
    if args.mode == 'payload':
        if args.platform == 'both':
            parser.error('payload requires one platform')
        payload(raw, args.to_version, args.from_version, args.platform)
    else:
        z = archive(raw)
        name = 'Maestro-Update-v' + args.to_version + ('' if args.platform == 'both' else '-' + args.platform)
        prefix = name + '/'
        expected = {'LEIA-ME-PRIMEIRO.md', 'PROMPT-1-PREPARAR.txt', 'PROMPT-2-VERIFICAR.txt',
                    'TESTE-MAC-WINDOWS.md', 'DIAGNOSTICO-HOOKS.md', 'UPDATE-CONTRACT.json', 'UPDATE-KIT.json'}
        metadata = json.loads(z.read(prefix + 'UPDATE-KIT.json'))
        if metadata != dict(schema_version=1, from_version=args.from_version,
                            target_version=args.to_version, platform=args.platform,
                            qualification='unqualified-candidate'):
            raise ValueError('Update-kit source/target/platform binding mismatch')
        for platform in platforms:
            base = 'Maestro-v' + args.to_version + '-' + platform
            expected.update({base + '.zip', base + '.sha256'})
            body = z.read(prefix + base + '.zip')
            checksum(body, z.read(prefix + base + '.sha256').decode())
            inner_contract = payload(body, args.to_version, args.from_version, platform)
            if z.read(prefix + 'UPDATE-CONTRACT.json') != inner_contract:
                raise ValueError('Outer contract differs from payload contract')
        actual = {n[len(prefix):] for n in z.namelist() if not n.endswith('/')}
        if actual != expected or any(not n.startswith(prefix) for n in z.namelist()):
            raise ValueError('Unexpected or missing update-kit files/platform')
        for entry in expected:
            if entry.endswith(('.md', '.txt')) and b'{{' in z.read(prefix + entry):
                raise ValueError('Unrendered update template')
    print('PASS package-contract: checksums, versions, v2 contract, scoped payloads, native runtime integrity and hook configuration; native execution unqualified')


if __name__ == '__main__':
    main()
