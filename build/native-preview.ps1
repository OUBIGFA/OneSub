param([string]$Output = 'build/output/native-preview/OneSub.exe')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$env:GOROOT = Join-Path $root '.local/toolchains/go1.27.3/go'
$env:PATH = "$env:GOROOT/bin;$env:PATH"
$env:GOMODCACHE = Join-Path $root '.local/gomodcache'
$env:GOCACHE = Join-Path $root '.local/gocache'
& (Join-Path $PSScriptRoot 'build-player.ps1')
$config = [IO.File]::ReadAllText((Join-Path $root 'apps/desktop/mygo.json')) | ConvertFrom-Json
$out = Join-Path $root $Output
New-Item -ItemType Directory (Split-Path $out -Parent) -Force | Out-Null
$flags = "-H=windowsgui -X github.com/egoist/mygo.packageName=$($config.name) -X github.com/egoist/mygo.packageVersion=$($config.version) -X github.com/egoist/mygo.packageIdentifier=$($config.identifier) -X main.BuildNumber=native-preview"
Push-Location (Join-Path $root 'apps/desktop')
try {
    & go build -ldflags $flags -o $out .
    if ($LASTEXITCODE -ne 0) { throw 'Native preview build failed' }
} finally { Pop-Location }
# Development-only dependency overrides; final package tests do not use these.
$env:ONESUB_SUBTITLE_WORKER = Join-Path $root 'build/output/subtitle-core/OneSub.SubtitleCore.exe'
$env:ONESUB_STYLE_WORKER = Join-Path $root 'build/output/style-core/OneSub.StyleCore.exe'
$env:ONESUB_SKILL_WORKER = Join-Path $root 'build/output/skill-tools/OneSub.SkillTools.exe'
$env:ONESUB_SKILL_ROOT = Join-Path $root 'third_party/skills/srt-subtitle-translator'
Write-Output $out
