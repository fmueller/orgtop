from pathlib import Path
exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil
OUT=Path('/tmp/orgtop-round3-extra-evidence'); OUT.mkdir(exist_ok=True)
SOCKET='orgtop-round3-extra'
ARGS=['--repo','acme/api','--path','acme/api:src']
def respond(path):
    p=urlparse(path).path
    if p.endswith('/events'):
        if mode=='ages':
            events=[]
            for i,age in enumerate([-60,0,290,310,890,910,3590,3610,86410,604810]):
                e=event(id=str(i),ref='refs/heads/age'+str(i)); e['actor']['login']='actor'+str(i)
                e['created_at']=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime(time.time()-age)); events.append(e)
            return 200,events,{}
        if mode=='rate': return 200,[event(id=str(i),head=f'{i+2:040x}') for i in range(8)],{}
        return 200,[event(ref='refs/heads/ordinary')],{}
    if '/commits/' in p:
        b=dict(sha=HEAD,parents=[dict(sha=BEFORE)],files=[dict(filename='src/a',status='modified')]); h={}
        if mode=='missing': del b['files']
        elif mode=='null': b['files']=None
        elif mode=='link-malformed': h['Link']=f'https://api.github.com{p}?per_page=100&page=2; rel="next"'
        elif mode=='rate': return 429,dict(message='fixture private raw body'),{'Retry-After':'120'}
        return 200,b,h
    return 404,{},{}
try:
    for scenario in ['missing','null','link-malformed']:
        mode=scenario; cache=ROOT/scenario; a=App(scenario+'-cold',ARGS,cache,width=220); a.capture(); a.quit()
        db=cache/'orgtop'/'enrichment-v1.db'; sql=sqlite3.connect(db)
        rows=sql.execute('SELECT operation,path_count,complete FROM evidence').fetchall(); sql.close()
        check(scenario+' malformed not persisted',not rows,rows)
        mode='valid'; start=count_commits(); a=App(scenario+'-warm',ARGS,cache,width=220); s=a.capture()
        check(scenario+' corrected upstream reacquired',count_commits()==start+1,count_commits()-start)
        check(scenario+' warm membership',s.count('1 activity')==2,s.splitlines()[:4]); a.quit()
    mode='ages'; a=App('ages',['--repo','acme/api','--no-cache'],width=240,height=40)
    check('ten age events retained','10 activity' in a.capture())
    a.key('2'); s=a.capture('age-stream')
    check('all age rows retained including future and old',all('actor'+str(i) in s for i in range(10)))
    check('Stream future youngest display','<1m' in next(line for line in s.splitlines() if 'actor0' in line))
    a.key('3'); a.key('p'); s=a.capture('age-rain')
    strip=s.split('Interesting Now')[-1]
    check('strip five inside 15m including future',all('actor'+str(i) in strip for i in range(5)) and all('actor'+str(i) not in strip for i in range(5,10)),strip)
    for _ in range(4): a.key('-')
    s=a.capture('age-15m'); check('finite lower bound','window 15m' in s and 'PAUSED' in s)
    for _ in range(6): a.key('+')
    s=a.capture('age-available'); check('available endpoint','window available' in s and 'PAUSED' in s)
    check('strip independent of rain window',all('actor'+str(i) not in s.split('Interesting Now')[-1] for i in range(5,10)))
    a.key('2'); check('window does not filter Stream',all('actor'+str(i) in a.capture('age-stream-after') for i in range(10))); a.quit()
    mode='rate'; start=len(requests); a=App('rate-stop',ARGS+['--no-cache'],width=240)
    s=a.capture(); check('rate publishes eight unknown with repository activity','PATH ?8' in s and '8 activity' in s and '8 unknown' in s,s.splitlines()[:5])
    check('rate stops dispatch',sum('/commits/' in r['path'] for r in requests[start:])<=4,requests[start:])
    before=len(requests); time.sleep(61)
    check('120-second retry holds across 61s floor',len(requests)==before)
    check('rate response sanitized','fixture private raw body' not in a.capture('rate-after-floor')); a.quit()
finally:
    (OUT/'requests.json').write_text(json.dumps(requests,indent=2)); (OUT/'runtime.json').write_text(json.dumps(dict(root=str(ROOT))))
    server.shutdown(); server.server_close(); tm('kill-server')
