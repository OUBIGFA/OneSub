# Build and signed updates

`build-windows.ps1` keeps product version `0.0.1`, identifier
`io.github.oubigfa.onesub` and installer name `OneSub-0.0.1-Setup.exe` stable.
Use a numeric internal build number in `yyyyMMddHHmmss` format.

## Signing key

The update public key is in `apps/desktop/mygo.json`. Keep its matching secret
outside Git, either in `.local/update-keys/mygo-update.key` or in
`MYGO_UPDATER_PRIVATE_KEY`. Back it up securely. Do not regenerate or replace it
for an ordinary build: installed apps trust this key. Do not put it in the
application, an artifact, a log or a pull request.

MyGo signs update archives with Ed25519. This is separate from Windows
Authenticode: the current installer is not Authenticode-signed.

## Build and test

With the fixed runtimes prepared and their hashes verified:

```powershell
./build/build-windows.ps1
```

This runs `build/test.ps1`, builds the installer with MyGo and writes the fixed
local artifacts in `build/output/release/`:

- `OneSub-0.0.1-Setup.exe` and its `.sha256`;
- `onesub-0.0.1-windows-amd64.tar.gz` and its `.sha256`;
- `update-windows-amd64.json` and `build-info.json`.

`tools/finalize_update.py` preserves MyGo's archive and signature. It assigns
feed version `0.0.1.<build>` and an immutable `build-<build>` release URL.
MyGo compares the numeric fourth component. SemVer `+build` metadata would not
order builds. Product, installer and user-data identities do not change.
Full archives are used; product-version deltas are disabled for these builds.

`build/test-updates.ps1` is part of the full test entry. It uses a temporary key
and executable directory to test real MyGo signature verification, same-product
build ordering, refusal of invalid signatures and tampered archives, actual
file replacement and byte progress. It does not modify an installed OneSub.

## Local OneAsr runtime

The installer uses the pinned OneAsr v1.3.0 region-batch port, not a new inference engine.
`third_party/manifests/oneasr.json` records the source archive, patch, Cargo inputs and Rust 1.96.0 distribution checksums.
Keep the verified archives in `.local/downloads/` and the merged Rust compiler, Cargo, standard library, rustfmt and Clippy components in `.local/toolchains/rust1.96.0/`.
Visual Studio x64 C++ tools are required. A workspace containing spaces needs NTFS short names for the upstream resampler's CMake build.
Run `build/build-oneasr.ps1` to test and build this runtime; the Windows build calls it and reuses an exact verified match.
The port and its original-clock behavior are documented in `third_party/patches/oneasr/README.md`.

## Publish boundary

Building does not create a tag, upload files or publish a Release. Public
updates need a separately authorized, non-prerelease GitHub Release tagged
`build-<build>`. Publish the matching signed archive and manifest together,
along with the installer and checksums. Make that release the latest only
when its artifacts and release gates have passed.

The app checks the latest release's `update-windows-amd64.json`. A missing
release or manifest is shown as unavailable, not as evidence of being current.
Automatic checking and installation default on. Installation waits for an
active workbench task to finish, uses its cancellation/exit guards and never
forces a restart. The new build runs at the next launch.
