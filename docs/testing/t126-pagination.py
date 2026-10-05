"""T-126 real binary HTTPS/PTY selection checks; build /tmp/orgtop-round2 first.

Run from the repository root. Synthetic auth and disposable TLS/HOME only.
"""
from pathlib import Path
exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil

OUT = Path('/tmp/orgtop-t126-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t126'
suffixes = {
    'duplicate-page': '&page=999',
    'duplicate-size': '&per_page=1',
    'duplicate-type': '&type=public',
    'duplicate-sort': '&sort=updated',
    'duplicate-direction': '&direction=desc',
    'bad-escape': '&ignored=%zz',
    'semicolon': '&ignored=x;y',
    'extra-query': '&ignored=x',
}


def respond(path):
    p = urlparse(path).path
    page = int(parse_qs(urlparse(path).query).get('page', ['1'])[0])
    if p.startswith('/orgs/'):
        headers = {}
        if page == 1:
            target = ('https://api.github.com/orgs/acme/repos?type=all&sort=full_name'
                      '&direction=asc&per_page=100&page=2')
            headers['Link'] = f'<{target}{suffixes.get(mode, "")}>; rel="next"'
        return 200, [record('acme', page)], headers
    if p.endswith('/events'):
        repo = '/'.join(p.split('/')[2:4])
        return 200, [event(repo=repo, id=repo, ref='refs/heads/main')], {}
    return 404, {'message': 'unexpected fixture endpoint'}, {}


try:
    for mode in ['valid', *suffixes]:
        start = len(requests)
        app = App(mode, ['--org', 'acme', '--repo', 'exact/keep', '--no-cache'], width=240)
        text = app.capture()
        sent = requests[start:]
        polls = [r['path'] for r in sent if '/events' in r['path']]
        listings = [r['path'] for r in sent if '/orgs/' in r['path']]
        invalid = mode != 'valid'
        check(mode + ' atomic state', 'ERROR' in text and not current(text)
              if invalid else current(text), text.splitlines()[:6])
        check(mode + ' exact request sequence', len(sent) == 1 and len(listings) == 1 and not polls
              if invalid else len(listings) == 2 and polls == [
                  '/repos/exact/keep/events?per_page=100',
                  '/repos/acme/r001/events?per_page=100',
                  '/repos/acme/r002/events?per_page=100'], sent)
        app.quit()
    check('HTTP contract', all(r['api'] == '2026-03-10'
                              and r['accept'] == 'application/vnd.github+json'
                              and r['credential_match'] for r in requests))
finally:
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown()
    server.server_close()
    tm('kill-server')
    shutil.rmtree(ROOT)

sys.exit(0 if all(r['pass_'] for r in results) else 1)
