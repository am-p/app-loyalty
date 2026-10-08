<?php
require '/var/www/html/vendor/autoload.php';
$app = require '/var/www/html/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
$backup = App\Models\ScheduledDatabaseBackup::findOrFail(3);
if ($backup->database_id !== 10) {
    throw new RuntimeException('Unexpected testing database');
}
$executions = Illuminate\Support\Facades\DB::table('scheduled_database_backup_executions');
$previousId = (int) $executions->where('scheduled_database_backup_id', $backup->id)->max('id');
dispatch_sync(new App\Jobs\DatabaseBackupJob($backup));
// The job catches its own exceptions. Inspect its persisted result before reporting success.
$execution = Illuminate\Support\Facades\DB::table('scheduled_database_backup_executions')
    ->where('scheduled_database_backup_id', $backup->id)->orderByDesc('id')->first();
if (!$execution || (int) $execution->id <= $previousId || $execution->status !== 'success') {
    throw new RuntimeException('Testing backup did not complete successfully');
}
echo "Testing backup completed successfully.\n";
