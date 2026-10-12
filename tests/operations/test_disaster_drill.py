import hashlib
import importlib.util
from pathlib import Path
import io
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('drill', Path(__file__).resolve().parents[2] / 'scripts/disaster-restore-drill.py')
drill = importlib.util.module_from_spec(spec)
spec.loader.exec_module(drill)


class DisasterGuards(unittest.TestCase):
    def test_checksum_tamper_and_required_components(self):
        with tempfile.TemporaryDirectory() as root:
            backup = Path(root)
            (backup / 'COMPLETE').touch()
            names = ('postgres.dump', 'runtime.env', 'deployment-lock.env', 'config.tar.gz', 'mmdb.tar.gz')
            records = []
            for name in names:
                (backup / name).write_bytes(b'fixture')
                records.append(hashlib.sha256(b'fixture').hexdigest() + '  ' + name)
            (backup / 'SHA256SUMS').write_text('\n'.join(records))
            drill.verify(backup)
            (backup / 'postgres.dump').write_bytes(b'tampered')
            with self.assertRaises(ValueError):
                drill.verify(backup)

    def test_archive_cannot_escape_target(self):
        with tempfile.TemporaryDirectory() as root:
            archive = Path(root) / 'unsafe.tar'
            with tarfile.open(archive, 'w') as t:
                entry = tarfile.TarInfo('../outside')
                entry.size = 1
                t.addfile(entry, io.BytesIO(b'x'))
            target = Path(root) / 'target'
            target.mkdir()
            with self.assertRaises(tarfile.FilterError):
                drill.extract(archive, target)
            self.assertFalse((Path(root) / 'outside').exists())
