#!/usr/bin/env python3
"""Delete only explicitly inventoried PostgreSQL dumps after 60 days."""
import argparse
import hashlib
import json
import pathlib
import time

MANIFEST = pathlib.Path('/opt/puntazo/manual-backups.json')
ALLOWED = (pathlib.Path('/root/puntazo-testing-cutover-20261001'),
           pathlib.Path('/opt/puntazo/releases/closed-20261003'))

def expired(manifest, now):
    for item in json.loads(manifest.read_text()):
        path = pathlib.Path(item['path'])
        if not path.is_absolute() or path.suffix not in ('.dump', '.dmp'):
            raise RuntimeError('Invalid reviewed backup path')
        if path.is_symlink() or path.resolve() != path or not any(root in path.parents for root in ALLOWED):
            raise RuntimeError('Reviewed backup escaped its original directory')
        if not path.exists():
            continue
        if item['created_at_epoch'] <= 0 or item['created_at_epoch'] > now:
            raise RuntimeError('Invalid audited creation date')
        if item['created_at_epoch'] >= now - 60 * 86400:
            continue
        with path.open('rb') as source:
            if source.read(5) != b'PGDMP':
                raise RuntimeError('Reviewed file is not a PostgreSQL dump')
        if hashlib.sha256(path.read_bytes()).hexdigest() != item['sha256']:
            raise RuntimeError('Backup changed since inventory; re-audit required')
        yield path

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    paths = list(expired(MANIFEST, time.time())) if MANIFEST.exists() else []
    print(json.dumps({'apply': args.apply, 'expired_reviewed_backups': [str(p) for p in paths]}))
    if args.apply:
        for path in paths:
            path.unlink()

if __name__ == '__main__':
    main()
