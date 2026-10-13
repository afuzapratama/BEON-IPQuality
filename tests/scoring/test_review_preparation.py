import importlib.util
import tempfile
from pathlib import Path
import unittest


def module(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).resolve().parents[2] / 'scripts' / (name + '.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


class PreparationTest(unittest.TestCase):
    def test_reproducible_public_sample(self):
        prepare = module('prepare-label-review')
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / 'ips.txt'
            source.write_text(''.join('8.8.8.' + str(i) + '\n' for i in range(1, 200)))
            first = prepare.candidates(source, 20, 42)
            self.assertEqual(first, prepare.candidates(source, 20, 42))
            self.assertEqual(len(first), 20)
            source.write_text('127.0.0.1\n')
            with self.assertRaises(ValueError):
                prepare.candidates(source, 20, 42)
