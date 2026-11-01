"""Invoke the verified managed runtime; origin authority stays in Go."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import stat
import subprocess
import sys


def regular_source(path):
    """Inspect a source without accepting static file or ancestor aliases."""
    path = Path(path).absolute()
    for parent in reversed(path.parents):
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or getattr(info, 'st_file_attributes', 0) & 0x400:
            raise OSError('aliased source directory')
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or getattr(info, 'st_file_attributes', 0) & 0x400:
        raise OSError('aliased or nonregular source')
    return info


def read_regular_text(path):
    """Read only a regular single-link source; unsupported/unsafe input raises."""
    before = regular_source(path)
    flags = os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0) | getattr(os, 'O_NONBLOCK', 0)
    descriptor = os.open(path, flags)
    with os.fdopen(descriptor, encoding='utf-8', errors='replace') as source:
        after = os.fstat(source.fileno())
        if not stat.S_ISREG(after.st_mode) or after.st_nlink != 1 or (after.st_dev, after.st_ino) != (before.st_dev, before.st_ino):
            raise OSError('source changed or is not regular')
        return source.read()


def consume_paths(argv, script, write=False):
    """Remove local binding flags and obtain the canonical admitted paths."""
    try:
        product = Path(script).resolve().parents[3]
        user_home = None
        for flag in ('--product-root', '--user-home'):
            if flag in argv:
                i = argv.index(flag)
                if i + 1 >= len(argv) or argv[i + 1].startswith('--'):
                    raise ValueError('local path option requires a value')
                value = argv[i + 1]
                del argv[i:i + 2]
                if flag in argv:
                    raise ValueError('duplicate local path option')
                if flag == '--product-root':
                    product = Path(value).resolve()
                else:
                    user_home = value
        system = {'Darwin': 'darwin', 'Linux': 'linux', 'Windows': 'windows'}[platform.system()]
        arch = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64',
                'amd64': 'amd64'}[platform.machine().lower()]
        relative = f'runtime/{system}-{arch}/maestro-runtime' + ('.exe' if system == 'windows' else '')
        helper = product / relative
        manifest = product / 'runtime/manifest.json'
        for candidate in (product / 'runtime', helper.parent, helper, manifest, product / 'VERSION'):
            info = candidate.lstat()
            if stat.S_ISLNK(info.st_mode) or getattr(info, 'st_file_attributes', 0) & 0x400:
                raise ValueError('aliased managed runtime')
        version = (product / 'VERSION').read_text(encoding='utf-8').replace('\r', '').replace('\n', '')
        metadata = json.loads(manifest.read_text(encoding='utf-8'))
        if type(metadata.get('schema_version')) is not int or metadata['schema_version'] != 1 or not version or metadata.get('version') != version:
            raise ValueError('managed runtime version mismatch')
        selected = [entry for entry in metadata['artifacts']
                    if entry.get('os') == system and entry.get('arch') == arch]
        if len(selected) != 1 or selected[0].get('path') != relative:
            raise ValueError('managed runtime artifact ambiguous')
        expected = selected[0].get('sha256', '')
        if not re.fullmatch('[a-f0-9]{64}', expected):
            raise ValueError('managed runtime checksum invalid')
        actual = hashlib.sha256(helper.read_bytes()).hexdigest()
        if actual != expected:
            raise ValueError('managed runtime checksum mismatch')
        command = [str(helper), 'home', 'paths', '--product-root', str(product)]
        if user_home is not None:
            command += ['--user-home', user_home]
        result = subprocess.run(command, stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=30)
        paths = json.loads(result.stdout)
        if result.returncode or paths.get('mode') not in ('external', 'legacy'):
            raise ValueError(paths.get('diagnostic') or 'HOME unavailable')
        if write and paths.get('writable') is not True:
            raise ValueError('HOME mutation unavailable')
        for key in ('product_root', 'brain_root', 'metadata_root', 'owner_scope_root'):
            if not isinstance(paths.get(key), str) or not os.path.isabs(paths[key]):
                raise ValueError('managed runtime path decision incomplete')
        if paths['mode'] == 'external' and (not isinstance(paths.get('state_root'), str) or not os.path.isabs(paths['state_root'])):
            raise ValueError('managed runtime state decision incomplete')
        return paths
    except (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.SubprocessError) as error:
        print(f'Maestro paths unavailable: {error}', file=sys.stderr)
        raise SystemExit(2)
