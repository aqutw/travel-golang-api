#!/usr/bin/env python3
"""Print a stable logical database snapshot, independent of SQLite file headers."""
import argparse
import json
from pathlib import Path
import sqlite3


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('path', nargs='?', type=Path, default=Path(__file__).resolve().parents[1] / 'data/travel.sqlite3')
    args = parser.parse_args()
    with sqlite3.connect(args.path.resolve().as_uri()+'?mode=ro', uri=True) as db:
        assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
        assert not db.execute('PRAGMA foreign_key_check').fetchall()
        schema = db.execute("SELECT type,name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name").fetchall()
        rows = {}
        for kind, name, _ in schema:
            if kind != 'table':
                continue
            columns = [row[1] for row in db.execute(f'PRAGMA table_info("{name}")')]
            order = ','.join('"'+column+'"' for column in columns)
            records = db.execute(f'SELECT * FROM "{name}" ORDER BY {order}').fetchall()
            rows[name] = [[{'hex': value.hex()} if isinstance(value, bytes) else value for value in row] for row in records]
        print(json.dumps({'schema': schema, 'rows': rows}, ensure_ascii=False, sort_keys=True, separators=(',', ':')))


if __name__ == '__main__':
    main()
