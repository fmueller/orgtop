"""Concurrent Retry-After composition through real binary HTTP and wall time."""
from pathlib import Path
exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil
OUT = Path('/tmp/orgtop-final-retry-evidence'); OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-final-retry'
barrier = threading.Barrier(2)
responses = []

def respond(path):
    p = urlparse(path).path
    if p.endswith('/events'):
        return 200, [event(id=str(i), head=f'{i+2:040x}', ref='refs/heads/main') for i in range(2)], {}
    if '/commits/' in p:
        if phase == 0:
            barrier.wait(timeout=5)
            delay = 65 if p.endswith('2') else 95
            time.sleep(.1 if delay == 65 else .5)
            responses.append(dict(path=p, retry=delay, time=time.monotonic()))
            return 429, {'message':'private concurrent fixture'}, {'Retry-After':str(delay)}
        return 200, dict(sha=p.rsplit('/',1)[1], parents=[dict(sha=BEFORE)], files=[dict(filename='src/a',status='modified')]), {}
    return 404, {}, {}

try:
    app = App('concurrent', ['--repo','acme/api','--path','acme/api:src','--no-cache'], width=240)
    initial = app.capture('initial')
    check('both in-flight 429s received', sorted(r['retry'] for r in responses)==[65,95], responses)
    check('initial current with two unknown', current(initial) and 'PATH ?2' in initial and 'RATE LIMITED' in initial, initial.splitlines()[:5])
    deadline = max(r['time']+r['retry'] for r in responses)
    earliest = min(r['time'] for r in responses)
    count = len(requests)
    phase = 1
    time.sleep(max(0, earliest+70-time.monotonic()))
    check('later 95s instruction prevents retry at 70s', len(requests)==count, [{'path':r['path'],'elapsed':r['time']-earliest} for r in requests[count:]])
    app.capture('at-70s')
    time.sleep(max(0, deadline+5-time.monotonic()))
    text = app.capture('recovered')
    later = requests[count:]
    check('all automatic recovery GETs respect latest deadline', bool(later) and all(r['time']>=deadline for r in later), [{'path':r['path'],'offset_latest_deadline':r['time']-deadline} for r in later])
    check('recovery complete and sanitized', current(text) and text.count('2 activity')==2 and 'PATH ?' not in text and 'RATE LIMITED' not in text and 'private concurrent fixture' not in initial+text)
    app.quit()
finally:
    (OUT/'requests.json').write_text(json.dumps(requests,indent=2))
    (OUT/'responses.json').write_text(json.dumps(responses,indent=2))
    server.shutdown(); server.server_close(); tm('kill-server'); shutil.rmtree(ROOT)
raise SystemExit(0 if all(r['pass_'] for r in results) else 1)
