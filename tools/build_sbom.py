"""Catalog the actual release and retain separately identified build declarations."""

import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import quote


ROOT = Path(__file__).resolve().parents[1]


def augment(bom, package, info, declared):
    """Attach packaged file hashes without turning discovery into release approval."""
    receipt = json.loads((package / 'build-info.json').read_text('utf-8'))
    if any(receipt.get(key) != info.get(key) for key in ('version', 'build', 'sourceTree')):
        raise ValueError('Package receipt differs from the requested release source')
    result = copy.deepcopy(bom)
    metadata = result['metadata']
    root_ref = metadata['component']['bom-ref']
    metadata['properties'] = [
        {'name': 'onesub:source-tree', 'value': info['sourceTree']},
        {'name': 'onesub:build', 'value': info['build']},
        {'name': 'onesub:release-approved', 'value': 'false'},
        {'name': 'onesub:coverage', 'value': 'Binary discovery plus explicitly identified production package declarations; static native library and clean-machine acceptance are not inferred.'},
    ]
    components = result.setdefault('components', [])
    dependencies = result.setdefault('dependencies', [])
    added_refs = []
    for path in sorted(package.rglob('*')):
        if path.is_symlink():
            raise ValueError('Release file inventory cannot contain a symbolic link')
        if not path.is_file():
            continue
        relative = path.relative_to(package).as_posix()
        reference = 'onesub:file:' + relative
        with path.open('rb') as stream:
            digest = hashlib.file_digest(stream, 'sha256').hexdigest()
        components.append({'bom-ref': reference, 'type': 'file', 'name': relative, 'hashes': [{'alg': 'SHA-256', 'content': digest}], 'properties': [{'name': 'onesub:evidence', 'value': 'Actual packaged file'}]})
        added_refs.append(reference)
    known = {component['bom-ref'] for component in components}
    for component in declared:
        if component['bom-ref'] not in known:
            components.append(copy.deepcopy(component))
            known.add(component['bom-ref'])
        added_refs.append(component['bom-ref'])
    root_edge = next((edge for edge in dependencies if edge['ref'] == root_ref), None)
    if root_edge is None:
        root_edge = {'ref': root_ref, 'dependsOn': []}
        dependencies.append(root_edge)
    root_edge['dependsOn'] = sorted(set(root_edge.get('dependsOn', []) + added_refs))
    result['compositions'] = [{'aggregate': 'incomplete', 'assemblies': [root_ref]}]
    return result


def javascript_components(project):
    """Describe only the installed closure of production dependency declarations."""
    metadata = json.loads((project / 'package.json').read_text('utf-8'))
    pending = list(metadata.get('dependencies', {}))
    seen = set()
    components = []
    while pending:
        name = pending.pop()
        if name in seen:
            continue
        seen.add(name)
        path = project / 'node_modules' / name / 'package.json'
        package = json.loads(path.read_text('utf-8'))
        version = package['version']
        purl = 'pkg:npm/' + quote(name, safe='/') + '@' + quote(version, safe='')
        component = {'bom-ref': purl, 'type': 'library', 'name': name, 'version': version, 'purl': purl, 'properties': [{'name': 'onesub:evidence', 'value': 'Locked production package declaration'}, {'name': 'onesub:package-metadata-sha256', 'value': hashlib.sha256(path.read_bytes()).hexdigest()}], 'externalReferences': [{'type': 'distribution', 'url': 'https://registry.npmjs.org/' + name + '/' + version}]}
        if package.get('license'):
            component['licenses'] = [{'expression': package['license']}]
        components.append(component)
        pending.extend(package.get('dependencies', {}))
    return components


def verified_declarations(package):
    """Verify that reviewed player metadata and license bytes match this build."""
    manifest = json.loads((ROOT / 'third_party/manifests/player-licenses.json').read_text('utf-8'))
    for entry in manifest['packages']:
        metadata = ROOT / 'apps/desktop/player/node_modules' / entry['name'] / 'package.json'
        if hashlib.sha256(metadata.read_bytes()).hexdigest() != entry['packageMetadataSha256']:
            raise ValueError('Player package metadata changed: ' + entry['name'])
        notice = package / 'licenses' / Path(entry['notice']).name
        if hashlib.sha256(notice.read_bytes()).hexdigest() != entry['noticeSha256']:
            raise ValueError('Packaged player notice changed: ' + entry['name'])
    binary = (package / 'OneSub.exe').read_bytes()
    for entry in manifest['jassubSource']['wasm']:
        asset = ROOT / 'apps/desktop/player/generated' / entry['file']
        if hashlib.sha256(asset.read_bytes()).hexdigest() != entry['sha256']:
            raise ValueError('JASSUB asset differs from its fixed source registration')
    for asset in (ROOT / 'apps/desktop/player/generated').iterdir():
        if asset.is_file() and asset.read_bytes() not in binary:
            raise ValueError('Player asset differs from the packaged binary: ' + asset.name)
    components = javascript_components(ROOT / 'apps/desktop/player')
    components.extend(javascript_components(ROOT / 'workers/style-core'))
    runtime = json.loads((ROOT / 'third_party/manifests/runtimes.json').read_text('utf-8'))
    for key in ('mygo', 'lobeMygo'):
        entry = runtime[key]
        name = entry.get('module', 'github.com/egoist/mygo')
        purl = 'pkg:golang/' + name + '@' + entry['version']
        components.append({'bom-ref': purl, 'type': 'library', 'name': name, 'version': entry['version'], 'purl': purl, 'licenses': [{'expression': entry['spdx']}], 'properties': [{'name': 'onesub:evidence', 'value': 'Pinned Go source; local build patch is registered separately'}]})
    for entry in manifest.get('wasmLibraries', []):
        purl = 'pkg:generic/' + entry['name'] + '@' + entry['revision']
        components.append({'bom-ref': purl, 'type': 'library', 'name': entry['name'], 'version': entry['revision'], 'purl': purl, 'licenses': [{'expression': entry['spdx']}], 'externalReferences': [{'type': 'vcs', 'url': entry['source']}], 'properties': [{'name': 'onesub:evidence', 'value': 'Pinned build submodule of the byte-verified JASSUB source snapshot'}]})
    return components


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--package', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--work', required=True, type=Path)
    args = parser.parse_args()
    package = args.package.resolve()
    info = json.loads((package / 'build-info.json').read_text('utf-8'))
    source_tree = subprocess.check_output(['git', 'write-tree'], cwd=ROOT).decode('ascii').strip()
    if source_tree != info['sourceTree']:
        raise ValueError('SBOM source declarations differ from the package source tree')
    tool = json.loads((ROOT / 'third_party/manifests/sbom-tool.json').read_text('utf-8'))
    executable = ROOT / '.local/toolchains' / ('syft' + tool['version']) / 'syft.exe'
    if hashlib.sha256(executable.read_bytes()).hexdigest() != tool['binarySha256']:
        raise ValueError('SBOM cataloger differs from the verified tool')
    args.work.mkdir(parents=True, exist_ok=True)
    raw = args.work / 'catalog.cdx.json'
    env = os.environ.copy()
    env.pop('MYGO_UPDATER_PRIVATE_KEY', None)
    env['SYFT_CHECK_FOR_APP_UPDATE'] = 'false'
    subprocess.run([str(executable), 'scan', 'dir:' + str(package), '--base-path', str(package), '--source-name', 'OneSub', '--source-version', info['version'], '-o', 'cyclonedx-json=' + str(raw)], check=True, env=env, creationflags=subprocess.CREATE_NO_WINDOW)
    bom = augment(json.loads(raw.read_text('utf-8')), package, info, verified_declarations(package))
    args.output.mkdir(parents=True, exist_ok=True)
    target = args.output / ('OneSub-' + info['version'] + '-sbom.cdx.json')
    target.write_text(json.dumps(bom, ensure_ascii=False, indent=2) + chr(10), encoding='utf-8')
    digest = hashlib.sha256(target.read_bytes()).hexdigest()
    target.with_suffix(target.suffix + '.sha256').write_text(digest + '  ' + target.name + chr(10), encoding='utf-8')
    print('SBOM: ' + str(target))
    print('Components: ' + str(len(bom['components'])))
    print('Coverage remains explicitly incomplete; SBOM generation does not approve the release.')


if __name__ == '__main__':
    main()
