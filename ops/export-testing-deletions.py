#!/usr/bin/env python3
import json
import subprocess

names = subprocess.check_output([
    'docker', 'ps', '--filter', 'label=coolify.applicationId=26', '--format', '{{.Names}}'
], text=True).splitlines()
current = []
for name in names:
    info = json.loads(subprocess.check_output(['docker', 'inspect', name]))[0]
    env = dict(x.split('=', 1) for x in info['Config']['Env'] if '=' in x)
    if env.get('EXPECTED_SCHEMA_VERSION') == '0035' and info['State']['Running']:
        current.append(name)
if len(current) != 1:
    raise SystemExit('Expected exactly one migrated testing API; export and backup blocked')
subprocess.run([
    'docker', 'exec', '--user', '0', current[0], 'puntazo-deletion-journal',
    '-mode', 'export', '-environment', 'testing',
    '-file', '/var/lib/puntazo/deletions/testing.json'
], check=True, stdout=subprocess.DEVNULL)
print('Testing deletion journal exported')
