"""Focused T-129 regression, not a new global adversarial round.

Build /tmp/orgtop-round2, then run from the repository root. Reuses the round5
HTTPS fixture with synthetic credentials and disposable PTYs/SQLite databases.
Evidence is written to /tmp/orgtop-t129-evidence.
"""
from pathlib import Path

exec(Path('docs/testing/round5/link-probes.py').read_text().split('\ntry:\n')[0])

OUT = Path('/tmp/orgtop-t129-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t129-' + str(os.getpid())
fixture_respond = respond
terminal_requested = threading.Event()
release_terminal = threading.Event()


def respond(path):
    if '/commits/' in path and parse_qs(urlparse(path).query).get('page') == ['2']:
        terminal_requested.set()
        if not release_terminal.wait(8):
            return 503, {}, {}
        # Exercise completion synchronization, not a fixed post-release sleep.
        time.sleep(.8)
    return fixture_respond(path)


def cache_rows(cache):
    db = cache / 'orgtop' / 'enrichment-v1.db'
    if not db.exists():
        return [], []
    return rows(cache)


try:
    for variant in ['plain', 'escaped-next', 'escaped-list', 'escaped-last', 'unclosed-title']:
        kind = 'commit'
        mode = kind + '-' + variant
        cache = ROOT / mode
        terminal_requested.clear()
        release_terminal.clear()
        before = count_commits()
        app = App(mode, ARGS, cache, width=240)
        paginated = variant in ['plain', 'escaped-next', 'escaped-list']
        valid = variant != 'unclosed-title'
        check(mode + ' terminal requested', terminal_requested.is_set() == paginated)
        if paginated:
            check(mode + ' no premature complete cache', cache_rows(cache) == ([], []), cache_rows(cache))
        expected = ([(1, 2)], [('other/last.go',), ('src/first.go',)]) if paginated else (
            ([(1, 1)], [('src/first.go',)]) if valid else ([], []))
        release_terminal.set()
        deadline = time.monotonic() + 5
        while True:
            text = app.capture()
            evidence_ready = (current(text) and ('PATH ?1' in text) == (not valid)
                              and text.count('1 activity') == (2 if paginated else 1))
            if cache_rows(cache) == expected and evidence_ready or time.monotonic() >= deadline:
                break
            time.sleep(.1)
        check(mode + ' exact complete cache', cache_rows(cache) == expected, cache_rows(cache))
        check(mode + ' evidence', evidence_ready, text.splitlines()[:6])
        check(mode + ' commit GETs', count_commits() - before == (2 if paginated else 1))
        app.quit()
        if paginated:
            before = count_commits()
            app = App(mode + '-corrected-warm', ARGS, cache, width=240)
            text = app.capture()
            check(mode + ' warm terminal membership', current(text) and 'PATH ?1' not in text
                  and text.count('1 activity') == 2, text.splitlines()[:6])
            check(mode + ' warm zero enrichment GETs', count_commits() == before)
            check(mode + ' warm exact cache', cache_rows(cache) == expected, cache_rows(cache))
            app.quit()

    for variant in ['plain', 'escaped-next', 'escaped-list', 'escaped-last', 'unclosed-title']:
        kind = 'org'
        mode = kind + '-' + variant
        before = len(requests)
        app = App(mode, ['--org', 'acme', '--repo', 'exact/keep', '--no-cache'], width=240)
        text = app.capture()
        sent = requests[before:]
        valid = variant != 'unclosed-title'
        listings = 1 if variant in ['escaped-last', 'unclosed-title'] else 2
        polls = (2 if listings == 1 else 3) if valid else 0
        check(mode + ' selection', (current(text) and 'ERROR' not in text) if valid
              else ('ERROR' in text), text.splitlines()[:6])
        expected_polls = []
        if valid:
            expected_polls = ['/repos/acme/r001/events?per_page=100', '/repos/exact/keep/events?per_page=100']
            if listings == 2:
                expected_polls.append('/repos/acme/r002/events?per_page=100')
        check(mode + ' listings before every event GET', len(sent) == listings + polls
              and all('/orgs/' in r['path'] for r in sent[:listings])
              and sorted(r['path'] for r in sent[listings:]) == sorted(expected_polls), sent)
        app.quit()
    check('request headers', all(r['api'] == '2026-03-10' and r['accept'] == 'application/vnd.github+json'
                                and r['credential_match'] for r in requests))
finally:
    release_terminal.set()
    server.shutdown()
    server.server_close()
    tm('kill-server')
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    shutil.rmtree(ROOT)

raise SystemExit(0 if all(r['pass_'] for r in results) else 1)
