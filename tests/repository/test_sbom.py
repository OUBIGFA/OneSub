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


if __name__ == '__main__':
    unittest.main()
