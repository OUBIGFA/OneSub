# Third-party source and build material

Keep these source-material archives and SHA-256 files locally for distribution
compliance. They are not approved GitHub Release attachments:

- `OneSub-0.1.0-third-party-media-source.tar.gz`;
- `OneSub-0.1.0-third-party-runtime-source.tar.gz`.

Both use a `sources/` directory. The material manifest maps each archive name
to its original URL, fixed revision, size, SHA-256 and local cache destination.
It also records source-only exports and the original archive hashes. Source
exports exclude font files. The SkiaSharp export also excludes the specified
test/sample directories and prebuilt file extensions; these are not inputs
for the shipped library. No Windows installation image, user media, product
signing secret or unrelated local file is included.

Before public binary distribution, provide the corresponding source through
an approved compliance channel. Local retention alone is not a public source
offer. Do not claim the installer-only Release includes these archives.

## Restore fixed inputs

1. Verify both downloaded archives against their SHA-256 files.
2. Extract them and restore each `sources/<archive>` to the manifest's `file`
   path under the project checkout. Verify the individual archive hashes.
3. Use `third_party/manifests/` and the dependency lock files, not floating
   latest versions, when preparing a build.

The media archive contains pinned MSYS2 source packages. Each package retains
its PKGBUILD, upstream source and patches. Use the recorded UCRT64 package
versions and original MSYS2 recipes for FFmpeg and its library dependencies.
The runtime archive contains the pinned Go module, npm, Bun, JSCOnly WebKit,
JASSUB, subtitle library, .NET, Skia and Python source/build material.

JASSUB's fixed Dockerfile uses Emscripten 6.0.12. Restore its five library
submodules to the recorded revisions, then use its original build scripts
for both WASM variants. The application source embeds these resources; it
can be rebuilt with modified/relinked resources. Bun retains its original
CMake recipes and the corresponding JSCOnly source export. Hardcoded Bun
version strings are not treated as proof of actual binary linkage.

libse's 784 production source files match the fixed upstream archive. Its
license is MIT; UTF.Unknown 2.8.0 declares MPL-1.1. The original Python Windows
license, Microsoft distributable-code terms, .NET and SkiaSharp native notices,
and PyInstaller's GPL terms with bootloader exception are retained with the
application's license material. OneAsr uses the registered region-batch patch
and locked Cargo inputs; its source/patch definitions remain in the checkout.

## Build boundary

Use `build/build-windows.ps1 -ReleaseTag '0.1.2'` after preparing the recorded
toolchains and staging the reviewed source. The entry verifies the fixed Bun
executable/revision, uses the local Go toolchain and records the staged Git tree.
The product update signing secret is not part of source material; a local
rebuild must use its own key rather than the product's private key.

The Cargo source collection includes a conservative development/non-Windows
superset. Source material and SBOM declarations do not claim an exhaustive
linked-symbol inventory or byte-for-byte reproducibility of upstream binaries.
Windows desktop and clean-machine acceptance are not build or release gates.
