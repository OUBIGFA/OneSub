import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess


ROOT = Path(__file__).resolve().parents[1]


def resolve_packages(packages, start):
    providers = {}
    for name, package in packages.items():
        for provided in package.get('PROVIDES', []):
            providers.setdefault(re.split('[<>=]', provided)[0], []).append(name)
    selected = {}
    pending = [start]
    while pending:
        dependency = re.split('[<>=]', pending.pop())[0]
        name = dependency
        if name not in packages:
            candidates = providers.get(name, [])
            if len(candidates) != 1:
                raise ValueError('Missing or ambiguous media dependency: ' + name)
            name = candidates[0]
        if name in selected:
            continue
        selected[name] = packages[name]
        pending.extend(packages[name].get('DEPENDS', []))
    return selected


def cached_archive(entry, cache):
    archive = cache / entry['archive']
    if not archive.is_file():
        downloader = os.environ.get('ARIA2_NEXT') or shutil.which('aria2-next')
        if not downloader:
            raise FileNotFoundError('Set ARIA2_NEXT to download the missing pinned media archive: ' + entry['archive'])
        cache.mkdir(parents=True, exist_ok=True)
        environment = os.environ.copy()
        environment.pop('MYGO_UPDATER_PRIVATE_KEY', None)
        subprocess.run([downloader, '-c', '-d', str(cache), '-o', entry['archive'],
                        '--auto-file-renaming=false', entry['url']], check=True, env=environment)
    if hashlib.sha256(archive.read_bytes()).hexdigest() != entry['sha256']:
        raise ValueError('Media archive checksum differs from its lock: ' + entry['archive'])
    return archive


def prepare_runtime(manifest, cache, output):
    verified = {}
    for package in manifest['packages']:
        archive = cached_archive(package, cache)
        for entry in package['files']:
            data = subprocess.check_output(['tar.exe', '-xOf', str(archive), entry['member']])
            if hashlib.sha256(data).hexdigest() != entry['sha256']:
                raise ValueError('Media member checksum differs from its lock: ' + entry['member'])
            verified[entry['path']] = data
    for entry in manifest.get('notices', []):
        data = (ROOT / entry['file']).read_bytes()
        if hashlib.sha256(data).hexdigest() != entry['sha256']:
            raise ValueError('Media source notice checksum differs from its lock: ' + entry['file'])
        verified[entry['path']] = data
    for name, data in verified.items():
        path = output / name
        path.parent.mkdir(parents=True, exist_ok=True)
        if not path.is_file() or path.read_bytes() != data:
            path.write_bytes(data)
    return len(verified)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--manifest', type=Path, default=ROOT / 'third_party/manifests/media-runtime.json')
    parser.add_argument('--cache', type=Path, default=ROOT / '.local/downloads/media-packages')
    parser.add_argument('--output', type=Path, default=ROOT / 'build/output/media-tools')
    arguments = parser.parse_args()
    manifest = json.loads(arguments.manifest.read_text(encoding='utf-8'))
    count = prepare_runtime(manifest, arguments.cache, arguments.output)
    print('Verified media binaries and notices: ' + str(count))


if __name__ == '__main__':
    main()
