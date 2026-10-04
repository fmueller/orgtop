import os, sys, json, time, ssl, socketserver, http.server, threading, subprocess, tempfile, pathlib, shlex, sqlite3, fcntl, hashlib
from urllib.parse import urlparse, parse_qs

ROOT = pathlib.Path(tempfile.mkdtemp(prefix='orgtop-r2-'))
OUT = pathlib.Path('/tmp/orgtop-round2-evidence'); OUT.mkdir(exist_ok=True)
BIN = '/tmp/orgtop-round2'
results = []; requests = []; mode = 'normal'; phase = 0
def check(name, ok, detail=None):
    results.append(dict(name=name, pass_=bool(ok), detail=detail))
    print(('PASS ' if ok else 'FAIL ') + name + (' '+str(detail) if detail is not None else ''), flush=True)
    (OUT/'results.json').write_text(json.dumps(results, indent=2))
def run(args, **kw): return subprocess.run(args, text=True, capture_output=True, **kw)
subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(ROOT/'key'),'-out',str(ROOT/'cert'),'-days','1','-subj','/CN=api.github.com','-addext','subjectAltName=DNS:api.github.com'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); ctx.load_cert_chain(ROOT/'cert',ROOT/'key')
NOW = time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())
BEFORE='1'*40; HEAD='2'*40
def event(repo='acme/api', id='1', ref='refs/heads/release-\u202eabc', head=HEAD):
    return dict(id=str(id),type='PushEvent',repo=dict(name=repo),actor=dict(login='tester'),created_at=NOW,payload=dict(size=1,before=BEFORE,head=head,ref=ref))
def record(org, n):
    name=f'r{n:03d}'
    return dict(full_name=f'{org}/{name}',name=name,owner=dict(login=org,type='Organization'),disabled=False,archived=False,fork=False)
def respond(path):
    u=urlparse(path); p=u.path; q=parse_qs(u.query); headers={}
    if p.startswith('/orgs/'):
        org=p.split('/')[2]; page=int(q.get('page',['1'])[0]); body=[record(org,n) for n in range((page-1)*100,page*100)]
        if mode=='org-oversize': return 200,body+[record(org,999)],headers
        if mode=='org-badnext':
            headers['Link']=f'<https://example.invalid/orgs/{org}/repos?type=all&sort=full_name&direction=asc&per_page=100&page=2>; rel="next"'
        elif mode=='org-partial' and org=='beta': return 503,dict(message='temporary'),headers
        elif mode=='org-conflict': body=[record(org,0),dict(record(org,0),fork=True)]
        elif mode=='org-empty': body=[dict(record(org,0),disabled=True)]
        else: headers['Link']=f'<https://api.github.com/orgs/{org}/repos?type=all&sort=full_name&direction=asc&per_page=100&page={page+1}>; rel="next"'
        return 200,body,headers
    if p.endswith('/events'):
        repo='/'.join(p.split('/')[2:4])
        if mode=='cycle' and phase==1 and repo=='acme/other': return 503,dict(message='source temporarily failed'),headers
        if mode=='cycle': return 200,[event(repo,str(phase*10+(1 if repo=='acme/api' else 2)),ref=f'refs/heads/phase{phase}')],headers
        if mode=='capacity': return 200,[event(repo,f'{repo}-{i}',ref='refs/heads/main') for i in range(100)],headers
        if mode=='hostile-entity': return 200,[event(repo,head='bad\u202eHEAD')],headers
        if mode.startswith('org'): return 200,[event(repo,repo,ref='refs/heads/main')],headers
        return 200,[event(repo)],headers
    if '/commits/' in p:
        if mode=='cycle' and phase==2: return 403,dict(message='denied'),headers
        return 200,dict(sha=HEAD,parents=[dict(sha=BEFORE)],files=[dict(filename='src/main.go',status='modified')]),headers
    return 404,dict(message='unexpected fixture endpoint'),headers
class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version='HTTP/1.1'
    def log_message(self,*args): pass
    def do_CONNECT(self):
        if self.path!='api.github.com:443': self.send_error(403); return
        self.send_response(200); self.end_headers()
        try:
            s=ctx.wrap_socket(self.connection,server_side=True)
            Handler(s,self.client_address,self.server)
        except (OSError,ssl.SSLError): pass
        self.close_connection=True
    def do_GET(self):
        requests.append(dict(mode=mode,phase=phase,path=self.path,api=self.headers.get('X-GitHub-Api-Version'),accept=self.headers.get('Accept'),credential_match=self.headers.get('Authorization')=='Bearer round2-synthetic-token',time=time.monotonic()))
        code,body,headers=respond(self.path); data=json.dumps(body).encode()
        self.send_response(code); self.send_header('Content-Type','application/json'); self.send_header('Content-Length',str(len(data)))
        for k,v in headers.items(): self.send_header(k,v)
        self.end_headers()
        try: self.wfile.write(data)
        except OSError: pass
class Server(socketserver.ThreadingMixIn,http.server.HTTPServer): daemon_threads=True
server=Server(('127.0.0.1',0),Handler); threading.Thread(target=server.serve_forever,daemon=True).start()
def environment(cache):
    home=ROOT/'home'; home.mkdir(exist_ok=True)
    return dict(PATH=os.environ['PATH'],HOME=str(home),XDG_CACHE_HOME=str(cache),GH_TOKEN='round2-synthetic-token',HTTPS_PROXY=f'http://127.0.0.1:{server.server_port}',SSL_CERT_FILE=str(ROOT/'cert'),NO_PROXY='',TERM='xterm-256color',LANG='C.UTF-8')
SOCKET='orgtop-round2'
def tm(*args): return run(['tmux','-L',SOCKET,*args])
class App:
    def __init__(self,name,args,cache=None,width=160,height=32):
        self.name=name; self.cache=cache or ROOT/name; self.args=args
        env=environment(self.cache)
        command='env -i '+ ' '.join(shlex.quote(k+'='+v) for k,v in env.items())+' '+shlex.join([BIN,*args])+'; echo EXIT:$?; sleep 1'
        r=tm('new-session','-d','-s',name,'-x',str(width),'-y',str(height),command)
        if r.returncode: raise RuntimeError(r.stderr)
        time.sleep(1.6)
    def key(self,key): tm('send-keys','-t',self.name,key); time.sleep(.35)
    def capture(self,label=None):
        text=tm('capture-pane','-p','-t',self.name).stdout
        (OUT/f'{label or self.name}.txt').write_text(text)
        return text
    def resize(self,w,h): tm('resize-window','-t',self.name,'-x',str(w),'-y',str(h)); time.sleep(.3)
    def quit(self):
        self.key('q'); time.sleep(.2)
        text=self.capture(self.name+'-exit')
        check(self.name+' responsive quit','EXIT:0' in text or tm('has-session','-t',self.name).returncode!=0)
        tm('kill-session','-t',self.name)
def count_commits(): return sum('/commits/' in r['path'] for r in requests)
def current(text):
    return 'updated ' in text and not any(x in text.splitlines()[0] for x in ['STALE','ERROR','LOADING'])

try:
    # Informational and bare boolean regressions use actual process and cache sentinel.
    sentinel=ROOT/'cli'/'orgtop'/'enrichment-v1.db'; sentinel.parent.mkdir(parents=True); sentinel.write_bytes(b'untouched disposable cache sentinel')
    cli=[]
    for info in ['--help','-h','--version','-v']:
        for args in [['--bad',info],[info,'--reset-cache','--bad'],[info,'--repo'],['--repo=broken',info]]:
            p=run([BIN,*args],env=environment(ROOT/'cli'),timeout=5); cli.append(dict(args=args,exit=p.returncode,stdout=p.stdout,stderr=p.stderr))
            check('T119 '+shlex.join(args),p.returncode==0 and ('Usage:' in p.stderr if info in ['--help','-h'] else p.stdout=='orgtop dev\n'))
    for flag in ['--reset-cache','--no-cache','--include-forks','--include-archived']:
        for val in ['true','false']:
            args=['--reset-cache',flag+'='+val]; p=run([BIN,*args],env=environment(ROOT/'cli'),timeout=5); cli.append(dict(args=args,exit=p.returncode,stderr=p.stderr))
            check('T120 '+shlex.join(args),p.returncode==2 and sentinel.read_bytes()==b'untouched disposable cache sentinel')
    (OUT/'cli.json').write_text(json.dumps(cli,indent=2))
    args=['--repo','acme/api','--path','acme/api:src']
    a=App('hostile',args+['--no-cache']); a.key('2'); s=a.capture(); a.key('Enter'); d=a.capture('hostile-detail')
    check('T121 Stream and detail escaped', '\u202e' not in s+d and '\\u{202E}' in s and '\\u{202E}' in d)
    a.key('Escape'); a.resize(40,12); check('T121 constrained Stream safe','\u202e' not in a.capture('hostile-40')); a.quit()
    cache=ROOT/'shared'; start=count_commits(); a=App('cold',args,cache); check('cold CURRENT',current(a.capture())); a.quit()
    b=App('warm',args,cache); check('warm reuse no extra commit',count_commits()==start+1); b.quit()
    db=cache/'orgtop'/'enrichment-v1.db'; check('real SQLite cache exists',db.exists())
    digest=hashlib.sha256(db.read_bytes()).hexdigest()
    p=run([BIN,'--reset-cache','--no-cache=false'],env=environment(cache),timeout=5)
    check('T120 populated SQLite preserved',p.returncode==2 and hashlib.sha256(db.read_bytes()).hexdigest()==digest)
    # Keep one process alive while another uses the same cache, then reset under lifecycle lock.
    a=App('concurrent-a',args,cache); b=App('concurrent-b',args,cache)
    check('two simultaneous cache clients current',current(a.capture()) and current(b.capture()))
    started=time.monotonic(); p=run([BIN,'--reset-cache'],env=environment(cache),timeout=5)
    check('active cache reset bounded refusal',p.returncode!=0 and db.exists() and time.monotonic()-started<3,dict(exit=p.returncode,stderr=p.stderr.strip()))
    a.quit(); b.quit()
    # Actual SQLite writer contention, not model injection.
    sql=sqlite3.connect(db); sql.execute('BEGIN IMMEDIATE')
    a=App('sqlite-busy',args,cache); s=a.capture(); check('SQLite contention stays current/degraded',current(s) and 'CACHE DEGRADED' in s); a.quit(); sql.rollback(); sql.close()
    p=run([BIN,'--reset-cache'],env=environment(cache),timeout=5); check('idle reset succeeds',p.returncode==0 and not db.exists())
    a=App('rebuilt',args,cache); check('reset rebuild current',current(a.capture()) and db.exists()); a.quit()
    for scenario in ['org-oversize','org-badnext','org-partial','org-conflict','org-empty']:
        mode=scenario; start=len(requests); a=App(scenario,['--org','alpha','--org','beta','--repo','exact/keep','--no-cache']); text=a.capture()
        if scenario!='org-empty': check(scenario+' atomic failure', 'ERROR' in text and not any('/events' in r['path'] for r in requests[start:]))
        else: check('org-empty mixed retains exact', current(text) and any('/repos/exact/keep/events' in r['path'] for r in requests[start:]))
        a.quit()
    mode='org-pages'; start=len(requests); a=App('org-pages',['--org','alpha','--org','beta','--repo','exact/keep','--no-cache'],width=240)
    s=a.capture(); req=requests[start:]; pages=[r['path'] for r in req if '/orgs/' in r['path']]; polls=[r['path'] for r in req if '/events' in r['path']]
    check('org 5-page fairness',[p.split('/')[2] for p in pages]==['alpha','beta','alpha','beta','alpha'],pages)
    expected=['exact/keep']+[f'{o}/r{i:03d}' for i in range(10) for o in ['alpha','beta']][:19]
    check('org 20-repo capacity exact precedence',sorted(p.split('/events')[0].removeprefix('/repos/') for p in polls)==sorted(expected),len(polls))
    check('org disclosure known and unknown omission','481 eligible omitted' in s and 'more eligible may be omitted' in s,s.splitlines()[:5]); a.quit()
    mode='capacity'; start=len(requests); many=[]
    for i in range(6): many+=['--repo',f'acme/r{i}']
    a=App('events-capacity',many+['--no-cache'],width=240); s=a.capture(); check('600 events globally truncated to 500','newest 500 of 600' in s,s.splitlines()[:5]); a.quit()
    mode='normal'; many=['--repo','acme/api']
    for i in range(49): many+=['--path',f'acme/api:src/p{i}']
    a=App('many-scopes',many+['--no-cache'],width=160,height=40); s=a.capture(); check('50 scope overview accounting','scopes 1-38 of 50' in s)
    a.key('3'); a.key('p'); a.capture('many-rain-wide')
    for w,h in [(80,24),(40,12),(20,6),(8,3),(1,1),(160,40)]: a.resize(w,h); a.capture(f'many-rain-{w}x{h}')
    a.key('Right'); a.capture('many-rain-page'); a.key('1'); check('multi-scope resize restores responsive overview','scopes 1-38 of 50' in a.capture('many-restored')); a.quit()
    # Full source success -> failure -> success-with-unknown -> complete recovery, each real floor.
    mode='cycle'; phase=0; a=App('cycle',['--repo','acme/api','--repo','acme/other','--path','acme/api:src','--no-cache'],width=200)
    check('cycle initial complete',current(a.capture('cycle-0')) and 'PATH ?' not in a.capture('cycle-0'))
    for ph in [1,2,3]:
        phase=ph; print(f'WAIT real refresh floor phase {ph}',flush=True); time.sleep(61); a.key('r'); time.sleep(1); s=a.capture(f'cycle-{ph}')
        if ph==1:
            check('source failure retains stale snapshot','STALE' in s and s.count('1 activity')==3,s.splitlines()[:6]); a.key('2'); st=a.capture('cycle-stale-stream'); check('no partial phase1 event publication','phase1' not in st and 'phase0' in st); a.key('1')
        elif ph==2: check('source recovery current with independent unknown',current(s) and 'PATH ?1' in s,s.splitlines()[:6])
        else: check('enrichment recovery clears unknown',current(s) and 'PATH ?' not in s,s.splitlines()[:6])
    a.quit()
    mode='hostile-entity'; a=App('hostile-entity',['--repo','acme/api','--no-cache'],width=240); a.key('3'); s=a.capture(); check('malformed entity no bidi leak','\u202e' not in s); a.quit()
    check('all HTTP requests contract headers',all(r['api']=='2026-03-10' and r['accept']=='application/vnd.github+json' and r['credential_match'] for r in requests),len(requests))
finally:
    (OUT/'requests.json').write_text(json.dumps(requests,indent=2))
    (OUT/'runtime.json').write_text(json.dumps(dict(root=str(ROOT),binary=BIN),indent=2))
    server.shutdown()
    tm('kill-server')
    print('Evidence: '+str(OUT),flush=True)
