"""T-124 built binary HTTPS/PTY/cache regression using round3's fixtures.

Build /tmp/orgtop-round2, then run with mise exec -- python this-file.py.
All credentials, transport responses and cache directories are disposable.
"""
from pathlib import Path
exec(Path('docs/testing/round3/harness.py').read_text().split('\ntry:\n')[0])
OUT = Path('/tmp/orgtop-t124-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t124'
round3_respond = respond


def respond(path):
    status, body, headers = round3_respond(path)
    if '/commits/' in urlparse(path).path and mode in ['link-extra-quote', 'link-no-separator', 'link-list-no-separator', 'link-no-equals']:
        target = 'https://api.github.com' + urlparse(path).path + '?per_page=100&page=2'
        headers['Link'] = {
            'link-extra-quote': f'<{target}>; rel="next""',
            'link-no-separator': f'<{target}> rel="next"',
            'link-list-no-separator': f'<{target}> rel="prev next"',
            'link-no-equals': f'<{target}> rel "next"',
        }[mode]
    return status, body, headers


try:
    for scenario in ['link-malformed', 'link-reverse', 'link-duplicate-query',
                     'link-cycle', 'link-extra-quote', 'link-no-separator',
                     'link-list-no-separator', 'link-no-equals', 'paged-good', 'empty', 'paged-missing']:
        mode = scenario
        invalid = scenario not in ['paged-good', 'empty']
        one(scenario + '-no-cache', unknown=invalid, complete_member=scenario == 'paged-good')
        cache = ROOT / scenario
        for phase in ['cold', 'repeat', 'corrected', 'reused']:
            if phase in ['corrected', 'reused']:
                mode = 'normal' if invalid else scenario
            start = len(requests)
            a = App(scenario + '-' + phase, ARGS, cache, width=220)
            text = a.capture()
            unknown = invalid and phase in ['cold', 'repeat']
            member = not unknown and scenario != 'empty'
            check(scenario + ' ' + phase + ' current repository',
                  current(text) and '1 activity' in text)
            check(scenario + ' ' + phase + ' path evidence',
                  ('PATH ?1' in text and '1 unknown' in text) if unknown
                  else 'PATH ?' not in text)
            check(scenario + ' ' + phase + ' membership',
                  text.count('1 activity') == (2 if member else 1))
            lookups = [r['path'] for r in requests[start:] if '/commits/' in r['path']]
            expected = 0 if phase == 'reused' or (not invalid and phase != 'cold') else (
                2 if phase in ['cold', 'repeat'] and scenario in ['link-reverse', 'link-cycle', 'paged-missing', 'paged-good'] else 1)
            check(scenario + ' ' + phase + ' dispatched pages', len(lookups) == expected, lookups)
            a.quit()
            with sqlite3.connect(cache / 'orgtop' / 'enrichment-v1.db') as sql:
                rows = sql.execute('SELECT path_count,complete FROM evidence').fetchall()
            expected_paths = 2 if scenario == 'paged-good' else 0 if scenario == 'empty' else 1
            check(scenario + ' ' + phase + ' persistence',
                  not rows if unknown else rows == [(expected_paths, 1)], rows)
finally:
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown()
    server.server_close()
    tm('kill-server')
    shutil.rmtree(ROOT)
if not all(r['pass_'] for r in results):
    sys.exit(1)
