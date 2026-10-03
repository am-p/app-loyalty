# Closed test rollout

Apply migration 0035 before deploying the API and the new Android binary.
Deploy backend and web together: existing accounts must confirm 18+ before domain APIs work.
The endpoint POST /v1/me/age-confirmation accepts {"confirmed":true}; GET /v1/me returns adult_confirmed.
Keep login, GET /me, export, logout and account deletion accessible before age confirmation.

Set LOG_DIRECTORY=/var/log/puntazo and mount a dedicated writable volume owned by the API UID.
The logger deletes daily files older than 60 days. Existing Docker json-file logs are
not managed by this writer: rotate/recreate the Puntazo containers at cutover and remove
their retired log files through Docker, after confirming the exact container identities.
Do not change retention for unrelated applications on the VPS.

Install ops/retain-backups.py in /opt/puntazo/ and run hourly with --apply after reviewing
its dry-run. Configure Coolify's backup retention to 60 days as well. Audit the earlier
/root/puntazo-* manual dumps separately: the script deliberately does not guess which
shared directories and keys can safely be removed.

## Restore protection

Export account_deletion_journal and profile_media_deletions to a root-only file outside
the database backup set before every backup and at least every minute. A database restore
must remain isolated from ingress, mail, push and billing until the current external
journal has been replayed. For every matching restored account, run the current account
deletion transaction; restore the profile deletion queue and process it before opening
ingress. Never use the journal contained in the restored backup as the sole source.
This is a required deployment gate; the journal table alone does not prevent resurrection.

The new Android binary must hide external checkout and reject checkout calls on Android.
Set EAS production environment API URL to https://api-testing.puntazo.pro; use the
production Google web OAuth client validated by scripts/validate-android-google.mjs.
Build a new AAB with autoIncrement, verify versionCode > 11, then replace the closed draft.
Do not publish documents or the app until runtime deletion, storage and restore checks pass.

## Applied to the Puntazo testing environment (2026-10-03)

- API Coolify application 26 (`qr5wvgsjhookhyjrjkms9s3w`) runs the verified backend commit `1c2fe6e5cd579b3e5d3cbd938b9e0911bedb72ae`, schema 0035. Web application 24 runs frontend commit `ca8e77689a0fc3b5d9df4a2d9f7679f7ae116c9d`. Both Git revisions are pinned in Coolify; the code is on the `closed-test-readiness` branches. Merging either PR does not automatically deploy a different revision.
- Testing has `MERCADO_PAGO_PROVIDER=disabled`. The production API was not migrated or replaced during this testing rollout.
- Bind `/var/log/puntazo-testing` (UID 100/GID 101) to `/var/log/puntazo`. Bind root-only `/var/lib/puntazo/deletions` to the same path in the API. The journal is deliberately outside PostgreSQL backup directories and API-user access.
- Install `export-testing-deletions.py`, `testing-backup-with-journal.sh` and `backup-testing-with-journal.php` in `/opt/puntazo`. Install `puntazo-testing-privacy.cron` as `/etc/cron.d/puntazo-testing-privacy` and the logrotate configuration as `/etc/logrotate.d/puntazo-privacy-ops`. Keep Python/shell files in LF format. Install the executable scripts with mode 755 and the PHP wrapper with mode 644 so Coolify's unprivileged PHP process can read it; none of these scripts contain credentials. Keep the journal, manual-backup inventory and operations log root-only.
- Coolify backup schedule 3 is disabled to avoid an independent scheduler bypassing the journal export. Its backup configuration remains present. The host cron invokes that same Coolify backup job synchronously at 03:13 in the VPS timezone, immediately after exporting the latest journal. Exports also run every minute. A failed export blocks that backup. PHP is copied into the current Coolify container on every run, so restarting Coolify does not lose the wrapper.
- Coolify schedules 2 (production) and 3 (testing) have local retention of 60 days and no count-based limit. The hourly retention script handles both `pg-dump-puntazo-<epoch>.dmp` and `pg-dump-puntazo_preview_testing-<epoch>.dmp` inside their corresponding, fixed directories.
- Four manual dumps were inventoried using filesystem birth timestamps and SHA-256 hashes in root-only `/opt/puntazo/manual-backups.json`. `retain-reviewed-backups.py` removes only matching, inventoried PGDMP files after 60 days; changed or relocated files require another audit. Install `puntazo-reviewed-backup-retention.cron` as `/etc/cron.d/puntazo-reviewed-backup-retention`. The inventory is operational state, not a repository file.
- A pre-migration dump and private Coolify configuration snapshot are retained under root-only `/opt/puntazo/releases/closed-20261003`. They are recovery material, not a journal source for restoring accounts.

### Evidence and recovery

Live HTTP checks passed: health/readiness, customer registration/login, mandatory age gate, persistent and idempotent age acceptance, profile image upload/read, account anonymization, immediate access revocation and physical image removal. These checks used disposable QA accounts only. A restored database with no network ingress, mail, push or billing reapplied the current external journal, deactivated the restored deleted QA account and passed a repeated replay. The isolated restore container and its anonymous volume were removed.

Before any future restore, stop public API ingress and workers, restore into an isolated environment and run `puntazo-deletion-journal -mode replay -environment testing -file /var/lib/puntazo/deletions/testing.json` first in preview and then with `-apply`. Drain photo deletions and verify inactivity before reconnecting ingress. Never substitute an older journal from the backup itself. To create a manual backup, export the current journal first.

Migration 0035 is forward-only. Keep the external journal even during recovery; a database rollback must not reactivate deleted accounts or discard writes made after cutover. If the new API fails, close testing ingress and fix forward; coordinate the frontend and Android clients before reopening it. Do not downgrade schema automatically.

Public release still requires the reviewer credentials, final legal documents, Data safety declaration and installation checks with Play's signature. The locally signed APK used for QA is not proof of Google login with Play signing.
