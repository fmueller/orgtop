"""Real-time composed Retry-After/reset-header recovery, using the round4 fixture."""
from pathlib import Path
exec(Path('docs/testing/round4/harness.py').read_text().split('\ntry:\n')[0])
OUT = Path('/tmp/orgtop-round4-headers-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-round4-headers'
reset_at = 0


def respond(path):
    global reset_at
    p = urlparse(path).path
    if p.endswith('/events'):
        return 200, [event(ref='refs/heads/main')], {}
    if '/commits/' in p:
        if phase == 0:
            reset_at = int(time.time()) + 75
            return 403, {'message': 'private header fixture'}, {
                'Retry-After': '65', 'X-RateLimit-Remaining': '0',
                'X-RateLimit-Reset': str(reset_at)}
        return 200, dict(sha=HEAD, parents=[dict(sha=BEFORE)],
                         files=[dict(filename='src/a.go', status='modified')]), {}
    return 404, {}, {}


try:
    app = App('headers', ARGS + ['--no-cache'], width=240)
    initial = app.capture('initial')
    check('403 composed headers current unknown', current(initial) and 'PATH ?1' in initial and 'RATE LIMITED' in initial)
    initial_count = len(requests)
    settled = requests[-1]['time']
    reset_monotonic = time.monotonic() + reset_at - time.time()
    time.sleep(max(0, settled + 66 - time.monotonic()))
    check('later reset wins over 65s Retry-After', len(requests) == initial_count)
    phase = 1
    time.sleep(max(0, settled + 81 - time.monotonic()))
    recovered = app.capture('recovered')
    later = requests[initial_count:]
    check('retry at reset not earlier', len(later) == 2 and all(r['time'] >= reset_monotonic for r in later),
          {'reset_monotonic': reset_monotonic, 'requests': later})
    check('header recovery membership', current(recovered) and recovered.count('1 activity') == 2
          and 'PATH ?' not in recovered and 'RATE LIMITED' not in recovered)
    check('header private body absent', 'private header fixture' not in initial + recovered)
    app.quit()
finally:
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown()
    server.server_close()
    tm('kill-server')
    shutil.rmtree(ROOT)
sys.exit(0 if all(r['pass_'] for r in results) else 1)
