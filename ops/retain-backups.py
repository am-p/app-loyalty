#!/usr/bin/env python3
"""Puntazo backup cleanup. Preview by default; --apply removes expired files."""
import argparse,datetime,json,pathlib,re,stat

ROOT=pathlib.Path('/data/coolify/backups/databases/puntazo-1')
DATABASES=('puntazo-db-production-cyteflibvyp3bigscehixwav','puntazo-db-testing-etgqnghwdsne7nqscht94r3m')
DATABASE_NAMES=dict(zip(DATABASES,('puntazo','puntazo_preview_testing')))

def candidates(root, now):
    cutoff=now-60*86400
    for database in DATABASES:
        directory=root/database
        if not directory.exists(): continue
        if directory.is_symlink() or directory.resolve().parent != root.resolve(): raise RuntimeError('Unsafe backup directory')
        for path in directory.rglob('*'):
            if path.is_symlink(): continue
            info=path.lstat()
            if not stat.S_ISREG(info.st_mode): continue
            if root.resolve() not in path.resolve().parents: raise RuntimeError('Backup escapes root')
            # Coolify uses a Unix creation timestamp, not the file's mtime.
            # Unknown/manual backup names need a separate operator audit.
            prefix=re.escape(DATABASE_NAMES[database])
            match=re.fullmatch(r'pg-dump-'+prefix+r'-(\d{10})\.dmp',path.name)
            if match and int(match.group(1)) < cutoff:
                yield path

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--apply',action='store_true');args=parser.parse_args()
    expired=list(candidates(ROOT,datetime.datetime.now(datetime.timezone.utc).timestamp()))
    print(json.dumps({'apply':args.apply,'expired_files':[str(p) for p in expired]}))
    if args.apply:
        for path in expired: path.unlink()
if __name__=='__main__':main()
