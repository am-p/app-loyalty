import importlib.util
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('retention', pathlib.Path(__file__).with_name('retain-backups.py'))
retention = importlib.util.module_from_spec(spec)
spec.loader.exec_module(retention)

class BackupRetentionTests(unittest.TestCase):
    def test_actual_database_names_and_creation_time(self):
        now = 1791064000
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            expected = []
            for database in retention.DATABASES:
                directory = root / database
                directory.mkdir()
                prefix = retention.DATABASE_NAMES[database]
                old = directory / f'pg-dump-{prefix}-{now - 61 * 86400}.dmp'
                old.write_bytes(b'old fixture')
                expected.append(old)
                (directory / f'pg-dump-{prefix}-{now - 59 * 86400}.dmp').write_bytes(b'new fixture')
                (directory / 'manual-unknown.dmp').write_bytes(b'unknown fixture')
            self.assertCountEqual(retention.candidates(root, now), expected)

    def test_symlink_is_not_a_backup_to_delete(self):
        now = 1791064000
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            directory = root / retention.DATABASES[0]
            directory.mkdir()
            outside = root / 'unrelated.txt'
            outside.write_text('preserve')
            link = directory / f'pg-dump-puntazo-{now - 61 * 86400}.dmp'
            link.symlink_to(outside)
            self.assertEqual(list(retention.candidates(root, now)), [])
            self.assertEqual(outside.read_text(), 'preserve')

if __name__ == '__main__':
    unittest.main()
