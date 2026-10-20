#!/usr/bin/env python3
"""Scan working-tree file names and bytes for supplied terms, excluding Git metadata."""
import argparse
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument('--forbid', action='append', required=True)
    args = parser.parse_args()
    terms = [term.encode('utf-8').lower() for term in args.forbid]
    if any(not term for term in terms):
        parser.error('forbidden terms must not be empty')
    failures = []
    scanned = 0
    for path in sorted(args.root.rglob('*')):
        relative = path.relative_to(args.root)
        if '.git' in relative.parts or path.is_symlink():
            continue
        name = relative.as_posix().encode('utf-8').lower()
        if any(term in name for term in terms):
            failures.append(str(relative))
        if path.is_file():
            scanned += 1
            content = path.read_bytes().lower()
            if any(term in content for term in terms):
                failures.append(str(relative))
    if failures:
        print('\n'.join(sorted(set(failures))))
        raise SystemExit(1)
    print(f'PASS: scanned {scanned} files, including ignored files and binaries; no matches.')


if __name__ == '__main__':
    main()
