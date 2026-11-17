"""DXC preparation must reject changed bytes and reuse a verified runtime."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('prepare_dxc', ROOT / 'tools/prepare_dxc.py')
DXC = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(DXC)


class DXCTests(unittest.TestCase):
    def test_preparation_checks_archive_members_and_reuses_complete_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / '.local/downloads/dxc.nupkg'
            archive.parent.mkdir(parents=True)
            with zipfile.ZipFile(archive, 'w') as package:
                package.writestr('bin/x64/dxcompiler.dll', b'verified compiler')
            pin = {'archive': {'file': archive.name, 'url': 'https://example.invalid/dxc', 'sha256': hashlib.sha256(archive.read_bytes()).hexdigest()},
                   'files': [{'member': 'bin/x64/dxcompiler.dll', 'target': 'build/output/oneasr/dxcompiler.dll', 'bytes': 17, 'sha256': hashlib.sha256(b'verified compiler').hexdigest()}]}
            manifest = root / 'third_party/manifests/dxc.json'
            manifest.parent.mkdir(parents=True)
            manifest.write_text(json.dumps(pin), encoding='utf-8')
            self.assertEqual(DXC.prepare(root), 1)
            target = root / pin['files'][0]['target']
            stamp = target.stat().st_mtime_ns
            DXC.prepare(root)
            self.assertEqual(target.stat().st_mtime_ns, stamp)
            target.write_bytes(b'changed')
            DXC.prepare(root)
            self.assertEqual(target.read_bytes(), b'verified compiler')
            pin['files'][0]['sha256'] = '0' * 64
            manifest.write_text(json.dumps(pin), encoding='utf-8')
            with self.assertRaisesRegex(ValueError, 'member changed'):
                DXC.prepare(root)
            archive.write_bytes(b'changed archive')
            with self.assertRaisesRegex(ValueError, 'verified DXC package'):
                DXC.prepare(root)
