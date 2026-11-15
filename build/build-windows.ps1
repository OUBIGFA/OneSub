param([string]$BuildNumber = (Get-Date -Format 'yyyyMMddHHmmss'), [switch]$SkipTests, [string]$ReleaseTag)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if ($BuildNumber -notmatch '^[0-9]{14}$') { throw 'Build number must use yyyyMMddHHmmss' }
$config = Get-Content -LiteralPath (Join-Path $root 'apps/desktop/mygo.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$version = $config.version
if ($ReleaseTag -and $ReleaseTag -ne $version) { throw 'Release tag must equal the product version' }
& git -C $root diff --quiet
if ($LASTEXITCODE -ne 0) { throw 'Stage all release source changes before building' }
$untracked = @(& git -C $root ls-files --others --exclude-standard)
if ($LASTEXITCODE -ne 0 -or $untracked.Count -gt 0) { throw 'Stage all release source changes before building' }
$sourceTree = (& git -C $root write-tree).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot identify the staged release source' }
$updateKey = Join-Path $root '.local/update-keys/mygo-update.key'
if (-not $env:MYGO_UPDATER_PRIVATE_KEY -and -not (Test-Path $updateKey)) { throw 'The matching update signing key is required: MYGO_UPDATER_PRIVATE_KEY or .local/update-keys/mygo-update.key' }
$providedUpdateKey = $env:MYGO_UPDATER_PRIVATE_KEY
$env:MYGO_UPDATER_PRIVATE_KEY = $null
try {
$env:GOROOT = Join-Path $root '.local/toolchains/go1.27.4/go'
$env:GOTOOLCHAIN = 'local'
$env:PATH = "$env:GOROOT/bin;$env:PATH"
$env:GOMODCACHE = Join-Path $root '.local/gomodcache'
$env:GOCACHE = Join-Path $root '.local/gocache'
$materials = Get-Content -LiteralPath (Join-Path $root 'third_party/manifests/distribution-materials.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$bunCommand = Get-Command bun
if ($bunCommand.CommandType -ne 'Application' -or (Get-FileHash -LiteralPath $bunCommand.Source -Algorithm SHA256).Hash.ToLowerInvariant() -ne $materials.bun.executableSha256) { throw 'Pinned Bun executable checksum differs' }
$bunRevision = (& $bunCommand.Source --revision).Trim()
if ($LASTEXITCODE -ne 0 -or $bunRevision -ne $materials.bun.cliRevision) { throw 'Pinned Bun CLI revision differs' }
$patchedMod = & python (Join-Path $root 'tools/prepare_mygo.py')
if ($LASTEXITCODE -ne 0) { throw 'MyGo menu patch preparation failed' }
$env:GOFLAGS = "`"-modfile=$patchedMod`""
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
    $media = Join-Path $root 'build/output/media-tools'
    & python (Join-Path $root 'tools/prepare_media.py')
    if ($LASTEXITCODE -ne 0) { throw 'Pinned media runtime preparation failed' }
    foreach ($name in @('ffmpeg','ffprobe')) {
        $source = Join-Path $media "$name.exe"
        $mediaVersion = (& $source -version 2>$null) -join "`n"
        if ($LASTEXITCODE -ne 0 -or $mediaVersion -notmatch 'version 9.0.2' -or $mediaVersion -match 'enable-nonfree') { throw "Unapproved media build: $name" }
    }
    & (Join-Path $PSScriptRoot 'build-oneasr.ps1')
    $runtime = [IO.File]::ReadAllText((Join-Path $root 'third_party/manifests/runtimes.json')) | ConvertFrom-Json
    if (-not (Test-Path 'build/output/oneasr/oneasr-cli.exe')) { throw 'Verified OneAsr region-batch runtime is required' }
    if ((Get-FileHash 'build/output/oneasr/oneasr-cli.exe' -Algorithm SHA256).Hash.ToLowerInvariant() -ne $runtime.oneasr.sha256) { throw 'OneAsr hash differs from the recorded build' }
    & (Join-Path $PSScriptRoot 'prepare-vad.ps1')
    & (Join-Path $PSScriptRoot 'prepare-sherpa.ps1')
    $channel = if ($ReleaseTag) { 'release' } else { 'local-test' }
    $tag = if ($ReleaseTag) { $ReleaseTag } else { "build-$BuildNumber" }
    $info = [ordered]@{version=$version;build=$BuildNumber;updateVersion="$version.$BuildNumber";releaseTag=$tag;updatesSigned=$true;channel=$channel;signed=$false;sourceTree=$sourceTree;worktree='staged source snapshot'}

    [IO.File]::WriteAllText((Join-Path $root 'build/output/build-info.json'),($info|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    if (-not $SkipTests) { & (Join-Path $PSScriptRoot 'test.ps1') }
    $package = Join-Path $root "_temp/build-output/packages/$BuildNumber"
    if (Test-Path $package) { throw 'Use a new internal build number; package directories are immutable' }
    $previousUpdateKey = $providedUpdateKey
    Push-Location apps/desktop
    try {
        # Read the ignored secret only for MyGo's signing process, never for
        # dependency installers, generated resources, logs or the application.
        if ($previousUpdateKey) { $env:MYGO_UPDATER_PRIVATE_KEY = $previousUpdateKey }
        else { $env:MYGO_UPDATER_PRIVATE_KEY = [IO.File]::ReadAllText($updateKey).Trim() }
        & go run github.com/egoist/mygo/cmd/mygo build -platform windows/amd64 -o $package
        if ($LASTEXITCODE -ne 0) { throw 'MyGo Windows build failed' }
    } finally {
        $env:MYGO_UPDATER_PRIVATE_KEY = $null
        Pop-Location
    }
    $out=Join-Path $root 'build/output/release'
    & git diff --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Release source changed during the build' }
    $currentTree = (& git write-tree).Trim()
    if ($LASTEXITCODE -ne 0 -or $currentTree -ne $sourceTree) { throw 'Release source changed during the build' }
    $untracked = @(& git ls-files --others --exclude-standard)
    if ($LASTEXITCODE -ne 0 -or $untracked.Count -gt 0) { throw 'Release source changed during the build' }
    New-Item -ItemType Directory $out -Force | Out-Null
    $releaseArguments = @()
    if ($ReleaseTag) { $releaseArguments = @('--release-tag', $ReleaseTag) }
    & python (Join-Path $root 'tools/finalize_update.py') --config (Join-Path $root 'apps/desktop/mygo.json') --package (Join-Path $package 'windows-amd64') --build $BuildNumber --output $out @releaseArguments
    if ($LASTEXITCODE -ne 0) { throw 'Signed update artifact validation failed' }
    $setupName="OneSub-$version-Setup.exe"
    $setup=Join-Path $out $setupName
    # MyGo names the installer <slug>-setup-<version>-<arch>.exe.
    Copy-Item (Join-Path $package "windows-amd64/onesub-setup-$version-amd64.exe") $setup -Force
    $hash=(Get-FileHash $setup -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText(($setup+'.sha256'),"$hash  $setupName`n",[Text.UTF8Encoding]::new($false))
    Copy-Item 'build/output/build-info.json' $out -Force
    & python (Join-Path $root 'tools/build_sbom.py') --package (Join-Path $package 'windows-amd64') --output $out --work (Join-Path $package 'sbom')
    if ($LASTEXITCODE -ne 0) { throw 'Release SBOM generation failed' }
    Write-Output "Build $BuildNumber : $setup"
    Write-Output "SHA-256: $hash"
} finally { Pop-Location }
} finally { $env:MYGO_UPDATER_PRIVATE_KEY = $providedUpdateKey }
