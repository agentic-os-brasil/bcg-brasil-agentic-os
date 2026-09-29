"""Extract documented producer contracts; not native-agent behavior evidence."""
import json
from pathlib import Path
import re
import unittest

ROOT=Path(__file__).resolve().parents[1]

class BrainContracts(unittest.TestCase):
    def test_new_daily_example_has_complete_valid_frontmatter(self):
        text=(ROOT/'bundles/base/skills/start-day/SKILL.md').read_text()
        example=re.search(r'\x60\x60\x60markdown\n(.*?)\n\x60\x60\x60',text,re.S).group(1)
        self.assertTrue(example.startswith('---\n'),'new daily example must carry metadata before H1')
        front,body=example[4:].split('\n---\n',1)
        fields=dict(line.split(':',1) for line in front.splitlines())
        self.assertEqual(set(fields),{'id','title','summary','type','scope','status','sensitivity','updated'})
        fields={k:v.strip().strip('"') for k,v in fields.items()}
        self.assertEqual(fields['id'],'daily/YYYY-MM-DD')
        self.assertEqual(fields['title'],body.strip().splitlines()[0][2:])
        self.assertEqual(fields['type'],'daily')
        self.assertEqual(fields['scope'],'owner')
        self.assertEqual(fields['status'],'active')
        self.assertEqual(fields['sensitivity'],'owner-private')
        self.assertEqual(fields['updated'],'YYYY-MM-DD')
        self.assertLessEqual(len(fields['summary']),150)

    def test_authored_index_consumers_use_migrated_canonical_paths(self):
        for rel in ['bundles/base/skills/craft-update/SKILL.md', 'bundles/base/skills/learnings-bridge/SKILL.md']:
            text=(ROOT/rel).read_text()
            with self.subTest(path=rel):
                self.assertIn('brain/craft/craft.md',text)
                self.assertNotIn('brain/craft/index.md',text)
                self.assertNotIn('brain/learnings/index.md',text)
        self.assertIn('brain/learnings/learnings.md',(ROOT/'bundles/base/skills/learnings-bridge/SKILL.md').read_text())
        craft=(ROOT/'bundles/base/skills/craft-update/SKILL.md').read_text()
        learnings=(ROOT/'bundles/base/skills/learnings-bridge/SKILL.md').read_text()
        self.assertEqual(craft.count('[Índice de craft](../craft.md)'),2)
        self.assertIn('[Índice de learnings](learnings.md)',learnings)
        self.assertNotIn('](../index.md)',craft)
        self.assertNotIn('](index.md)',learnings)

    def test_powershell_scaffold_uses_canonical_authored_indexes(self):
        text=(ROOT/'installers/zip/user-template/.claude/hooks/lib/Maestro.Hooks.ps1').read_text()
        for canonical,old in [('craft/craft.md','craft/index.md'),('learnings/learnings.md','learnings/index.md')]:
            self.assertIn("Join-Path $data '"+canonical+"'",text)
            self.assertNotIn("Join-Path $data '"+old+"'",text)

    def test_typed_yoda_contracts_match_their_own_consumer(self):
        expected={'intent':{'approve','refine','clarify','hold_exceptional'},'delivery':set(json.loads((ROOT/'schemas/yoda-review.schema.json').read_text())['properties']['verdict']['enum'])}
        for path in ['bundles/base/skills/yoda/SKILL.md','bundles/base/agents/yoda/AGENT.md','installers/zip/user-template/.claude/agents/yoda.md','installers/zip/user-template/.codex/agents/yoda.toml']:
            with self.subTest(path=path):
                text=(ROOT/path).read_text()
                blocks=re.findall(r'REVIEW_TYPE: (intent|delivery)\nVERDICT: ([^\n]+)',text)
                contracts={kind:{value.strip() for value in choices.split('|')} for kind,choices in blocks}
                self.assertEqual(contracts,expected,'producer must identify separate typed output sets')
                self.assertTrue(contracts['intent'].isdisjoint(contracts['delivery']))

if __name__=='__main__':unittest.main()
