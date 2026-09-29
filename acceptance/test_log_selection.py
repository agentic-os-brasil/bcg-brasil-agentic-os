"""Offline behavioral privacy tests; these do not qualify an agent or host."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

MODULE = Path(__file__).resolve().parents[1] / 'bundles/base/tools/log-selection.py'
spec = importlib.util.spec_from_file_location('log_selection', MODULE)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class SelectionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.workspace = self.root / 'Workspace Unicode á'
        self.workspace.mkdir()
        self.logs = self.root / 'chosen-project'
        self.logs.mkdir()
        self.file = self.logs / 'one.jsonl'
        self.file.write_text(json.dumps({'type':'user','cwd':str(self.workspace),'message':{'content':'PRIVATE BODY'}})+'\n')
        self.request = {'consent':True,'workspaces':[str(self.workspace)],'sources':[{'kind':'claude_code','workspace':str(self.workspace),'root':str(self.logs),'sessions':[{'path':'one.jsonl','id':'one'}]}]}

    def test_only_selected_session_read_and_audit_excludes_content(self):
        (self.logs/'foreign.jsonl').write_text('FOREIGN SECRET')
        plan=m.prepare(self.request)
        result=m.read_batch(plan,m.digest(plan))
        self.assertEqual(len(result['sessions']),1)
        self.assertIn('PRIVATE BODY',result['sessions'][0]['body'])
        audit=json.dumps(result['audit'])
        self.assertNotIn('PRIVATE',audit)
        self.assertNotIn(str(self.workspace),audit)

    def test_foreign_workspace_rejected_before_body_open(self):
        self.request['sources'][0]['workspace']=str(self.logs)
        with patch('builtins.open',side_effect=AssertionError('body opened')):
            with self.assertRaises(m.SelectionError):m.prepare(self.request)

    def test_missing_consent_and_unbounded_sources_rejected(self):
        self.request['consent']=False
        with self.assertRaises(m.SelectionError):m.prepare(self.request)
        self.request['consent']=True
        self.request['sources']*=5
        with self.assertRaises(m.SelectionError):m.prepare(self.request)

    def test_oversized_file_rejected_before_read(self):
        with self.file.open('wb') as f:f.truncate(m.MAX_FILE_BYTES+1)
        with patch('builtins.open',side_effect=AssertionError('body opened')):
            with self.assertRaises(m.SelectionError):m.prepare(self.request)

    def test_symlink_and_traversal_denied(self):
        (self.logs/'alias.jsonl').symlink_to(self.file)
        for path in ['alias.jsonl','../chosen-project/one.jsonl','/one.jsonl']:
            self.request['sources'][0]['sessions'][0]['path']=path
            with self.assertRaises(m.SelectionError):m.prepare(self.request)

    def test_selection_mutation_and_source_replacement_denied_before_read(self):
        plan=m.prepare(self.request);approval=m.digest(plan)
        plan['sources'][0]['sessions'][0]['path']='foreign.jsonl'
        with self.assertRaises(m.SelectionError):m.read_batch(plan,approval)
        plan=m.prepare(self.request);approval=m.digest(plan)
        self.file.write_text('replacement')
        with self.assertRaises(m.SelectionError):m.read_batch(plan,approval)

    def test_batch_resume_is_bound_to_selection(self):
        for n in range(1,4):
            name=f'{n}.jsonl';(self.logs/name).write_text('{"type":"user"}\n')
            self.request['sources'][0]['sessions'].append({'path':name,'id':str(n)})
        plan=m.prepare(self.request);approval=m.digest(plan)
        first=m.read_batch(plan,approval,batch_size=2)
        second=m.read_batch(plan,approval,cursor=first['cursor'],batch_size=2)
        self.assertEqual(len(first['sessions']),2)
        self.assertEqual(len(second['sessions']),2)
        self.assertTrue(second['complete'])
        other=m.prepare(self.request);other['consent']=False
        with self.assertRaises(m.SelectionError):m.read_batch(other,approval,cursor=first['cursor'])

    def test_selected_export_has_same_limits(self):
        export=self.logs/'chosen.json';export.write_text('[{"uuid":"approved","chat_messages":[]}]')
        self.request['sources'][0]['kind']='claude_export'
        self.request['sources'][0]['sessions']=[{'path':'chosen.json','id':'chosen-export'}]
        plan=m.prepare(self.request);result=m.read_batch(plan,m.digest(plan))
        self.assertEqual(len(result['sessions']),1)
        export.write_bytes(b'x'*(m.MAX_FILE_BYTES+1))
        with self.assertRaises(m.SelectionError):m.prepare(self.request)

    def test_session_and_total_byte_limits(self):
        self.request['sources'][0]['sessions']*=21
        with self.assertRaises(m.SelectionError):m.prepare(self.request)
        sessions=[]
        for n in range(5):
            p=self.logs/f'big{n}.jsonl'
            with p.open('wb') as f:f.truncate(m.MAX_FILE_BYTES)
            sessions.append({'path':p.name,'id':str(n)})
        self.request['sources'][0]['sessions']=sessions
        with self.assertRaises(m.SelectionError):m.prepare(self.request)

    def test_worktree_cwd_is_not_automatic_consent(self):
        self.file.write_text(json.dumps({'cwd':str(self.workspace/'.claude/worktrees/other'),'message':'PRIVATE'})+'\n')
        plan=m.prepare(self.request)
        with self.assertRaises(m.SelectionError):m.read_batch(plan,m.digest(plan))

    def test_export_conversation_count_is_bounded(self):
        p=self.logs/'chosen.json';p.write_text(json.dumps([{'uuid':str(n)} for n in range(21)]))
        self.request['sources'][0].update(kind='claude_export',sessions=[{'path':p.name,'id':'selected'}])
        plan=m.prepare(self.request)
        with self.assertRaises(m.SelectionError):m.read_batch(plan,m.digest(plan))

    def test_unknown_export_container_is_unavailable(self):
        p=self.logs/'chosen.json';p.write_text('{"opaque_container":{"unbounded":[]}}')
        self.request['sources'][0].update(kind='claude_export',sessions=[{'path':p.name,'id':'selected'}])
        plan=m.prepare(self.request)
        with self.assertRaises(m.SelectionError):m.read_batch(plan,m.digest(plan))

if __name__=='__main__':unittest.main()
