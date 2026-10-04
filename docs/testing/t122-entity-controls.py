"""Finite T-122 executable regression using round 2's HTTPS/PTY transport.

Usage: python docs/testing/t122-entity-controls.py /tmp/orgtop-t122-after after
Build the unmodified executable separately and use label 'before' for the red run.
Only synthetic credentials and loopback fixture traffic are used.
"""
from pathlib import Path

exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
BIN = sys.argv[1]
label = sys.argv[2]
SOCKET = 'orgtop-t122-' + label
OUT = Path('/tmp/orgtop-t122-' + label + '-evidence')
OUT.mkdir(exist_ok=True)
fixture = json.loads(Path('docs/testing/round2/hostile-entity.json').read_text())
fixture[0]['created_at'] = NOW
# Keep the branch ordinary: the actual hostile route is payload.head.
fixture[0]['payload']['ref'] = 'refs/heads/ordinary-branch'
def respond(path):
    if urlparse(path).path == '/repos/acme/api/events':
        return 200, fixture, {}
    return 404, dict(message='unexpected fixture endpoint'), {}

app = None
try:
    app = App('entity', ['--repo', 'acme/api', '--no-cache'], width=240, height=32)
    app.key('3')
    rain = app.capture('rain-240x32')
    check('wide entity escape', '\u202e' not in rain and 'commit bad\\u{202E}HEAD' in rain)
    for width, height in [(80, 24), (40, 12), (20, 6)]:
        app.resize(width, height)
        text = app.capture(f'rain-{width}x{height}')
        check(f'constrained {width}x{height} safe', '\u202e' not in text and all(len(line) <= width for line in text.splitlines()))
    app.resize(240, 32)
    app.key('2')
    app.key('Enter')
    detail = app.capture('detail-240x32')
    check('detail entity parity', '\u202e' not in detail and 'commit · bad\\u{202E}HEAD' in detail)
    check('bounded actual HTTPS request', len(requests) == 1 and requests[0]['path'] == '/repos/acme/api/events?per_page=100' and requests[0]['credential_match'])
    app.quit()
    app = None
finally:
    (OUT/'requests.json').write_text(json.dumps(requests, indent=2))
    if app is not None:
        tm('kill-session', '-t', app.name)
    server.shutdown()
    server.server_close()
    tm('kill-server')
    import shutil
    shutil.rmtree(ROOT)
sys.exit(0 if all(result['pass_'] for result in results) else 1)
