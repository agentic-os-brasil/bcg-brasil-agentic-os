"""Resolve native discovery pointers against actual release ZIP members.

Run after rebuilding: python3 -m unittest acceptance/test_codex_artifact_paths.py
Override artifact paths with MAESTRO_TEST_ZIPS (os.pathsep-separated).
No native runtime qualification is inferred.
"""
import json
import os
from pathlib import Path
import re
import unittest
import zipfile

ROOT=Path(__file__).resolve().parents[1]

class CodexArtifactPaths(unittest.TestCase):
    def test_native_role_and_skill_pointers_resolve_in_release(self):
        configured=os.environ.get('MAESTRO_TEST_ZIPS')
        archives=[Path(p) for p in configured.split(os.pathsep)] if configured else sorted((ROOT/'dist').glob('Maestro-v*.zip'))
        self.assertTrue(archives,'build an actual Maestro ZIP before artifact validation')
        for archive in archives:
            with self.subTest(archive=archive.name),zipfile.ZipFile(archive) as z:
                names=set(z.namelist());prefix='Maestro/'
                catalog=json.loads(z.read(prefix+'bundles/base/agents/catalog.json'))
                policy=json.loads(z.read(prefix+'bundles/base/agents/activation-policy.json'))
                roles={a['id'] for a in catalog['agents']}
                for agent in policy['agents']:
                    if agent['status']=='active' and agent['dispatchable']:
                        self.assertTrue(prefix+'.codex/agents/'+agent['id']+'.toml' in names,'active spoke lacks native Codex projection: '+agent['id'])
                definitions=[p for p in names if p.startswith(prefix+'.codex/agents/') and p.endswith('.toml')]
                self.assertTrue(definitions)
                for path in definitions:
                    text=z.read(path).decode();role=json.loads(re.search(r'^name\s*=\s*("[^"]+")',text,re.M).group(1))
                    self.assertIn(role,roles,'phantom role')
                    refs=re.findall(r'(?:os|bundles)/[A-Za-z0-9_./-]+/AGENT\.md',text)
                    self.assertTrue(refs,'role needs a canonical contract pointer')
                    for ref in refs:self.assertTrue(prefix+ref in names,f'{path} points outside packaged files: {ref}')
                orientation=z.read(prefix+'AGENTS.md').decode()
                for ref in re.findall(r'\.agents/skills/[A-Za-z0-9_-]+/SKILL\.md',orientation):
                    self.assertTrue(prefix+ref in names,'native skill pointer missing: '+ref)
                self.assertIn(prefix+'bundles/base/agents/activation-policy.json',names)

if __name__=='__main__':unittest.main()
