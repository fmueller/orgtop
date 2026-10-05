"""T-130 real binary, barrier-controlled 65/95-second HTTPS/PTY regression.

Build /tmp/orgtop-round2, then run from the repository root. No input, clock
injection or deadline tolerance; only synthetic credentials and disposable data.
"""
from pathlib import Path
exec(Path('docs/testing/round5/concurrent-retry.py').read_text().split('\ntry:\n')[0])
OUT = Path('/tmp/orgtop-t130-evidence'); OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t130'
responses = []
original_end_headers = Handler.end_headers


def timed_end_headers(self):
    for header in self._headers_buffer:
        if header.lower().startswith(b'retry-after:'):
            # Timestamp immediately before the buffered headers are emitted:
            # a server-side lower bound, not an invented client receipt time.
            responses.append(dict(path=self.path, retry=int(header.split(b':', 1)[1]), time=time.monotonic()))
    return original_end_headers(self)


Handler.end_headers = timed_end_headers


def respond(path):
    p = urlparse(path).path
    if p.endswith('/events'):
        return 200, [event(id=str(i), head=f'{i+2:040x}', ref='refs/heads/main') for i in range(2)], {}
    if '/commits/' in p:
        if phase == 0:
            barrier.wait(timeout=5)
            retry = 65 if p.endswith('2') else 95
            time.sleep(.1 if retry == 65 else .5)
            return 429, {'message': 'private concurrent fixture'}, {'Retry-After': str(retry)}
        return 200, dict(sha=p.rsplit('/', 1)[1], parents=[dict(sha=BEFORE)],
                         files=[dict(filename='src/a', status='modified')]), {}
    return 404, {}, {}


try:
    app = App('concurrent', ['--repo', 'acme/api', '--path', 'acme/api:src', '--no-cache'], width=240)
    initial = app.capture('initial')
    check('both requests in flight before either response', len(responses) == 2
          and max(r['time'] for r in requests if '/commits/' in r['path']) < min(r['time'] for r in responses), responses)
    check('current repository and two path unknowns', current(initial)
          and 'PATH ?2' in initial and 'RATE LIMITED' in initial and '2 activity' in initial)
    deadline = max(r['time'] + r['retry'] for r in responses)
    earliest = min(r['time'] for r in responses)
    count = len(requests)
    phase = 1
    time.sleep(max(0, earliest + 70 - time.monotonic()))
    check('no requests at 70s while later deadline applies', len(requests) == count, requests[count:])
    waiting = app.capture('at-70s')
    check('waiting retains current unknown membership', current(waiting) and 'PATH ?2' in waiting and 'RATE LIMITED' in waiting)
    time.sleep(max(0, deadline - 1 - time.monotonic()))
    check('no requests one second before latest deadline', len(requests) == count, requests[count:])
    time.sleep(max(0, deadline + 5 - time.monotonic()))
    recovered = app.capture('recovered')
    later = requests[count:]
    check('all automatic GETs respect latest response deadline', len(later) == 3
          and all(r['time'] >= deadline for r in later),
          [{'path': r['path'], 'offset_latest_deadline': r['time'] - deadline} for r in later])
    check('complete sanitized recovery', current(recovered) and recovered.count('2 activity') == 2
          and 'PATH ?' not in recovered and 'RATE LIMITED' not in recovered
          and 'private concurrent fixture' not in initial + waiting + recovered)
    app.quit()
finally:
    (OUT/'requests.json').write_text(json.dumps(requests, indent=2))
    (OUT/'responses.json').write_text(json.dumps(responses, indent=2))
    server.shutdown(); server.server_close(); tm('kill-server'); shutil.rmtree(ROOT)
raise SystemExit(0 if all(r['pass_'] for r in results) else 1)
