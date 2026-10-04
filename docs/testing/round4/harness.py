"""Round 4 adversarial binary HTTPS/PTY/cache cases; run from repository root.

Build /tmp/orgtop-round2 first. Uses only disposable data and synthetic auth.
Exit zero means assertions passed; fixtures never contact the public API.
"""
from pathlib import Path
exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil

OUT = Path('/tmp/orgtop-round4-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-round4'
ARGS = ['--repo', 'acme/api', '--path', 'acme/api:src']
original_respond = respond


def respond(path):
    p = urlparse(path).path
    query = parse_qs(urlparse(path).query)
    page = int(query.get('page', ['1'])[0])
    if p.startswith('/orgs/'):
        headers = {}
        if page == 1:
            target = ('https://api.github.com/orgs/acme/repos?type=all&sort=full_name'
                      '&direction=asc&per_page=100&page=2')
            suffix = {'org-duplicate-page': '&page=999',
                      'org-duplicate-size': '&per_page=1',
                      'org-bad-escape': '&ignored=%zz',
                      'org-semicolon': '&ignored=x;y',
                      'org-extra-query': '&ignored=x'}.get(mode, '')
            headers['Link'] = f'<{target}{suffix}>; rel="next"'
        return 200, [record('acme', page)], headers
    if p.endswith('/events'):
        e = event(repo='/'.join(p.split('/')[2:4]), ref='refs/heads/main')
        if mode == 'different-parent':
            e['payload']['before'] = '3' * 40
        headers = {'X-Poll-Interval': '130'} if mode == 'composed-rate' else {}
        return 200, [e], headers
    if '/commits/' in p:
        if mode == 'composed-rate' and phase == 0:
            return 429, {'message': 'private fixture body'}, {'Retry-After': '120'}
        body = dict(sha=HEAD, parents=[dict(sha=BEFORE)],
                    files=[dict(filename='src/main.go' if page == 1 else 'other/end.go', status='modified')])
        headers = {}
        if mode.startswith('link-') and page == 1:
            target = f'https://api.github.com{p}?per_page=100&page=2'
            headers['Link'] = {
                'link-normal': f'<{target}>; rel="next"',
                'link-rel-list': f'<{target}>; rel="prev next"',
                'link-title-comma': f'<{target}>; title="page, two"; rel="next"',
                'link-title-semicolon': f'<{target}>; title="page; two"; rel="next"',
                'link-other-first': f'<{target}>; rel="last", <{target}>; rel="next"',
                'link-duplicate': f'<{target}>; rel="next", <{target}>; rel="next"',
                'link-encoded-query': f'<https://api.github.com{p}?per_page=100&%70age=2>; rel="next"',
            }[mode]
        return 200, body, headers
    return original_respond(path)


try:
    for mode in ['org-normal', 'org-duplicate-page', 'org-duplicate-size',
                 'org-bad-escape', 'org-semicolon', 'org-extra-query']:
        start = len(requests)
        app = App(mode, ['--org', 'acme', '--repo', 'exact/keep', '--no-cache'], width=240)
        text = app.capture()
        sent = requests[start:]
        invalid = mode != 'org-normal'
        check(mode + ' atomic result', ('ERROR' in text and not any('/events' in r['path'] for r in sent))
              if invalid else current(text) and sum('/events' in r['path'] for r in sent) == 3, sent)
        app.quit()
    for mode in ['link-normal', 'link-rel-list', 'link-title-comma', 'link-title-semicolon',
                 'link-other-first', 'link-duplicate', 'link-encoded-query']:
        start = len(requests)
        app = App(mode, ARGS + ['--no-cache'], width=240)
        text = app.capture()
        invalid = mode == 'link-duplicate'
        check(mode + ' completeness', current(text) and ('PATH ?1' in text) == invalid,
              text.splitlines()[:6])
        check(mode + ' request count', sum('/commits/' in r['path'] for r in requests[start:]) == (1 if invalid else 2))
        app.quit()
    cache = ROOT / 'query-boundaries'
    for label, mode, path, expected, lookup in [
        ('seed', 'normal', 'src', 2, 1),
        ('sibling', 'normal', 'src2', 1, 0),
        ('case', 'normal', 'SRC', 1, 0),
        ('glob', 'normal', 'src/*.go', 2, 0),
        ('parent', 'different-parent', 'src', 1, 0),
        ('restore', 'normal', 'src', 2, 0),
    ]:
        start = count_commits()
        app = App('cache-' + label, ['--repo', 'acme/api', '--path', 'acme/api:' + path], cache, width=240)
        text = app.capture()
        check(label + ' query-specific membership', current(text) and text.count('1 activity') == expected
              and ('PATH ?1' in text) == (label == 'parent'), text.splitlines()[:6])
        check(label + ' immutable cache reuse', count_commits() - start == lookup)
        app.quit()
    mode = 'composed-rate'
    phase = 0
    start = len(requests)
    app = App('composed-rate', ARGS + ['--no-cache'], width=240)
    initial = app.capture('rate-initial')
    initial_count = len(requests)
    settled = max(r['time'] for r in requests[start:])
    check('composed rate initial current unknown', current(initial) and 'PATH ?1' in initial and 'RATE LIMITED' in initial)
    time.sleep(max(0, settled + 121 - time.monotonic()))
    check('source 130s floor outlasts 120s retry', len(requests) == initial_count)
    phase = 1
    time.sleep(max(0, settled + 136 - time.monotonic()))
    recovered = app.capture('rate-recovered')
    later = requests[initial_count:]
    check('composed automatic recovery requests', len(later) == 2 and all(r['time'] >= settled + 129.5 for r in later), later)
    check('composed recovery clears unknown and rate', current(recovered) and recovered.count('1 activity') == 2
          and 'PATH ?' not in recovered and 'RATE LIMITED' not in recovered)
    check('private response absent', 'private fixture body' not in initial + recovered)
    app.quit()
    check('HTTP contract headers', all(r['api'] == '2026-03-10' and r['accept'] == 'application/vnd.github+json'
                                     and r['credential_match'] for r in requests))
finally:
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown()
    server.server_close()
    tm('kill-server')
    shutil.rmtree(ROOT)

sys.exit(0 if all(r['pass_'] for r in results) else 1)
