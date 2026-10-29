"""Prepare the pinned MyGo source patch without modifying the module cache.

Go forbids overlays below GOMODCACHE. A generated -modfile points only MyGo
at an ignored patched source copy; the checked-in go.mod/go.sum remain pinned.
"""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/egoist/mygo@v0.6.5-0.20261026090713-558e498142b0"
BASE_HASHES = {
    "menu.go": "4694b0f050a10f85512a8f55ecefeaf15252d47994b27fd69f1235c15bf83fc5",
    "menudraw.go": "aa49e1a0bbd0b763c69223b520251a27777d820de2334ae014dad62e1fcdd366",
    "menudraw_test.go": "aed92f41e8f97b0b17668e3643d829a068f75b447eb0514e5112e7a820014edf",
    "surface.go": "2888504461944780a2a425042d0bcdc5d8f1e8722386f8af382fa182433e857a",
    "webview_embed.go": "64fd60d9f67bf9f682ae397d7df660469c92ad53413a09b6abc3bc7807264784",
}


def mirror(source, output):
    """Copy the module's files over output, adding any that are missing.

    Only contents are copied: the module cache's read-only attributes would
    keep the copy from being updated. Nothing is deleted, so an update needs
    no removal of the previous copy.
    """
    for folder, _, files in os.walk(source):
        target = output / Path(folder).relative_to(source)
        target.mkdir(parents=True, exist_ok=True)
        for name in files:
            shutil.copyfile(Path(folder) / name, target / name)


def prepare():
    source = ROOT / ".local/gomodcache" / MODULE
    patch = ROOT / "third_party/patches/mygo/windows-native.patch"
    relative = ".local/mygo-menu-source"
    output = ROOT / relative
    for name, expected in BASE_HASHES.items():
        data = (source / "internal/windows" / name).read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f"MyGo source changed; review menu patch: {name}")
    stamp = output / ".onesub-patch-sha256"
    fingerprint = hashlib.sha256(patch.read_bytes()).hexdigest()
    if not stamp.exists() or stamp.read_text() != fingerprint:
        # The pristine module, then the current patch on top of it.
        mirror(source, output)
        subprocess.run(
            ["git", "apply", "--whitespace=nowarn", "--directory=" + relative, str(patch)],
            cwd=ROOT, check=True,
        )
        stamp.write_text(fingerprint, encoding="ascii")
    modfile = ROOT / ".local/onesub-patched.mod"
    modfile.write_bytes((ROOT / "apps/desktop/go.mod").read_bytes())
    modfile.with_suffix(".sum").write_bytes((ROOT / "apps/desktop/go.sum").read_bytes())
    subprocess.run(
        ["go", "mod", "edit", "-modfile=" + str(modfile),
         "-replace=github.com/egoist/mygo=" + str(output)], cwd=ROOT / "apps/desktop", check=True,
    )
    print(modfile)


if __name__ == "__main__":
    prepare()
