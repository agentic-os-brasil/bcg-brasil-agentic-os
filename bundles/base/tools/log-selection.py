"""Optional bounded history selector. Never discovers or enrolls global history.

prepare uses filesystem metadata only. read_batch requires approval of its exact
plan digest. Bodies are ephemeral output; audit/cursor contain metadata only.
Secure body reads require dir_fd + O_NOFOLLOW; unsupported hosts fail closed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import sys

MAX_SOURCES = 4
MAX_SESSIONS = 20
MAX_FILE_BYTES = 2 * 1024 * 1024
MAX_TOTAL_BYTES = 8 * 1024 * 1024
MAX_CONFIG_BYTES = 64 * 1024

class SelectionError(ValueError):
    pass

def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()

def canonical(value, directory=True):
    if not isinstance(value,str) or not value or '\x00' in value:
        raise SelectionError('invalid selected path')
    p=Path(value)
    if not p.is_absolute() or '..' in p.parts or str(p)!=value:
        raise SelectionError('select an exact canonical absolute path')
    for part in [*reversed(p.parents),p]:
        if part.is_symlink():
            raise SelectionError('symlink selections are unavailable')
    if str(p.resolve(strict=True))!=value or (directory and not p.is_dir()):
        raise SelectionError('canonical directory required')
    return p

def identity(p):
    s=p.lstat()
    if not stat.S_ISREG(s.st_mode) or s.st_size>MAX_FILE_BYTES:
        raise SelectionError('session is not a bounded regular file')
    return [s.st_dev,s.st_ino,s.st_size,s.st_mtime_ns,s.st_ctime_ns]

def prepare(request):
    if not isinstance(request,dict) or request.get('consent') is not True:
        raise SelectionError('explicit invocation consent required')
    workspaces=request.get('workspaces',[])
    sources=request.get('sources',[])
    if not isinstance(workspaces,list) or not 1<=len(workspaces)<=MAX_SOURCES or not isinstance(sources,list) or not 1<=len(sources)<=MAX_SOURCES:
        raise SelectionError('bounded workspace and source allowlists required')
    allowed=[str(canonical(w)) for w in workspaces]
    result={'schema_version':1,'consent':True,'workspaces':allowed,'sources':[]}
    count=total=0
    seen=set()
    for source in sources:
        if source.get('workspace') not in allowed:
            raise SelectionError('source workspace was not explicitly enrolled')
        kind=source.get('kind')
        if kind not in ('claude_code','claude_export'):
            raise SelectionError('unsupported source type')
        root=canonical(source.get('root'))
        sessions=source.get('sessions',[])
        if not isinstance(sessions,list) or not sessions:
            raise SelectionError('explicit session file allowlist required')
        selected={'kind':kind,'workspace':source['workspace'],'root':str(root),'sessions':[]}
        for session in sessions:
            count+=1
            if count>MAX_SESSIONS:
                raise SelectionError('session selection exceeds batch ceiling')
            name=session.get('path');sid=session.get('id')
            if not isinstance(name,str) or not isinstance(sid,str) or not sid or len(sid)>128:
                raise SelectionError('explicit session ID and path required')
            rel=Path(name)
            if rel.is_absolute() or '..' in rel.parts or '\\' in name or any(c in name for c in '*?[]\x00') or str(rel)!=name:
                raise SelectionError('relative session path is unsafe')
            if rel.suffix!=('.jsonl' if kind=='claude_code' else '.json'):
                raise SelectionError('select JSONL sessions or an explicitly approved JSON export; ZIPs unavailable')
            path=canonical(str(root/rel),directory=False)
            if (str(path),sid) in seen or any(x[0]==str(path) for x in seen):
                raise SelectionError('duplicate session selection')
            seen.add((str(path),sid))
            stamp=identity(path);total+=stamp[2]
            if total>MAX_TOTAL_BYTES:
                raise SelectionError('selection exceeds total byte ceiling')
            selected['sessions'].append({'path':name,'id':sid,'identity':stamp})
        result['sources'].append(selected)
    return result

def secure_read(path, expected):
    if not hasattr(os,'O_NOFOLLOW') or os.open not in os.supports_dir_fd:
        raise SelectionError('secure bounded history reader unavailable on this host')
    parts=Path(path).parts
    fd=os.open(parts[0],os.O_RDONLY|os.O_DIRECTORY)
    try:
        for part in parts[1:-1]:
            nxt=os.open(part,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd)
            os.close(fd);fd=nxt
        datafd=os.open(parts[-1],os.O_RDONLY|os.O_NOFOLLOW,dir_fd=fd)
        try:
            def stamp():
                s=os.fstat(datafd)
                if not stat.S_ISREG(s.st_mode):raise SelectionError('source is not regular')
                return [s.st_dev,s.st_ino,s.st_size,s.st_mtime_ns,s.st_ctime_ns]
            if stamp()!=expected:raise SelectionError('selected source changed; obtain fresh approval')
            chunks=[];size=0
            while True:
                b=os.read(datafd,min(65536,MAX_FILE_BYTES+1-size))
                if not b:break
                size+=len(b)
                if size>expected[2] or size>MAX_FILE_BYTES:raise SelectionError('selected source grew')
                chunks.append(b)
            if stamp()!=expected or size!=expected[2]:raise SelectionError('selected source changed during read')
            return b''.join(chunks).decode('utf-8-sig')
        finally:os.close(datafd)
    finally:os.close(fd)

def read_batch(plan, approval, cursor=None, batch_size=5):
    if not isinstance(approval,str) or digest(plan)!=approval:
        raise SelectionError('exact selection approval required')
    # Validate every path and stat before opening any selected body, including resume.
    if prepare(plan)!=plan:
        raise SelectionError('selection changed; obtain fresh approval')
    if type(batch_size) is not int or not 1<=batch_size<=5:
        raise SelectionError('batch size must be between one and five')
    start=0
    if cursor is not None:
        if not isinstance(cursor,dict) or cursor.get('selection')!=approval or type(cursor.get('next')) is not int:
            raise SelectionError('resume belongs to another selection')
        start=cursor['next']
    entries=[(source,session) for source in plan['sources'] for session in source['sessions']]
    if not 0<=start<=len(entries):raise SelectionError('invalid resume position')
    end=min(start+batch_size,len(entries));sessions=[];audit=[]
    for source,session in entries[start:end]:
        body=secure_read(str(Path(source['root'])/session['path']),session['identity'])
        if source['kind']=='claude_code':
            for line in body.splitlines():
                row=json.loads(line)
                if not isinstance(row,dict):raise SelectionError('unsupported session record')
                if row.get('cwd') is not None and row['cwd']!=source['workspace']:
                    raise SelectionError('session scope differs from enrolled workspace; no automatic worktree mapping')
        else:
            export=json.loads(body)
            if not isinstance(export,list) or len(export)>MAX_SESSIONS:
                raise SelectionError('export container exceeds supported conversation count')
        token=digest({'selection':approval,'id':session['id'],'root':source['root']})
        sessions.append({'session':token,'kind':source['kind'],'body':body})
        audit.append({'session':token,'bytes':session['identity'][2],'sha256':hashlib.sha256(body.encode()).hexdigest()})
    return {'sessions':sessions,'audit':{'selection':approval,'read':audit},'cursor':{'selection':approval,'next':end},'complete':end==len(entries)}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode',choices=['prepare','read'])
    parser.add_argument('--approved-digest')
    parser.add_argument('--resume',type=int,default=0)
    args=parser.parse_args()
    try:
        raw=sys.stdin.buffer.read(MAX_CONFIG_BYTES+1)
        if len(raw)>MAX_CONFIG_BYTES:raise SelectionError('selection manifest too large')
        request=json.loads(raw)
        if args.mode=='prepare':
            plan=prepare(request);result={'plan':plan,'approval_digest':digest(plan)}
        else:
            result=read_batch(request,args.approved_digest,{'selection':args.approved_digest,'next':args.resume})
        json.dump(result,sys.stdout,ensure_ascii=False);sys.stdout.write('\n')
    except (SelectionError,OSError,ValueError,TypeError,KeyError,AttributeError):
        print('History selection unavailable: verify explicit scope, limits and unchanged source metadata. No audit content retained.',file=sys.stderr)
        return 2
    return 0

if __name__=='__main__':sys.exit(main())
