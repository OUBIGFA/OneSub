"""Exercise the native test launcher without opening the user's desktop."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
HELPER = ROOT / "tests/e2e/private_desktop.ps1"


def literal(value):
    return "'" + str(value).replace("'", "''") + "'"


@unittest.skipUnless(os.name == "nt", "Windows private desktops only")
class PrivateDesktopTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="onesub-private-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.log = self.root / "output.log"

    def run_command(self, body, timeout=15):
        script = self.root / "child.ps1"
        script.write_text(body, encoding="utf-8-sig")
        command = "& " + literal(script)
        launcher = (
            "$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); . " + literal(HELPER)
            + "; $code=Invoke-OneSubPrivateDesktop -Command " + literal(command)
            + " -Log " + literal(self.log) + " -WorkingDirectory " + literal(self.root)
            + " -TimeoutSeconds " + str(timeout) + "; exit $code"
        )
        return subprocess.run(
            ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", launcher],
            capture_output=True, timeout=60, creationflags=subprocess.CREATE_NO_WINDOW,
        )

    def test_driver_is_on_a_new_non_input_desktop(self):
        result = self.run_command(
            ". " + literal(HELPER)
            + "; Write-Output ('private='+[PrivateSmoke]::IsPrivateDesktop());"
            + " Write-Output ([PrivateSmoke]::CurrentDesktopName()); Write-Output '隔离日志'"
        )
        self.assertEqual(result.returncode, 0, result.stderr.decode(errors="replace"))
        text = self.log.read_text(encoding="utf-8-sig")
        self.assertIn("private=True", text)
        self.assertIn("OneSub-test-", text)
        self.assertIn("隔离日志", text)

    def test_child_failure_is_not_reported_as_success(self):
        result = self.run_command("Write-Output 'before-failure'; exit 7")
        self.assertEqual(result.returncode, 7, result.stderr.decode(errors="replace"))
        self.assertIn("before-failure", self.log.read_text(encoding="utf-8-sig"))

    def test_timeout_terminates_the_test_process(self):
        pid_file = self.root / "child.pid"
        result = self.run_command(
            "[IO.File]::WriteAllText(" + literal(pid_file) + ", [string]$PID); Start-Sleep 60", timeout=3
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(pid_file.exists(), result.stderr.decode(errors="replace"))
        pid = int(pid_file.read_text())
        check = subprocess.run(
            ["powershell.exe", "-NoProfile", "-Command", f"if(Get-Process -Id {pid} -ErrorAction SilentlyContinue){{exit 1}}"],
            capture_output=True, timeout=10, creationflags=subprocess.CREATE_NO_WINDOW,
        )
        self.assertEqual(check.returncode, 0, "the timed-out test process survived")

    def test_mixed_error_streams_preserve_the_failure_evidence(self):
        result = self.run_command(
            "Write-Error 'fixture-typed-error' -ErrorAction Continue; "
            "[Console]::Error.WriteLine('fixture-native-error'); exit 7"
        )
        self.assertEqual(result.returncode, 7, result.stderr.decode(errors="replace"))
        text = self.log.read_text(encoding="utf-8-sig")
        self.assertIn("fixture-typed-error", text)
        self.assertIn("fixture-native-error", text)
        self.assertNotIn("ProcessStreamReader_CliXmlError", text)


if __name__ == "__main__":
    unittest.main()
