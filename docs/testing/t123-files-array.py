"""T-123 actual binary HTTPS/PTY and SQLite regression; no live GitHub access.

Build /tmp/orgtop-round2, then run with mise exec -- python this-file.py.
Reuses round3's fixture response vectors and round2's isolated transport/PTY.
"""
from pathlib import Path
exec(Path('docs/testing/round3/harness.py').read_text().split('\ntry:\n')[0])
OUT = Path('/tmp/orgtop-t123-evidence')
OUT.mkdir(exist_ok=True)
SOCKET = 'orgtop-t123'
try:
    for scenario in ['missing', 'null', 'compare-missing', 'compare-null',
                     'paged-missing', 'empty', 'compare-empty', 'paged-good']:
        mode = scenario
        malformed = scenario in ['missing', 'null', 'compare-missing',
                                 'compare-null', 'paged-missing']
        member = scenario == 'paged-good'
        cache = ROOT / scenario
        a = App(scenario + '-cold', ARGS, cache, width=220)
        text = a.capture()
        check(scenario + ' current repository', current(text) and '1 activity' in text)
        check(scenario + ' quantitative unknown',
              ('PATH ?1' in text and '1 unknown' in text) if malformed
              else 'PATH ?' not in text)
        check(scenario + ' no false membership',
              text.count('1 activity') == (2 if member else 1))
        a.quit()
        db = cache / 'orgtop' / 'enrichment-v1.db'
        with sqlite3.connect(db) as sql:
            rows = sql.execute('SELECT operation,path_count,complete FROM evidence').fetchall()
        check(scenario + ' persisted evidence', not rows if malformed
              else len(rows) == 1 and rows[0][1:] == (2 if member else 0, 1), rows)
        # Correct both endpoint kinds, preserving the immutable request identity.
        if malformed:
            if scenario.startswith('compare'):
                mode = 'compare-299'
            else:
                mode = 'normal'
        start = len(requests)
        a = App(scenario + '-warm', ARGS, cache, width=220)
        text = a.capture()
        lookups = sum('/commits/' in r['path'] or '/compare/' in r['path']
                      for r in requests[start:])
        check(scenario + ' warm lookup count', lookups == (1 if malformed else 0), lookups)
        check(scenario + ' warm correct membership', current(text) and 'PATH ?' not in text
              and text.count('1 activity') == (2 if malformed or member else 1))
        a.quit()
finally:
    (OUT / 'requests.json').write_text(json.dumps(requests, indent=2))
    server.shutdown()
    server.server_close()
    tm('kill-server')
    shutil.rmtree(ROOT)
if not all(r['pass_'] for r in results):
    sys.exit(1)
