$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$source = Join-Path $root 'third_party/sources/subtitleedit-e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/libse/LibSE.csproj'
if (-not (Test-Path $source)) { throw 'Pinned libse source missing; see third_party/manifests/subtitle-core.json' }
$env:NUGET_PACKAGES = Join-Path $root '.local/nuget'
Push-Location (Join-Path $root 'workers/subtitle-core')
try {
    & dotnet restore --locked-mode
    if ($LASTEXITCODE -ne 0) { throw 'Subtitle worker restore failed' }
    & dotnet publish -c Release --no-restore -o (Join-Path $root 'build/output/subtitle-core')
    if ($LASTEXITCODE -ne 0) { throw 'Subtitle worker build failed' }
} finally { Pop-Location }
