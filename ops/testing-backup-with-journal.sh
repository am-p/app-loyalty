#!/bin/sh
set -eu
exec 9>/run/puntazo-testing-backup.lock
flock -n 9 || exit 0
/opt/puntazo/export-testing-deletions.py
docker cp /opt/puntazo/backup-testing-with-journal.php coolify:/tmp/backup-testing-with-journal.php
docker exec coolify php /tmp/backup-testing-with-journal.php
