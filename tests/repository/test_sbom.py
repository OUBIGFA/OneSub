"""Release SBOMs describe packaged bytes without approving unverified gates."""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('build_sbom', ROOT / 'tools/build_sbom.py')
SBOM = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SBOM)


class SbomTests(unittest.TestCase):
    def setUp(self):
        scratch = ROOT / '_temp/repository-tests'
        scratch.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=scratch)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.package = self.root / 'package'
        self.package.mkdir()
        self.info = {'version': '0.1.0', 'build': '20261115000001', 'sourceTree': 'a' * 40, 'p0Accepted': False}
        (self.package / 'build-info.json').write_text(json.dumps(self.info), encoding='utf-8')
        (self.package / 'OneSub.exe').write_bytes(b'packaged-binary')
        self.bom = {'bomFormat': 'CycloneDX', 'specVersion': '1.7', 'version': 1, 'metadata': {'component': {'bom-ref': 'root', 'name': 'OneSub', 'type': 'application', 'version': '0.1.0'}}, 'components': [], 'dependencies': []}

    def test_actual_files_have_digests_and_release_identity(self):
        result = SBOM.augment(self.bom, self.package, self.info, [])
        executable = next(entry for entry in result['components'] if entry['name'] == 'OneSub.exe')
        self.assertEqual(executable['hashes'][0]['alg'], 'SHA-256')
        self.assertEqual(executable['hashes'][0]['content'], hashlib.sha256(b'packaged-binary').hexdigest())
        properties = {entry['name']: entry['value'] for entry in result['metadata']['properties']}
        self.assertEqual(properties['onesub:source-tree'], 'a' * 40)
        self.assertEqual(properties['onesub:build'], '20261115000001')
        self.assertEqual(properties['onesub:release-approved'], 'false')
        self.assertNotIn(str(self.root), json.dumps(result))

    def test_changed_package_receipt_is_rejected(self):
        changed = dict(self.info, build='20261115000002')
        with self.assertRaisesRegex(ValueError, 'receipt'):
            SBOM.augment(self.bom, self.package, changed, [])

    def test_only_production_javascript_dependency_closure_is_cataloged(self):
        project = self.root / 'javascript'
        project.mkdir()
        (project / 'package.json').write_text(json.dumps({'dependencies': {'runtime': '1.0.0'}, 'devDependencies': {'not-shipped': '2.0.0'}}), encoding='utf-8')
        for name, metadata in {'runtime': {'name': 'runtime', 'version': '1.0.0', 'license': 'MIT', 'dependencies': {'leaf': '1.1.0'}}, 'leaf': {'name': 'leaf', 'version': '1.1.0', 'license': 'ISC'}, 'not-shipped': {'name': 'not-shipped', 'version': '2.0.0'}}.items():
            directory = project / 'node_modules' / name
            directory.mkdir(parents=True)
            (directory / 'package.json').write_text(json.dumps(metadata), encoding='utf-8')
        components = SBOM.javascript_components(project)
        self.assertEqual({component['name'] for component in components}, {'runtime', 'leaf'})
        self.assertEqual({component['purl'] for component in components}, {'pkg:npm/runtime@1.0.0', 'pkg:npm/leaf@1.1.0'})
        self.assertTrue(all(component['properties'][0]['value'] == 'Locked production package declaration' for component in components))

    def test_supplemented_declared_dependencies_keep_provenance(self):
        dependencies = [{'bom-ref': 'pkg:npm/example@1.2.3', 'type': 'library', 'name': 'example', 'version': '1.2.3', 'purl': 'pkg:npm/example@1.2.3', 'licenses': [{'license': {'id': 'MIT'}}], 'properties': [{'name': 'onesub:evidence', 'value': 'locked production package declaration'}]}]
        result = SBOM.augment(self.bom, self.package, self.info, dependencies)
        component = next(entry for entry in result['components'] if entry['name'] == 'example')
        self.assertEqual(component['properties'][0]['value'], 'locked production package declaration')
        references = {entry['bom-ref'] for entry in result['components']} | {'root'}
        for edge in result['dependencies']:
            self.assertIn(edge['ref'], references)
            self.assertTrue(set(edge.get('dependsOn', [])) <= references)

    def media_fixture(self):
        directory = self.package / 'media-tools'
        directory.mkdir()
        (directory / 'ffmpeg.exe').write_bytes(b'locked media binary')
        return {'packages': [{'name': 'msys2-test', 'version': '1.2-3',
                              'url': 'https://mirror.msys2.org/package.tar.zst', 'sha256': 'a' * 64,
                              'licenses': ['spdx:MIT'],
                              'source': {'url': 'https://mirror.msys2.org/package.src.tar.zst', 'sha256': 'b' * 64},
                              'files': [{'path': 'ffmpeg.exe', 'sha256': hashlib.sha256(b'locked media binary').hexdigest()}]}]}

    def test_media_components_are_bound_to_packaged_files_and_sources(self):
        manifest = self.media_fixture()
        self.assertTrue(callable(getattr(SBOM, 'media_components', None)), 'Pinned media package evidence is missing')
        components = SBOM.media_components(self.package, manifest)
        self.assertEqual(components[0]['name'], 'msys2-test')
        self.assertEqual(components[0]['version'], '1.2-3')
        references = {entry['type']: entry['url'] for entry in components[0]['externalReferences']}
        self.assertEqual(references['other'], 'https://mirror.msys2.org/package.src.tar.zst')
        self.assertEqual(components[0]['hashes'][0]['content'], 'a' * 64)

    def test_changed_packaged_media_cannot_reuse_package_provenance(self):
        manifest = self.media_fixture()
        (self.package / 'media-tools/ffmpeg.exe').write_bytes(b'changed binary')
        self.assertTrue(callable(getattr(SBOM, 'media_components', None)), 'Pinned media package evidence is missing')
        with self.assertRaisesRegex(ValueError, 'Media'):
            SBOM.media_components(self.package, manifest)

    def material_fixture(self):
        directory = self.package / 'licenses'
        directory.mkdir()
        (directory / 'runtime.txt').write_bytes(b'original upstream notice')
        return {'scope': 'Fixed build-source material, not an exhaustive linked-symbol inventory',
                'notices': [{'path': 'licenses/runtime.txt', 'sha256': hashlib.sha256(b'original upstream notice').hexdigest()}],
                'components': [{'name': 'Bun', 'version': '1.3.9', 'license': 'MIT',
                                'repository': 'https://github.com/oven-sh/bun',
                                'source': {'url': 'https://example.org/bun-source.tar.gz', 'sha256': 'c' * 64}}]}

    def test_source_materials_keep_scope_source_digest_and_notice_evidence(self):
        manifest = self.material_fixture()
        self.assertTrue(callable(getattr(SBOM, 'material_components', None)), 'Source-material verification is missing')
        components = SBOM.material_components(self.package, manifest)
        self.assertEqual(components[0]['name'], 'Bun')
        self.assertEqual(components[0]['version'], '1.3.9')
        self.assertEqual(components[0]['hashes'][0]['content'], 'c' * 64)
        self.assertIn(manifest['scope'], {entry['value'] for entry in components[0]['properties']})
        self.assertIn('https://example.org/bun-source.tar.gz', {entry['url'] for entry in components[0]['externalReferences']})

    def test_changed_packaged_material_notice_is_rejected(self):
        manifest = self.material_fixture()
        (self.package / 'licenses/runtime.txt').write_bytes(b'changed notice')
        self.assertTrue(callable(getattr(SBOM, 'material_components', None)), 'Source-material verification is missing')
        with self.assertRaisesRegex(ValueError, 'material notice'):
            SBOM.material_components(self.package, manifest)

    def test_source_material_does_not_disappear_when_binary_identity_matches(self):
        manifest = self.material_fixture()
        identity = 'pkg:generic/Bun@1.3.9'
        self.bom['components'].append({'bom-ref': identity, 'type': 'library', 'name': 'Bun',
                                       'version': '1.3.9', 'purl': identity,
                                       'hashes': [{'alg': 'SHA-256', 'content': 'd' * 64}]})
        result = SBOM.augment(self.bom, self.package, self.info,
                              SBOM.material_components(self.package, manifest))
        records = [entry for entry in result['components'] if entry.get('purl') == identity]
        self.assertEqual(len(records), 2)
        self.assertEqual({entry['hashes'][0]['content'] for entry in records}, {'c' * 64, 'd' * 64})
        self.assertEqual(len({entry['bom-ref'] for entry in records}), 2)


if __name__ == '__main__':
    unittest.main()
