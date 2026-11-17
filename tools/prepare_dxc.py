"""Verify the pinned Microsoft redistributables before staging them for OneAsr."""

import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def prepare(root=ROOT):
    pin = json.loads((root / 'third_party/manifests/dxc.json').read_text('utf-8'))
    archive = root / '.local/downloads' / pin['archive']['file']
    if not archive.is_file() or hashlib.sha256(archive.read_bytes()).hexdigest() != pin['archive']['sha256']:
        raise ValueError('Provide the verified DXC package: ' + pin['archive']['url'])
    with zipfile.ZipFile(archive) as package:
        for entry in pin['files']:
            data = package.read(entry['member'])
            if len(data) != entry['bytes'] or hashlib.sha256(data).hexdigest() != entry['sha256']:
                raise ValueError('DXC package member changed: ' + entry['member'])
            target = root / entry['target']
            if target.is_file() and target.read_bytes() == data:
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
    return len(pin['files'])


if __name__ == '__main__':
    print('Verified DXC runtime and license files:', prepare())
