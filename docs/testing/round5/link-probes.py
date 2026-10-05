"""Final bounded binary HTTPS/PTY/SQLite probes; run from repository root."""
from pathlib import Path
exec(Path('docs/testing/round2/harness.py').read_text().split('\ntry:\n')[0])
import shutil

OUT = Path('/tmp/orgtop-final-evidence'); OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-final'
ARGS = ['--repo', 'acme/api', '--path', 'acme/api:other']
kind = 'commit'
variant = 'plain'

def respond(path):
    p = urlparse(path).path
    page = int(parse_qs(urlparse(path).query).get('page', ['1'])[0])
    if p.endswith('/events'):
        return 200, [event(repo='/'.join(p.split('/')[2:4]), ref='refs/heads/main')], {}
    headers = {}
    if p.startswith('/orgs/'):
        body = [record('acme', page)]
        target = 'https://api.github.com/orgs/acme/repos?type=all&sort=full_name&direction=asc&per_page=100&page=2'
    else:
        body = dict(sha=HEAD, parents=[dict(sha=BEFORE)], files=[dict(filename='src/first.go' if page == 1 else 'other/last.go', status='modified')])
        target = f'https://api.github.com{p}?per_page=100&page=2'
    if page == 1:
        relation = 'next'
        title = '; title="page, two; \\"quoted\\""'
        if variant == 'escaped-next': relation = r'ne\xt'
        if variant == 'escaped-list': relation = r'prev\ next'
        if variant == 'escaped-last': relation = r'la\st'
        if variant == 'cross-host': target = target.replace('api.github.com', 'example.invalid')
        if variant == 'userinfo': target = target.replace('api.github.com', 'intruder@api.github.com')
        if variant == 'fragment': target += '#other'
        if variant == 'encoded-path': target = target.replace('/acme/', '/%61cme/')
        if variant == 'encoded-duplicate': target += '&%70age=9'
        if variant == 'malformed-query': target += '&discard=%zz'
        if variant == 'cross-entity': target = target.replace('/acme/', '/other/')
        if variant == 'unclosed-title': title = '; title="unclosed'
        headers['Link'] = f'<{target}>; rel="{relation}"{title}'
    return 200, body, headers

def rows(cache):
    with sqlite3.connect(cache/'orgtop'/'enrichment-v1.db') as db:
        return db.execute('SELECT complete,path_count FROM evidence').fetchall(), db.execute('SELECT path FROM evidence_path ORDER BY path').fetchall()

try:
    for kind in ['commit', 'org']:
        for variant in ['plain', 'escaped-next', 'escaped-list', 'escaped-last', 'cross-host', 'userinfo', 'fragment', 'encoded-path', 'encoded-duplicate', 'malformed-query', 'cross-entity', 'unclosed-title']:
            mode = kind + '-' + variant
            cache = ROOT / mode
            start = len(requests)
            args = ARGS if kind == 'commit' else ['--org', 'acme', '--repo', 'exact/keep', '--no-cache']
            app = App(mode, args, cache, width=240)
            text = app.capture()
            sent = requests[start:]
            valid = variant in ['plain', 'escaped-next', 'escaped-list', 'escaped-last']
            terminal = variant == 'escaped-last'
            if kind == 'commit':
                check(mode+' evidence', current(text) and ('PATH ?1' in text) == (not valid)
                      and text.count('1 activity') == (2 if valid and not terminal else 1), text.splitlines()[:6])
                check(mode+' GET count', sum('/commits/' in r['path'] for r in sent) == (2 if valid and not terminal else 1), sent)
                expected = ([(1, 1)], [('src/first.go',)]) if terminal else ([(1, 2)], [('other/last.go',), ('src/first.go',)])
                check(mode+' atomic cache', rows(cache) == (expected if valid else ([], [])), rows(cache))
            else:
                check(mode+' atomic selection', (current(text) and 'ERROR' not in text) if valid else ('ERROR' in text and len(sent) == 1), text.splitlines()[:6])
                listings = 1 if terminal or not valid else 2
                polls = (2 if terminal else 3) if valid else 0
                check(mode+' ordered GETs', len(sent) == listings+polls and all('/orgs/' in r['path'] for r in sent[:listings]) and all('/events' in r['path'] for r in sent[listings:]), sent)
            app.quit()
            if kind == 'commit' and variant in ['escaped-next', 'escaped-list']:
                oldvariant = variant
                variant = 'plain'
                before = count_commits()
                app = App(mode+'-corrected-warm', ARGS, cache, width=240)
                text = app.capture()
                check(mode+' corrected warm terminal membership', current(text) and text.count('1 activity') == 2 and 'PATH ?1' not in text, text.splitlines()[:6])
                check(mode+' corrected cache complete', rows(cache) == ([(1,2)], [('other/last.go',), ('src/first.go',)]), dict(rows=rows(cache), new_lookups=count_commits()-before))
                app.quit()
                variant = oldvariant
    check('all request headers', all(r['api']=='2026-03-10' and r['accept']=='application/vnd.github+json' and r['credential_match'] for r in requests))
finally:
    (OUT/'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown(); server.server_close(); tm('kill-server'); shutil.rmtree(ROOT)
raise SystemExit(0 if all(r['pass_'] for r in results) else 1)
