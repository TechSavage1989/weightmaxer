"""Renamed downloads preserve the exact archive contract of installed updaters."""
import hashlib
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build  # noqa: E402


class PackagingTests(unittest.TestCase):
    def test_both_packages_have_one_executable_and_verified_checksums(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            exe = out / 'input.exe'
            exe.write_bytes(b'fixture executable shared by both packages')
            with patch.object(build, 'run') as run:
                build.package(exe, '1.3.0', out)
            sums = dict(line.split('  ')[::-1] for line in (out / 'SHA256SUMS.txt').read_text().splitlines())
            self.assertEqual(set(sums), {'WeightMaxxer-win-x64.zip', 'Weightmaxer-win-x64.zip'})
            for archive, name, verifier in [
                ('WeightMaxxer-win-x64.zip', 'WeightMaxxer.exe', 'verify-branded'),
                ('Weightmaxer-win-x64.zip', 'Weightmaxer.exe', 'verify'),
            ]:
                path = out / archive
                self.assertEqual(hashlib.sha256(path.read_bytes()).hexdigest(), sums[archive])
                with zipfile.ZipFile(path) as z:
                    self.assertEqual(set(z.namelist()), {name, 'README.txt', 'THIRD_PARTY_NOTICES.txt'})
                    self.assertEqual(len(z.namelist()), 3)
                    self.assertEqual(z.read(name), exe.read_bytes())
                    self.assertIn(b'WeightMaxxer', z.read('README.txt'))
                run.assert_any_call(['go', 'run', './cmd/wmxtool', verifier, str(path), '1.3.0'])


if __name__ == '__main__':
    unittest.main()
