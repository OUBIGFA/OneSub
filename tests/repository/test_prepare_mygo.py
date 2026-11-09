"""A previously copied module must remain refreshable on Windows."""
import importlib.util
from pathlib import Path
import stat
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("prepare_mygo", ROOT / "tools/prepare_mygo.py")
PREPARE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREPARE)


class PrepareMygoTests(unittest.TestCase):
    def test_mirror_refreshes_readonly_copy_without_changing_source(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, output = root / "source", root / "output"
            source.mkdir()
            output.mkdir()
            original, copied = source / "widget.go", output / "widget.go"
            original.write_text("new version", encoding="utf-8")
            copied.write_text("previous version", encoding="utf-8")
            original.chmod(stat.S_IREAD)
            copied.chmod(stat.S_IREAD)
            try:
                PREPARE.mirror(source, output)
                self.assertEqual(copied.read_text(encoding="utf-8"), "new version")
                self.assertFalse(original.stat().st_mode & stat.S_IWRITE)
                self.assertTrue(copied.stat().st_mode & stat.S_IWRITE)
            finally:
                original.chmod(stat.S_IREAD | stat.S_IWRITE)
                copied.chmod(stat.S_IREAD | stat.S_IWRITE)


if __name__ == "__main__":
    unittest.main()
