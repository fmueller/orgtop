from pathlib import Path
exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil
OUT=Path('/tmp/orgtop-round3-evidence'); OUT.mkdir(exist_ok=True)
SOCKET='orgtop-round3'
base_respond=respond
ARGS=['--repo','acme/api','--path','acme/api:src']
def respond(path):
    p=urlparse(path).path; q=parse_qs(urlparse(path).query)
    if p.endswith('/events'):
        e=event(ref='refs/heads/main')
        if mode.startswith('compare'): e['payload']['size']=2
        if mode=='parent-coalesce':
            other=event(id='2',ref='refs/heads/other'); other['payload']['before']='3'*40
            return 200,[e,other],{}
        return 200,[e],{}
    if '/compare/' in p:
        b=dict(url='https://api.github.com'+p,base_commit=dict(sha=BEFORE),files=[])
        if mode=='compare-missing': del b['files']
        elif mode=='compare-null': b['files']=None
        elif mode in ['compare-299','compare-300']:
            b['files']=[dict(filename=f'src/f{i}',status='modified') for i in range(int(mode[-3:]))]
        return 200,b,{}
    if '/commits/' in p:
        b=dict(sha=HEAD,parents=[dict(sha=BEFORE)],files=[dict(filename='src/a',status='modified')]); h={}
        page=int(q.get('page',['1'])[0])
        target=f'https://api.github.com{p}?per_page=100&page=2'
        if mode=='missing': del b['files']
        elif mode=='null': b['files']=None
        elif mode=='empty': b['files']=[]
        elif mode=='unknown-status': b['files'].append(dict(filename='outside/b',status='invented'))
        elif mode=='rename-missing': b['files'].append(dict(filename='src/b',status='renamed'))
        elif mode=='duplicate': b['files']*=2
        elif mode=='link-malformed': h['Link']=f'{target}; rel="next"'
        elif mode=='link-cycle': h['Link']=f'<{target}>; rel="next"'; b['files'][0]['filename']=f'src/p{page}'
        elif mode=='link-reverse':
            if page!=2: h['Link']=f'<https://api.github.com{p}?per_page=100&page={3 if page==1 else 2}>; rel="next"'
            b['files'][0]['filename']=f'src/p{page}'
        elif mode=='link-duplicate-query':
            if page==1: h['Link']=f'<{target}&page=999>; rel="next"'
            b['files'][0]['filename']=f'src/p{page}'
        elif mode in ['paged-good','paged-missing','paged-denied','paged-rate']:
            if page==1: h['Link']=f'<{target}>; rel="next"'
            else:
                b['files']=[dict(filename='outside/b',status='modified')]
                if mode=='paged-missing': del b['files']
                elif mode=='paged-denied': return 403,dict(message='private response must not leak'),{}
                elif mode=='paged-rate': return 429,dict(message='private response must not leak'),{'Retry-After':'120'}
        return 200,b,h
    return base_respond(path)

def one(name,unknown=False,complete_member=True,cache=None):
    start=len(requests); a=App(name,ARGS+([] if cache else ['--no-cache']),cache,width=220)
    s=a.capture(); check(name+' current',current(s))
    check(name+' explicit unknown' if unknown else name+' complete',('PATH ?1' in s)==unknown,s.splitlines()[:5])
    if not unknown: check(name+' membership',s.count('1 activity')==(2 if complete_member else 1))
    check(name+' no raw body','private response' not in s)
    a.quit(); return requests[start:]

try:
    for mode,unknown,member in [('empty',False,False),('missing',True,False),('null',True,False),('compare-empty',False,False),('compare-missing',True,False),('compare-null',True,False),('compare-299',False,True),('compare-300',True,False),('unknown-status',True,False),('rename-missing',True,False),('duplicate',True,False),('paged-good',False,True),('paged-missing',True,False),('paged-denied',True,False),('paged-rate',True,False),('link-malformed',True,False),('link-cycle',True,False),('link-reverse',True,False),('link-duplicate-query',True,False)]:
        req=one(mode,unknown,member)
        check(mode+' bounded requests',len(req)<=4,len(req))
    mode='parent-coalesce'; start=len(requests); a=App(mode,ARGS+['--no-cache'],width=220); s=a.capture()
    check('shared head per-event before validated','PATH ?1' in s and '2 activity' in s and '1 activity' in s)
    check('shared head one lookup',sum('/commits/' in r['path'] for r in requests[start:])==1); a.quit()
    mode='normal'
    for mutation in ['version','foreign','zero-id','schema','count','ordinal','future','expired','tombstone','bootstrap','crash']:
        cache=ROOT/mutation; one('seed-'+mutation,cache=cache)
        db=cache/'orgtop'/'enrichment-v1.db'
        if mutation in ['version','foreign','zero-id','schema','count','ordinal','future','expired']:
            sql=sqlite3.connect(db)
            statement={'version':'PRAGMA user_version=99','foreign':'PRAGMA application_id=123','zero-id':'PRAGMA application_id=0','schema':'DROP TABLE evidence_path','count':'UPDATE evidence SET path_count=2','ordinal':'UPDATE evidence_path SET ordinal=3','future':f'UPDATE evidence SET acquired_at={int(time.time())+1000},last_used_at={int(time.time())+1000}','expired':f'UPDATE evidence SET acquired_at={int(time.time())-30*86400},last_used_at={int(time.time())-30*86400}'}[mutation]
            sql.execute(statement); sql.commit(); sql.close()
        if mutation=='tombstone':
            db.rename(db.with_name('enrichment-v1.resetting'))
            for suffix in ['-wal','-shm','-journal']: db.with_name(db.name+suffix).write_bytes(b'orphaned sidecar fixture')
        if mutation=='bootstrap':
            db.unlink(); db.with_name('enrichment-v1.bootstrap').write_bytes(b'interrupted bootstrap fixture')
        if mutation=='crash':
            a=App('kill-live',ARGS,cache); pid=int(tm('list-panes','-t','kill-live','-F','#{pane_pid}').stdout.strip())
            children=run(['pgrep','-P',str(pid)]).stdout.split()
            for child in children:
                if Path('/proc/'+child+'/comm').read_text().strip()=='orgtop-round2': os.kill(int(child),9)
            time.sleep(.3); tm('kill-session','-t','kill-live')
        digest=hashlib.sha256(db.read_bytes()).hexdigest() if db.exists() else None
        start=count_commits(); a=App('recover-'+mutation,ARGS,cache,width=220); s=a.capture()
        check(mutation+' current complete fallback',current(s) and s.count('1 activity')==2 and 'PATH ?' not in s)
        if mutation in ['version','foreign','zero-id']:
            check(mutation+' degraded preserved','CACHE DEGRADED' in s and hashlib.sha256(db.read_bytes()).hexdigest()==digest)
        else: check(mutation+' recovered without degradation','CACHE DEGRADED' not in s)
        check(mutation+' lookup or crash reuse',count_commits()-start==(0 if mutation=='crash' else 1))
        a.quit()
        if mutation in ['foreign','zero-id','version']:
            p=run([BIN,'--reset-cache'],env=environment(cache),timeout=5)
            check(mutation+' reset ownership',p.returncode==0 and not db.exists() if mutation=='version' else p.returncode!=0 and hashlib.sha256(db.read_bytes()).hexdigest()==digest)
    sentinel=ROOT/'grammar'/'orgtop'/'enrichment-v1.db'; sentinel.parent.mkdir(parents=True); sentinel.write_bytes(b'unchanged')
    cases=[['--repo','acme/api','--path','../bad'],['--path','acme/api:src:bad'],['--org','acme','--path','src'],['--reset-cache','positional'],['--reset-cache','--org=acme'],['--include-forks'],['--repo','acme/api','--path','src/**x'],['--repo','acme/api','--path','src\\q'],['--no-cache=true'],['--repo=acme/api','--','positional'],['--org=acme/other'],['--repo','acme/api','--path','/root']]
    start=len(requests)
    for i,args in enumerate(cases):
        p=run([BIN,*args],env=environment(ROOT/'grammar'),timeout=5)
        check('grammar '+str(i),p.returncode==2 and sentinel.read_bytes()==b'unchanged',dict(args=args,exit=p.returncode,stderr=p.stderr.splitlines()[:2]))
    check('grammar no network',len(requests)==start)
    check('all HTTP headers',all(r['api']=='2026-03-10' and r['accept']=='application/vnd.github+json' and r['credential_match'] for r in requests))
finally:
    (OUT/'requests.json').write_text(json.dumps(requests,indent=2)); (OUT/'runtime.json').write_text(json.dumps(dict(root=str(ROOT))))
    server.shutdown(); server.server_close(); tm('kill-server')
