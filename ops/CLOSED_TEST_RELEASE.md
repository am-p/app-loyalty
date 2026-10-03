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
