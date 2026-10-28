param([string]$BuildNumber = (Get-Date -Format 'yyyyMMddHHmmss'), [switch]$SkipTests)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$env:GOROOT = Join-Path $root '.local/toolchains/go1.27.3/go'
$env:PATH = "$env:GOROOT/bin;$env:PATH"
$env:GOMODCACHE = Join-Path $root '.local/gomodcache'
$env:GOCACHE = Join-Path $root '.local/gocache'
$env:MYGO_LDFLAGS = "-X main.BuildNumber=$BuildNumber"
Push-Location $root
try {
    & (Join-Path $PSScriptRoot 'build-player.ps1')
    & (Join-Path $PSScriptRoot 'build-subtitle.ps1')
    & (Join-Path $PSScriptRoot 'build-skill-tools.ps1')
    Push-Location workers/style-core
    try {
        & bun install --frozen-lockfile --ignore-scripts
        if ($LASTEXITCODE -ne 0) { throw 'Style dependencies failed' }
        & bun run build
        if ($LASTEXITCODE -ne 0) { throw 'Style worker build failed' }
    } finally { Pop-Location }
    $runtime = [IO.File]::ReadAllText((Join-Path $root 'third_party/manifests/runtimes.json')) | ConvertFrom-Json
    $media = Join-Path $root 'build/output/media-tools'
    New-Item -ItemType Directory $media -Force | Out-Null
    foreach ($name in @('ffmpeg','ffprobe')) {
        $source = (Get-Command $name -ErrorAction Stop).Source
        $version = (& $source -version 2>$null | Select-Object -First 3) -join "`n"
        if ($version -notmatch '8.1-full_build-www.gyan.dev' -or $version -match 'enable-nonfree') { throw "Unapproved media build: $name" }
        $expected = $runtime.media | Where-Object { $_.file -eq "$name.exe" }
        if (-not $expected -or (Get-FileHash $source -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected.sha256) { throw "Media hash differs from the recorded build: $name" }
        Copy-Item $source (Join-Path $media "$name.exe") -Force
    }
    if (-not (Test-Path 'build/output/oneasr/oneasr-cli.exe')) { throw 'Verified OneAsr v1.3.0 portable payload is required' }
    if ((Get-FileHash 'build/output/oneasr/oneasr-cli.exe' -Algorithm SHA256).Hash.ToLowerInvariant() -ne $runtime.oneasr.sha256) { throw 'OneAsr hash differs from the recorded build' }
    $sourceIndex = Join-Path $root '.local/source-tree.index'
    if (Test-Path $sourceIndex) { Remove-Item $sourceIndex -Force }
    Copy-Item (Join-Path $root '.git/index') $sourceIndex -Force
    $previousIndex = $env:GIT_INDEX_FILE
    $env:GIT_INDEX_FILE = $sourceIndex
    try {
        # The verification note records this installer's hash. Exclude it from
        # provenance so updating that record cannot change the installer again.
        & git update-index --force-remove -- 'docs/verification/windows-0.0.1.md'
        if ($LASTEXITCODE -ne 0) { throw 'Could not prepare the source provenance index' }
        $sourceTree = (& git write-tree).Trim()
    } finally {
        if ($null -eq $previousIndex) { Remove-Item Env:GIT_INDEX_FILE -ErrorAction SilentlyContinue } else { $env:GIT_INDEX_FILE = $previousIndex }
        Remove-Item $sourceIndex -Force -ErrorAction SilentlyContinue
    }
    $info = [ordered]@{version='0.0.1';build=$BuildNumber;channel='local-test';signed=$false;p0Accepted=$false;sourceTree=$sourceTree;sourceTreeExcludes=@('docs/verification/windows-0.0.1.md');worktree='staged source snapshot'}
    [IO.File]::WriteAllText((Join-Path $root 'build/output/build-info.json'),($info|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    if (-not $SkipTests) { & (Join-Path $PSScriptRoot 'test.ps1') }
    $package = Join-Path $root "build/output/packages/$BuildNumber"
    if (Test-Path $package) { throw 'Use a new internal build number; package directories are immutable' }
    Push-Location apps/desktop
    try {
        & go run github.com/egoist/mygo/cmd/mygo build -platform windows/amd64 -o $package
        if ($LASTEXITCODE -ne 0) { throw 'MyGo Windows build failed' }
    } finally { Pop-Location }
    $out=Join-Path $root 'build/output/release'
    New-Item -ItemType Directory $out -Force | Out-Null
    $setup=Join-Path $out 'OneSub-0.0.1-Setup.exe'
    # MyGo names the installer <slug>-setup-<version>-<arch>.exe.
    Copy-Item (Join-Path $package 'windows-amd64/onesub-setup-0.0.1-amd64.exe') $setup -Force
    $hash=(Get-FileHash $setup -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText(($setup+'.sha256'),"$hash  OneSub-0.0.1-Setup.exe`n",[Text.UTF8Encoding]::new($false))
    Copy-Item 'build/output/build-info.json' $out -Force
    Write-Output "Build $BuildNumber : $setup"
    Write-Output "SHA-256: $hash"
} finally { Pop-Location }
