"""T-127 binary HTTPS/PTY/cache checks using the round4 reproduction.

Build /tmp/orgtop-round2, then run from the repository root. All credentials,
HTTPS responses, PTYs and cache data are disposable; no public API is contacted.
Evidence is written to /tmp/orgtop-t127-evidence.
"""
from pathlib import Path

exec(Path('docs/testing/round4/harness.py').read_text().split('\ntry:\n')[0])

OUT = Path('/tmp/orgtop-t127-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t127-' + str(os.getpid())
quoted_respond = respond
terminal_requested = threading.Event()
release_terminal = threading.Event()


def respond(path):
    if '/commits/' in path and parse_qs(urlparse(path).query).get('page') == ['2'] and phase == 1:
        terminal_requested.set()
        if not release_terminal.wait(8):
            return 503, {}, {}
    code, body, headers = quoted_respond(path)
    if mode == 'org-normal' and 'Link' in headers:
        headers['Link'] = headers['Link'].replace('; rel=', '; title="page, two"; rel=')
    return code, body, headers


def cache_rows(cache):
    db = cache / 'orgtop' / 'enrichment-v1.db'
    if not db.exists():
        return [], []
    with sqlite3.connect(db) as connection:
        return (connection.execute('SELECT complete, path_count FROM evidence').fetchall(),
                connection.execute('SELECT path FROM evidence_path ORDER BY path').fetchall())


try:
    mode = 'link-title-comma'
    phase = 1
    cache = ROOT / 'quoted-pagination'
    app = App('cold', ARGS, cache, width=240)
    check('terminal page requested over HTTPS', terminal_requested.is_set())
    check('no partial paths cached before terminal response', cache_rows(cache) == ([], []))
    release_terminal.set()
    time.sleep(1)
    cold = app.capture()
    check('cold complete confirmed membership', current(cold) and 'PATH ?1' not in cold
          and cold.count('1 activity') == 2, cold.splitlines()[:6])
    check('cold two commit GETs', count_commits() == 2)
    check('both paths admitted only after terminal response', cache_rows(cache) ==
          ([(1, 2)], [('other/end.go',), ('src/main.go',)]), cache_rows(cache))
    app.quit()

    phase = 0
    before = count_commits()
    app = App('warm-terminal-path', ['--repo', 'acme/api', '--path', 'acme/api:other'], cache, width=240)
    warm = app.capture()
    check('warm terminal-page path confirmed', current(warm) and 'PATH ?1' not in warm
          and warm.count('1 activity') == 2, warm.splitlines()[:6])
    check('warm zero enrichment lookups', count_commits() == before)
    app.quit()

    mode = 'link-duplicate'
    malformed_cache = ROOT / 'ambiguous-pagination'
    before = count_commits()
    app = App('ambiguous', ARGS, malformed_cache, width=240)
    text = app.capture()
    check('ambiguous next stays unknown', current(text) and 'PATH ?1' in text)
    check('ambiguous next not followed or cached', count_commits() == before + 1
          and cache_rows(malformed_cache) == ([], []))
    app.quit()

    mode = 'org-normal'
    before = len(requests)
    app = App('organization', ['--org', 'acme', '--no-cache'], width=240)
    text = app.capture()
    sent = requests[before:]
    check('quoted organization listing current', current(text) and 'ERROR' not in text)
    check('organization follows both pages then polls both repositories',
          [request['path'] for request in sent[:2]] == [
              '/orgs/acme/repos?type=all&sort=full_name&direction=asc&per_page=100&page=1',
              '/orgs/acme/repos?type=all&sort=full_name&direction=asc&per_page=100&page=2']
          and sorted(request['path'] for request in sent[2:]) ==
          ['/repos/acme/r001/events?per_page=100', '/repos/acme/r002/events?per_page=100'], sent)
    app.quit()
finally:
    release_terminal.set()
    server.shutdown()
    tm('kill-server')
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    shutil.rmtree(ROOT)

raise SystemExit(0 if all(result['pass_'] for result in results) else 1)
