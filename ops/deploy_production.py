#!/usr/bin/env python3
"""Actions client for the VPS's restricted, durable production release worker."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time


def main():
    sha = os.environ['GITHUB_SHA']
    host = os.environ['PUNTAZO_PRODUCTION_SSH_HOST']
    key = os.environ['PUNTAZO_PRODUCTION_SSH_KEY']
    if not re.fullmatch(r'[0-9a-f]{40}', sha) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]*', host) or not key.strip():
        raise SystemExit('A full commit SHA, SSH host and restricted deployment key are required')
    os.umask(0o077)
    with tempfile.TemporaryDirectory() as directory:
        identity = Path(directory) / 'identity'
        identity.write_text(key.rstrip() + '\n')
        ssh = ['ssh', '-T', '-i', str(identity), '-o', 'IdentitiesOnly=yes', '-o', 'BatchMode=yes',
               '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=10',
               '-o', 'HostKeyAlias=puntazo-production', '-o', 'UserKnownHostsFile=' +
               str(Path(__file__).with_name('production_known_hosts')), 'root@' + host]

        def request(operation):
            result = subprocess.run([*ssh, operation + ' ' + sha], text=True, capture_output=True, timeout=30)
            if result.returncode:
                raise RuntimeError('Restricted release SSH request failed; check host/key and VPS status')
            value = json.loads(result.stdout)
            if value.get('sha') != sha:
                raise RuntimeError('Release status does not belong to the requested commit')
            return value

        print(json.dumps(request('deploy')), flush=True)
        previous = None
        deadline = time.monotonic() + 3300
        while time.monotonic() < deadline:
            value = request('status')
            phase = value.get('phase')
            if phase != previous or value.get('status') in {'success', 'failed'}:
                print(json.dumps(value), flush=True)
                previous = phase
            if value.get('status') == 'success':
                return
            if value.get('status') not in {'queued', 'running'}:
                raise SystemExit('Production release failed; inspect its private VPS record. No database rollback was attempted.')
            time.sleep(10)
        raise SystemExit('Release wait timed out. The durable VPS worker may still be running; query status before retrying.')


if __name__ == '__main__':
    main()
