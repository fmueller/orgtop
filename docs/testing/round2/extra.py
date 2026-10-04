from pathlib import Path
exec(Path('/tmp/orgtop-round2.py').read_text().split('\ntry:\n')[0])
SOCKET='orgtop-round2-extra'
OUT=Path('/tmp/orgtop-round2-extra-evidence'); OUT.mkdir(exist_ok=True)
base_respond=respond
def respond(path):
    if mode=='fairness':
        if '/events' in path: return 200,[event(id=str(i),ref='refs/heads/main') for i in range(100)],{}
        if '/commits/' in path: return 200,dict(sha=HEAD,parents=[dict(sha=BEFORE)],files=[dict(filename=f'src/p{i}/main.go',status='modified') for i in range(49)]),{}
    if mode=='hostile-entity':
        if '/events' in path: return 200,[event(head='bad\u202eHEAD')],{}
    return base_respond(path)
try:
    mode='fairness'; args=['--repo','acme/api']
    for i in range(49): args+=['--path',f'acme/api:src/p{i}']
    a=App('fairness',args+['--no-cache'],width=240,height=40); s=a.capture(); check('50 overlapping scopes fully enriched',s.count('100 activity')==38 and 'PATH ?' not in s)
    a.key('3'); a.key('p'); s=a.capture('fairness-rain'); check('50-scope fair 500 admitted / 4500 omitted', '+320 items hidden' in s and '4500' in s and 'PAUSED' in s)
    for w,h in [(80,24),(40,12),(20,6),(8,3),(1,1),(240,40)]: a.resize(w,h); a.capture(f'fairness-{w}x{h}')
    a.key(']'); s=a.capture('fairness-page'); check('Rain advances 18-scope page','scopes 19-36 of 50' in s)
    a.key(']'); s=a.capture('fairness-last-page'); check('Rain final page retains fair tail','scopes 37-50 of 50' in s and '+360 items hidden' in s)
    a.key('['); a.key('['); check('Rain page anchor returns after resize','scopes 1-18 of 50' in a.capture('fairness-return')); a.quit()
    mode='hostile-entity'; a=App('entity-repeat',['--repo','acme/api','--no-cache'],width=240)
    a.key('3'); s=a.capture('entity-rain'); check('entity reference escapes bidi', '\u202e' not in s and '\\u{202E}' in s)
    a.key('2'); a.key('Enter'); d=a.capture('entity-detail'); check('entity detail comparison safe','\u202e' not in d and '\\u{202E}' in d)
    a.quit()
    mode='normal'; args=['--repo','acme/api','--path','acme/api:src']; cache=ROOT/'locked'
    a=App('seed',args,cache); a.quit()
    lock=open(cache/'orgtop'/'enrichment-v1.lock','r+b'); fcntl.lockf(lock,fcntl.LOCK_EX,1,1)
    a=App('admission-busy',args,cache); s=a.capture(); check('admission lock degrades without losing membership',current(s) and 'CACHE DEGRADED' in s and s.count('1 activity')==2)
    fcntl.lockf(lock,fcntl.LOCK_UN,1,1); lock.close()
    print('WAIT actual 61-second cache recovery',flush=True); time.sleep(61); a.key('r'); time.sleep(1); s=a.capture('admission-recovered'); check('next refresh retries cache and clears degradation',current(s) and 'CACHE DEGRADED' not in s and s.count('1 activity')==2); a.quit()
finally:
    (OUT/'requests.json').write_text(json.dumps(requests,indent=2))
    (OUT/'runtime.json').write_text(json.dumps(dict(root=str(ROOT)),indent=2))
    server.shutdown(); tm('kill-server')
