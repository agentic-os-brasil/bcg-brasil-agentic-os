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
