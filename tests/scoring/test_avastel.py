import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('avastel', Path(__file__).resolve().parents[2] / 'scripts/compare-avastel.py')
avastel = importlib.util.module_from_spec(spec)
spec.loader.exec_module(avastel)


class AvastelTest(unittest.TestCase):
    def test_dedup_normalization_and_reject_bad_source(self):
        self.assertEqual(avastel.parse_ips(b'# comment\n8.8.8.8\n::ffff:8.8.8.8\n1.1.1.1\n'), ['1.1.1.1', '8.8.8.8'])
        self.assertEqual(avastel.parse_ips(b'ip_address;autonomous_system\n8.8.8.8;Google\n'), ['8.8.8.8'])
        for source in (b'127.0.0.1\n', b'# empty\n', b'<html>Error</html>'):
            with self.assertRaises(ValueError): avastel.parse_ips(source)

    def test_wilson_boundary_and_center(self):
        self.assertIsNone(avastel.wilson(0, 0))
        low, high = avastel.wilson(50, 100)
        self.assertAlmostEqual(low+high, 1)
        self.assertLess(low, 0.5)
        self.assertGreater(high, 0.5)
        self.assertGreater(avastel.wilson(100, 100)[0], 0.96)


if __name__ == '__main__': unittest.main()
