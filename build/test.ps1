param([string]$GoRoot = $env:GOROOT, [string[]]$Packages = @('./...'), [switch]$GoOnly, [string]$PackageRoot)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (-not $GoRoot) { $GoRoot = Join-Path $root '.local/toolchains/go1.27.3/go' }
$env:GOROOT = $GoRoot
$env:PATH = "$GoRoot/bin;$env:PATH"
$env:GOMODCACHE = Join-Path $root '.local/gomodcache'
$env:GOCACHE = Join-Path $root '.local/gocache'
$env:PYTHONUTF8 = '1'
$env:ONESUB_SUBTITLE_WORKER = Join-Path $root 'build/output/subtitle-core/OneSub.SubtitleCore.exe'
$env:ONESUB_STYLE_WORKER = Join-Path $root 'build/output/style-core/OneSub.StyleCore.exe'
$env:ONESUB_SKILL_WORKER = Join-Path $root 'build/output/skill-tools/OneSub.SkillTools.exe'
$env:ONESUB_SKILL_ROOT = Join-Path $root 'third_party/skills/srt-subtitle-translator'
if (-not (Test-Path $env:ONESUB_SKILL_WORKER)) { throw 'Run build/build-skill-tools.ps1 first' }
if (-not (Test-Path $env:ONESUB_STYLE_WORKER)) { throw 'Build workers/style-core first' }
$env:ONESUB_FFMPEG = (Get-Command ffmpeg -ErrorAction Stop).Source
$env:ONESUB_FFPROBE = (Get-Command ffprobe -ErrorAction Stop).Source
if ($PackageRoot) {
    $PackageRoot = [IO.Path]::GetFullPath($PackageRoot)
    $env:ONESUB_SUBTITLE_WORKER = Join-Path $PackageRoot 'subtitle-core/OneSub.SubtitleCore.exe'
    $env:ONESUB_STYLE_WORKER = Join-Path $PackageRoot 'style-core/OneSub.StyleCore.exe'
    $env:ONESUB_SKILL_WORKER = Join-Path $PackageRoot 'skill-tools/OneSub.SkillTools.exe'
    $env:ONESUB_SKILL_ROOT = Join-Path $PackageRoot 'srt-subtitle-translator'
    $env:ONESUB_FFMPEG = Join-Path $PackageRoot 'media-tools/ffmpeg.exe'
    $env:ONESUB_FFPROBE = Join-Path $PackageRoot 'media-tools/ffprobe.exe'
}
if (-not (Test-Path $env:ONESUB_SUBTITLE_WORKER)) { throw 'Run build/build-subtitle.ps1 first' }
Push-Location $root
try {
    Push-Location apps/desktop
    try {
        # Message accessors are generated from locales/*.json and not tracked.
        & go run github.com/egoist/mygo/cmd/mygo generate
        if ($LASTEXITCODE -ne 0) { throw 'Message catalog generation failed' }
        & go test @Packages -count=1
        if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    } finally { Pop-Location }
    if ($GoOnly) { return }
    Push-Location workers/style-core
    try {
        & bun test
        if ($LASTEXITCODE -ne 0) { throw 'SmartSub style tests failed' }
    } finally { Pop-Location }
    & python -m unittest discover -s tests/regression -v
    if ($LASTEXITCODE -ne 0) { throw 'Subtitle worker regression failed' }
    & python tools/check_repo.py
    if ($LASTEXITCODE -ne 0) { throw 'Repository check failed' }
    & python -m unittest discover -s tests/repository -v
    if ($LASTEXITCODE -ne 0) { throw 'Repository tests failed' }
} finally { Pop-Location }
