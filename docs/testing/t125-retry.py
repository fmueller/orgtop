"""Real-binary HTTPS/PTY Retry-After regression; no input during the wait.

Run: mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-t125 ./cmd/orgtop
     mise exec -- python docs/testing/t125-retry.py
Uses round2's isolated CONNECT transport, synthetic token and disposable HOME.
"""
from pathlib import Path

exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil

BIN = '/tmp/orgtop-t125'
OUT = Path('/tmp/orgtop-t125-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t125'
mode = 'rate'
rate_times = []


def respond(path):
    p = urlparse(path).path
    if p.endswith('/events'):
        return 200, [event(id=str(i), head=f'{i+2:040x}', ref='refs/heads/main') for i in range(8)], {}
    if '/commits/' in p:
        if phase == 0:
            rate_times.append(time.monotonic())
            return 429, {'message': 'fixture private raw body'}, {'Retry-After': '120'}
        return 200, dict(sha=p.rsplit('/', 1)[1], parents=[dict(sha=BEFORE)],
                         files=[dict(filename='src/main.go', status='modified')]), {}
    return 404, {}, {}


def wait_until(at):
    time.sleep(max(0, at - time.monotonic()))


try:
    app = App('retry', ['--repo', 'acme/api', '--path', 'acme/api:src', '--no-cache'], width=240)
    initial = app.capture('initial')
    check('current repository and quantitative path unknowns',
          current(initial) and '8 activity' in initial and '8 unknown' in initial
          and 'PATH ?8' in initial and 'RATE LIMITED' in initial)
    check('enrichment dispatch stopped', 1 <= len(rate_times) <= 4, len(rate_times))
    assert rate_times, 'fixture received no rate-limited request'
    settled = max(rate_times)
    initial_count = len(requests)
    wait_until(settled + 61)
    check('no source or enrichment at 61s', len(requests) == initial_count,
          {'elapsed': time.monotonic() - settled, 'requests': len(requests)})
    waiting = app.capture('at-61s')
    check('waiting snapshot preserved', current(waiting) and '8 activity' in waiting
          and '8 unknown' in waiting and 'PATH ?8' in waiting)
    wait_until(settled + 119)
    check('no source or enrichment at 119s', len(requests) == initial_count,
          {'elapsed': time.monotonic() - settled, 'requests': len(requests)})
    phase = 1  # Upstream recovers; no keyboard input is sent.
    wait_until(settled + 125)
    recovered = app.capture('recovered')
    later = requests[initial_count:]
    check('automatic retry no earlier than 120s', bool(later)
          and all(r['time'] >= settled + 120 for r in later),
          [{'path': r['path'], 'elapsed': r['time'] - settled} for r in later])
    check('source and all eight enrichment requests recover',
          sum(urlparse(r['path']).path.endswith('/events') for r in later) == 1
          and sum('/commits/' in r['path'] for r in later) == 8)
    check('membership recovers and rate limit clears', current(recovered)
          and recovered.count('8 activity') == 2 and 'unknown' not in recovered
          and 'RATE LIMITED' not in recovered and 'PATH ?' not in recovered)
    check('private response absent', 'fixture private raw body' not in initial + waiting + recovered)
    app.quit()  # First and only keypress, after the timed recovery.
finally:
    (OUT/'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown()
    server.server_close()
    tm('kill-server')
    shutil.rmtree(ROOT)

sys.exit(0 if all(r['pass_'] for r in results) else 1)
