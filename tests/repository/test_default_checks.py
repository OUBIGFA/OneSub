"""Default build checks do not require a Windows desktop or an installed app."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


@unittest.skipUnless(os.name == 'nt', 'PowerShell build entry only')
class DefaultChecksTests(unittest.TestCase):
    def setUp(self):
        scratch = ROOT / '_temp/repository-tests'
        scratch.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(prefix='onesub-default-checks-', dir=scratch)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / 'build').mkdir()
        (self.root / 'tools').mkdir()
        (self.root / 'tests/repository').mkdir(parents=True)
        shutil.copyfile(ROOT / 'build/test.ps1', self.root / 'build/test.ps1')
        self.checker = self.root / 'tools/check_repo.py'
        self.checker.write_text("print('fixture repository check')\n", encoding='utf-8')
        self.test = self.root / 'tests/repository/test_fixture.py'
        self.test.write_text(
            "import unittest\nclass Fixture(unittest.TestCase):\n"
            "    def test_fixture(self):\n        self.assertTrue(True)\n", encoding='utf-8',
        )

    def run_checks(self):
        environment = os.environ.copy()
        environment['PYTHONUTF8'] = '1'
        environment.pop('MYGO_UPDATER_PRIVATE_KEY', None)
        return subprocess.run(
            ['powershell.exe', '-NoProfile', '-NonInteractive', '-File', str(self.root / 'build/test.ps1')],
            cwd=self.root, env=environment, capture_output=True, text=True,
            encoding='utf-8', errors='replace', timeout=30, creationflags=subprocess.CREATE_NO_WINDOW,
        )

    def test_default_checks_work_without_private_desktop_helpers_or_app_runtimes(self):
        result = self.run_checks()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('fixture repository check', result.stdout)
        self.assertIn('Ran 1 test', result.stderr)
        self.assertFalse((self.root / 'build/output').exists())

    def test_failed_repository_check_stops_the_default_entry(self):
        self.checker.write_text('import sys\nsys.exit(3)\n', encoding='utf-8')
        result = self.run_checks()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Repository check failed', result.stderr)
        self.assertNotIn('Ran 1 test', result.stderr)

    def test_failed_unit_check_is_not_reported_as_success(self):
        self.test.write_text(
            "import unittest\nclass Fixture(unittest.TestCase):\n"
            "    def test_fixture(self):\n        self.fail('fixture failure')\n", encoding='utf-8',
        )
        result = self.run_checks()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Repository tests failed', result.stderr)


if __name__ == '__main__':
    unittest.main()
