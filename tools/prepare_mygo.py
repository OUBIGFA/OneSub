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
    "internal/windows/menu.go": "4694b0f050a10f85512a8f55ecefeaf15252d47994b27fd69f1235c15bf83fc5",
    "internal/windows/menudraw.go": "aa49e1a0bbd0b763c69223b520251a27777d820de2334ae014dad62e1fcdd366",
    "internal/windows/menudraw_test.go": "aed92f41e8f97b0b17668e3643d829a068f75b447eb0514e5112e7a820014edf",
    "internal/windows/surface.go": "2888504461944780a2a425042d0bcdc5d8f1e8722386f8af382fa182433e857a",
    "internal/windows/webview_embed.go": "64fd60d9f67bf9f682ae397d7df660469c92ad53413a09b6abc3bc7807264784",
    "ui/editor.go": "ca355433603a60456e3734af661ada751f2ec1d09f01b85cd03483d8c7897ba9",
    "ui/editor_layout.go": "9f8a8f7754fb260b54591f28ea0189185fd38755e4e66d14890a8f72b0c81fa9",
    "ui/list.go": "29438b0610a27eac698e614d932103318bface2550c2c867d9c13a7579e53289",
    "ui/textinput.go": "2b7a1b59aa2b564140724614c37eb71b0b6f3f3fe3bfa2af5f326b6a7d572f1c",
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
    patches = [ROOT / "third_party/patches/mygo" / name for name in
               ("windows-native.patch", "ui-interaction.patch")]
    relative = ".local/mygo-menu-source"
    output = ROOT / relative
    for name, expected in BASE_HASHES.items():
        data = (source / name).read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f"MyGo source changed; review native patches: {name}")
    stamp = output / ".onesub-patch-sha256"
    fingerprint = hashlib.sha256(b"".join(p.read_bytes() for p in patches)).hexdigest()
    if not stamp.exists() or stamp.read_text() != fingerprint:
        # The pristine module, then the current patch on top of it.
        mirror(source, output)
        # Upstream files are reset by mirror. Reset this patch-added test too,
        # so a changed or interrupted patch can be applied again safely.
        (output / "ui/onesub_interaction_test.go").unlink(missing_ok=True)
        for patch in patches:
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
