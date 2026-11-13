param([switch]$Force)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$env:PYTHONUTF8 = '1'
$prepared = (& python (Join-Path $root 'tools/prepare_oneasr.py')) | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'OneAsr source preparation failed' }
$pin = Get-Content (Join-Path $root 'third_party/manifests/oneasr.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$runtimePath = Join-Path $root 'third_party/manifests/runtimes.json'
$runtime = Get-Content $runtimePath -Raw -Encoding UTF8 | ConvertFrom-Json
$destination = Join-Path $root 'build/output/oneasr/oneasr-cli.exe'
if (-not $Force -and $runtime.oneasr.integrationSha256 -eq $prepared.fingerprint -and (Test-Path -LiteralPath $destination)) {
    if ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant() -eq $runtime.oneasr.sha256) {
        Write-Output 'Reusing verified OneAsr region-batch runtime.'
        return
    }
}
if ($root.Contains(' ')) {
    if (-not ('OneAsrBuildPath' -as [type])) {
        Add-Type -TypeDefinition 'using System.Text; using System.Runtime.InteropServices; public static class OneAsrBuildPath { [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] public static extern uint GetShortPathName(string path, StringBuilder shortPath, uint length); }'
    }
    $shortPath = [Text.StringBuilder]::new(4096)
    if ([OneAsrBuildPath]::GetShortPathName($root, $shortPath, 4096) -eq 0 -or $shortPath.ToString().Contains(' ')) {
        throw 'The upstream resampler needs a workspace path without spaces or with NTFS short names enabled.'
    }
    $buildRoot = $shortPath.ToString()
} else { $buildRoot = $root }
$rust = Join-Path $buildRoot ('.local/toolchains/rust' + $pin.rust.version)
$compiler = Join-Path $rust 'bin/rustc.exe'
if (-not (Test-Path -LiteralPath $compiler)) { throw ('Provide the pinned local Rust toolchain: ' + $pin.rust.url) }
if ((& $compiler --version) -notmatch ('^rustc ' + [regex]::Escape($pin.rust.version) + ' ')) { throw 'Rust toolchain version differs from the source lock.' }
if (-not (Get-Command cl.exe -ErrorAction SilentlyContinue)) {
    $vcvars = Get-ChildItem -Path (Join-Path $env:ProgramFiles 'Microsoft Visual Studio/*/*/VC/Auxiliary/Build/vcvars64.bat') -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $vcvars) { throw 'Visual Studio x64 C++ build tools are required.' }
    $environment = & cmd.exe /c ('"' + $vcvars.FullName + '" >nul && set')
    if ($LASTEXITCODE -ne 0) { throw 'C++ build environment initialization failed.' }
    foreach ($entry in $environment) { if ($entry -match '^([^=]+)=(.*)$') { Set-Item -LiteralPath ('Env:' + $Matches[1]) -Value $Matches[2] } }
}
$env:PATH = (Join-Path $rust 'bin') + ';' + $env:PATH
$env:CARGO_HOME = Join-Path $rust 'cargo-home'
$env:CARGO_TARGET_DIR = Join-Path $rust 'oneasr-target'
$env:CARGO_NET_GIT_FETCH_WITH_CLI = 'true'
$env:GIT_TERMINAL_PROMPT = '0'
New-Item -ItemType Directory -Path $env:CARGO_HOME,$env:CARGO_TARGET_DIR -Force | Out-Null
Push-Location $prepared.directory
try {
    & cargo fmt -p oneasr-core -- --check
    if ($LASTEXITCODE -ne 0) { throw 'OneAsr format check failed.' }
    & cargo clippy --locked -p oneasr-core --all-targets -- -D warnings
    if ($LASTEXITCODE -ne 0) { throw 'OneAsr lint failed.' }
    & cargo test --locked -p oneasr-core
    if ($LASTEXITCODE -ne 0) { throw 'OneAsr regression tests failed.' }
    & cargo build --locked -p oneasr-core --release --bin oneasr-cli
    if ($LASTEXITCODE -ne 0) { throw 'OneAsr release build failed.' }
} finally { Pop-Location }
Copy-Item -LiteralPath (Join-Path $env:CARGO_TARGET_DIR 'release/oneasr-cli.exe') -Destination $destination -Force
& python -c "import hashlib,json,pathlib,sys; path=pathlib.Path(sys.argv[1]); binary=pathlib.Path(sys.argv[2]); data=json.loads(path.read_text(encoding='utf-8')); digest=hashlib.file_digest(binary.open('rb'),'sha256').hexdigest(); data['oneasr'].update(bytes=binary.stat().st_size,sha256=digest,integration='third_party/manifests/oneasr.json',integrationSha256=sys.argv[3]); path.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8'); print('Built OneAsr region-batch runtime: '+digest)" $runtimePath $destination $prepared.fingerprint
if ($LASTEXITCODE -ne 0) { throw 'OneAsr artifact registration failed.' }
