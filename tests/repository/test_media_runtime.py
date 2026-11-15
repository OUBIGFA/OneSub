import hashlib
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]


class MediaRuntimeTests(unittest.TestCase):
    def setUp(self):
        source = ROOT / 'tools/prepare_media.py'
        self.assertTrue(source.is_file(), 'Pinned media preparation is not implemented')
        specification = importlib.util.spec_from_file_location('prepare_media', source)
        self.media = importlib.util.module_from_spec(specification)
        specification.loader.exec_module(self.media)
        scratch = ROOT / '_temp/repository-tests'
        scratch.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=scratch)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def fixture(self):
        cache = self.root / 'cache'
        cache.mkdir()
        archive = cache / 'approved.tar'
        with tarfile.open(archive, 'w') as bundle:
            for name, data in {'ucrt64/bin/ffmpeg.exe': b'approved ffmpeg',
                               'ucrt64/bin/unrelated.exe': b'not shipped',
                               'ucrt64/share/licenses/ffmpeg/COPYING': b'approved license'}.items():
                member = tarfile.TarInfo(name)
                member.size = len(data)
                bundle.addfile(member, io.BytesIO(data))
        return cache, {'packages': [{'archive': 'approved.tar',
                                    'sha256': hashlib.sha256(archive.read_bytes()).hexdigest(),
                                    'files': [{'member': 'ucrt64/bin/ffmpeg.exe', 'path': 'ffmpeg.exe',
                                               'sha256': hashlib.sha256(b'approved ffmpeg').hexdigest()},
                                              {'member': 'ucrt64/share/licenses/ffmpeg/COPYING',
                                               'path': 'licenses/ffmpeg/COPYING',
                                               'sha256': hashlib.sha256(b'approved license').hexdigest()}]}]}

    def test_only_registered_binaries_and_notices_are_staged(self):
        cache, manifest = self.fixture()
        output = self.root / 'runtime'
        self.media.prepare_runtime(manifest, cache, output)
        self.assertEqual((output / 'ffmpeg.exe').read_bytes(), b'approved ffmpeg')
        self.assertEqual((output / 'licenses/ffmpeg/COPYING').read_bytes(), b'approved license')
        self.assertEqual({path.relative_to(output).as_posix() for path in output.rglob('*') if path.is_file()},
                         {'ffmpeg.exe', 'licenses/ffmpeg/COPYING'})

    def test_changed_archive_is_rejected_before_replacing_runtime(self):
        cache, manifest = self.fixture()
        (cache / 'approved.tar').write_bytes(b'changed download')
        output = self.root / 'runtime'
        output.mkdir()
        (output / 'ffmpeg.exe').write_bytes(b'existing runtime')
        with self.assertRaisesRegex(ValueError, 'archive'):
            self.media.prepare_runtime(manifest, cache, output)
        self.assertEqual((output / 'ffmpeg.exe').read_bytes(), b'existing runtime')

    def test_changed_member_is_rejected_before_replacing_runtime(self):
        cache, manifest = self.fixture()
        manifest['packages'][0]['files'][1]['sha256'] = '0' * 64
        output = self.root / 'runtime'
        output.mkdir()
        (output / 'ffmpeg.exe').write_bytes(b'existing runtime')
        with self.assertRaisesRegex(ValueError, 'member'):
            self.media.prepare_runtime(manifest, cache, output)
        self.assertEqual((output / 'ffmpeg.exe').read_bytes(), b'existing runtime')

    def test_verified_source_notices_are_shipped_with_runtime(self):
        cache, manifest = self.fixture()
        notice = self.root / 'third_party/licenses/source.txt'
        notice.parent.mkdir(parents=True)
        notice.write_bytes(b'copyright from matching source')
        manifest['notices'] = [{'file': 'third_party/licenses/source.txt',
                                'path': 'licenses/source/NOTICE',
                                'sha256': hashlib.sha256(notice.read_bytes()).hexdigest()}]
        output = self.root / 'runtime'
        with patch.object(self.media, 'ROOT', self.root):
            self.media.prepare_runtime(manifest, cache, output)
        self.assertTrue((output / 'licenses/source/NOTICE').is_file(), 'Matching-source notice is missing')
        self.assertEqual((output / 'licenses/source/NOTICE').read_bytes(), b'copyright from matching source')

    def test_changed_source_notice_does_not_replace_runtime(self):
        cache, manifest = self.fixture()
        notice = self.root / 'third_party/licenses/source.txt'
        notice.parent.mkdir(parents=True)
        notice.write_bytes(b'changed notice')
        manifest['notices'] = [{'file': 'third_party/licenses/source.txt',
                                'path': 'licenses/source/NOTICE', 'sha256': '0' * 64}]
        output = self.root / 'runtime'
        output.mkdir()
        (output / 'ffmpeg.exe').write_bytes(b'existing runtime')
        with self.assertRaisesRegex(ValueError, 'notice'):
            with patch.object(self.media, 'ROOT', self.root):
                self.media.prepare_runtime(manifest, cache, output)
        self.assertEqual((output / 'ffmpeg.exe').read_bytes(), b'existing runtime')

    def test_virtual_dependencies_resolve_to_their_recorded_provider(self):
        packages = {'ffmpeg': {'NAME': ['ffmpeg'], 'DEPENDS': ['sdl2', 'gomp>=1']},
                    'sdl2-compat': {'NAME': ['sdl2-compat'], 'PROVIDES': ['sdl2'], 'DEPENDS': ['gomp']},
                    'gcc-libs': {'NAME': ['gcc-libs'], 'PROVIDES': ['gomp=2']},
                    'not-used': {'NAME': ['not-used']}}
        self.assertEqual(set(self.media.resolve_packages(packages, 'ffmpeg')),
                         {'ffmpeg', 'sdl2-compat', 'gcc-libs'})

    def test_missing_dependency_stops_manifest_generation(self):
        with self.assertRaisesRegex(ValueError, 'missing-library'):
            self.media.resolve_packages({'ffmpeg': {'NAME': ['ffmpeg'], 'DEPENDS': ['missing-library']}}, 'ffmpeg')


if __name__ == '__main__':
    unittest.main()
